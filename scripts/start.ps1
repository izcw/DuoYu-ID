param([string]$EnvFile = '.env.local', [string]$Go = 'go', [switch]$BuildWeb, [switch]$MigrateOnly, [string]$DatabaseURL)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (-not (Test-Path -LiteralPath $EnvFile)) { throw "Missing $EnvFile. Run scripts/init-dev.ps1 or copy .env.example." }
foreach ($line in Get-Content -LiteralPath $EnvFile) {
  if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') { [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process') }
}
if ($DatabaseURL) { $env:DATABASE_URL=$DatabaseURL }
$env:DUOYU_MIGRATE_ONLY=if ($MigrateOnly) {'true'} else {'false'}
if (!$MigrateOnly -and ($BuildWeb -or -not (Test-Path 'admin/dist/index.html'))) {
  Push-Location admin
  try {
    pnpm install --frozen-lockfile --ignore-scripts
    if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed' }
    pnpm build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
  } finally { Pop-Location }
}
& $Go run ./server
exit $LASTEXITCODE
