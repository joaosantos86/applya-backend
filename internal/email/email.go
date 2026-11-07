// Package email centraliza o envio de e-mails transacionais da aplicação.
//
// A interface Sender representa o limite entre a aplicação e um provedor de
// envio. Existem duas implementações:
//
//   - LogSender: implementação falsa para desenvolvimento e testes, que apenas
//     registra a mensagem no log (sem envio real). Nunca use em produção, pois
//     registra o conteúdo completo, incluindo links de recuperação.
//   - ResendSender: envio real pela API HTTP do Resend (https://resend.com),
//     implementado com net/http, sem dependências adicionais.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// resendAPIURL é o endpoint de envio do Resend.
const resendAPIURL = "https://api.resend.com/emails"

// resendTimeout limita cada chamada à API do Resend.
const resendTimeout = 10 * time.Second

// resendMaxErrorBody limita o tamanho do corpo de erro incluído na mensagem
// de erro, para não poluir os logs com respostas gigantes.
const resendMaxErrorBody = 1024

// Message descreve um e-mail transacional a ser enviado.
type Message struct {
	// From é o remetente (ex.: "Applya <no-reply@applya.dev>").
	From string
	// To é o endereço do destinatário.
	To string
	// Subject é o assunto do e-mail.
	Subject string
	// TextBody é a versão em texto puro, para clientes sem HTML.
	TextBody string
	// HTMLBody é a versão HTML.
	HTMLBody string
}

// Sender envia um e-mail transacional.
//
// O contrato é pequeno de propósito: os templates e a montagem das mensagens
// ficam no Mailer, e a entrega em cada provedor fica na implementação.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender é a implementação falsa de Sender usada em desenvolvimento e
// testes: registra a mensagem no log em vez de enviá-la.
//
// ATENÇÃO: ela registra o corpo completo da mensagem, incluindo links de
// confirmação e recuperação de senha. É aceitável apenas em desenvolvimento
// local, onde o desenvolvedor precisa desses links para testar os fluxos.
// Nunca configure EMAIL_PROVIDER=log em produção.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender cria um LogSender que registra mensagens no logger informado.
func NewLogSender(logger *slog.Logger) *LogSender {
	return &LogSender{logger: logger}
}

// Send registra a mensagem no log em nível Info, sem enviar nada.
func (s *LogSender) Send(_ context.Context, msg Message) error {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("e-mail (não enviado, provedor log)",
		slog.String("from", msg.From),
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.TextBody),
	)

	return nil
}

// ResendSender envia e-mails pela API HTTP do Resend.
type ResendSender struct {
	apiKey string
	client *http.Client
	// apiURL permite apontar para outro endpoint (apenas testes).
	apiURL string
}

// NewResendSender cria um Sender que usa a API do Resend com a chave
// informada. O client permite injetar um cliente HTTP configurado (timeout,
// transporte) nos testes.
func NewResendSender(apiKey string, client *http.Client) *ResendSender {
	return &ResendSender{apiKey: apiKey, client: client, apiURL: resendAPIURL}
}

// resendRequest é o corpo enviado à API do Resend.
type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

// Send envia a mensagem pela API do Resend.
func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(resendRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Text:    msg.TextBody,
		HTML:    msg.HTMLBody,
	})
	if err != nil {
		return fmt.Errorf("montar requisição do resend: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("criar requisição do resend: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := s.client
	if client == nil {
		client = &http.Client{Timeout: resendTimeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("enviar e-mail pelo resend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// O corpo da resposta pode trazer detalhes úteis para diagnóstico e
		// não contém credenciais; ele é truncado por segurança.
		details, readErr := io.ReadAll(io.LimitReader(resp.Body, resendMaxErrorBody))
		if readErr != nil {
			details = nil
		}
		return fmt.Errorf("resend respondeu status %d: %s", resp.StatusCode, strings.TrimSpace(string(details)))
	}

	return nil
}

var _ Sender = (*LogSender)(nil)
var _ Sender = (*ResendSender)(nil)
