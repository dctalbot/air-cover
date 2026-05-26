package apperrors

import "errors"

var (
	ErrInvalid   = errors.New("invalid input")
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

// TODO
// conventional validation error messages:
// accepted
// blank
// present
// confirmation
// empty
// equal_to
// even
// exclusion
// greater_than
// greater_than_or_equal_to
// inclusion
// invalid
// less_than
// less_than_or_equal_to
// model_invalid
// not_a_number
// not_an_integer
// odd
// other_than
// required
// taken
// too_long
// too_short
// wrong_length
