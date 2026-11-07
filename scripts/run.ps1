# run.ps1 — sobe a API do Applya carregando as variáveis do .env
#
# Uso (a partir da raiz do projeto):
#
#   .\scripts\run.ps1
#
# O script:
#   1. carrega o .env no ambiente do processo (o binário NÃO lê .env sozinho);
#   2. confere as variáveis obrigatórias;
#   3. executa `go run ./cmd/api` na raiz do projeto.
#
# Observações:
#   - valores do .env sobrescrevem variáveis já exportadas no shell;
#   - nenhum VALOR de variável é impresso (podem ser segredos);
#   - se o PowerShell bloquear a execução: powershell -ExecutionPolicy Bypass -File scripts/run.ps1

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$envFile = Join-Path $root ".env"
if (-not (Test-Path $envFile)) {
    [Console]::Error.WriteLine("applya: arquivo .env nao encontrado em $root")
    [Console]::Error.WriteLine("Copie .env.example para .env e ajuste os valores.")
    exit 1
}

# Carrega o .env: ignora comentarios (#) e linhas vazias; preserva "=" no valor.
$loaded = 0
Get-Content $envFile | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }

    $name, $value = $line -split "=", 2
    if ([string]::IsNullOrWhiteSpace($name)) { return }

    Set-Item -Path "Env:$($name.Trim())" -Value $value.Trim()
    $loaded++
}

Write-Host "applya: $loaded variaveis carregadas de .env"

# Confere as obrigatorias antes de compilar (falha com mensagem clara).
$required = @("DATABASE_URL", "CORS_ALLOWED_ORIGINS", "FRONTEND_URL")
foreach ($var in $required) {
    if ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($var))) {
        [Console]::Error.WriteLine("applya: variavel obrigatoria ausente ou vazia no .env: $var")
        exit 1
    }
}

Write-Host "applya: iniciando a API (Ctrl+C para encerrar)..."
go run ./cmd/api
exit $LASTEXITCODE
