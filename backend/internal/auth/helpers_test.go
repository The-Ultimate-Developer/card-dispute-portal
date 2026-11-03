package auth

import (
	"errors"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
)

func apperrCode(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}
