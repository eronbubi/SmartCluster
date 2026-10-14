#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Removes everything setup.ps1 created: tasks, engine account and its profile (including
    the WSL distro registration), rights, firewall rules, port proxy and C:\ProgramData\SmartClusterSpike.
    Restarting WslService terminates ALL running WSL distributions on this machine.
    WSL itself stays installed.
#>
[CmdletBinding(SupportsShouldProcess, ConfirmImpact = 'High')]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

if (-not $PSCmdlet.ShouldProcess($env:COMPUTERNAME, 'Remove the SmartCluster spike (account sc-engine, its WSL distro and data)')) { return }

$account = "$env:COMPUTERNAME\$($Spike.User)"
$configFile = Join-Path $Spike.Bin 'config.json'
$port = if (Test-Path $configFile) { [int](Get-Content -Raw $configFile | ConvertFrom-Json).port } else { 8080 }

Write-Step 'Removing scheduled tasks'
Get-ScheduledTask -TaskPath $Spike.TaskPath -ErrorAction SilentlyContinue | ForEach-Object {
    Stop-ScheduledTask -InputObject $_ -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -InputObject $_ -Confirm:$false
}

Write-Step 'Restarting WslService (terminates all WSL sessions, including the engine VM)'
Restart-Service -Name WslService -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 3

$user = Get-LocalUser -Name $Spike.User -ErrorAction SilentlyContinue
if ($user) {
    Write-Step "Removing account $account"
    foreach ($right in 'SeBatchLogonRight', 'SeDenyInteractiveLogonRight', 'SeDenyRemoteInteractiveLogonRight') {
        [ScLsa]::Remove($account, $right)
    }
    Get-CimInstance Win32_UserProfile | Where-Object { $_.SID -eq $user.SID.Value } | Remove-CimInstance
    Remove-LocalUser -Name $Spike.User
}
$userListKey = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList'
Remove-ItemProperty -Path $userListKey -Name $Spike.User -ErrorAction SilentlyContinue

Write-Step 'Removing firewall rules and port proxy'
Get-NetFirewallRule -Name "$($Spike.RulePrefix)*" -ErrorAction SilentlyContinue | Remove-NetFirewallRule
if (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) {
    Get-NetFirewallHyperVRule -Name "$($Spike.RulePrefix)-HV-$port" -ErrorAction SilentlyContinue | Remove-NetFirewallHyperVRule
}
Invoke-Native 'netsh.exe' @('interface', 'portproxy', 'delete', 'v4tov4', "listenport=$port", 'listenaddress=0.0.0.0') | Out-Null

Write-Step "Deleting $($Spike.Root)"
if (Test-Path $Spike.Root) { Remove-Item -Path $Spike.Root -Recurse -Force }

Write-Host ''
Write-Host 'Spike removed. WSL itself is still installed (remove it under Settings > Apps if you want).' -ForegroundColor Green
