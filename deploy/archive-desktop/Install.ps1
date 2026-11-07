# Run with the current Windows user. No administrator password is needed.
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
$profilePath = Join-Path $PSScriptRoot 'server.json'
$serverProfile = Get-Content -LiteralPath $profilePath -Raw | ConvertFrom-Json
$sourceExe = Join-Path $PSScriptRoot 'archive-desktop.exe'
if (-not (Test-Path -LiteralPath $sourceExe)) { throw 'Extract the entire ZIP before running Install.cmd.' }
$selector = New-Object System.Windows.Forms.FolderBrowserDialog
$selector.Description = 'Choose a local disk folder for your permanent conversation archive.'
$selector.SelectedPath = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'Sub2API'
$selector.ShowNewFolderButton = $true
if ($selector.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) { Write-Host 'Installation cancelled.'; exit 0 }
$archivePath = $selector.SelectedPath
if ($archivePath.StartsWith('\\')) { throw 'Choose a local disk, not a network share.' }
$installPath = Join-Path $env:LOCALAPPDATA 'Sub2APIArchive'
$configPath = Join-Path $installPath 'config'
New-Item -ItemType Directory -Force -Path $installPath | Out-Null
$exePath = Join-Path $installPath 'archive-desktop.exe'
if ($sourceExe -ne $exePath) { Copy-Item -LiteralPath $sourceExe -Destination $exePath -Force }
& $exePath init --config $configPath --directory $archivePath --host $serverProfile.host --host-key $serverProfile.host_key --web $serverProfile.web_url
if ($LASTEXITCODE -ne 0) { throw 'Could not initialize the archive.' }
Write-Host ''
Write-Host 'Pair this public key on the administrator page opened in your browser.'
Write-Host 'Leave server migration OFF until your first local archive is verified.'
Read-Host 'After pairing, press Enter to enable automatic sync' | Out-Null
$userName = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$userSID = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$taskName = 'Sub2APIArchive-' + $userSID
$action = New-ScheduledTaskAction -Execute $exePath -Argument ('sync --config "' + $configPath + '"') -WorkingDirectory $installPath
$logonTrigger = New-ScheduledTaskTrigger -AtLogOn -User $userName
$repeatTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(2) -RepetitionInterval (New-TimeSpan -Minutes 30)
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -RunOnlyIfNetworkAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Hours 4) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
$principal = New-ScheduledTaskPrincipal -UserId $userName -LogonType Interactive -RunLevel Limited
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger @($logonTrigger, $repeatTrigger) -Settings $settings -Principal $principal -Description 'Encrypted Sub2API conversation backup to your local disk, every 30 minutes while signed in.' -Force | Out-Null
$shellObject = New-Object -ComObject WScript.Shell
$shortcut = $shellObject.CreateShortcut((Join-Path ([Environment]::GetFolderPath('Desktop')) 'Sub2API Archive.lnk'))
$shortcut.TargetPath = $exePath
$shortcut.Arguments = 'view --config "' + $configPath + '"'
$shortcut.WorkingDirectory = $installPath
$shortcut.Save()
Start-ScheduledTask -TaskName $taskName
Start-Process -FilePath $exePath -ArgumentList ('view --config "' + $configPath + '"')
Write-Host ('Installed. Archive folder: ' + $archivePath)
Write-Host 'Open the desktop shortcut to see sync status and conversations.'
Write-Host 'Automatic sync runs while this Windows account is signed in and the PC is online.'
