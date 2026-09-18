package ghclient

import (
	"errors"

	"github.com/google/go-github/v92/github"
)

func asErrorResponse(err error, target **github.ErrorResponse) bool {
	return errors.As(err, target)
}
