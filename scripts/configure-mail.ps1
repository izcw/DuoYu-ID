param([string]$EnvFile = '.env.local')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (-not (Test-Path -LiteralPath $EnvFile)) { throw 'Run scripts/init-dev.ps1 first.' }
$email = Read-Host 'QQ sender email (Enter for 2405824084@qq.com)'
if ([string]::IsNullOrWhiteSpace($email)) { $email = '2405824084@qq.com' }
if ($email -notmatch '^[0-9]+@qq\.com$') { throw 'Enter your QQ email address.' }
$secret = Read-Host 'New QQ SMTP authorization code (hidden)' -AsSecureString
$ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
try {
  $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr).Trim()
  if ($plain -notmatch '^[a-zA-Z0-9]{12,64}$') { throw 'Invalid authorization code format.' }
  $values = @{ SMTP_HOST='smtp.qq.com'; SMTP_PORT='465'; SMTP_USER=$email; SMTP_FROM=$email; SMTP_PASSWORD=$plain; SMTP_PLAIN_LOCAL='false' }
  $lines = @(Get-Content -LiteralPath $EnvFile | Where-Object { $_ -notmatch '^SMTP_(HOST|PORT|USER|FROM|PASSWORD|PLAIN_LOCAL)=' })
  foreach ($key in ($values.Keys | Sort-Object)) { $lines += "$key=$($values[$key])" }
  $target = (Resolve-Path -LiteralPath $EnvFile).Path
  $pendingFile = Join-Path ([IO.Path]::GetDirectoryName($target)) ('.env-mail-' + [guid]::NewGuid().ToString('N') + '.tmp')
  try {
    [IO.File]::WriteAllLines($pendingFile, $lines, [Text.UTF8Encoding]::new($false))
    [IO.File]::Replace($pendingFile, $target, [Management.Automation.Language.NullString]::Value)
  } finally {
    if ([IO.File]::Exists($pendingFile)) { [IO.File]::Delete($pendingFile) }
  }
  Write-Host 'QQ SMTP configuration saved locally. Restart DuoYu ID, then request a login code. No email was sent by this script.'
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  $plain = $null; $values = $null; $lines = $null
  $secret.Dispose()
}
