package notify

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPMailer sends plaintext email over SMTP. In dev this points at Mailpit
// (no auth); in prod, at a real relay.
type SMTPMailer struct {
	addr string // host:port
	from string
}

func NewSMTPMailer(addr, from string) *SMTPMailer {
	return &SMTPMailer{addr: addr, from: from}
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		m.from, to, subject, body)
	if err := smtp.SendMail(m.addr, nil, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("notify: send mail: %w", err)
	}
	return nil
}
