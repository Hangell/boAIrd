// Package capability defines provider-independent permission identifiers.
package capability

import (
	"errors"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

type Capability string

func New(identifier string) (Capability, error) {
	if strings.TrimSpace(identifier) != identifier || !identifierPattern.MatchString(identifier) {
		return "", errors.New("capability must be a lowercase dotted identifier")
	}
	return Capability(identifier), nil
}

func (c Capability) Valid() bool {
	return identifierPattern.MatchString(string(c))
}
