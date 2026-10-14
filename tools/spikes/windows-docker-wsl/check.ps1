<#
.SYNOPSIS
    Spike report. Without parameters (run as administrator after the reboot test) it writes
    a report to C:\ProgramData\SmartClusterSpike\report-<time>.txt.
    With -SystemProbe it runs as the SYSTEM startup task and records when the test container
    first became reachable from the SYSTEM account.
#>
[CmdletBinding()]
param([switch]$SystemProbe)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$config = Get-Content -Raw -Path (Join-Path $Spike.Bin 'config.json') | ConvertFrom-Json
$port = [int]$config.port

function Get-Targets {
    $targets = @()
    if ($config.networking -eq 'mirrored') { $targets += '127.0.0.1' }
    $ipFile = Join-Path $Spike.Data 'wsl-ip.txt'
    if ($config.networking -eq 'nat' -and (Test-Path $ipFile)) {
        $ip = (Get-Content -Raw -Path $ipFile).Trim()
        if (Test-Ipv4 $ip) { $targets += $ip }
    }
    $targets + @(Get-LanIpv4)
}

function Test-Http([string]$HostAddress) {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 -Uri "http://$($HostAddress):$port/"
        [pscustomobject]@{ target = $HostAddress; ok = $true; status = [int]$response.StatusCode; error = $null }
    } catch {
        [pscustomobject]@{ target = $HostAddress; ok = $false; status = $null; error = $_.Exception.Message }
    }
}

if ($SystemProbe) {
    $probeFile = Join-Path $Spike.Data 'check-system.json'
    Remove-Item -Path $probeFile -Force -ErrorAction SilentlyContinue
    $deadline = (Get-Date).AddMinutes(30)
    do {
        $results = @(Get-Targets | ForEach-Object { Test-Http $_ })
        $reached = @($results | Where-Object { $_.ok }).Count -gt 0
        if (-not $reached) { Start-Sleep -Seconds 15 }
    } until ($reached -or (Get-Date) -gt $deadline)
    [ordered]@{
        ok               = $reached
        secondsSinceBoot = Get-SecondsSinceBoot
        boot             = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToString('o')
        timestamp        = (Get-Date).ToString('o')
        account          = [Security.Principal.WindowsIdentity]::GetCurrent().Name
        results          = $results
    } | ConvertTo-Json -Depth 4 | Set-Content -Path $probeFile -Encoding UTF8
    exit 0
}

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw 'Run check.ps1 as administrator (needed for the security event log).' }

$os = Get-CimInstance Win32_OperatingSystem
$boot = $os.LastBootUpTime
$report = New-Object System.Collections.Generic.List[string]
function Add-Line([string]$Text = '') { $report.Add($Text) }
function Read-Json([string]$Name) {
    $path = Join-Path $Spike.Data $Name
    if (Test-Path $path) { Get-Content -Raw -Path $path | ConvertFrom-Json } else { $null }
}

Add-Line "SmartCluster spike 1.1 report  $((Get-Date).ToString('o'))"
Add-Line "Windows: $($os.Caption) build $($os.BuildNumber)"
Add-Line "WSL: $((Invoke-Native 'wsl.exe' @('--version')).Output -replace "`n", ' | ')"
Add-Line "Networking: $($config.networking)  Port: $port"
Add-Line "Last boot: $($boot.ToString('o'))"

# First interactive logon since boot (types 2 = console, 10 = RDP, 11 = cached), ignoring
# the engine account and the DWM/font driver pseudo accounts.
$xpath = "*[System[EventID=4624] and EventData[Data[@Name='LogonType']='2' or Data[@Name='LogonType']='10' or Data[@Name='LogonType']='11']]"
$firstLogon = Get-WinEvent -LogName Security -FilterXPath $xpath -ErrorAction SilentlyContinue |
    Where-Object { $_.TimeCreated -ge $boot } |
    ForEach-Object {
        $xml = [xml]$_.ToXml()
        $data = @{}
        foreach ($d in $xml.Event.EventData.Data) { $data[$d.Name] = $d.InnerText }
        [pscustomobject]@{ Time = $_.TimeCreated; User = $data['TargetUserName']; Domain = $data['TargetDomainName'] }
    } |
    Where-Object { $_.User -ne $Spike.User -and $_.Domain -notin 'Font Driver Host', 'Window Manager' } |
    Sort-Object Time | Select-Object -First 1
$firstLogonSeconds = $null
if ($firstLogon) {
    $firstLogonSeconds = [int]($firstLogon.Time - $boot).TotalSeconds
    Add-Line "First interactive logon: $($firstLogon.Domain)\$($firstLogon.User) at +$firstLogonSeconds s"
} else {
    Add-Line 'First interactive logon: none found since boot'
}

$status = Read-Json 'status.json'
$probe = Read-Json 'check-system.json'
Add-Line ''
Add-Line "Engine status: $(if ($status) { $status | ConvertTo-Json -Compress } else { 'missing' })"
Add-Line "SYSTEM probe:  $(if ($probe) { $probe | ConvertTo-Json -Compress -Depth 4 } else { 'missing' })"

Add-Line ''
Add-Line 'Tasks:'
Get-ScheduledTask -TaskPath $Spike.TaskPath -ErrorAction SilentlyContinue | ForEach-Object {
    $info = Get-ScheduledTaskInfo -InputObject $_
    Add-Line ("  {0,-12} state={1} lastRun={2} lastResult=0x{3:X}" -f $_.TaskName, $_.State, $info.LastRunTime, $info.LastTaskResult)
}

Add-Line ''
Add-Line 'HTTP now (from this administrator session):'
Get-Targets | ForEach-Object { Test-Http $_ } | ForEach-Object {
    Add-Line ("  {0,-15} ok={1} status={2} {3}" -f $_.target, $_.ok, $_.status, $_.error)
}

Add-Line ''
Add-Line 'Verdict (automatic part):'
$bootIso = $boot.ToString('o')
if ($status -and $status.boot -ne $bootIso) { Add-Line '  WARNING: engine status is from an earlier boot' }
if ($probe -and $probe.boot -ne $bootIso) { Add-Line '  WARNING: SYSTEM probe result is from an earlier boot' }
$ready = $status -and $status.phase -eq 'ready' -and $status.boot -eq $bootIso
$probeOk = $probe -and $probe.ok -and $probe.boot -eq $bootIso
$beforeLogon = $probeOk -and ($null -eq $firstLogonSeconds -or $probe.secondsSinceBoot -lt $firstLogonSeconds)
Add-Line "  H1/H2 engine ready and heartbeat alive ........ $ready"
Add-Line "  H5 reachable from SYSTEM ...................... $probeOk"
Add-Line "  reachable from SYSTEM before any logon ........ $beforeLogon$(if ($probeOk) { " (at +$($probe.secondsSinceBoot) s)" })"
Add-Line '  H3 LAN access from another device ............. check manually and add to the report'
Add-Line '  H4 survives logoff of another user ............ check manually and add to the report'

Add-Line ''
Add-Line 'keeper.log (last 60 lines):'
$keeperLog = Join-Path $Spike.Data 'keeper.log'
if (Test-Path $keeperLog) { Get-Content -Path $keeperLog -Tail 60 | ForEach-Object { Add-Line "  $_" } }
$proxyLog = Join-Path $Spike.Data 'portproxy.log'
if (Test-Path $proxyLog) {
    Add-Line ''
    Add-Line 'portproxy.log (last 20 lines):'
    Get-Content -Path $proxyLog -Tail 20 | ForEach-Object { Add-Line "  $_" }
}

$file = Join-Path $Spike.Root ("report-{0}.txt" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
$report | Set-Content -Path $file -Encoding UTF8
$report | ForEach-Object { Write-Host $_ }
Write-Host ''
Write-Host "Report written to $file" -ForegroundColor Green
