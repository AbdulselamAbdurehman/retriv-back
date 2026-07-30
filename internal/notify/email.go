package notify

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPMailer sends plaintext email over authenticated SMTP (e.g. Gmail on
// smtp.gmail.com:587). smtp.SendMail negotiates STARTTLS automatically when the
// server advertises it and auth is supplied.
type SMTPMailer struct {
	host string
	addr string // host:port
	auth smtp.Auth
	from string
}

func NewSMTPMailer(host, port, username, password, from string) *SMTPMailer {
	if from == "" {
		from = username // Gmail requires the From to match the authenticated account
	}
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	return &SMTPMailer{host: host, addr: host + ":" + port, auth: auth, from: from}
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		m.from, to, subject, body)
	if err := smtp.SendMail(m.addr, m.auth, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("notify: send mail: %w", err)
	}
	return nil
}
