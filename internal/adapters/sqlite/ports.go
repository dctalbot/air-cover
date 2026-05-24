package sqlite

import (
	adminapp "air-cover/internal/app/admin"
	authapp "air-cover/internal/app/auth"
	bootstrapapp "air-cover/internal/app/bootstrap"
	subrequestsapp "air-cover/internal/app/subrequests"
)

var (
	_ adminapp.Repository       = (*Repository)(nil)
	_ authapp.Repository        = (*Repository)(nil)
	_ bootstrapapp.Repository   = (*Repository)(nil)
	_ subrequestsapp.Repository = (*Repository)(nil)
)
