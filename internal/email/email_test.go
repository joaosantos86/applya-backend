package email

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// captureSender registra as mensagens "enviadas" em memória.
type captureSender struct {
	msgs []Message
}

func newCaptureSender() *captureSender {
	return &captureSender{}
}

func (c *captureSender) Send(_ context.Context, msg Message) error {
	c.msgs = append(c.msgs, msg)
	return nil
}

func (c *captureSender) last() Message {
	return c.msgs[len(c.msgs)-1]
}

func testMailer(sender Sender) *Mailer {
	return NewMailer(sender, "Applya <no-reply@applya.dev>", "http://localhost:5173")
}

func TestMailerVerificationEmail(t *testing.T) {
	t.Parallel()

	capture := newCaptureSender()
	mailer := testMailer(capture)

	if err := mailer.SendVerification(context.Background(), "ana@example.com", "Ana", "token-secreto"); err != nil {
		t.Fatalf("SendVerification() erro inesperado: %v", err)
	}

	msg := capture.last()
	if msg.From != "Applya <no-reply@applya.dev>" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.To != "ana@example.com" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.Subject != "Confirme seu e-mail no Applya" {
		t.Errorf("Subject = %q", msg.Subject)
	}

	wantLink := "http://localhost:5173/verify-email?token=token-secreto"
	if !strings.Contains(msg.TextBody, wantLink) {
		t.Errorf("texto não contém o link %q: %s", wantLink, msg.TextBody)
	}
	if !strings.Contains(msg.HTMLBody, wantLink) {
		t.Errorf("HTML não contém o link %q", wantLink)
	}
	for _, want := range []string{"applya", "#F7F5EF", "#FFCA91", "Confirmar e-mail"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("HTML de confirmação não contém %q", want)
		}
	}
}

func TestMailerPasswordResetEmail(t *testing.T) {
	t.Parallel()

	capture := newCaptureSender()
	mailer := testMailer(capture)

	if err := mailer.SendPasswordReset(context.Background(), "ana@example.com", "Ana", "token-reset"); err != nil {
		t.Fatalf("SendPasswordReset() erro inesperado: %v", err)
	}

	wantLink := "http://localhost:5173/reset-password?token=token-reset"
	if !strings.Contains(capture.last().TextBody, wantLink) {
		t.Errorf("texto não contém o link %q: %s", wantLink, capture.last().TextBody)
	}
}

func TestMailerPasswordChangedHasNoLink(t *testing.T) {
	t.Parallel()

	capture := newCaptureSender()
	mailer := testMailer(capture)

	if err := mailer.SendPasswordChanged(context.Background(), "ana@example.com", "Ana"); err != nil {
		t.Fatalf("SendPasswordChanged() erro inesperado: %v", err)
	}

	msg := capture.last()
	if strings.Contains(msg.TextBody, "http") || strings.Contains(msg.HTMLBody, "http") {
		t.Error("notificação de senha alterada não deve conter links")
	}
	if strings.Contains(msg.TextBody, "token") {
		t.Error("notificação de senha alterada não deve mencionar tokens")
	}
}

func TestMailerEscapesUserName(t *testing.T) {
	t.Parallel()

	capture := newCaptureSender()
	mailer := testMailer(capture)

	name := `<script>alert("xss")</script>`
	if err := mailer.SendVerification(context.Background(), "ana@example.com", name, "token"); err != nil {
		t.Fatalf("SendVerification() erro inesperado: %v", err)
	}

	if strings.Contains(capture.last().HTMLBody, "<script>") {
		t.Error("nome do usuário não foi escapado no HTML")
	}
}

func TestLogSender(t *testing.T) {
	t.Parallel()

	buf := &strings.Builder{}
	logger := slog.New(slog.NewTextHandler(buf, nil))

	sender := NewLogSender(logger)
	if err := sender.Send(context.Background(), Message{
		From: "Applya <no-reply@applya.dev>", To: "ana@example.com",
		Subject: "Assunto", TextBody: "Corpo com link http://x/verify?token=t",
	}); err != nil {
		t.Fatalf("Send() erro inesperado: %v", err)
	}

	logged := buf.String()
	for _, want := range []string{"Assunto", "ana@example.com", "Corpo com link"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log não contém %q: %s", want, logged)
		}
	}
}

func TestResendSender(t *testing.T) {
	t.Parallel()

	var gotAuth, gotContentType, gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// client apontando para o servidor de teste (mesmo transporte do real).
	sender := NewResendSender("chave-secreta", server.Client())
	sender.apiURL = server.URL

	err := sender.Send(context.Background(), Message{
		From: "Applya <no-reply@applya.dev>", To: "ana@example.com",
		Subject: "Assunto", TextBody: "texto", HTMLBody: "<p>html</p>",
	})
	if err != nil {
		t.Fatalf("Send() erro inesperado: %v", err)
	}

	if gotAuth != "Bearer chave-secreta" {
		t.Errorf("Authorization = %q, esperado %q", gotAuth, "Bearer chave-secreta")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}

	var payload struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
		HTML    string   `json:"html"`
	}
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("corpo enviado não é JSON válido: %v; corpo: %s", err, gotBody)
	}
	if payload.From != "Applya <no-reply@applya.dev>" {
		t.Errorf("from = %q", payload.From)
	}
	if len(payload.To) != 1 || payload.To[0] != "ana@example.com" {
		t.Errorf("to = %v", payload.To)
	}
	if payload.Subject != "Assunto" {
		t.Errorf("subject = %q", payload.Subject)
	}
	if payload.Text != "texto" {
		t.Errorf("text = %q", payload.Text)
	}
	if payload.HTML != "<p>html</p>" {
		t.Errorf("html = %q", payload.HTML)
	}
}

func TestResendSenderErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"api key inválida"}`))
	}))
	defer server.Close()

	sender := NewResendSender("chave-invalida", server.Client())
	sender.apiURL = server.URL

	err := sender.Send(context.Background(), Message{From: "a@b.c", To: "c@d.e", Subject: "s"})
	if err == nil {
		t.Fatal("Send() esperava erro para status 401, recebeu nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("erro = %q, esperado conter o status HTTP", err.Error())
	}
}

func TestResendSenderTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 20 * time.Millisecond

	sender := NewResendSender("chave", client)
	sender.apiURL = server.URL

	err := sender.Send(context.Background(), Message{From: "a@b.c", To: "c@d.e", Subject: "s"})
	if err == nil {
		t.Fatal("Send() esperava erro de timeout, recebeu nil")
	}
}
