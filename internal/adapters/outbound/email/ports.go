package email

import (
	authapp "air-cover/internal/app/auth"
	subrequestsapp "air-cover/internal/app/subrequests"
)

var (
	_ authapp.Sender          = (*ConsoleSender)(nil)
	_ authapp.Sender          = (*ResendSender)(nil)
	_ authapp.Sender          = (*SendGridSender)(nil)
	_ subrequestsapp.Notifier = (*SubRequestNotifier)(nil)
	_ subrequestsapp.Notifier = (*AsyncNotifier)(nil)
)
