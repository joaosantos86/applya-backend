package email

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
)

// Mailer monta e envia os e-mails transacionais da aplicação, em português
// brasileiro. Ele conhece os templates e os links (FRONTEND_URL), enquanto a
// entrega fica com o Sender injetado.
type Mailer struct {
	sender      Sender
	from        string
	frontendURL string
}

// NewMailer cria um Mailer.
//
// frontendURL deve ser a origem do frontend, sem barra no final, por exemplo
// "https://applya.dev". Ela é usada para montar os links de confirmação e
// recuperação enviados por e-mail.
func NewMailer(sender Sender, from, frontendURL string) *Mailer {
	return &Mailer{sender: sender, from: from, frontendURL: frontendURL}
}

// SendVerification envia o e-mail de confirmação de conta.
func (m *Mailer) SendVerification(ctx context.Context, to, name, token string) error {
	link := m.link("/verify-email", token)

	return m.send(ctx, Message{
		From:     m.from,
		To:       to,
		Subject:  "Confirme seu e-mail no Applya",
		TextBody: render(verifyTextTmpl, name, link),
		HTMLBody: render(verifyHTMLTmpl, name, link),
	})
}

// SendPasswordReset envia o e-mail de recuperação de senha.
func (m *Mailer) SendPasswordReset(ctx context.Context, to, name, token string) error {
	link := m.link("/reset-password", token)

	return m.send(ctx, Message{
		From:     m.from,
		To:       to,
		Subject:  "Recuperação de senha do Applya",
		TextBody: render(resetTextTmpl, name, link),
		HTMLBody: render(resetHTMLTmpl, name, link),
	})
}

// SendPasswordChanged envia a notificação de que a senha foi alterada.
// Este e-mail não contém links nem a nova senha.
func (m *Mailer) SendPasswordChanged(ctx context.Context, to, name string) error {
	return m.send(ctx, Message{
		From:     m.from,
		To:       to,
		Subject:  "Sua senha do Applya foi alterada",
		TextBody: render(changedTextTmpl, name, ""),
		HTMLBody: render(changedHTMLTmpl, name, ""),
	})
}

// send delega a entrega ao Sender configurado.
func (m *Mailer) send(ctx context.Context, msg Message) error {
	if m.sender == nil {
		return fmt.Errorf("email: nenhum sender configurado")
	}
	return m.sender.Send(ctx, msg)
}

// link monta o link enviado ao usuário, escapando o token na query string.
func (m *Mailer) link(path, token string) string {
	return m.frontendURL + path + "?token=" + url.QueryEscape(token)
}

// render aplica um template com o nome e o link informados.
//
// Os templates são constantes verificadas em init (template.Must); erros de
// parsing indicam bug de programação e falham o startup, nunca em produção.
func render(tmpl *template.Template, name, link string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		Name string
		Link string
	}{Name: name, Link: link}); err != nil {
		// Execute só falha com erro de tipo nos dados, o que é bug de
		// programação; registrar via panic é aceitável aqui por ser
		// impossível de acontecer com o struct anônimo acima.
		panic(fmt.Sprintf("email: executar template: %v", err))
	}

	return buf.String()
}

// ---------------------------------------------------------------------------
// Templates (português brasileiro)
// ---------------------------------------------------------------------------

// As variáveis interpoladas são apenas o nome do usuário (escapado pelo
// html/template) e o link (montado com token escapado na query string).

var verifyTextTmpl = template.Must(template.New("verify_text").Parse(`Olá, {{.Name}}!

Obrigado por se cadastrar no Applya. Para ativar sua conta, confirme seu e-mail clicando no link abaixo:

{{.Link}}

O link é válido por 24 horas e pode ser usado uma única vez. Se você não criou uma conta no Applya, pode ignorar este e-mail com segurança.

Até logo,
Equipe Applya
`))

var verifyHTMLTmpl = template.Must(template.New("verify_html").Parse(`<!doctype html>
<html lang="pt-BR">
  <body style="margin:0;padding:0;background-color:#F7F5EF;font-family:Inter,Arial,Helvetica,sans-serif;color:#1E1E1E;">
    <span style="display:none!important;font-size:1px;color:#F7F5EF;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;">Confirme seu e-mail para começar a criar seu currículo.</span>
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background-color:#F7F5EF;padding:40px 16px;">
      <tr><td align="center">
        <table role="presentation" width="480" cellspacing="0" cellpadding="0" border="0" style="max-width:480px;width:100%;background-color:#FFFFFF;border:1px solid #E6E1D8;border-radius:28px;overflow:hidden;">
          <tr><td style="padding:28px 36px 20px;"><span style="font-size:24px;font-weight:700;letter-spacing:-1px;color:#1E1E1E;">applya<span style="color:#FFCA91;">.</span></span></td></tr>
          <tr><td style="padding:0 36px;"><table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr><td width="8" height="8" style="width:8px;height:8px;background-color:#FFCA91;border-radius:4px;"></td><td width="8"></td><td width="8" height="8" style="width:8px;height:8px;background-color:#F4B66F;border-radius:4px;"></td><td width="8"></td><td width="8" height="8" style="width:8px;height:8px;background-color:#FFCA91;border-radius:4px;"></td></tr></table></td></tr>
          <tr><td style="padding:24px 36px 36px;">
            <h1 style="margin:0 0 14px;font-size:26px;line-height:1.2;font-weight:600;letter-spacing:-0.5px;color:#1E1E1E;">Confirme seu e-mail</h1>
            <p style="margin:0 0 28px;font-size:15px;line-height:1.65;color:#77736D;">Olá, <strong style="color:#1E1E1E;">{{.Name}}</strong>. Falta só um passo para começar a criar um currículo que acompanha seus próximos capítulos.</p>
            <p style="margin:0 0 28px;"><a href="{{.Link}}" style="display:inline-block;background-color:#FFCA91;color:#1E1E1E;text-decoration:none;padding:14px 22px;border-radius:6px;font-size:15px;font-weight:600;">Confirmar e-mail</a></p>
            <p style="margin:0;font-size:13px;line-height:1.65;color:#77736D;">O link é válido por 24 horas e pode ser usado uma única vez.</p>
          </td></tr>
          <tr><td style="background-color:#F7F5EF;border-top:1px solid #E6E1D8;padding:18px 36px;font-size:12px;line-height:1.5;color:#77736D;">Se você não criou uma conta no Applya, pode ignorar este e-mail com segurança.</td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>
`))

var resetTextTmpl = template.Must(template.New("reset_text").Parse(`Olá, {{.Name}}!

Recebemos uma solicitação para redefinir a senha da sua conta no Applya. Clique no link abaixo para escolher uma nova senha:

{{.Link}}

O link é válido por 30 minutos e pode ser usado uma única vez. Se você não solicitou a redefinição, pode ignorar este e-mail: sua senha atual continua valendo.

Até logo,
Equipe Applya
`))

var resetHTMLTmpl = template.Must(template.New("reset_html").Parse(`<!doctype html>
<html lang="pt-BR">
  <body style="margin:0;padding:0;background-color:#f4f4f5;font-family:Arial,Helvetica,sans-serif;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background-color:#f4f4f5;padding:32px 16px;">
      <tr>
        <td align="center">
          <table role="presentation" width="480" cellspacing="0" cellpadding="0" style="max-width:480px;width:100%;background-color:#ffffff;border-radius:12px;padding:32px;">
            <tr><td>
              <h1 style="margin:0 0 16px;font-size:22px;color:#18181b;">Redefinição de senha</h1>
              <p style="margin:0 0 24px;font-size:15px;line-height:1.6;color:#3f3f46;">Olá, <strong>{{.Name}}</strong>! Recebemos uma solicitação para redefinir a senha da sua conta no Applya. Escolha uma nova senha no botão abaixo:</p>
              <p style="margin:0 0 24px;text-align:center;">
                <a href="{{.Link}}" style="display:inline-block;background-color:#4f46e5;color:#ffffff;text-decoration:none;padding:12px 28px;border-radius:8px;font-size:15px;">Redefinir senha</a>
              </p>
              <p style="margin:0 0 24px;font-size:13px;line-height:1.6;color:#71717a;">Se o botão não funcionar, copie e cole este link no navegador:<br><a href="{{.Link}}" style="color:#4f46e5;word-break:break-all;">{{.Link}}</a></p>
              <p style="margin:0;font-size:13px;line-height:1.6;color:#71717a;">O link é válido por 30 minutos e pode ser usado uma única vez. Se você não solicitou a redefinição, pode ignorar este e-mail: sua senha atual continua valendo.</p>
            </td></tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>
`))

var changedTextTmpl = template.Must(template.New("changed_text").Parse(`Olá, {{.Name}}!

A senha da sua conta no Applya foi alterada com sucesso. Todas as sessões ativas foram encerradas por segurança.

Se você não fez essa alteração, entre em contato imediatamente e redefina sua senha.

Até logo,
Equipe Applya
`))

var changedHTMLTmpl = template.Must(template.New("changed_html").Parse(`<!doctype html>
<html lang="pt-BR">
  <body style="margin:0;padding:0;background-color:#f4f4f5;font-family:Arial,Helvetica,sans-serif;">
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background-color:#f4f4f5;padding:32px 16px;">
      <tr>
        <td align="center">
          <table role="presentation" width="480" cellspacing="0" cellpadding="0" style="max-width:480px;width:100%;background-color:#ffffff;border-radius:12px;padding:32px;">
            <tr><td>
              <h1 style="margin:0 0 16px;font-size:22px;color:#18181b;">Senha alterada</h1>
              <p style="margin:0 0 16px;font-size:15px;line-height:1.6;color:#3f3f46;">Olá, <strong>{{.Name}}</strong>! A senha da sua conta no Applya foi alterada com sucesso.</p>
              <p style="margin:0;font-size:15px;line-height:1.6;color:#3f3f46;">Todas as sessões ativas foram encerradas por segurança. Se você não fez essa alteração, entre em contato imediatamente e redefina sua senha.</p>
            </td></tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>
`))
