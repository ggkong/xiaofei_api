$ErrorActionPreference = 'Stop'
$userSID = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$taskName = 'Sub2APIArchive-' + $userSID
$task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($task) { Stop-ScheduledTask -TaskName $taskName; Unregister-ScheduledTask -TaskName $taskName -Confirm:$false }
$shortcut = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Sub2API Archive.lnk'
if (Test-Path -LiteralPath $shortcut) { Remove-Item -LiteralPath $shortcut }
Write-Host 'Automatic sync is disabled. Local archives and configuration were preserved.'
Write-Host 'Disable this computer on the server administrator page to revoke access.'
