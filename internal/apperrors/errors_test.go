package apperrors

import "testing"

func TestErrors(t *testing.T) {
	tests := []error{
		ErrInvalid,
		ErrNotFound,
		ErrConflict,
		ErrForbidden,
	}
	for _, err := range tests {
		if err == nil || err.Error() == "" {
			t.Fatalf("expected named application error, got %v", err)
		}
	}
}
