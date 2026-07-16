package mediawiki

import (
	"fmt"
	"strings"
)

var genericUserAgents = map[string]struct{}{
	"curl": {}, "wget": {}, "go-http-client": {}, "python-requests": {},
	"java": {}, "okhttp": {}, "mozilla": {},
}

// ValidateUserAgent checks that a User-Agent is descriptive enough for
// automated Wikimedia access. Contact information is strongly recommended.
func ValidateUserAgent(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 6 || !strings.Contains(value, "/") {
		return fmt.Errorf("%w: use product/version and contact information", ErrInvalidUserAgent)
	}
	product := strings.ToLower(strings.TrimSpace(strings.SplitN(value, "/", 2)[0]))
	if _, generic := genericUserAgents[product]; generic {
		return fmt.Errorf("%w: generic product %q", ErrInvalidUserAgent, product)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%w: contains a newline", ErrInvalidUserAgent)
	}
	return nil
}
