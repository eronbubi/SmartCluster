#Requires -RunAsAdministrator
<#
.SYNOPSIS
    SmartCluster spike 1.1 (ADR-0002): Docker Engine in a dedicated WSL2 distro that
    starts at boot without any user logon. Run on a TEST machine only.

.DESCRIPTION
    Creates a hidden, non-admin local account, a startup task that runs keeper.ps1 as that
    account in session 0, a SYSTEM probe task, firewall rules and (on Windows 10) a port proxy.
    Everything is removed again by cleanup.ps1.

.PARAMETER Networking
    auto = mirrored on Windows 11 22H2+ (build 22621), nat otherwise.

.PARAMETER AllowInteractiveLogon
    Do not deny interactive logon for the engine account. Use only to isolate a
    "logon type not granted" failure.

.PARAMETER Port
    Port of the test container (traefik/whoami) on the LAN.
#>
[CmdletBinding()]
param(
    [ValidateSet('auto', 'mirrored', 'nat')]
    [string]$Networking = 'auto',
    [switch]$AllowInteractiveLogon,
    [ValidateRange(1024, 65535)]
    [int]$Port = 8080
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

function New-RandomPassword([int]$Length) {
    $chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!#%+-='
    $bytes = New-Object byte[] $Length
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    # Fixed prefix guarantees the local password complexity policy is met.
    'Sc1!' + (-join ($bytes | ForEach-Object { $chars[$_ % $chars.Length] }))
}

# --- 1. Windows ----------------------------------------------------------------
Write-Step 'Checking Windows'
$os = Get-CimInstance Win32_OperatingSystem
$build = [int]$os.BuildNumber
Write-Host "$($os.Caption) build $build"
if (-not [Environment]::Is64BitOperatingSystem) { throw 'x64 Windows is required.' }
if ($build -lt 19045) { throw "Build $build is not supported. Need Windows 10 22H2 (19045) or Windows 11." }

if ($Networking -eq 'auto') {
    $Networking = if ($build -ge 22621) { 'mirrored' } else { 'nat' }
}
if ($Networking -eq 'mirrored' -and $build -lt 22621) {
    throw 'Mirrored networking needs Windows 11 22H2 (build 22621) or newer. Use -Networking nat.'
}
Write-Host "Networking mode: $Networking"

$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$computer = Get-CimInstance Win32_ComputerSystem
if (-not ($computer.HypervisorPresent -or $cpu.VirtualizationFirmwareEnabled)) {
    throw 'Virtualization (VT-x / AMD-V) is disabled in the firmware. Enable it in the BIOS and run again.'
}
Write-Host 'Virtualization: enabled'

# --- 2. WSL --------------------------------------------------------------------
Write-Step 'Checking WSL'
$wsl = Invoke-Native 'wsl.exe' @('--version')
if ($wsl.ExitCode -ne 0) {
    Write-Step 'Installing the WSL platform (no distribution)'
    $install = Invoke-Native 'wsl.exe' @('--install', '--no-distribution')
    Write-Host $install.Output
    if ($install.ExitCode -ne 0) { throw "wsl --install failed with exit code $($install.ExitCode)." }
    Write-Host ''
    Write-Host 'WSL was installed. Reboot, log in and run setup.ps1 again.' -ForegroundColor Yellow
    exit 3010
}
$update = Invoke-Native 'wsl.exe' @('--update')
Write-Host $update.Output
Write-Host (Invoke-Native 'wsl.exe' @('--version')).Output

# --- 3. Engine account -----------------------------------------------------------
Write-Step "Creating local account '$($Spike.User)' (no admin, hidden from the logon screen)"
$account = "$env:COMPUTERNAME\$($Spike.User)"
$password = New-RandomPassword 28
$securePassword = ConvertTo-SecureString $password -AsPlainText -Force
if (Get-LocalUser -Name $Spike.User -ErrorAction SilentlyContinue) {
    Set-LocalUser -Name $Spike.User -Password $securePassword
} else {
    New-LocalUser -Name $Spike.User -Password $securePassword -PasswordNeverExpires -UserMayNotChangePassword `
        -AccountNeverExpires -Description 'SmartCluster spike engine account' | Out-Null
}
try {
    Add-LocalGroupMember -SID 'S-1-5-32-545' -Member $Spike.User -ErrorAction Stop
} catch {
    if ($_.FullyQualifiedErrorId -notlike 'MemberExists*') { throw }
}

$userListKey = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList'
if (-not (Test-Path $userListKey)) { New-Item -Path $userListKey -Force | Out-Null }
New-ItemProperty -Path $userListKey -Name $Spike.User -Value 0 -PropertyType DWord -Force | Out-Null

[ScLsa]::Add($account, 'SeBatchLogonRight')
foreach ($right in 'SeDenyInteractiveLogonRight', 'SeDenyRemoteInteractiveLogonRight') {
    if ($AllowInteractiveLogon) { [ScLsa]::Remove($account, $right) } else { [ScLsa]::Add($account, $right) }
}
Write-Host "Rights: batch logon granted; interactive logon $(if ($AllowInteractiveLogon) { 'allowed' } else { 'denied' })"

# --- 4. Files --------------------------------------------------------------------
Write-Step "Preparing $($Spike.Root)"
foreach ($dir in $Spike.Root, $Spike.Bin, $Spike.Data, $Spike.DistroDir) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}
function Set-SpikeAcl([string]$Path, [string]$EngineRights) {
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    $inherit = [Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
    $none = [Security.AccessControl.PropagationFlags]::None
    foreach ($sid in 'S-1-5-18', 'S-1-5-32-544') {
        $identity = New-Object Security.Principal.SecurityIdentifier $sid
        $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', $inherit, $none, 'Allow')))
    }
    if ($EngineRights) {
        $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($account, $EngineRights, $inherit, $none, 'Allow')))
    }
    Set-Acl -Path $Path -AclObject $acl
}
# bin: scripts the SYSTEM tasks execute, so the engine account must not be able to change them.
Set-SpikeAcl $Spike.Root 'ReadAndExecute'
Set-SpikeAcl $Spike.Bin 'ReadAndExecute'
Set-SpikeAcl $Spike.Data 'Modify'
Set-SpikeAcl $Spike.DistroDir 'Modify'

foreach ($file in 'common.ps1', 'keeper.ps1', 'check.ps1', 'portproxy.ps1') {
    Copy-Item -Path (Join-Path $PSScriptRoot $file) -Destination $Spike.Bin -Force
}
[ordered]@{ networking = $Networking; port = $Port; distro = $Spike.Distro; distroDir = $Spike.DistroDir } |
    ConvertTo-Json | Set-Content -Path (Join-Path $Spike.Bin 'config.json') -Encoding ASCII
Remove-Item -Path (Join-Path $Spike.Data '*') -Force -ErrorAction SilentlyContinue

# --- 5. Firewall / port proxy ----------------------------------------------------
Write-Step "Opening TCP $Port for the local subnet"
Get-NetFirewallRule -Name "$($Spike.RulePrefix)*" -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -Name "$($Spike.RulePrefix)-$Port" -DisplayName "SmartCluster spike TCP $Port" `
    -Direction Inbound -Protocol TCP -LocalPort $Port -Action Allow -Profile Any -RemoteAddress LocalSubnet | Out-Null
if ($Networking -eq 'mirrored') {
    Get-NetFirewallHyperVRule -Name "$($Spike.RulePrefix)-HV-$Port" -ErrorAction SilentlyContinue | Remove-NetFirewallHyperVRule
    New-NetFirewallHyperVRule -Name "$($Spike.RulePrefix)-HV-$Port" -DisplayName "SmartCluster spike WSL TCP $Port" `
        -Direction Inbound -VMCreatorId $Spike.WslVmCreatorId -Protocol TCP -LocalPorts $Port | Out-Null
    Write-Host 'Hyper-V firewall rule for the WSL VM created.'
}

# --- 6. Scheduled tasks ----------------------------------------------------------
Write-Step 'Registering startup tasks'
Get-ScheduledTask -TaskPath $Spike.TaskPath -ErrorAction SilentlyContinue | ForEach-Object {
    Stop-ScheduledTask -InputObject $_ -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -InputObject $_ -Confirm:$false
}
$powershell = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
function New-SpikeAction([string]$Script, [string]$Arguments = '') {
    New-ScheduledTaskAction -Execute $powershell `
        -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$(Join-Path $Spike.Bin $Script)`" $Arguments"
}
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable `
    -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew `
    -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1)
$trigger = New-ScheduledTaskTrigger -AtStartup

Register-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.EngineTask -Action (New-SpikeAction 'keeper.ps1') `
    -Trigger $trigger -Settings $settings -User $account -Password $password -RunLevel Limited | Out-Null
Remove-Variable password, securePassword
Write-Host "Task $($Spike.TaskPath)$($Spike.EngineTask) runs keeper.ps1 as $account at startup."

$system = New-ScheduledTaskPrincipal -UserId 'S-1-5-18' -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.CheckTask -Action (New-SpikeAction 'check.ps1' '-SystemProbe') `
    -Trigger $trigger -Settings $settings -Principal $system | Out-Null
if ($Networking -eq 'nat') {
    Register-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.ProxyTask -Action (New-SpikeAction 'portproxy.ps1') `
        -Trigger $trigger -Settings $settings -Principal $system | Out-Null
}

# --- 7. First run in session 0, without reboot -----------------------------------
Write-Step 'Starting the engine task now (first run downloads Debian, docker.io and the test image)'
Start-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.EngineTask
if ($Networking -eq 'nat') { Start-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.ProxyTask }

$statusFile = Join-Path $Spike.Data 'status.json'
$logFile = Join-Path $Spike.Data 'keeper.log'
$deadline = (Get-Date).AddMinutes(25)
$shownLines = 0
$phase = ''
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 5
    if (Test-Path $logFile) {
        $lines = @(Get-Content -Path $logFile -ErrorAction SilentlyContinue)
        if ($lines.Count -gt $shownLines) { $lines[$shownLines..($lines.Count - 1)] | ForEach-Object { Write-Host "  keeper: $_" } ; $shownLines = $lines.Count }
    }
    try {
        # The keeper may be writing the file right now; a partial read just means "try again".
        $phase = (Get-Content -Raw -Path $statusFile | ConvertFrom-Json).phase
    } catch { }
    if ($phase -eq 'ready') { break }
}
if ($phase -ne 'ready') {
    $info = Get-ScheduledTaskInfo -TaskPath $Spike.TaskPath -TaskName $Spike.EngineTask
    Write-Host "Engine did not report 'ready' within 25 minutes (phase '$phase', last task result $($info.LastTaskResult))." -ForegroundColor Red
    Write-Host "Run check.ps1 and send the report. Try -AllowInteractiveLogon if the log shows a logon-type error." -ForegroundColor Red
    exit 1
}

Write-Step 'Probing from the SYSTEM account (path the agent service would use)'
Start-ScheduledTask -TaskPath $Spike.TaskPath -TaskName $Spike.CheckTask
$probeFile = Join-Path $Spike.Data 'check-system.json'
$deadline = (Get-Date).AddMinutes(3)
while (-not (Test-Path $probeFile) -and (Get-Date) -lt $deadline) { Start-Sleep -Seconds 3 }
if (Test-Path $probeFile) { Get-Content -Raw $probeFile | Write-Host } else { Write-Host 'No SYSTEM probe result yet.' -ForegroundColor Yellow }

Write-Step 'Interim result (session 0 without reboot)'
Write-Host "Engine phase: $phase"
Write-Host 'Now test from ANOTHER device in the same network (phone or PC browser):'
Get-LanIpv4 | ForEach-Object { Write-Host "  http://$($_):$Port/" -ForegroundColor Green }
Write-Host ''
Write-Host 'Next: reboot, do NOT log in, wait 5 minutes, open the URL above from the other device.'
Write-Host 'Then log in and run .\check.ps1 (as administrator) to write the report.'
