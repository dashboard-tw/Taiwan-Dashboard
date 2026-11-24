package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string
}

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	Tag     string
	ID      string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

type SMTP struct {
	cfg  Config
	from *mail.Address
}

func NewSMTP(cfg Config) (*SMTP, error) {
	if cfg.Host == "" {
		return nil, errors.New("smtp host is empty")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail from: %w", err)
	}
	if cfg.FromName != "" {
		from.Name = cfg.FromName
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail to: %w", err)
	}
	body, err := s.build(to, m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not offer STARTTLS")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (s *SMTP) build(to *mail.Address, m Message) ([]byte, error) {
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, errors.New("subject contains a line break")
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := s.from.Address[strings.LastIndexByte(s.from.Address, '@')+1:]
	var b bytes.Buffer
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", s.from.String())
	header("To", to.String())
	header("Subject", mime.BEncoding.Encode("UTF-8", m.Subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Auto-Submitted", "auto-generated")
	if m.HTML == "" {
		header("Content-Type", "text/plain; charset=UTF-8")
		header("Content-Transfer-Encoding", "base64")
		b.WriteString("\r\n")
		writeBase64(&b, m.Text)
		return b.Bytes(), nil
	}
	boundary := "dtw-" + hex.EncodeToString(id)
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ kind, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + part.kind + "; charset=UTF-8\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		writeBase64(&b, part.body)
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}

func writeBase64(b *bytes.Buffer, s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
}
