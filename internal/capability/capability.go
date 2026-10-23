// Package capability defines provider-independent permission identifiers.
package capability

import (
	"errors"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z0-9]+)*$`)

var ErrDenied = errors.New("authorization denied")

type Capability string

const (
	ShellExecute Capability = "shell.execute"

	GitRead   Capability = "git.read"
	GitDiff   Capability = "git.diff"
	GitBranch Capability = "git.branch"
	GitCommit Capability = "git.commit"
	GitPush   Capability = "git.push"
	GitMerge  Capability = "git.merge"
)

func New(identifier string) (Capability, error) {
	if strings.TrimSpace(identifier) != identifier || !identifierPattern.MatchString(identifier) {
		return "", errors.New("capability must be a lowercase dotted identifier")
	}
	return Capability(identifier), nil
}

func (c Capability) Valid() bool {
	return identifierPattern.MatchString(string(c))
}

// Principal is an authenticated actor with explicit capabilities and project scope.
type Principal struct {
	ID           string
	Capabilities []Capability
	ProjectIDs   []string
}

// Authorizer evaluates a capability and, when supplied, a project boundary.
type Authorizer struct{}

// Authorize denies invalid, missing, or out-of-scope grants.
func (Authorizer) Authorize(principal Principal, required Capability, projectID string) error {
	if strings.TrimSpace(principal.ID) == "" || !required.Valid() {
		return ErrDenied
	}
	granted := false
	for _, value := range principal.Capabilities {
		if value.Valid() && value == required {
			granted = true
			break
		}
	}
	if !granted {
		return ErrDenied
	}
	if projectID == "" {
		return nil
	}
	for _, value := range principal.ProjectIDs {
		if strings.TrimSpace(value) == projectID {
			return nil
		}
	}
	return ErrDenied
}
