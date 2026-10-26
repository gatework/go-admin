package account

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type SMTP struct {
	Host, Port, Username, Password, From string
	Insecure                             bool
	ImplicitTLS                          bool
}

func SMTPFromEnv() Mailer {
	host, from := os.Getenv("ACCOUNT_SMTP_HOST"), os.Getenv("ACCOUNT_SMTP_FROM")
	if host == "" || from == "" {
		return nil
	}
	port := os.Getenv("ACCOUNT_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return SMTP{Host: host, Port: port, From: from, Username: os.Getenv("ACCOUNT_SMTP_USER"), Password: os.Getenv("ACCOUNT_SMTP_PASSWORD"), Insecure: os.Getenv("ACCOUNT_SMTP_INSECURE") == "true", ImplicitTLS: os.Getenv("ACCOUNT_SMTP_TLS") == "implicit"}
}
func (m SMTP) Send(ctx context.Context, to, subject, body string) error {
	from, e := mail.ParseAddress(m.From)
	if e != nil || strings.ContainsAny(m.From+to+subject, "\r\n") {
		return ErrUnavailable
	}
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(m.Host, m.Port))
	if e != nil {
		return fmt.Errorf("mail connection failed: %w", e)
	}
	defer conn.Close()
	deadline := time.Now().Add(8 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if e = conn.SetDeadline(deadline); e != nil {
		return e
	}
	tlsConfig := &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}
	if m.ImplicitTLS {
		secured := tls.Client(conn, tlsConfig)
		if e = secured.HandshakeContext(ctx); e != nil {
			return e
		}
		conn = secured
	}
	client, e := smtp.NewClient(conn, m.Host)
	if e != nil {
		return e
	}
	defer client.Close()
	if !m.ImplicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if e = client.StartTLS(tlsConfig); e != nil {
				return e
			}
		} else if !m.Insecure {
			return ErrUnavailable
		}
	}
	if m.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); e != nil {
			return e
		}
	}
	if e = client.Mail(from.Address); e != nil {
		return e
	}
	if e = client.Rcpt(to); e != nil {
		return e
	}
	w, e := client.Data()
	if e != nil {
		return e
	}
	_, e = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", m.From, to, subject, strings.ReplaceAll(body, "\n", "\r\n"))
	if e != nil {
		return e
	}
	if e = w.Close(); e != nil {
		return e
	}
	return client.Quit()
}
