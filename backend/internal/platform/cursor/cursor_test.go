package cursor

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 30, 15, 123456789, time.UTC)
	id := "3f1c2b8e-9a4d-4c51-8f0e-2b7d6a1c9e05"

	pos, err := Decode(Encode(at, id))
	if err != nil {
		t.Fatal(err)
	}
	if !pos.At.Equal(at) || *pos.ID != id {
		t.Fatalf("round trip mismatch: %v %v", pos.At, *pos.ID)
	}
}

func TestEmptyIsFirstPage(t *testing.T) {
	pos, err := Decode("")
	if err != nil || pos.At != nil || pos.ID != nil {
		t.Fatalf("empty cursor should be the first page, got %+v %v", pos, err)
	}
}

func TestRejectsTampering(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	for _, c := range []string{
		"%%%",
		enc([]byte("no-separator")),
		enc([]byte("2026-10-01T12:00:00Z|not-a-uuid")),
		enc([]byte("yesterday|3f1c2b8e-9a4d-4c51-8f0e-2b7d6a1c9e05")),
		enc([]byte("2026-10-01T12:00:00Z|x'); DROP TABLE disputes;--")),
	} {
		if _, err := Decode(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}
