// Package cursor implements opaque keyset-pagination cursors. The cursor is the
// (timestamp, id) of the last row seen, so pages stay stable while new rows
// arrive and deep pages cost the same as the first.
package cursor

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/validate"
)

var ErrInvalid = errors.New("invalid cursor")

type Position struct {
	At *time.Time
	ID *string
}

func Encode(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// Decode returns an empty Position for an empty cursor (the first page).
func Decode(s string) (Position, error) {
	if s == "" {
		return Position{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Position{}, ErrInvalid
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok || !validate.UUID(id) {
		return Position{}, ErrInvalid
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Position{}, ErrInvalid
	}
	return Position{At: &at, ID: &id}, nil
}
