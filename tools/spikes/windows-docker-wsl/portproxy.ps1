<#
.SYNOPSIS
    Runs as SYSTEM at startup, only in nat mode (Windows 10). Keeps a netsh port proxy from
    0.0.0.0:<port> to the current WSL IP, which changes on every WSL start.
    In the product this is done by the agent service (ADR-0002).
#>
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$config = Get-Content -Raw -Path (Join-Path $Spike.Bin 'config.json') | ConvertFrom-Json
$port = [int]$config.port
$ipFile = Join-Path $Spike.Data 'wsl-ip.txt'
$logFile = Join-Path $Spike.Data 'portproxy.log'
$current = ''

function Write-Log([string]$Message) {
    Add-Content -Path $logFile -Value ('{0} {1}' -f (Get-Date -Format o), $Message) -Encoding UTF8
}

Write-Log "portproxy started, $(Get-SecondsSinceBoot) s after boot"
while ($true) {
    if (Test-Path $ipFile) {
        # Written by the less privileged engine account: validate strictly before passing it to netsh.
        $ip = (Get-Content -Raw -Path $ipFile).Trim()
        if ((Test-Ipv4 $ip) -and $ip -ne $current) {
            Invoke-Native 'netsh.exe' @('interface', 'portproxy', 'delete', 'v4tov4', "listenport=$port", 'listenaddress=0.0.0.0') | Out-Null
            $add = Invoke-Native 'netsh.exe' @('interface', 'portproxy', 'add', 'v4tov4', "listenport=$port", 'listenaddress=0.0.0.0', "connectport=$port", "connectaddress=$ip")
            Write-Log "proxy 0.0.0.0:$port -> $($ip):$port (exit $($add.ExitCode)) $($add.Output)"
            if ($add.ExitCode -eq 0) { $current = $ip }
        }
    }
    Start-Sleep -Seconds 15
}
