package email

import authapp "air-cover/internal/app/auth"

var (
	_ authapp.Sender = (*ConsoleSender)(nil)
	_ authapp.Sender = (*SendGridSender)(nil)
)
