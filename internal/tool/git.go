package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"boAIrd/internal/capability"
	"boAIrd/internal/logging"
)

const (
	DefaultGitTimeout     = 2 * time.Minute
	DefaultGitOutputLimit = 256 << 10
	maximumGitLogEntries  = 50
)

var (
	ErrGitDisabled = errors.New("git operations are disabled")
	ErrGitDenied   = errors.New("git operation denied")
	ErrGitPolicy   = errors.New("git policy rejected the operation")
)

var gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
var gitRemotePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// GitConfig bounds the governed Git tool set to an authenticated workspace.
type GitConfig struct {
	Enabled             bool
	AllowPush           bool
	AllowMerge          bool
	ResolveWorkspace    WorkspaceResolver
	HumanReviewRequired func(context.Context, string) (bool, error)
	TaskApproved        func(context.Context, string) (bool, error)
	Timeout             time.Duration
	MaxOutputBytes      int
}

type GitReadInput struct {
	Query            string `json:"query"`
	Reference        string `json:"reference,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitReadOutput struct {
	Stdout    string `json:"stdout,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type GitDiffInput struct {
	Reference        string `json:"reference,omitempty"`
	Staged           bool   `json:"staged,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitDiffOutput struct {
	Stdout    string `json:"stdout,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type GitBranchInput struct {
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitBranchOutput struct {
	Current  string   `json:"current,omitempty"`
	Branches []string `json:"branches"`
}

type GitCommitInput struct {
	Message          string `json:"message"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitCommitOutput struct {
	Revision string `json:"revision,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
}

type GitPushInput struct {
	Remote           string `json:"remote,omitempty"`
	Branch           string `json:"branch,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitPushOutput struct {
	Stdout    string `json:"stdout,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type GitMergeInput struct {
	Reference        string `json:"reference"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type GitMergeOutput struct {
	Revision  string `json:"revision,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// NewGitTools returns one governed tool per Git operation. Each operation
// carries its own capability, so commit, push, and merge never inherit from
// read-only grants.
func NewGitTools(config GitConfig) ([]Tool, error) {
	if !config.Enabled {
		return nil, ErrGitDisabled
	}
	if config.ResolveWorkspace == nil {
		return nil, fmt.Errorf("%w: project workspace resolver is required", ErrInvalidRequest)
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultGitTimeout
	}
	if config.Timeout < time.Millisecond || config.Timeout > maximumTimeout {
		return nil, fmt.Errorf("%w: git timeout must be between 1ms and one hour", ErrInvalidRequest)
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = DefaultGitOutputLimit
	}
	if config.MaxOutputBytes < 1 || config.MaxOutputBytes > DefaultMaxOutputBytes-4096 {
		return nil, fmt.Errorf("%w: git output limit must be between 1 and %d bytes", ErrInvalidRequest, DefaultMaxOutputBytes-4096)
	}
	tools := []Tool{
		gitTool("git.read", "Inspect repository status, log, a ref, or tracked files inside the authorized workspace.",
			capability.GitRead, gitReadSchema, gitReadOutputSchema, validateGitReadInput, runGitRead, config),
		gitTool("git.diff", "Show bounded working-tree, staged, or reference diffs inside the authorized workspace.",
			capability.GitDiff, gitDiffSchema, gitDiffOutputSchema, validateGitDiffInput, runGitDiff, config),
		gitTool("git.branch", "List repository branches and report the current branch inside the authorized workspace.",
			capability.GitBranch, gitBranchSchema, gitBranchOutputSchema, validateGitBranchInput, runGitBranch, config),
		gitTool("git.commit", "Commit staged changes with a message inside the authorized workspace.",
			capability.GitCommit, gitCommitSchema, gitCommitOutputSchema, validateGitCommitInput, runGitCommit, config),
		gitTool("git.push", "Push a branch to a named remote inside the authorized workspace; denied by default and subject to Human Review.",
			capability.GitPush, gitPushSchema, gitPushOutputSchema, validateGitPushInput, runGitPush, config),
		gitTool("git.merge", "Merge a reference into the current branch inside the authorized workspace; denied by default and subject to Human Review.",
			capability.GitMerge, gitMergeSchema, gitMergeOutputSchema, validateGitMergeInput, runGitMerge, config),
	}
	return tools, nil
}

const (
	gitReadSchema   = `{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","enum":["status","log","show","ls_files"]},"reference":{"type":"string"},"working_directory":{"type":"string"}}}`
	gitDiffSchema   = `{"type":"object","additionalProperties":false,"properties":{"reference":{"type":"string"},"staged":{"type":"boolean"},"working_directory":{"type":"string"}}}`
	gitBranchSchema = `{"type":"object","additionalProperties":false,"properties":{"working_directory":{"type":"string"}}}`
	gitCommitSchema = `{"type":"object","additionalProperties":false,"required":["message"],"properties":{"message":{"type":"string","minLength":1},"working_directory":{"type":"string"}}}`
	gitPushSchema   = `{"type":"object","additionalProperties":false,"properties":{"remote":{"type":"string"},"branch":{"type":"string"},"working_directory":{"type":"string"}}}`
	gitMergeSchema  = `{"type":"object","additionalProperties":false,"required":["reference"],"properties":{"reference":{"type":"string","minLength":1},"working_directory":{"type":"string"}}}`

	gitReadOutputSchema   = `{"type":"object","additionalProperties":false,"properties":{"stdout":{"type":"string"},"truncated":{"type":"boolean"}}}`
	gitDiffOutputSchema   = `{"type":"object","additionalProperties":false,"properties":{"stdout":{"type":"string"},"truncated":{"type":"boolean"}}}`
	gitBranchOutputSchema = `{"type":"object","additionalProperties":false,"properties":{"current":{"type":"string"},"branches":{"type":"array","items":{"type":"string"}}}}`
	gitCommitOutputSchema = `{"type":"object","additionalProperties":false,"properties":{"revision":{"type":"string"},"stdout":{"type":"string"}}}`
	gitPushOutputSchema   = `{"type":"object","additionalProperties":false,"properties":{"stdout":{"type":"string"},"truncated":{"type":"boolean"}}}`
	gitMergeOutputSchema  = `{"type":"object","additionalProperties":false,"properties":{"revision":{"type":"string"},"stdout":{"type":"string"},"truncated":{"type":"boolean"}}}`
)

// gitTool assembles one typed tool definition with a shared repo resolver.
func gitTool[I, O any](name, description string, required capability.Capability, inputSchema, outputSchema string, validate func(I) error, run func(context.Context, Request, GitConfig, string, I) (O, error), config GitConfig) Tool {
	return Definition[I, O]{
		Metadata: Contract{
			Name: name, Description: description,
			InputSchema:          json.RawMessage(inputSchema),
			OutputSchema:         json.RawMessage(outputSchema),
			RequiredCapabilities: []capability.Capability{required},
			Limits:               Limits{Timeout: config.Timeout, MaxInputBytes: 64 << 10, MaxOutputBytes: config.MaxOutputBytes + 4096},
		},
		Validate:       validate,
		ValidateOutput: func(O) error { return nil },
		RunWithRequest: func(ctx context.Context, request Request, input I) (O, error) {
			var output O
			workingDirectory := gitWorkingDirectory(input)
			repoRoot, err := resolveGitRepoRoot(ctx, config, request, workingDirectory)
			if err != nil {
				return output, err
			}
			return run(ctx, request, config, repoRoot, input)
		},
	}
}

func gitWorkingDirectory(value any) string {
	switch input := value.(type) {
	case GitReadInput:
		return input.WorkingDirectory
	case GitDiffInput:
		return input.WorkingDirectory
	case GitBranchInput:
		return input.WorkingDirectory
	case GitCommitInput:
		return input.WorkingDirectory
	case GitPushInput:
		return input.WorkingDirectory
	case GitMergeInput:
		return input.WorkingDirectory
	default:
		return ""
	}
}

func validateGitReadInput(input GitReadInput) error {
	switch input.Query {
	case "status", "log", "show", "ls_files":
	default:
		return errors.New("git.read query must be status, log, show, or ls_files")
	}
	if input.Query == "log" || input.Query == "show" {
		if err := validateGitReference(input.Reference, true); err != nil {
			return err
		}
	} else if input.Reference != "" {
		return errors.New("git.read reference is only valid with log or show")
	}
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateGitDiffInput(input GitDiffInput) error {
	if input.Reference != "" {
		if err := validateGitReference(input.Reference, true); err != nil {
			return err
		}
	}
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateGitBranchInput(input GitBranchInput) error {
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateGitCommitInput(input GitCommitInput) error {
	message := strings.TrimSpace(input.Message)
	if message == "" || message != input.Message || strings.ContainsRune(message, '\x00') || len(message) > 4000 {
		return errors.New("git.commit message is required and must be bounded plain text")
	}
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateGitPushInput(input GitPushInput) error {
	if input.Remote != "" && !gitRemotePattern.MatchString(input.Remote) {
		return errors.New("git.push remote must be a simple remote name")
	}
	if input.Branch != "" {
		if err := validateGitReference(input.Branch, false); err != nil {
			return errors.New("git.push branch is invalid")
		}
	}
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateGitMergeInput(input GitMergeInput) error {
	if err := validateGitReference(input.Reference, false); err != nil {
		return err
	}
	return validateWorkingDirectory(input.WorkingDirectory)
}

func validateWorkingDirectory(value string) error {
	if value == "" || !hasParentTraversal(value) && !strings.ContainsRune(value, '\x00') {
		return nil
	}
	return errors.New("git working directory is invalid")
}

func validateGitReference(value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("git reference is required")
	}
	if strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "-") || !gitRefPattern.MatchString(value) || value == ".." || strings.Contains(value, "..") {
		return errors.New("git reference is invalid")
	}
	return nil
}

func resolveGitRepoRoot(ctx context.Context, config GitConfig, request Request, workingDirectory string) (string, error) {
	workspace, err := config.ResolveWorkspace(ctx, request.ProjectID)
	if err != nil {
		return "", ErrGitPolicy
	}
	root, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return "", err
	}
	workingDir, err := resolveWorkingDirectory(root, workingDirectory)
	if err != nil {
		return "", err
	}
	repoRoot, err := gitToplevel(ctx, workingDir)
	if err != nil {
		return "", ErrGitPolicy
	}
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil || !pathWithin(root, resolvedRepo) {
		return "", ErrGitPolicy
	}
	return resolvedRepo, nil
}

func gitToplevel(ctx context.Context, directory string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(output))
	if value == "" || strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) {
		return "", errors.New("repository root is invalid")
	}
	return filepath.Clean(value), nil
}

func runGitCommand(ctx context.Context, repoRoot string, maxOutput int, args ...string) (string, string, bool, error) {
	full := append([]string{"-C", repoRoot}, args...)
	command := exec.CommandContext(ctx, "git", full...)
	capture := newShellOutputCapture(maxOutput)
	command.Stdout = shellOutputWriter{capture: capture, stderr: false}
	command.Stderr = shellOutputWriter{capture: capture, stderr: true}
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", "", false, fmt.Errorf("%w: git command deadline exceeded", ErrTimedOut)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return "", "", false, ErrExecution
		}
		return "", "", false, fmt.Errorf("%w: git command failed", ErrExecution)
	}
	stdout, stderr, truncated := capture.result()
	return logging.RedactSensitiveText(stdout), logging.RedactSensitiveText(stderr), truncated, nil
}

func runGitRead(ctx context.Context, _ Request, config GitConfig, repoRoot string, input GitReadInput) (GitReadOutput, error) {
	var args []string
	switch input.Query {
	case "status":
		args = []string{"status", "--short", "--branch"}
	case "log":
		args = []string{"log", "--oneline", "-n", fmt.Sprintf("%d", maximumGitLogEntries)}
		if input.Reference != "" {
			args = append(args, input.Reference)
		}
	case "show":
		args = []string{"show", "--stat", "--oneline"}
		if input.Reference != "" {
			args = append(args, input.Reference)
		}
	case "ls_files":
		args = []string{"ls-files"}
	}
	stdout, _, truncated, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, args...)
	if err != nil {
		return GitReadOutput{}, err
	}
	return GitReadOutput{Stdout: stdout, Truncated: truncated}, nil
}

func runGitDiff(ctx context.Context, _ Request, config GitConfig, repoRoot string, input GitDiffInput) (GitDiffOutput, error) {
	args := []string{"diff"}
	if input.Staged {
		args = append(args, "--cached")
	}
	if input.Reference != "" {
		args = append(args, input.Reference)
	}
	stdout, _, truncated, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, args...)
	if err != nil {
		return GitDiffOutput{}, err
	}
	return GitDiffOutput{Stdout: stdout, Truncated: truncated}, nil
}

func runGitBranch(ctx context.Context, _ Request, config GitConfig, repoRoot string, input GitBranchInput) (GitBranchOutput, error) {
	stdout, _, _, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return GitBranchOutput{}, err
	}
	branches := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	current, _, _, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return GitBranchOutput{}, err
	}
	return GitBranchOutput{Current: strings.TrimSpace(current), Branches: branches}, nil
}

func runGitCommit(ctx context.Context, _ Request, config GitConfig, repoRoot string, input GitCommitInput) (GitCommitOutput, error) {
	stdout, _, _, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "commit", "-m", input.Message)
	if err != nil {
		return GitCommitOutput{}, err
	}
	revision, _, _, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "rev-parse", "HEAD")
	if err != nil {
		return GitCommitOutput{}, err
	}
	return GitCommitOutput{Revision: strings.TrimSpace(revision), Stdout: stdout}, nil
}

func runGitPush(ctx context.Context, request Request, config GitConfig, repoRoot string, input GitPushInput) (GitPushOutput, error) {
	if !config.AllowPush {
		return GitPushOutput{}, ErrGitDenied
	}
	if err := requireHumanReviewGate(ctx, config, request); err != nil {
		return GitPushOutput{}, err
	}
	args := []string{"push"}
	if input.Remote != "" {
		args = append(args, input.Remote)
	}
	if input.Branch != "" {
		args = append(args, input.Branch)
	}
	stdout, _, truncated, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, args...)
	if err != nil {
		return GitPushOutput{}, err
	}
	return GitPushOutput{Stdout: stdout, Truncated: truncated}, nil
}

func runGitMerge(ctx context.Context, request Request, config GitConfig, repoRoot string, input GitMergeInput) (GitMergeOutput, error) {
	if !config.AllowMerge {
		return GitMergeOutput{}, ErrGitDenied
	}
	if err := requireHumanReviewGate(ctx, config, request); err != nil {
		return GitMergeOutput{}, err
	}
	stdout, _, _, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "merge", input.Reference)
	if err != nil {
		return GitMergeOutput{}, err
	}
	revision, _, truncated, err := runGitCommand(ctx, repoRoot, config.MaxOutputBytes, "rev-parse", "HEAD")
	if err != nil {
		return GitMergeOutput{}, err
	}
	return GitMergeOutput{Revision: strings.TrimSpace(revision), Stdout: stdout, Truncated: truncated}, nil
}

// requireHumanReviewGate enforces the configured Human Review requirement for
// push and merge. The operation must be tied to a task whose human review has
// been approved when the project demands it.
func requireHumanReviewGate(ctx context.Context, config GitConfig, request Request) error {
	if config.HumanReviewRequired == nil {
		return ErrGitDenied
	}
	required, err := config.HumanReviewRequired(ctx, request.ProjectID)
	if err != nil || !required {
		return nil
	}
	if request.TaskID == "" {
		return ErrGitDenied
	}
	if config.TaskApproved == nil {
		return ErrGitDenied
	}
	approved, err := config.TaskApproved(ctx, request.TaskID)
	if err != nil || !approved {
		return ErrGitDenied
	}
	return nil
}
