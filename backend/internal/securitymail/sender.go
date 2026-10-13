// Package securitymail provides bounded, fail-closed delivery for security
// challenges. SMTP acceptance is not proof that a user owns the destination;
// only successful challenge verification may establish that proof.
package securitymail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"signet-backend/internal/config"
)

type Sender struct{ cfg *config.Config }

func New(cfg *config.Config) *Sender { return &Sender{cfg: cfg} }

func validMailbox(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

func (s *Sender) Send(ctx context.Context, to, subject, body string) error {
	return s.send(ctx, to, subject, body, "")
}

func (s *Sender) SendHTML(ctx context.Context, to, subject, plain, html string) error {
	return s.send(ctx, to, subject, plain, html)
}

func mailContent(plain, html string) (string, string, error) {
	if html == "" {
		return "text/plain; charset=UTF-8", plain, nil
	}
	var buffer bytes.Buffer
	w := multipart.NewWriter(&buffer)
	for _, part := range []struct{ kind, body string }{{"text/plain", plain}, {"text/html", html}} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.kind+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		p, err := w.CreatePart(header)
		if err != nil {
			return "", "", err
		}
		encoded := quotedprintable.NewWriter(p)
		if _, err = encoded.Write([]byte(part.body)); err != nil {
			return "", "", err
		}
		if err = encoded.Close(); err != nil {
			return "", "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", "", err
	}
	return "multipart/alternative; boundary=" + w.Boundary(), buffer.String(), nil
}

func (s *Sender) send(ctx context.Context, to, subject, body, html string) error {
	if s.cfg == nil || !validMailbox(to) || !validMailbox(s.cfg.MailFrom) || subject == "" || strings.ContainsAny(subject, "\r\n") {
		return errors.New("invalid security mail configuration or destination")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(s.cfg.MailHost, s.cfg.MailPort))
	if err != nil {
		return errors.New("security mail transport unavailable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return errors.New("security mail deadline unavailable")
	}
	client, err := smtp.NewClient(conn, s.cfg.MailHost)
	if err != nil {
		return errors.New("security mail connection rejected")
	}
	defer client.Close()
	if err := client.Hello("signet-app"); err != nil {
		return errors.New("security mail greeting rejected")
	}
	ip := net.ParseIP(s.cfg.MailHost)
	allowPlain := s.cfg.AppEnv == "local" && ip != nil && ip.IsLoopback()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.MailHost, MinVersion: tls.VersionTLS12}); err != nil {
			return errors.New("security mail TLS failed")
		}
	} else if !allowPlain {
		return errors.New("security mail requires TLS")
	}
	if s.cfg.MailUser != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.MailUser, s.cfg.MailPass, s.cfg.MailHost)); err != nil {
			return errors.New("security mail authentication failed")
		}
	}
	if err := client.Mail(s.cfg.MailFrom); err != nil {
		return errors.New("security mail sender rejected")
	}
	if err := client.Rcpt(to); err != nil {
		return errors.New("security mail recipient rejected")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("security mail data rejected")
	}
	kind, content, err := mailContent(body, html)
	if err != nil {
		return errors.New("mail content unavailable")
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n\r\n%s", s.cfg.MailFrom, to, subject, kind, content)
	if _, err := writer.Write([]byte(message)); err != nil {
		writer.Close()
		return errors.New("security mail write failed")
	}
	if err := writer.Close(); err != nil {
		return errors.New("security mail was not accepted")
	}
	// DATA acceptance is the delivery boundary. A subsequent QUIT failure
	// must not trigger another challenge send with a new code.
	return nil
}
