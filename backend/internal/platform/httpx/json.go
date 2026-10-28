package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON sets headers, then status, then encodes v.
// Headers must be set BEFORE WriteHeader or they are silently dropped.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // status is already sent; nothing useful to do on error
}

// Problem is an RFC 7807 error body. One error format everywhere means the
// frontend needs exactly one error-handling path.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func WriteProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	p := Problem{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		RequestID: w.Header().Get("X-Request-ID"),
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
