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
