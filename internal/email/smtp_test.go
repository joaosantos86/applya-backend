package email

import (
	"strings"
	"testing"
)

func TestSMTPMessageContainsTextAndHTMLAlternatives(t *testing.T) {
	t.Parallel()

	data, err := smtpMessage(Message{
		From: "Applya <no-reply@example.com>", To: "ana@example.com",
		Subject: "Confirme sua conta", TextBody: "texto", HTMLBody: "<p>html</p>",
	})
	if err != nil {
		t.Fatalf("smtpMessage() erro inesperado: %v", err)
	}
	for _, want := range []string{"MIME-Version: 1.0", "multipart/alternative", "text/plain; charset=UTF-8", "text/html; charset=UTF-8", "texto", "<p>html</p>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("mensagem não contém %q: %s", want, data)
		}
	}
}

func TestSMTPMessageRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	_, err := smtpMessage(Message{Subject: "assunto\r\nBcc: attacker@example.com"})
	if err == nil {
		t.Fatal("smtpMessage() deveria rejeitar quebra de linha no assunto")
	}
}
