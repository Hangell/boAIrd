package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"boAIrd/internal/capability"
	"boAIrd/internal/logging"
)

const (
	DefaultGitHubTimeout     = 2 * time.Minute
	DefaultGitHubOutputLimit = 256 << 10
	maximumGitHubListItems   = 30
	maximumGitHubResponse    = 1 << 20
	defaultGitHubAPIBaseURL  = "https://api.github.com"
)

var (
	ErrGitHubDisabled = errors.New("github operations are disabled")
	ErrGitHubDenied   = errors.New("github operation denied")
	ErrGitHubPolicy   = errors.New("github policy rejected the operation")
)

// GitHubConfig bounds the governed GitHub tool set to one administrator-approved
// repository and one managed credential.
type GitHubConfig struct {
	Enabled             bool
	AllowMerge          bool
	APIBaseURL          string
	ResolveRepository   func(context.Context, string) (string, error)
	ResolveCredential   func(context.Context, string, string, string) (string, error)
	HumanReviewRequired func(context.Context, string) (bool, error)
	TaskApproved        func(context.Context, string, string) (bool, error)
	Timeout             time.Duration
	MaxOutputBytes      int
	Client              *http.Client
}

type GitHubIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"html_url,omitempty"`
}

type GitHubPullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"html_url,omitempty"`
	Head   string `json:"head,omitempty"`
	Base   string `json:"base,omitempty"`
}

type GitHubIssueReadInput struct {
	Action       string `json:"action"`
	Number       int    `json:"number,omitempty"`
	State        string `json:"state,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubIssueReadOutput struct {
	Issues []GitHubIssue `json:"issues"`
	Count  int           `json:"count"`
}

type GitHubIssueCreateInput struct {
	Title        string `json:"title"`
	Body         string `json:"body,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubIssueCreateOutput struct {
	Number  int    `json:"number"`
	URL     string `json:"html_url"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type GitHubIssueUpdateInput struct {
	Number       int    `json:"number"`
	Body         string `json:"body,omitempty"`
	State        string `json:"state,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubIssueUpdateOutput struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	URL    string `json:"html_url"`
}

type GitHubPRReadInput struct {
	Action       string `json:"action"`
	Number       int    `json:"number,omitempty"`
	State        string `json:"state,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubPRReadOutput struct {
	PullRequests []GitHubPullRequest `json:"pull_requests"`
	Count        int                 `json:"count"`
}

type GitHubPRCreateInput struct {
	Title        string `json:"title"`
	Head         string `json:"head"`
	Base         string `json:"base"`
	Body         string `json:"body,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubPRCreateOutput struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	State  string `json:"state"`
}

type GitHubPRReviewInput struct {
	Number       int    `json:"number"`
	Event        string `json:"event"`
	Body         string `json:"body,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubPRReviewOutput struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

type GitHubPRMergeInput struct {
	Number       int    `json:"number"`
	MergeMethod  string `json:"merge_method,omitempty"`
	CredentialID string `json:"credential_id"`
}

type GitHubPRMergeOutput struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// NewGitHubTools returns one governed tool per GitHub operation. Each operation
// carries its own capability; review never implies merge.
func NewGitHubTools(config GitHubConfig) ([]Tool, error) {
	if !config.Enabled {
		return nil, ErrGitHubDisabled
	}
	if config.ResolveRepository == nil || config.ResolveCredential == nil {
		return nil, fmt.Errorf("%w: repository and credential resolvers are required", ErrInvalidRequest)
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultGitHubTimeout
	}
	if config.Timeout < time.Millisecond || config.Timeout > maximumTimeout {
		return nil, fmt.Errorf("%w: github timeout must be between 1ms and one hour", ErrInvalidRequest)
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = DefaultGitHubOutputLimit
	}
	if config.MaxOutputBytes < 1 || config.MaxOutputBytes > DefaultMaxOutputBytes-4096 {
		return nil, fmt.Errorf("%w: github output limit must be between 1 and %d bytes", ErrInvalidRequest, DefaultMaxOutputBytes-4096)
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultGitHubAPIBaseURL
	}
	if _, err := url.ParseRequestURI(config.APIBaseURL); err != nil {
		return nil, fmt.Errorf("%w: github API base URL is invalid", ErrInvalidRequest)
	}
	tools := []Tool{
		githubTool("github.issue.read", "Read one issue or list issues in the project's approved GitHub repository.",
			capability.GitHubIssueRead, githubIssueReadSchema, githubIssueReadOutputSchema, validateGitHubIssueReadInput, runGitHubIssueRead, config),
		githubTool("github.issue.create", "Create an issue in the project's approved GitHub repository.",
			capability.GitHubIssueCreate, githubIssueCreateSchema, githubIssueCreateOutputSchema, validateGitHubIssueCreateInput, runGitHubIssueCreate, config),
		githubTool("github.issue.update", "Update an issue body or state in the project's approved GitHub repository.",
			capability.GitHubIssueUpdate, githubIssueUpdateSchema, githubIssueUpdateOutputSchema, validateGitHubIssueUpdateInput, runGitHubIssueUpdate, config),
		githubTool("github.pr.read", "Read one pull request or list pull requests in the project's approved GitHub repository.",
			capability.GitHubPRRead, githubPRReadSchema, githubPRReadOutputSchema, validateGitHubPRReadInput, runGitHubPRRead, config),
		githubTool("github.pr.create", "Create a pull request in the project's approved GitHub repository.",
			capability.GitHubPRCreate, githubPRCreateSchema, githubPRCreateOutputSchema, validateGitHubPRCreateInput, runGitHubPRCreate, config),
		githubTool("github.pr.review", "Submit a review on a pull request in the project's approved GitHub repository.",
			capability.GitHubPRReview, githubPRReviewSchema, githubPRReviewOutputSchema, validateGitHubPRReviewInput, runGitHubPRReview, config),
		githubTool("github.pr.merge", "Merge a pull request in the project's approved GitHub repository; denied by default and subject to Human Review.",
			capability.GitHubPRMerge, githubPRMergeSchema, githubPRMergeOutputSchema, validateGitHubPRMergeInput, runGitHubPRMerge, config),
	}
	return tools, nil
}

const (
	githubIssueReadSchema   = `{"type":"object","additionalProperties":false,"required":["action","credential_id"],"properties":{"action":{"type":"string","enum":["get","list"]},"number":{"type":"integer"},"state":{"type":"string","enum":["open","closed","all"]},"credential_id":{"type":"string","minLength":1}}}`
	githubIssueCreateSchema = `{"type":"object","additionalProperties":false,"required":["title","credential_id"],"properties":{"title":{"type":"string","minLength":1},"body":{"type":"string"},"credential_id":{"type":"string","minLength":1}}}`
	githubIssueUpdateSchema = `{"type":"object","additionalProperties":false,"required":["number","credential_id"],"properties":{"number":{"type":"integer"},"body":{"type":"string"},"state":{"type":"string","enum":["open","closed"]},"credential_id":{"type":"string","minLength":1}}}`
	githubPRReadSchema      = `{"type":"object","additionalProperties":false,"required":["action","credential_id"],"properties":{"action":{"type":"string","enum":["get","list"]},"number":{"type":"integer"},"state":{"type":"string","enum":["open","closed","all"]},"credential_id":{"type":"string","minLength":1}}}`
	githubPRCreateSchema    = `{"type":"object","additionalProperties":false,"required":["title","head","base","credential_id"],"properties":{"title":{"type":"string","minLength":1},"head":{"type":"string","minLength":1},"base":{"type":"string","minLength":1},"body":{"type":"string"},"credential_id":{"type":"string","minLength":1}}}`
	githubPRReviewSchema    = `{"type":"object","additionalProperties":false,"required":["number","event","credential_id"],"properties":{"number":{"type":"integer"},"event":{"type":"string","enum":["approve","request_changes","comment"]},"body":{"type":"string"},"credential_id":{"type":"string","minLength":1}}}`
	githubPRMergeSchema     = `{"type":"object","additionalProperties":false,"required":["number","credential_id"],"properties":{"number":{"type":"integer"},"merge_method":{"type":"string","enum":["merge","squash","rebase"]},"credential_id":{"type":"string","minLength":1}}}`

	githubIssueReadOutputSchema   = `{"type":"object","additionalProperties":false,"properties":{"issues":{"type":"array","items":{"type":"object"}},"count":{"type":"integer"}}}`
	githubIssueCreateOutputSchema = `{"type":"object","additionalProperties":false,"properties":{"number":{"type":"integer"},"html_url":{"type":"string"},"state":{"type":"string"},"message":{"type":"string"}}}`
	githubIssueUpdateOutputSchema = `{"type":"object","additionalProperties":false,"properties":{"number":{"type":"integer"},"state":{"type":"string"},"html_url":{"type":"string"}}}`
	githubPRReadOutputSchema      = `{"type":"object","additionalProperties":false,"properties":{"pull_requests":{"type":"array","items":{"type":"object"}},"count":{"type":"integer"}}}`
	githubPRCreateOutputSchema    = `{"type":"object","additionalProperties":false,"properties":{"number":{"type":"integer"},"html_url":{"type":"string"},"state":{"type":"string"}}}`
	githubPRReviewOutputSchema    = `{"type":"object","additionalProperties":false,"properties":{"id":{"type":"integer"},"state":{"type":"string"}}}`
	githubPRMergeOutputSchema     = `{"type":"object","additionalProperties":false,"properties":{"merged":{"type":"boolean"},"message":{"type":"string"}}}`
)

// githubTool assembles one typed GitHub tool with shared repository/credential resolution.
func githubTool[I, O any](name, description string, required capability.Capability, inputSchema, outputSchema string, validate func(I) error, run func(context.Context, Request, GitHubConfig, string, string, string, I) (O, error), config GitHubConfig) Tool {
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
		AuditResourceID: func(ctx context.Context, request Request, input I) (string, error) {
			owner, repo, err := resolveGitHubRepository(ctx, config, request.ProjectID)
			if err != nil {
				return "", err
			}
			return owner + "/" + repo, nil
		},
		RunWithRequest: func(ctx context.Context, request Request, input I) (O, error) {
			var output O
			owner, repo, err := resolveGitHubRepository(ctx, config, request.ProjectID)
			if err != nil {
				return output, err
			}
			if owner+"/"+repo != request.ResourceID {
				return output, ErrGitHubPolicy
			}
			token, err := resolveGitHubToken(ctx, config, request, githubCredentialID(input))
			if err != nil {
				return output, err
			}
			return run(ctx, request, config, owner, repo, token, input)
		},
	}
}

func githubCredentialID(value any) string {
	switch input := value.(type) {
	case GitHubIssueReadInput:
		return input.CredentialID
	case GitHubIssueCreateInput:
		return input.CredentialID
	case GitHubIssueUpdateInput:
		return input.CredentialID
	case GitHubPRReadInput:
		return input.CredentialID
	case GitHubPRCreateInput:
		return input.CredentialID
	case GitHubPRReviewInput:
		return input.CredentialID
	case GitHubPRMergeInput:
		return input.CredentialID
	default:
		return ""
	}
}

func resolveGitHubRepository(ctx context.Context, config GitHubConfig, projectID string) (string, string, error) {
	rawURL, err := config.ResolveRepository(ctx, projectID)
	if err != nil {
		return "", "", ErrGitHubPolicy
	}
	owner, repo, err := parseGitHubRepository(rawURL)
	if err != nil {
		return "", "", ErrGitHubPolicy
	}
	return owner, repo, nil
}

func resolveGitHubToken(ctx context.Context, config GitHubConfig, request Request, credentialID string) (string, error) {
	token, err := config.ResolveCredential(ctx, request.Principal.ID, request.ProjectID, credentialID)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", ErrGitHubDenied
	}
	return token, nil
}

// parseGitHubRepository extracts owner and repository from an approved GitHub URL.
func parseGitHubRepository(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", errors.New("repository URL is empty")
	}
	if strings.HasPrefix(value, "git@") {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "github.com") {
			return "", "", errors.New("only github.com repositories are supported")
		}
		return splitOwnerRepo(strings.TrimSuffix(parts[1], ".git"))
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return "", "", errors.New("repository URL must be an absolute HTTPS URL")
	}
	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", "", errors.New("only github.com repositories are supported")
	}
	return splitOwnerRepo(strings.TrimSuffix(strings.TrimPrefix(parsed.Path, "/"), ".git"))
}

func splitOwnerRepo(path string) (string, string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		strings.ContainsAny(parts[0]+parts[1], " \t\n\r/\\") {
		return "", "", errors.New("repository path must be owner/repository")
	}
	return parts[0], parts[1], nil
}

type githubClient struct {
	baseURL string
	client  *http.Client
}

func (g githubClient) call(ctx context.Context, method, path, token string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, ErrGitHubPolicy
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reader)
	if err != nil {
		return nil, ErrGitHubPolicy
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := g.client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: github request deadline exceeded", ErrTimedOut)
		}
		return nil, ErrGitHubPolicy
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumGitHubResponse))
	if err != nil {
		return nil, ErrGitHubPolicy
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrGitHubPolicy
	}
	return []byte(logging.RedactSensitiveText(string(data))), nil
}

func (g githubClient) doJSON(ctx context.Context, method, path, token string, body, target any) error {
	data, err := g.call(ctx, method, path, token, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return ErrGitHubPolicy
	}
	return nil
}

func validateGitHubCredentialID(id string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || strings.ContainsRune(id, '\x00') || len(id) > 128 {
		return errors.New("credential_id is required and must be a plain identifier")
	}
	return nil
}

func validateGitHubIssueReadInput(input GitHubIssueReadInput) error {
	if input.Action != "get" && input.Action != "list" {
		return errors.New("github.issue.read action must be get or list")
	}
	if input.Action == "get" && input.Number <= 0 {
		return errors.New("github.issue.read get requires a positive issue number")
	}
	if input.Action == "list" && input.State != "" && input.State != "open" && input.State != "closed" && input.State != "all" {
		return errors.New("github.issue.read state is invalid")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubIssueCreateInput(input GitHubIssueCreateInput) error {
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 4000 || strings.ContainsRune(input.Title, '\x00') {
		return errors.New("github.issue.create title is required and bounded")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubIssueUpdateInput(input GitHubIssueUpdateInput) error {
	if input.Number <= 0 {
		return errors.New("github.issue.update requires a positive issue number")
	}
	if input.Body == "" && input.State == "" {
		return errors.New("github.issue.update requires a body or state change")
	}
	if input.State != "" && input.State != "open" && input.State != "closed" {
		return errors.New("github.issue.update state is invalid")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubPRReadInput(input GitHubPRReadInput) error {
	if input.Action != "get" && input.Action != "list" {
		return errors.New("github.pr.read action must be get or list")
	}
	if input.Action == "get" && input.Number <= 0 {
		return errors.New("github.pr.read get requires a positive pull request number")
	}
	if input.Action == "list" && input.State != "" && input.State != "open" && input.State != "closed" && input.State != "all" {
		return errors.New("github.pr.read state is invalid")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubPRCreateInput(input GitHubPRCreateInput) error {
	if strings.TrimSpace(input.Title) == "" || len(input.Title) > 4000 || strings.ContainsRune(input.Title, '\x00') {
		return errors.New("github.pr.create title is required and bounded")
	}
	if !validRefName(input.Head) || !validRefName(input.Base) {
		return errors.New("github.pr.create head and base must be plain ref names")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubPRReviewInput(input GitHubPRReviewInput) error {
	if input.Number <= 0 {
		return errors.New("github.pr.review requires a positive pull request number")
	}
	if input.Event != "approve" && input.Event != "request_changes" && input.Event != "comment" {
		return errors.New("github.pr.review event is invalid")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validateGitHubPRMergeInput(input GitHubPRMergeInput) error {
	if input.Number <= 0 {
		return errors.New("github.pr.merge requires a positive pull request number")
	}
	if input.MergeMethod != "" && input.MergeMethod != "merge" && input.MergeMethod != "squash" && input.MergeMethod != "rebase" {
		return errors.New("github.pr.merge merge_method is invalid")
	}
	return validateGitHubCredentialID(input.CredentialID)
}

func validRefName(value string) bool {
	if value == "" || len(value) > 255 || strings.ContainsAny(value, " \t\n\r\\") || strings.Contains(value, "..") || strings.HasPrefix(value, "-") {
		return false
	}
	return true
}

func runGitHubIssueRead(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubIssueReadInput) (GitHubIssueReadOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	issues := make([]GitHubIssue, 0)
	if input.Action == "get" {
		var value GitHubIssue
		if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, input.Number), token, nil, &value); err != nil {
			return GitHubIssueReadOutput{}, ErrGitHubPolicy
		}
		issues = append(issues, value)
	} else {
		state := "open"
		if input.State != "" {
			state = input.State
		}
		var values []GitHubIssue
		path := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=%d", owner, repo, state, maximumGitHubListItems)
		if err := client.doJSON(ctx, http.MethodGet, path, token, nil, &values); err != nil {
			return GitHubIssueReadOutput{}, ErrGitHubPolicy
		}
		issues = values
	}
	return GitHubIssueReadOutput{Issues: issues, Count: len(issues)}, nil
}

func runGitHubIssueCreate(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubIssueCreateInput) (GitHubIssueCreateOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	var value GitHubIssueCreateOutput
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), token, map[string]string{"title": input.Title, "body": input.Body}, &value); err != nil {
		return GitHubIssueCreateOutput{}, ErrGitHubPolicy
	}
	return value, nil
}

func runGitHubIssueUpdate(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubIssueUpdateInput) (GitHubIssueUpdateOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	body := map[string]string{}
	if input.Body != "" {
		body["body"] = input.Body
	}
	if input.State != "" {
		body["state"] = input.State
	}
	var value GitHubIssueUpdateOutput
	if err := client.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, input.Number), token, body, &value); err != nil {
		return GitHubIssueUpdateOutput{}, ErrGitHubPolicy
	}
	return value, nil
}

func runGitHubPRRead(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubPRReadInput) (GitHubPRReadOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	pulls := make([]GitHubPullRequest, 0)
	if input.Action == "get" {
		var value GitHubPullRequest
		if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, input.Number), token, nil, &value); err != nil {
			return GitHubPRReadOutput{}, ErrGitHubPolicy
		}
		pulls = append(pulls, value)
	} else {
		state := "open"
		if input.State != "" {
			state = input.State
		}
		var values []GitHubPullRequest
		path := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d", owner, repo, state, maximumGitHubListItems)
		if err := client.doJSON(ctx, http.MethodGet, path, token, nil, &values); err != nil {
			return GitHubPRReadOutput{}, ErrGitHubPolicy
		}
		pulls = values
	}
	return GitHubPRReadOutput{PullRequests: pulls, Count: len(pulls)}, nil
}

func runGitHubPRCreate(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubPRCreateInput) (GitHubPRCreateOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	var value GitHubPRCreateOutput
	body := map[string]string{"title": input.Title, "head": input.Head, "base": input.Base, "body": input.Body}
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), token, body, &value); err != nil {
		return GitHubPRCreateOutput{}, ErrGitHubPolicy
	}
	return value, nil
}

func runGitHubPRReview(ctx context.Context, _ Request, config GitHubConfig, owner, repo, token string, input GitHubPRReviewInput) (GitHubPRReviewOutput, error) {
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	var value GitHubPRReviewOutput
	body := map[string]string{"event": input.Event, "body": input.Body}
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, input.Number), token, body, &value); err != nil {
		return GitHubPRReviewOutput{}, ErrGitHubPolicy
	}
	return value, nil
}

func runGitHubPRMerge(ctx context.Context, request Request, config GitHubConfig, owner, repo, token string, input GitHubPRMergeInput) (GitHubPRMergeOutput, error) {
	if !config.AllowMerge {
		return GitHubPRMergeOutput{}, ErrGitHubDenied
	}
	if err := requireGitHubHumanReviewGate(ctx, config, request); err != nil {
		return GitHubPRMergeOutput{}, err
	}
	client := githubClient{baseURL: config.APIBaseURL, client: githubHTTPClient(config)}
	body := map[string]string{}
	if input.MergeMethod != "" {
		body["merge_method"] = input.MergeMethod
	}
	var value GitHubPRMergeOutput
	if err := client.doJSON(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, input.Number), token, body, &value); err != nil {
		return GitHubPRMergeOutput{}, ErrGitHubPolicy
	}
	return value, nil
}

func githubHTTPClient(config GitHubConfig) *http.Client {
	if config.Client != nil {
		return config.Client
	}
	return &http.Client{Timeout: config.Timeout}
}

// requireGitHubHumanReviewGate enforces the configured Human Review requirement
// for pull-request merges. The operation must be tied to an approved task when
// the project demands human review.
func requireGitHubHumanReviewGate(ctx context.Context, config GitHubConfig, request Request) error {
	if config.HumanReviewRequired == nil {
		return ErrGitHubDenied
	}
	required, err := config.HumanReviewRequired(ctx, request.ProjectID)
	if err != nil || !required {
		return nil
	}
	if request.TaskID == "" || config.TaskApproved == nil {
		return ErrGitHubDenied
	}
	approved, err := config.TaskApproved(ctx, request.ProjectID, request.TaskID)
	if err != nil || !approved {
		return ErrGitHubDenied
	}
	return nil
}
