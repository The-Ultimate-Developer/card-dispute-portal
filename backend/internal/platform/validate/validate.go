package validate

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// UUID checks format before an ID reaches the database, so a malformed ID is a
// clean 404 instead of a database cast error.
func UUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// Errors collects field problems so a client sees every issue in one response.
type Errors map[string]string

func (e Errors) Add(field, msg string) {
	if _, exists := e[field]; !exists {
		e[field] = msg
	}
}

func (e Errors) Any() bool { return len(e) > 0 }

func (e Errors) Length(field, value string, lo, hi int) {
	n := utf8.RuneCountInString(strings.TrimSpace(value))
	switch {
	case n == 0 && lo > 0:
		e.Add(field, "is required")
	case n < lo:
		e.Add(field, "is too short")
	case n > hi:
		e.Add(field, "is too long")
	}
}
