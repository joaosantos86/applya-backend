package auth

import (
	"net/http"
	"time"
)

// Nomes e atributos comuns dos cookies da API.
//
// Path é "/api": o cookie é enviado apenas nas chamadas à API, nunca em
// requisições a outras áreas do mesmo domínio.
const (
	// SessionCookieName identifica o cookie de sessão (HTTP-only).
	SessionCookieName = "applya_session"
	// CSRFCookieName identifica o cookie do token CSRF (legível pelo JS do
	// frontend, usado no header X-CSRF-Token).
	CSRFCookieName = "applya_csrf"
	// CookiePath restringe o envio dos cookies às rotas da API.
	CookiePath = "/api"
)

// CookieConfig descreve os atributos de segurança dos cookies de autenticação.
//
// Em produção Secure deve ser true. Se SameSite for None (necessário quando
// frontend e backend vivem em domínios diferentes), Secure é forçado, pois
// navegadores ignoram cookies SameSite=None sem Secure.
type CookieConfig struct {
	Secure   bool
	SameSite http.SameSite
}

// secure calcula o valor efetivo do atributo Secure.
func (c CookieConfig) secure() bool {
	return c.Secure || c.SameSite == http.SameSiteNoneMode
}

// SetSessionCookie escreve o cookie de sessão na resposta.
//
// O cookie é HTTP-only (inacessível a JavaScript, mitigando roubo por XSS) e
// nunca deve ser armazenado no localStorage pelo frontend.
func SetSessionCookie(w http.ResponseWriter, token string, cfg CookieConfig, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     CookiePath,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   cfg.secure(),
		SameSite: cfg.SameSite,
	})
}

// ClearSessionCookie remove o cookie de sessão do navegador (logout e
// sessões inválidas).
func ClearSessionCookie(w http.ResponseWriter, cfg CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     CookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.secure(),
		SameSite: cfg.SameSite,
	})
}

// SessionTokenFromRequest extrai o token de sessão cru do cookie.
func SessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

// SetCSRFCookie escreve o cookie do token CSRF na resposta. Diferente do
// cookie de sessão, este é legível pelo JavaScript do frontend, que o devolve
// no header X-CSRF-Token das requisições que mudam estado.
func SetCSRFCookie(w http.ResponseWriter, token string, cfg CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     CookiePath,
		HttpOnly: false,
		Secure:   cfg.secure(),
		SameSite: cfg.SameSite,
	})
}

// ClearCSRFCookie remove o cookie CSRF do navegador.
func ClearCSRFCookie(w http.ResponseWriter, cfg CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     CookiePath,
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   cfg.secure(),
		SameSite: cfg.SameSite,
	})
}

// CSRFTokenFromRequest lê o token CSRF do cookie (usado pelo double-submit).
func CSRFTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}
