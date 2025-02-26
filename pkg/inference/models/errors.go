package models

import (
	"errors"
)

// ErrModelNotFound is a sentinel error returned by Manager.GetModel if the
// model could not be located. If returned in conjunction with an HTTP
// request, it should be paired with a 404 response status.
var ErrModelNotFound = errors.New("model not found")
