param([Parameter(Mandatory=$true)][string]$DestinationRoot)
$ErrorActionPreference='Stop'
if (![IO.Path]::IsPathFullyQualified($DestinationRoot)) { throw 'DestinationRoot must be absolute' }
$target=Join-Path $DestinationRoot ((Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8))
& (Join-Path $PSScriptRoot 'backup.ps1') -Destination $target
