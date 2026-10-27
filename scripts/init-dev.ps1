$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (Test-Path '.env.local') { throw '.env.local already exists; it has not been overwritten.' }
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$keyBytes = New-Object byte[] 32
$passwordBytes = New-Object byte[] 24
$rng.GetBytes($keyBytes)
$rng.GetBytes($passwordBytes)
$rng.Dispose()
$secret = ([BitConverter]::ToString($keyBytes)).Replace('-', '').ToLowerInvariant()
$password = [Convert]::ToBase64String($passwordBytes)
@"
DUOYU_ADDR=127.0.0.1:8080
DUOYU_ORIGIN=http://localhost:8080
DATABASE_URL=postgres://duoyu:duoyu_dev_only@localhost:55432/duoyu?sslmode=disable
DUOYU_DATA_DIR=./data
DUOYU_ADMIN_EMAIL=2405824084@qq.com
DUOYU_ADMIN_PASSWORD=$password
DUOYU_SECRET=$secret
DUOYU_DEV=true
SMTP_HOST=localhost
SMTP_PORT=11025
SMTP_USER=
SMTP_PASSWORD=
SMTP_FROM=2405824084@qq.com
SMTP_PLAIN_LOCAL=true
"@ | Set-Content -LiteralPath '.env.local' -Encoding utf8
Write-Host 'Created .env.local. Read your generated administrator password there. Local email: http://localhost:18025'
