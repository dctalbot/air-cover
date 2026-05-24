package spinitron

import (
	adminapp "air-cover/internal/app/admin"
	subrequestsapp "air-cover/internal/app/subrequests"
)

var (
	_ adminapp.Catalog       = (*Catalog)(nil)
	_ subrequestsapp.Catalog = (*Catalog)(nil)
	_ PageClient             = (*Client)(nil)
)
