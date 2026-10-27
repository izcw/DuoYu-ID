param(
 [Parameter(Mandatory=$true)][string]$Backup,
 [string]$Container='duoyu-postgres-dev',
 [string]$User='duoyu',
 [string]$TargetDatabase=('duoyu_restore_'+[guid]::NewGuid().ToString('N').Substring(0,12))
)
$ErrorActionPreference='Stop'
if($TargetDatabase -notmatch '^duoyu_restore_[a-z0-9_]+$' -or $User -notmatch '^[a-zA-Z0-9_]+$'){throw 'Restore check only accepts a new duoyu_restore_* database'}
$root=[IO.Path]::GetFullPath($Backup)
$manifest=Get-Content -LiteralPath (Join-Path $root 'manifest.json') -Raw | ConvertFrom-Json
if($manifest.format -ne 1){throw 'Unsupported backup format'}
foreach($file in $manifest.files){
 $path=[IO.Path]::GetFullPath((Join-Path $root $file.path))
 if(!$path.StartsWith($root+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Invalid backup path'}
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
