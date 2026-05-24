package email

import coreemail "air-cover/internal/email"

type Sender = coreemail.Sender
type ConsoleSender = coreemail.ConsoleSender
type SendGridSender = coreemail.SendGridSender

func NewSender(apiKey, fromEmail, env string) Sender {
	return coreemail.NewSender(apiKey, fromEmail, env)
}
