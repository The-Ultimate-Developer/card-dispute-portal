package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
)

// WriteJSON sets headers, then status, then encodes v.
// Headers must be set before WriteHeader or they are silently dropped.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Problem is an RFC 7807 body. One error shape everywhere means one error path in the client.
type Problem struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Code      string            `json:"code,omitempty"`
	Detail    string            `json:"detail,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
	RequestID string            `json:"requestId,omitempty"`
}

func WriteProblem(w http.ResponseWriter, _ *http.Request, status int, detail string) {
	writeProblem(w, Problem{Status: status, Detail: detail})
}

func writeProblem(w http.ResponseWriter, p Problem) {
	p.Type = "about:blank"
	p.Title = http.StatusText(p.Status)
	p.RequestID = w.Header().Get("X-Request-ID")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

var statusByKind = map[apperr.Kind]int{
	apperr.KindValidation:       http.StatusBadRequest,
	apperr.KindUnauthorized:     http.StatusUnauthorized,
	apperr.KindForbidden:        http.StatusForbidden,
	apperr.KindNotFound:         http.StatusNotFound,
	apperr.KindConflict:         http.StatusConflict,
	apperr.KindUnprocessable:    http.StatusUnprocessableEntity,
	apperr.KindLocked:           http.StatusLocked,
	apperr.KindRateLimited:      http.StatusTooManyRequests,
	apperr.KindUnavailable:      http.StatusServiceUnavailable,
	apperr.KindUnsupportedMedia: http.StatusUnsupportedMediaType,
	apperr.KindTooLarge:         http.StatusRequestEntityTooLarge,
}

// WriteError maps domain errors to responses. Anything unrecognised is logged
// in full server-side and returned as a generic 500, so internals never leak.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var ae *apperr.Error
	status, known := 0, false
	if errors.As(err, &ae) {
		status, known = statusByKind[ae.Kind]
	}
	if !known {
		log.ErrorContext(r.Context(), "unhandled error",
			"request_id", w.Header().Get("X-Request-ID"), "path", r.URL.Path, "error", err)
		writeProblem(w, Problem{Status: http.StatusInternalServerError, Code: "INTERNAL", Detail: "An unexpected error occurred."})
		return
	}
	if ae.Kind == apperr.KindUnavailable && ae.Err != nil {
		log.WarnContext(r.Context(), "dependency unavailable",
			"request_id", w.Header().Get("X-Request-ID"), "code", ae.Code, "error", ae.Err)
	}
	if ae.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(ae.RetryAfter.Seconds()+0.999)))
	}
	writeProblem(w, Problem{Status: status, Code: ae.Code, Detail: ae.Message, Errors: ae.Fields})
}

// DecodeJSON requires a JSON content type (which plain HTML forms cannot send,
// a cheap CSRF defence), rejects unknown fields and trailing data.
func DecodeJSON(r *http.Request, dst any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return &apperr.Error{Kind: apperr.KindUnsupportedMedia, Code: "UNSUPPORTED_MEDIA_TYPE", Message: "Content-Type must be application/json."}
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &tooLarge):
			return &apperr.Error{Kind: apperr.KindTooLarge, Code: "BODY_TOO_LARGE", Message: "Request body is too large."}
		case errors.As(err, &typeErr):
			return apperr.Validation(map[string]string{typeErr.Field: "has the wrong type"})
		case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
			return apperr.BadRequest("MALFORMED_JSON", "Request body is not valid JSON.")
		default:
			return apperr.BadRequest("MALFORMED_JSON", fmt.Sprintf("Request body could not be read: %v", err))
		}
	}
	if dec.More() {
		return apperr.BadRequest("MALFORMED_JSON", "Request body must contain a single JSON object.")
	}
	return nil
}
