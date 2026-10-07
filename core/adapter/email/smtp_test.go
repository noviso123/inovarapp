package email

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSMTPSenderTransmitsUTF8HTML(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		write := func(line string) bool {
			if _, err := fmt.Fprint(writer, line+"\r\n"); err != nil {
				return false
			}
			return writer.Flush() == nil
		}
		if !write("220 local test SMTP") {
			return
		}
		data := false
		var body strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				serverErr <- readErr
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if data {
				if line == "." {
					data = false
					received <- body.String()
					body.Reset()
					if !write("250 queued") {
						return
					}
				} else {
					if strings.HasPrefix(line, "..") {
						line = line[1:]
					}
					body.WriteString(line + "\r\n")
				}
				continue
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO "):
				_, _ = fmt.Fprint(writer, "250-localhost\r\n250 AUTH PLAIN\r\n")
				_ = writer.Flush()
			case strings.HasPrefix(upper, "AUTH PLAIN "):
				raw, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
				if decodeErr != nil || !strings.Contains(string(raw), "smtp-secret") {
					serverErr <- fmt.Errorf("auth payload invalid: %q", raw)
					return
				}
				if !write("235 authenticated") {
					return
				}
			case strings.HasPrefix(upper, "MAIL FROM:"):
				if !write("250 sender accepted") {
					return
				}
			case strings.HasPrefix(upper, "RCPT TO:"):
				if !write("250 recipient accepted") {
					return
				}
			case upper == "DATA":
				data = true
				if !write("354 send data") {
					return
				}
			case upper == "QUIT":
				_ = write("221 bye")
				return
			default:
				if !write("250 ok") {
					return
				}
			}
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = (SMTPSender{}).Send(ctx, Config{Username: "inovar@example.com", Password: "smtp-secret", Host: "localhost", Port: fmt.Sprint(address.Port)}, Message{To: "cliente@example.com", Subject: "Orçamento — João", HTML: "<p>Olá, João! ❄️</p>"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		if !strings.Contains(message, "Content-Type: text/html; charset=UTF-8") || !strings.Contains(message, "Content-Transfer-Encoding: quoted-printable") || !strings.Contains(message, "Subject: =?UTF-8?q?Or=C3=A7amento") || !strings.Contains(message, "Ol=C3=A1, Jo=C3=A3o!") {
			t.Fatalf("SMTP message did not preserve MIME UTF-8: %s", message)
		}
	case err := <-serverErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("SMTP server timed out")
	}
}

func TestSMTPSenderRejectsHeaderInjectionAndInvalidAddresses(t *testing.T) {
	for _, message := range []Message{{To: "cliente@example.com", Subject: "assunto\r\nBcc: attacker@example.com", HTML: "x"}, {To: "não é e-mail", Subject: "ok", HTML: "x"}, {To: "cliente@example.com", Subject: "sem corpo", HTML: ""}} {
		if _, err := (SMTPSender{}).Send(context.Background(), Config{Username: "inovar@example.com", Password: "secret", Host: "127.0.0.1", Port: "1"}, message); err == nil {
			t.Fatalf("invalid message accepted: %#v", message)
		}
	}
}
