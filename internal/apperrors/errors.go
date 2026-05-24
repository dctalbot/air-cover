package apperrors

import "errors"

var (
	ErrInvalid   = errors.New("invalid input")
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)
