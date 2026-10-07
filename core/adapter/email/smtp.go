package email

import (
	"context"
	"crypto/tls"
	"errors"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Username           string
	Password           string
	Host               string
	Port               string
	APIKey             string
	From               string
	GoogleRefreshToken string
	GoogleClientID     string
	GoogleClientSecret string
}

type Message struct {
	To      string
	Subject string
	HTML    string
}

type Sender interface {
	Send(context.Context, Config, Message) (string, error)
}

func Configured(config Config) bool {
	return strings.TrimSpace(config.GoogleRefreshToken) != "" ||
		strings.TrimSpace(config.APIKey) != "" && strings.TrimSpace(config.From) != "" ||
		strings.TrimSpace(config.Username) != "" && strings.TrimSpace(config.Password) != ""
}

type SMTPSender struct{ Dialer *net.Dialer }

func (s SMTPSender) Send(ctx context.Context, config Config, message Message) (string, error) {
	config.Username, config.Password = strings.TrimSpace(config.Username), strings.TrimSpace(config.Password)
	if config.Username == "" || config.Password == "" {
		return "", errors.New("gmail não configurado")
	}
	if config.Host == "" {
		config.Host = "smtp.gmail.com"
	}
	if config.Port == "" {
		config.Port = "587"
	}
	if strings.ContainsAny(message.To+message.Subject, "\r\n") || strings.TrimSpace(message.HTML) == "" {
		return "", errors.New("mensagem inválida")
	}
	address, err := mail.ParseAddress(message.To)
	if err != nil || address.Address != message.To {
		return "", errors.New("e-mail inválido")
	}
	from, err := mail.ParseAddress(config.Username)
	if err != nil || from.Address != config.Username {
		return "", errors.New("remetente inválido")
	}
	server := net.JoinHostPort(config.Host, config.Port)
	dialer := s.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 12 * time.Second}
	}
	conn, err := dialer.DialContext(ctx, "tcp", server)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	_ = conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return "", err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return "", err
		}
	} else if config.Host == "smtp.gmail.com" {
		return "", errors.New("servidor Gmail não oferece STARTTLS")
	}
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return "", err
		}
	} else {
		return "", errors.New("servidor SMTP não oferece autenticação")
	}
	if err := client.Mail(from.Address); err != nil {
		return "", err
	}
	if err := client.Rcpt(address.Address); err != nil {
		return "", err
	}
	writer, err := client.Data()
	if err != nil {
		return "", err
	}
	headers := "From: " + mime.QEncoding.Encode("UTF-8", "Inovar Refrigeração") + " <" + from.Address + ">\r\n" +
		"To: " + address.String() + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n"
	if _, err := writer.Write([]byte(headers)); err != nil {
		_ = writer.Close()
		return "", err
	}
	encoded := quotedprintable.NewWriter(writer)
	if _, err := encoded.Write([]byte(message.HTML + "\r\n")); err != nil {
		_ = encoded.Close()
		_ = writer.Close()
		return "", err
	}
	if err := encoded.Close(); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	if err := client.Quit(); err != nil {
		return "", err
	}
	return "", nil
}
