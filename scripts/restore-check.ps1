param(
 [Parameter(Mandatory=$true)][string]$Backup,
 [string]$Container='duoyu-postgres-dev',
 [string]$User='duoyu',
 [string]$TargetDatabase=('duoyu_restore_'+[guid]::NewGuid().ToString('N').Substring(0,12))
)
$ErrorActionPreference='Stop'
if($TargetDatabase -notmatch '^duoyu_restore_[a-z0-9_]+$' -or $User -notmatch '^[a-zA-Z0-9_]+$'){throw 'Restore check only accepts a new duoyu_restore_* database'}
$root=[IO.Path]::GetFullPath($Backup)
function Assert-PlainEntry($item){
 if($item.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Backup links are not accepted'}
}
Assert-PlainEntry (Get-Item -LiteralPath $root -Force)
$manifestPath=Join-Path $root 'manifest.json'
Assert-PlainEntry (Get-Item -LiteralPath $manifestPath -Force)
$manifest=Get-Content -LiteralPath (Join-Path $root 'manifest.json') -Raw | ConvertFrom-Json
if($manifest.format -ne 1 -or $manifest.files -isnot [array] -or $manifest.files.Count -lt 2){throw 'Incomplete or unsupported backup manifest'}
$listed=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach($file in $manifest.files){
 if($file.path -isnot [string] -or $file.sha256 -isnot [string] -or $file.sha256 -notmatch '^[a-fA-F0-9]{64}$'){throw 'Invalid backup entry'}
 $relative=$file.path.Replace('\','/')
 if($relative -ne 'database.dump' -and !$relative.StartsWith('data/',[StringComparison]::Ordinal)){throw 'Invalid backup path'}
 foreach($part in $relative.Split('/')){
  if(!$part -or $part -eq '.' -or $part -eq '..' -or $part -match '[:*?<>|]' -or $part.EndsWith('.') -or $part.EndsWith(' ')){throw 'Invalid backup path'}
 }
 if(!$listed.Add($relative)){throw 'Duplicate backup path'}
}
if(!$listed.Contains('database.dump') -or !$listed.Contains('data/signing.pem')){throw 'Missing database dump or signing key'}
# Enumerate explicitly so no linked directory is followed, including on Windows.
$actual=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
$pending=[Collections.Generic.Stack[string]]::new()
$pending.Push($root)
while($pending.Count){
 foreach($item in Get-ChildItem -LiteralPath $pending.Pop() -Force){
  Assert-PlainEntry $item
  if($item.PSIsContainer){$pending.Push($item.FullName);continue}
  $relative=$item.FullName.Substring($root.TrimEnd([IO.Path]::DirectorySeparatorChar).Length+1).Replace('\','/')
  if($relative -ne 'manifest.json'){[void]$actual.Add($relative)}
 }
}
if(!$listed.SetEquals($actual)){throw 'Backup file list is incomplete or contains missing files'}
foreach($file in $manifest.files){
 $path=Join-Path $root $file.path
 if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $file.sha256){throw 'Backup checksum mismatch'}
}
# createdb refuses an existing name. Never restore into or delete the active database.
& docker exec $Container createdb -U $User $TargetDatabase
if($LASTEXITCODE -ne 0){throw 'Target database already exists or cannot be created'}
$dump='/tmp/duoyu-restore-'+[guid]::NewGuid().ToString('N')+'.dump'
try{
 & docker cp (Join-Path $root 'database.dump') "${Container}:$dump"
 if($LASTEXITCODE -ne 0){throw 'Cannot copy dump'}
 & docker exec $Container pg_restore -U $User -d $TargetDatabase --no-owner --no-acl --exit-on-error --single-transaction $dump
 if($LASTEXITCODE -ne 0){throw 'Restore failed; isolated database retained for inspection'}
 & docker exec $Container psql -U $User -d $TargetDatabase -v ON_ERROR_STOP=1 -c 'SELECT count(*) AS accounts FROM users; SELECT count(*) AS projects FROM apps;'
 if($LASTEXITCODE -ne 0){throw 'Restored schema validation failed'}
 Write-Output "Restore verified in isolated database: $TargetDatabase"
 Write-Output 'Application data remains in backup/data. No production files or database were overwritten. Do not start mail workers against a restored copy.'
}finally{ & docker exec $Container rm -f -- $dump | Out-Null }
