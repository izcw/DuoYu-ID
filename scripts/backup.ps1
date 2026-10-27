param(
 [string]$Container='duoyu-postgres-dev',
 [string]$Database='duoyu',
 [string]$User='duoyu',
 [string]$Destination=(Join-Path (Split-Path $PSScriptRoot -Parent) ('backups/'+(Get-Date -Format 'yyyyMMdd-HHmmss'))),
 [string]$DataDirectory=(Join-Path (Split-Path $PSScriptRoot -Parent) 'data')
)
$ErrorActionPreference='Stop'
if ($Database -notmatch '^[a-zA-Z0-9_]+$' -or $User -notmatch '^[a-zA-Z0-9_]+$') { throw 'Invalid database/user name' }
$target=[IO.Path]::GetFullPath($Destination)
if(Test-Path -LiteralPath $target){throw 'Backup destination already exists'}
New-Item -ItemType Directory -Path $target | Out-Null
$dump='/tmp/duoyu-backup-'+[guid]::NewGuid().ToString('N')+'.dump'
try {
 & docker exec $Container pg_dump -U $User -d $Database -Fc -f $dump
 if($LASTEXITCODE -ne 0){throw 'Database dump failed'}
 & docker cp "${Container}:$dump" (Join-Path $target 'database.dump')
 if($LASTEXITCODE -ne 0){throw 'Backup copy failed'}
 if(!(Test-Path -LiteralPath $DataDirectory)){throw 'Application data directory missing'}
 Copy-Item -LiteralPath $DataDirectory -Destination (Join-Path $target 'data') -Recurse
 $files=Get-ChildItem -LiteralPath $target -File -Recurse | ForEach-Object { [pscustomobject]@{path=$_.FullName.Substring($target.Length+1);sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash} }
 @{format=1;created=(Get-Date).ToUniversalTime().ToString('o');database=$Database;files=@($files)} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $target 'manifest.json') -Encoding UTF8
 Write-Output "Backup completed: $target"
 Write-Output 'Keep DUOYU_SECRET and SMTP credentials separately in secure storage. This backup contains private signing keys; restrict access.'
}finally{ & docker exec $Container rm -f -- $dump | Out-Null }
