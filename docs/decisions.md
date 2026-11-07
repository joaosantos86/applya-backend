# Decisões técnicas

Registro curto das decisões que moldaram a fundação do backend. Cada item
explica **o que** foi decidido e **por quê**, para que futuras sessões não
reintroduzam complexidade sem necessidade.

## 1. Configuração apenas por variáveis de ambiente

`internal/config` lê e valida as variáveis diretamente com a biblioteca padrão
(`os.Getenv`, `net/url`, `strconv`). Não há biblioteca de configuração nem
carregamento automático de `.env`.

Motivos:

- menos dependências e menos comportamento mágico;
- o mesmo binário funciona em qualquer ambiente;
- erros de configuração são explícitos, agregados e ocorrem no startup;
- em desenvolvimento local as variáveis são carregadas no shell (ver README).

O `.env.example` serve como documentação dos valores esperados. Se no futuro
carregar `.env` automaticamente for realmente útil, a decisão deve ser
reavaliada e justificada aqui.

## 2. PostgreSQL com pgxpool, falha rápida na inicialização

`internal/database` cria um `pgxpool.Pool`, define timeouts e faz `Ping` antes
de a API começar a servir. Se o banco estiver indisponível, a aplicação falha
com mensagem descritiva.

Motivos:

- todas as funcionalidades futuras dependem do banco;
- falhar cedo evita uma API que responde `/health` mas quebra em toda rota real;
- não há reconexão automática: o processo é reiniciado e verifica de novo.

A `DATABASE_URL` nunca é registrada em log. Apenas host e nome do banco são
logados.

## 3. Healthcheck sem dependências

`GET /health` verifica apenas o processo HTTP. Ele não consulta o banco.

Motivo: separar "processo vivo" de "dependências saudáveis". Um readiness check
com banco pode ser adicionado depois, se houver orquestração que justifique.

## 4. Goose pela CLI, sem migrations automáticas

As migrations ficam em `db/migrations` e são executadas explicitamente pela CLI
do Goose. O binário da API não executa migrations nem embute o pacote de
migrations.

Motivos:

- o esquema evolui de forma explícita e revisável;
- a API não precisa de permissão de DDL em tempo de execução;
- evita que múltiplas instâncias concorram para aplicar a mesma migration.

O Goose **não** é declarado como dependência do módulo: a dependência do CLI
arrasta drivers de todos os bancos suportados. Ele é instalado como ferramenta
local (`go install`) e documentado no README.

## 5. sqlc fica para depois

O sqlc será adotado quando existirem consultas de negócio reais. Antes disso não
há ganho: sem queries e sem migrations, a configuração geraria apenas arquivos
vazios.

Plano futuro:

1. criar `sqlc.yaml` na raiz;
2. apontar `schema` para `db/migrations` e `queries` para `internal/**/queries`;
3. gerar código por módulo (`internal/users/sqlc`, etc.);
4. não editar o código gerado à mão — trate-o como saída de build.

Até lá, todo acesso ao banco usa `pgx` diretamente, em consultas próximas do
módulo que as utiliza.

## 6. CORS com credenciais e origens explícitas

`CORS_ALLOWED_ORIGINS` aceita uma lista separada por vírgula e o valor `*` é
rejeitado na validação. `AllowCredentials` é habilitado desde já.

Motivo: a autenticação futura usará cookies HTTP-only; navegadores não aceitam
credenciais junto com origem curinga. Rejeitar `*` no startup evita uma
configuração que falharia silenciosamente no navegador.

## 7. Logging com log/slog

Formato texto em desenvolvimento e JSON em produção, sempre em `stdout`.
Nenhum wrapper ou biblioteca de logging é usada.

## 8. Testes

- Unitários com `testing` + `net/http/httptest`, sem banco.
- Testes de integração marcados com a build tag `integration`, executados apenas
  quando necessário e ignorados se `DATABASE_URL` não estiver definida.

## 9. Autenticação por sessões em banco, sem JWT

A autenticação usa **sessões persistidas no PostgreSQL**: um token aleatório de
256 bits vive no cookie HTTP-only do navegador e apenas o hash SHA-256 dele é
armazenado em `sessions`. Cada requisição autenticada valida a sessão no banco
(existência, expiração, revogação).

Motivos:

- revogação imediata e real (logout, redefinição de senha), sem blacklist de JWT;
- um vazamento do banco não expõe tokens utilizáveis (só hashes);
- o custo de uma query por requisição autenticada é aceitável neste porte;
- nenhuma dependência nova.

JWT fica reservado para quando houver necessidade concreta (ex.: tokens
delegados a terceiros).

## 10. Argon2id para senhas (golang.org/x/crypto)

Senhas são armazenadas com **Argon2id** (vencedor do Password Hashing
Competition), no formato PHC `$argon2id$v=19$m=65536,t=3,p=2$...` com 64 MiB de
memória, 3 iterações e 2 threads — alinhado às recomendações da OWASP (RFC
9106, perfil de baixa memória). Como os parâmetros viajam junto do hash, é
possível endurecê-los no futuro sem invalidar senhas existentes.

Única dependência nova do módulo: `golang.org/x/crypto` (oficial, mantida,
mínima). Bcrypt foi descartado pelo limite de 72 bytes e pelo custo de memória
menor. Nada de criptografia própria.

## 11. Tokens de uso único: crypto/rand + SHA-256

Todos os tokens (sessão, confirmação de e-mail, recuperação de senha) são 32
bytes de `crypto/rand` em base64url. O banco guarda apenas o hash SHA-256; o
consumo é atômico (`UPDATE ... WHERE consumed_at IS NULL`) dentro de transação,
impedindo reuso por corrida. Tokens de confirmação valem 24h; de recuperação,
30min. Emitir um novo token da mesma finalidade apaga os pendentes anteriores.

## 12. Provedor de e-mail: Resend, com fake para desenvolvimento

Escolhido o **Resend** (https://resend.com): plano gratuito permanente (3.000
e-mails/mês, 100/dia — verificado em nov/2026), API HTTP simples e boa
reputação de entregabilidade para e-mails transacionais. A integração usa
apenas `net/http` (nenhuma dependência nova) e a interface `email.Sender`
isola o provedor: trocar depois exige só uma nova implementação.

Alternativas consideradas: Brevo (300/dia, mas API mais complexa), Mailtrap,
SMTP2GO. SendGrid não tem mais plano gratuito permanente. Gmail pessoal foi
descartado por violar os termos para envio transacional e ter reputação ruim.

Em desenvolvimento/testes o provedor `log` registra o e-mail completo no log
(com os links, para o desenvolvedor testar). Ele NUNCA deve ser usado em
produção; lá os links sensíveis nunca são logados.

## 13. Rate limiting em janela fixa no PostgreSQL, fail-open

Os limites (login: 10/15min por e-mail+IP; recuperação: 5/15min; reenvio de
confirmação: 3/h por e-mail; cadastro: 10/h por IP) são contados na tabela
`rate_limits` em **janelas fixas** alinhadas ao relógio.

Motivo: a API roda no Render, que pode ter múltiplas instâncias; um mapa em
memória não compartilharia os limites. A janela fixa (e não deslizante) permite
um contador único por chave+janela, com upsert atômico — simples e
multi-instância por construção. Limitação conhecida: picos na fronteira entre
janelas; irrelevante para os limites atuais. Janelas antigas são removidas
probabilisticamente para limitar o tamanho da tabela.

Se o contador falhar (ex.: banco indisponível), a requisição é liberada
(fail-open): perder temporariamente a proteção contra abuso é preferível a
derrubar login/cadastro. O incidente é registrado no log.

## 14. CSRF: dupla submissão + validação de origem (cross-site)

Frontend e backend serão hospedados em **domínios diferentes** (decisão do
autor), então o cookie de sessão usa `SameSite=None; Secure` (via
`COOKIE_SAMESITE=none`). SameSite não pode ser a única defesa nesse cenário;
as duas camadas de CSRF são:

1. **Origem**: toda mutação com `Origin` (ou `Referer`) precisa casar com
   `CORS_ALLOWED_ORIGINS`. Requisições sem os dois headers (clientes
   não-navegador) passam: elas não são vetor de CSRF.
2. **Double-submit**: o cookie `applya_csrf` (legível por JS) deve ser
   devolvido no header `X-CSRF-Token`; a comparação é em tempo constante após
   SHA-256. Um site atacante não lê o cookie da vítima, então não forja o
   header. O token é rotacionado no login.

CORS continua apenas controlando leitura de respostas — nunca é tratado como
proteção CSRF. Limitação conhecida do double-submit: subdomínios sob controle
de atacante no mesmo registro; não é o caso aqui. A recomendação de longo prazo
é migrar para o mesmo site (ex.: `app.applya.dev` + `api.applya.dev`) com
`SameSite=Lax`, o que simplifica a superfície de CSRF.

## 15. Cookies de autenticação

`applya_session`: HTTP-only, `Secure` em produção (forçado com
`SameSite=None`), `SameSite` configurável (`lax` por padrão; `none` para
domínios diferentes), `Path=/api`, expiração = `SESSION_DURATION` (padrão
720h). Sessões têm duração fixa (sem extensão por atividade). Tokens nunca vão
para `localStorage`.

## 16. Toolchain mínimo seguro fixado em go.mod

A auditoria de segurança (govulncheck, nov/2026) encontrou 12 CVEs na
biblioteca padrão do Go 1.27.0 (HTTP/2, x509, tls, textproto, html/template).
O `go.mod` declara `toolchain go1.27.3` (primeira versão com correções): o Go
baixa o toolchain automaticamente e todos os builds — locais e no Render —
usam a versão corrigida.

## 17. IP real do cliente sem spoofing

O `middleware.RealIP` do chi foi substituído por implementação própria
(`middleware.RealIP(trust)`): ele estava deprecated e vulnerável (GHSA-3fxj-6jh8-hvhx),
confiando em `X-Forwarded-For` mesmo em acesso direto — um atacante poderia
rotacionar o IP visto pela aplicação e burlar os rate limits por IP.

O header só é considerado quando `TRUST_PROXY_HEADERS=true`, configuração
explícita para rodar atrás de proxy confiável (Render, que sobrescreve o
header). Padrão `false` (seguro em acesso direto). O primeiro IP válido da
lista é usado (o cliente real, colocado pelo proxy).

## 18. Cabeçalhos de segurança globais

Todas as respostas da API recebem: `Cache-Control: no-store` (dados de sessão
não são cacheados), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`
e `Referrer-Policy: no-referrer` (evita vazar URLs com token pelo header
Referer).

## 19. Provedor de e-mail fake bloqueado em produção

`EMAIL_PROVIDER=log` (que registra os e-mails completos, incluindo links de
confirmação/redefinição com tokens) é rejeitado pela validação de configuração
quando `APP_ENV=production`: falha no startup em vez de vazar tokens de tomada
de conta nos logs.

## 20. Rate limiting também em confirmação e redefinição

Além de login, cadastro, recuperação e reenvio, a auditoria adicionou limites
por IP para `verify-email` (30/15min) e `reset-password` (10/15min). O limite
da redefinição é verificado antes do hash Argon2, contendo abuso de CPU.

## 21. JSON de entrada com campos desconhecidos rejeitados

Os endpoints de autenticação têm contrato fechado: `DisallowUnknownFields` no
decodificador. Campo inesperado (typo do cliente) responde 400 em vez de ser
ignorado silenciosamente.

## 22. Risco residual aceito: x/crypto openpgp

O govulncheck aponta vulnerabilidade no pacote `openpgp` de
`golang.org/x/crypto` (GO-2026-5932). O projeto importa apenas `argon2` do
módulo; `openpgp` não é compilado no binário. A correção existe somente em
pseudo-versão não tagada; a atualização ficará para o próximo release estável.
