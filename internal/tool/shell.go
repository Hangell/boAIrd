package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"boAIrd/internal/capability"
	"boAIrd/internal/logging"
)

const (
	DefaultShellTimeout       = 2 * time.Minute
	DefaultShellOutputLimit   = 256 << 10
	maximumShellArgumentCount = 256
	maximumShellArgumentBytes = 16 << 10
)

var (
	ErrShellDisabled = errors.New("shell execution is disabled")
	ErrShellPolicy   = errors.New("shell policy rejected the command")
)

type WorkspaceResolver func(context.Context, string) (string, error)

type ShellConfig struct {
	Enabled            bool
	AllowedExecutables []string
	ResolveWorkspace   WorkspaceResolver
	Timeout            time.Duration
	MaxOutputBytes     int
}

type ShellInput struct {
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
}

type ShellOutput struct {
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated,omitempty"`
}

type shellDefinition struct {
	definition Definition[ShellInput, ShellOutput]
}

func NewShellTool(config ShellConfig) (Tool, error) {
	if !config.Enabled {
		return nil, ErrShellDisabled
	}
	if config.ResolveWorkspace == nil {
		return nil, fmt.Errorf("%w: project workspace resolver is required", ErrInvalidRequest)
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultShellTimeout
	}
	if config.Timeout < time.Millisecond || config.Timeout > maximumTimeout {
		return nil, fmt.Errorf("%w: shell timeout must be between 1ms and one hour", ErrInvalidRequest)
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = DefaultShellOutputLimit
	}
	if config.MaxOutputBytes < 1 || config.MaxOutputBytes > DefaultMaxOutputBytes-4096 {
		return nil, fmt.Errorf("%w: shell output limit must be between 1 and %d bytes", ErrInvalidRequest, DefaultMaxOutputBytes-4096)
	}
	commands, err := resolveShellExecutables(config.AllowedExecutables)
	if err != nil {
		return nil, err
	}
	inputSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["executable"],"properties":{"executable":{"type":"string","minLength":1},"arguments":{"type":"array","items":{"type":"string"},"maxItems":256},"working_directory":{"type":"string"}}}`)
	outputSchema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"stdout":{"type":"string"},"stderr":{"type":"string"},"exit_code":{"type":"integer"},"truncated":{"type":"boolean"}}}`)
	definition := Definition[ShellInput, ShellOutput]{
		Metadata: Contract{
			Name: "shell.execute", Description: "Run one allowlisted executable with structured arguments inside the authenticated project's workspace.",
			InputSchema: inputSchema, OutputSchema: outputSchema,
			RequiredCapabilities: []capability.Capability{capability.ShellExecute},
			Limits:               Limits{Timeout: config.Timeout, MaxInputBytes: 64 << 10, MaxOutputBytes: config.MaxOutputBytes + 4096},
		},
		Validate:       validateShellInput,
		ValidateOutput: func(ShellOutput) error { return nil },
		RunWithRequest: func(ctx context.Context, request Request, input ShellInput) (ShellOutput, error) {
			return runShellCommand(ctx, request, input, config, commands)
		},
	}
	if !definition.validDefinition() {
		return nil, fmt.Errorf("%w: shell tool contract is invalid", ErrInvalidRequest)
	}
	return shellDefinition{definition: definition}, nil
}

func (s shellDefinition) Contract() Contract { return s.definition.Contract() }

func (s shellDefinition) validDefinition() bool { return s.definition.validDefinition() }

func (s shellDefinition) frozen() Tool {
	return shellDefinition{definition: s.definition.frozen().(Definition[ShellInput, ShellOutput])}
}

func (s shellDefinition) decodeAndValidate(raw json.RawMessage) (any, error) {
	return s.definition.decodeAndValidate(raw)
}

func (s shellDefinition) execute(ctx context.Context, request Request, value any) (json.RawMessage, error) {
	return s.definition.execute(ctx, request, value)
}

func validateShellInput(input ShellInput) error {
	if strings.TrimSpace(input.Executable) == "" || strings.TrimSpace(input.Executable) != input.Executable ||
		filepath.Base(input.Executable) != input.Executable || strings.ContainsRune(input.Executable, '\x00') {
		return errors.New("an executable name is required")
	}
	if len(input.Arguments) > maximumShellArgumentCount {
		return errors.New("too many command arguments")
	}
	for _, argument := range input.Arguments {
		if strings.ContainsRune(argument, '\x00') || len(argument) > maximumShellArgumentBytes {
			return errors.New("a command argument is invalid")
		}
	}
	if strings.ContainsRune(input.WorkingDirectory, '\x00') {
		return errors.New("working directory is invalid")
	}
	return nil
}

func resolveShellExecutables(values []string) (map[string]string, error) {
	if len(values) == 0 || len(values) > 64 {
		return nil, fmt.Errorf("%w: at least one and at most 64 shell executables must be allowlisted", ErrInvalidRequest)
	}
	commands := make(map[string]string, len(values))
	paths := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("%w: allowlisted executable is invalid", ErrInvalidRequest)
		}
		resolved, err := exec.LookPath(value)
		if err != nil {
			return nil, fmt.Errorf("%w: allowlisted executable is unavailable", ErrInvalidRequest)
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return nil, fmt.Errorf("%w: allowlisted executable cannot be resolved", ErrInvalidRequest)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("%w: allowlisted executable cannot be resolved", ErrInvalidRequest)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || !isExecutable(info.Mode()) {
			return nil, fmt.Errorf("%w: allowlisted path is not an executable file", ErrInvalidRequest)
		}
		name := filepath.Base(value)
		if filepath.Base(value) == "." || name == string(filepath.Separator) || strings.ContainsAny(name, `/\\`) {
			return nil, fmt.Errorf("%w: allowlisted executable name is invalid", ErrInvalidRequest)
		}
		if _, exists := commands[name]; exists {
			return nil, fmt.Errorf("%w: duplicate allowlisted executable name", ErrInvalidRequest)
		}
		if _, exists := paths[resolved]; exists {
			return nil, fmt.Errorf("%w: duplicate allowlisted executable path", ErrInvalidRequest)
		}
		commands[name] = resolved
		paths[resolved] = struct{}{}
	}
	return commands, nil
}

func runShellCommand(ctx context.Context, request Request, input ShellInput, config ShellConfig, commands map[string]string) (ShellOutput, error) {
	if !config.Enabled {
		return ShellOutput{}, ErrShellDisabled
	}
	executable, exists := commands[input.Executable]
	if !exists {
		return ShellOutput{}, ErrShellPolicy
	}
	workspace, err := config.ResolveWorkspace(ctx, request.ProjectID)
	if err != nil {
		return ShellOutput{}, ErrShellPolicy
	}
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return ShellOutput{}, err
	}
	workingDirectory, err := resolveWorkingDirectory(root, input.WorkingDirectory)
	if err != nil {
		return ShellOutput{}, err
	}
	if err := validateArgumentPaths(root, workingDirectory, input.Arguments); err != nil {
		return ShellOutput{}, err
	}
	environment, cleanupEnvironment, err := safeShellEnvironment(root, commands)
	if err != nil {
		return ShellOutput{}, ErrExecution
	}
	defer cleanupEnvironment()
	command := exec.CommandContext(ctx, executable, input.Arguments...)
	command.Dir = workingDirectory
	command.Env = environment
	configureProcessGroup(command)
	capture := newShellOutputCapture(config.MaxOutputBytes)
	command.Stdout = shellOutputWriter{capture: capture, stderr: false}
	command.Stderr = shellOutputWriter{capture: capture, stderr: true}
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ShellOutput{}, fmt.Errorf("%w: command deadline exceeded", ErrTimedOut)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return ShellOutput{}, ErrExecution
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return ShellOutput{}, fmt.Errorf("%w: command exited with status %d", ErrExecution, exitError.ExitCode())
		}
		return ShellOutput{}, ErrExecution
	}
	stdout, stderr, truncated := capture.result()
	return ShellOutput{
		Stdout: logging.RedactSensitiveText(stdout), Stderr: logging.RedactSensitiveText(stderr),
		ExitCode: 0, Truncated: truncated,
	}, nil
}

func resolveWorkspaceRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: project workspace is invalid", ErrShellPolicy)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("%w: project workspace cannot be resolved", ErrShellPolicy)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: project workspace is not a directory", ErrShellPolicy)
	}
	return filepath.Clean(resolved), nil
}

func resolveWorkingDirectory(root, value string) (string, error) {
	if value == "" {
		return root, nil
	}
	if hasParentTraversal(value) {
		return "", ErrShellPolicy
	}
	resolved, err := resolveWorkspacePath(root, root, value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", ErrShellPolicy
	}
	return resolved, nil
}

func validateArgumentPaths(root, workingDirectory string, values []string) error {
	for _, value := range values {
		candidate, pathLike := pathValue(value)
		if candidate == "" {
			continue
		}
		if hasParentTraversal(candidate) {
			return ErrShellPolicy
		}
		if !pathLike {
			if _, err := os.Lstat(filepath.Join(workingDirectory, candidate)); errors.Is(err, fs.ErrNotExist) {
				continue
			} else if err != nil {
				return ErrShellPolicy
			}
		}
		if _, err := resolveWorkspacePath(root, workingDirectory, candidate); err != nil {
			return err
		}
	}
	return nil
}

func pathValue(argument string) (string, bool) {
	value := argument
	value = strings.TrimPrefix(value, "@")
	if index := strings.LastIndex(value, "="); index >= 0 && index < len(value)-1 {
		value = value[index+1:]
	} else if strings.HasPrefix(value, "-") {
		if index := strings.IndexAny(value, `/\\`); index > 1 {
			value = value[index:]
		}
	}
	if value == "." || value == ".." || filepath.IsAbs(value) || strings.ContainsAny(value, `/\\`) {
		return value, true
	}
	return value, false
}

func resolveWorkspacePath(root, base, value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", ErrShellPolicy
	}
	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	candidate = filepath.Clean(candidate)
	if !pathWithin(root, candidate) {
		return "", ErrShellPolicy
	}
	current := candidate
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if !pathWithin(root, resolved) {
				return "", ErrShellPolicy
			}
			for index := len(tail) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, tail[index])
			}
			resolved = filepath.Clean(resolved)
			if !pathWithin(root, resolved) {
				return "", ErrShellPolicy
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", ErrShellPolicy
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrShellPolicy
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

func pathWithin(root, value string) bool {
	relative, err := filepath.Rel(root, value)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func hasParentTraversal(value string) bool {
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return true
		}
	}
	return false
}

func safeShellEnvironment(workspace string, commands map[string]string) ([]string, func(), error) {
	path, err := os.MkdirTemp("", "boaird-shell-path-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.Chmod(path, 0o700)
		_ = os.RemoveAll(path)
	}
	for name, executable := range commands {
		if err := os.Symlink(executable, filepath.Join(path, name)); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if err := os.Chmod(path, 0o500); err != nil {
		cleanup()
		return nil, nil, err
	}
	return []string{"PATH=" + path, "HOME=" + workspace, "TMPDIR=" + workspace, "LANG=C", "LC_ALL=C"}, cleanup, nil
}

type shellOutputCapture struct {
	mu        sync.Mutex
	limit     int
	used      int
	stdout    []byte
	stderr    []byte
	truncated bool
}

type shellOutputWriter struct {
	capture *shellOutputCapture
	stderr  bool
}

func newShellOutputCapture(limit int) *shellOutputCapture {
	return &shellOutputCapture{limit: limit}
}

func (w shellOutputWriter) Write(value []byte) (int, error) {
	w.capture.mu.Lock()
	defer w.capture.mu.Unlock()
	remaining := w.capture.limit - w.capture.used
	if remaining <= 0 {
		w.capture.truncated = true
		return len(value), nil
	}
	count := len(value)
	if count > remaining {
		count = remaining
		w.capture.truncated = true
	}
	if w.stderr {
		w.capture.stderr = append(w.capture.stderr, value[:count]...)
	} else {
		w.capture.stdout = append(w.capture.stdout, value[:count]...)
	}
	w.capture.used += count
	return len(value), nil
}

func (capture *shellOutputCapture) result() (string, string, bool) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return string(capture.stdout), string(capture.stderr), capture.truncated
}

func isExecutable(mode fs.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm()&0o111 != 0
}
