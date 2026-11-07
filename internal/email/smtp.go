package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const smtpTimeout = 10 * time.Second

// SMTPSender entrega e-mails por SMTP autenticado usando STARTTLS obrigatório.
// Ele atende provedores como Gmail que exigem senha de aplicativo.
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
}

// NewSMTPSender cria um remetente SMTP. A configuração é validada no startup.
func NewSMTPSender(host string, port int, username, password string) *SMTPSender {
	return &SMTPSender{host: host, port: port, username: username, password: password}
}

// Send envia as alternativas texto e HTML pelo canal SMTP protegido por TLS.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	from, err := mailbox(msg.From)
	if err != nil {
		return fmt.Errorf("remetente SMTP inválido: %w", err)
	}
	to, err := mailbox(msg.To)
	if err != nil {
		return fmt.Errorf("destinatário SMTP inválido: %w", err)
	}
	data, err := smtpMessage(msg)
	if err != nil {
		return err
	}

	dialer := net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)))
	if err != nil {
		return fmt.Errorf("conectar ao servidor SMTP: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("iniciar cliente SMTP: %w", err)
	}
	defer client.Quit()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("servidor SMTP não oferece STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
		return fmt.Errorf("iniciar TLS no SMTP: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
		return fmt.Errorf("autenticar no SMTP: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("definir remetente SMTP: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("definir destinatário SMTP: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("iniciar dados SMTP: %w", err)
	}
	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return fmt.Errorf("escrever mensagem SMTP: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("finalizar mensagem SMTP: %w", err)
	}
	return nil
}

func mailbox(value string) (string, error) {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address == "" {
		return "", fmt.Errorf("endereço inválido")
	}
	return address.Address, nil
}

func smtpMessage(msg Message) ([]byte, error) {
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return nil, errors.New("assunto SMTP contém quebra de linha")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, part := range []struct{ contentType, content string }{{"text/plain; charset=UTF-8", msg.TextBody}, {"text/html; charset=UTF-8", msg.HTMLBody}} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", part.contentType)
		h.Set("Content-Transfer-Encoding", "8bit")
		pw, err := w.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("montar mensagem SMTP: %w", err)
		}
		if _, err := pw.Write([]byte(part.content)); err != nil {
			return nil, fmt.Errorf("montar mensagem SMTP: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("montar mensagem SMTP: %w", err)
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", msg.From, msg.To, mime.QEncoding.Encode("UTF-8", msg.Subject), w.Boundary())
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

var _ Sender = (*SMTPSender)(nil)
