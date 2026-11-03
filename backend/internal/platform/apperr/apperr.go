// Package apperr defines the error vocabulary shared by every module.
// Services return these; the HTTP layer maps each Kind to one status code.
package apperr

import (
	"errors"
	"time"
)

type Kind int

const (
	KindInternal Kind = iota
	KindValidation
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindUnprocessable
	KindLocked
	KindRateLimited
	KindUnavailable
	KindUnsupportedMedia
	KindTooLarge
)

type Error struct {
	Kind       Kind
	Code       string
	Message    string
	Fields     map[string]string
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func KindOf(err error) Kind {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Kind
	}
	return KindInternal
}

func Validation(fields map[string]string) *Error {
	return &Error{Kind: KindValidation, Code: "VALIDATION_FAILED", Message: "One or more fields are invalid.", Fields: fields}
}

func BadRequest(code, msg string) *Error {
	return &Error{Kind: KindValidation, Code: code, Message: msg}
}

func Unauthorized(code, msg string) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Message: msg}
}

func Forbidden(code, msg string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: msg}
}

func NotFound(code, msg string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: msg}
}

func Conflict(code, msg string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: msg}
}

func Unprocessable(code, msg string) *Error {
	return &Error{Kind: KindUnprocessable, Code: code, Message: msg}
}

func Locked(code, msg string) *Error {
	return &Error{Kind: KindLocked, Code: code, Message: msg}
}

func RateLimited(retryAfter time.Duration) *Error {
	return &Error{Kind: KindRateLimited, Code: "RATE_LIMITED", Message: "Too many requests. Please wait and try again.", RetryAfter: retryAfter}
}

func Unavailable(code, msg string, cause error) *Error {
	return &Error{Kind: KindUnavailable, Code: code, Message: msg, Err: cause}
}
