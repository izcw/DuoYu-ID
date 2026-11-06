param([string]$EnvFile = '.env.local', [string]$Go = 'go', [switch]$BuildWeb, [switch]$MigrateOnly, [string]$DatabaseURL)
$ErrorActionPreference = 'Stop'
$original = @{}
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
  if (-not (Test-Path -LiteralPath $EnvFile)) { throw "Missing $EnvFile. Run scripts/init-dev.ps1 or copy .env.example." }
  # The frontend build does not need access to database, SMTP or signing secrets.
  if (!$MigrateOnly -and ($BuildWeb -or -not (Test-Path 'admin/dist/index.html'))) {
    Push-Location admin
    try {
      pnpm install --frozen-lockfile --ignore-scripts
      if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed' }
      pnpm build
      if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    } finally { Pop-Location }
  }
  $values = @{}
  foreach ($line in Get-Content -LiteralPath $EnvFile) {
    if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') { $values[$matches[1]] = $matches[2] }
  }
  if ($DatabaseURL) { $values['DATABASE_URL'] = $DatabaseURL }
  $values['DUOYU_MIGRATE_ONLY'] = if ($MigrateOnly) { 'true' } else { 'false' }
  foreach ($key in $values.Keys) {
    $original[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
    [Environment]::SetEnvironmentVariable($key, $values[$key], 'Process')
  }
  & $Go run ./server
  $result = $LASTEXITCODE
} finally {
  foreach ($key in $original.Keys) {
    if ($null -eq $original[$key]) { Remove-Item -LiteralPath "Env:$key" -ErrorAction SilentlyContinue }
    else { [Environment]::SetEnvironmentVariable($key, $original[$key], 'Process') }
  }
  Pop-Location
}
exit $result
