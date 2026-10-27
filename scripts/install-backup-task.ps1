param(
 [Parameter(Mandatory=$true)][string]$DestinationRoot,
 [string]$TaskName='DuoyuID-DailyBackup',
 [datetime]$At='03:00'
)
$ErrorActionPreference='Stop'
# Installation is explicit. Never replace an existing task or store account passwords.
if (![IO.Path]::IsPathFullyQualified($DestinationRoot) -or $DestinationRoot -match '["\r\n]') { throw 'Use an absolute destination without quotes or newlines' }
if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { throw 'Task already exists; inspect it before updating' }
$script=Join-Path $PSScriptRoot 'backup-scheduled.ps1'
if ($script -match '["\r\n]') { throw 'Unsupported script path' }
$shell=(Get-Command pwsh -ErrorAction Stop).Source
$arguments='-NoProfile -NonInteractive -File "'+$script+'" -DestinationRoot "'+$DestinationRoot+'"'
$action=New-ScheduledTaskAction -Execute $shell -Argument $arguments -WorkingDirectory (Split-Path $PSScriptRoot -Parent)
$trigger=New-ScheduledTaskTrigger -Daily -At $At
$settings=New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Hours 2)
$principal=New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal | Select-Object TaskName,State
Write-Output 'Runs only while this user is logged in, with Docker available. Check task LastTaskResult and backup manifests. Configure a service identity separately for unattended production.'
