# Negative cases must fail before any Docker command or database creation.
$ErrorActionPreference='Stop'
$root=Join-Path ([IO.Path]::GetTempPath()) ('duoyu-manifest-test-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $root 'data') -Force | Out-Null
$dockerProbe=@{calls=0}
function docker { $dockerProbe.calls++; throw 'Unexpected Docker call before manifest rejection' }
try {
 Set-Content -LiteralPath (Join-Path $root 'database.dump') -Value 'synthetic dump'
 Set-Content -LiteralPath (Join-Path $root 'data/signing.pem') -Value 'synthetic key, not a private key'
 $dump=[pscustomobject]@{path='database.dump';sha256=(Get-FileHash (Join-Path $root 'database.dump')).Hash}
 $key=[pscustomobject]@{path='data/signing.pem';sha256=(Get-FileHash (Join-Path $root 'data/signing.pem')).Hash}
 $cases=@(
  @{name='empty manifest';files=@()},
  @{name='dump omitted';files=@($key)},
  @{name='data omitted';files=@($dump)},
  @{name='duplicate dump';files=@($dump,$dump,$key)},
  @{name='missing listed file';files=@($dump,$key,@{path='data/missing.pem';sha256=$key.sha256})},
  @{name='path alias';files=@($dump,@{path='data/../database.dump';sha256=$dump.sha256},$key)},
  @{name='invalid digest';files=@(@{path='database.dump';sha256='bad'},$key)},
  @{name='checksum mismatch';files=@(@{path='database.dump';sha256=('0'*64)},$key)}
 )
 foreach($case in $cases){
  @{format=1;files=$case.files} | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $root 'manifest.json') -Encoding UTF8
  $rejected=$false
  try { & (Join-Path $PSScriptRoot 'restore-check.ps1') -Backup $root -Container 'never-contact-a-real-container' }
  catch { $rejected=$true }
  if(!$rejected -or $dockerProbe.calls -ne 0){throw ('Manifest guard failed: '+$case.name+'; Docker calls='+$dockerProbe.calls)}
  Write-Output ('PASS '+$case.name)
 }
 @{format=1;files=@($dump,$key)} | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $root 'manifest.json') -Encoding UTF8
 Set-Content (Join-Path $root 'data/unlisted') 'unlisted file'
 $reason=''
 try { & (Join-Path $PSScriptRoot 'restore-check.ps1') -Backup $root -Container 'never-contact-a-real-container' }
 catch { $reason=$_.Exception.Message }
 if($reason -notlike '*file list is incomplete*' -or $dockerProbe.calls -ne 0){throw 'Unlisted file was not rejected'}
 Write-Output 'PASS unlisted file'
 New-Item -ItemType Junction -Path (Join-Path $root 'linked-data') -Target (Join-Path $root 'data') | Out-Null
 $reason=''
 try { & (Join-Path $PSScriptRoot 'restore-check.ps1') -Backup $root -Container 'never-contact-a-real-container' }
 catch { $reason=$_.Exception.Message }
 if($reason -notlike '*links are not accepted*' -or $dockerProbe.calls -ne 0){throw 'Linked directory was not rejected'}
 Write-Output 'PASS linked directory'
}finally{
 # Exact generated temporary directory, never an arbitrary caller path.
 $resolved=[IO.Path]::GetFullPath($root)
 $temp=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)+[IO.Path]::DirectorySeparatorChar
 if(!$resolved.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolved) -notlike 'duoyu-manifest-test-*'){throw 'Unsafe test cleanup path'}
 Remove-Item -LiteralPath $resolved -Recurse -Force
}
