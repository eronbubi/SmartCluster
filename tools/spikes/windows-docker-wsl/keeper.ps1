<#
.SYNOPSIS
    Runs as the engine account in session 0 (startup task). Installs and provisions the
    WSL distro on first run, then keeps the WSL instance alive and the test container running.
    Prototype of the later "smartcluster-agent engine-keeper" (ADR-0002).
#>
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$config = Get-Content -Raw -Path (Join-Path $Spike.Bin 'config.json') | ConvertFrom-Json
$logFile = Join-Path $Spike.Data 'keeper.log'
$statusFile = Join-Path $Spike.Data 'status.json'

function Write-Log([string]$Message) {
    $line = '{0} [{1}] {2}' -f (Get-Date -Format o), $PID, $Message
    Add-Content -Path $logFile -Value $line -Encoding UTF8
}

function Set-Status([string]$Phase, [string]$Detail) {
    [ordered]@{
        phase            = $Phase
        detail           = $Detail
        timestamp        = (Get-Date).ToString('o')
        secondsSinceBoot = Get-SecondsSinceBoot
        boot             = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToString('o')
        account          = "$env:USERDOMAIN\$env:USERNAME"
        session          = (Get-Process -Id $PID).SessionId
        networking       = $config.networking
    } | ConvertTo-Json | Set-Content -Path $statusFile -Encoding UTF8
}

function Invoke-Wsl([string[]]$Arguments) { Invoke-Native 'wsl.exe' $Arguments }

# Scripts go in base64 so neither PowerShell 5.1 nor wsl.exe argument quoting can mangle them.
# They run from a temp file, not from stdin, so commands like apt-get cannot swallow the rest.
function Invoke-InDistro([string]$Script) {
    $encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes(($Script -replace "`r", '')))
    $runner = 'f=$(mktemp); echo {0} | base64 -d > $f; sh $f; rc=$?; rm -f $f; exit $rc' -f $encoded
    Invoke-Wsl @('-d', $config.distro, '-u', 'root', '--exec', 'sh', '-c', $runner)
}

function Test-DistroInstalled {
    $list = Invoke-Wsl @('--list', '--quiet')
    @($list.Output -split "`n" | ForEach-Object { $_.Trim() }) -contains $config.distro
}

function Set-WslConfig {
    $path = Join-Path $env:USERPROFILE '.wslconfig'
    $lines = @('[wsl2]', "networkingMode=$($config.networking)", 'vmIdleTimeout=-1')
    if ($config.networking -eq 'nat') { $lines += 'localhostForwarding=false' }
    $lines += @('', '[general]', 'instanceIdleTimeout=-1')
    $wanted = $lines -join "`r`n"
    $current = if (Test-Path $path) { (Get-Content -Raw -Path $path).TrimEnd() } else { '' }
    if ($current -ne $wanted) {
        Set-Content -Path $path -Value $wanted -Encoding ASCII
        Write-Log "Wrote $path; shutting down WSL so it takes effect"
        Invoke-Wsl @('--shutdown') | Out-Null
    }
}

$provisionScript = @'
set -eu
cat > /etc/wsl.conf <<'EOF'
[boot]
systemd=true
[automount]
enabled=false
mountFsTab=false
[interop]
enabled=false
appendWindowsPath=false
[user]
default=root
EOF
printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d
chmod +x /usr/sbin/policy-rc.d
export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -y -q --no-install-recommends docker.io ca-certificates
rm -f /usr/sbin/policy-rc.d
systemctl enable docker.service >/dev/null 2>&1 || true
touch /etc/smartcluster-spike-provisioned
echo provisioned
'@

$containerScript = @'
set -eu
if ! docker inspect spike-whoami >/dev/null 2>&1; then
  docker run -d --restart unless-stopped --name spike-whoami -p __PORT__:80 traefik/whoami >/dev/null
fi
docker start spike-whoami >/dev/null 2>&1 || true
docker inspect -f '{{.State.Status}}' spike-whoami
'@ -replace '__PORT__', [string][int]$config.port

Set-Status 'booting' 'keeper started'
Write-Log "keeper started as $env:USERDOMAIN\$env:USERNAME in session $((Get-Process -Id $PID).SessionId), $(Get-SecondsSinceBoot) s after boot"
Write-Log ((Invoke-Wsl @('--version')).Output -replace "`n", ' | ')
Set-WslConfig

while ($true) {
    $keepAlive = $null
    try {
        if (-not (Test-DistroInstalled)) {
            Set-Status 'installing' 'Installing Debian as WSL distro'
            Write-Log 'Installing Debian distro'
            $install = Invoke-Wsl @('--install', 'Debian', '--name', $config.distro, '--location', $config.distroDir, '--no-launch', '--web-download')
            Write-Log "wsl --install exit $($install.ExitCode): $($install.Output -replace "`n", ' | ')"
            if ($install.ExitCode -ne 0) {
                Write-Log 'Retrying without --location'
                $install = Invoke-Wsl @('--install', 'Debian', '--name', $config.distro, '--no-launch', '--web-download')
                Write-Log "wsl --install exit $($install.ExitCode): $($install.Output -replace "`n", ' | ')"
                if ($install.ExitCode -ne 0) { throw 'Distro installation failed' }
            }
        }

        $check = Invoke-InDistro 'test -f /etc/smartcluster-spike-provisioned && echo yes || echo no'
        if ($check.Output -notmatch 'yes') {
            Set-Status 'provisioning' 'Installing docker.io inside the distro'
            Write-Log 'Provisioning distro (docker.io, wsl.conf with systemd)'
            $provision = Invoke-InDistro $provisionScript
            Write-Log "provision exit $($provision.ExitCode): $(($provision.Output -split "`n" | Select-Object -Last 5) -join ' | ')"
            if ($provision.ExitCode -ne 0) { throw 'Provisioning failed' }
            Invoke-Wsl @('--terminate', $config.distro) | Out-Null
        }

        Set-Status 'starting' 'Starting distro and dockerd'
        $keepAlive = Start-Process -FilePath 'wsl.exe' -WindowStyle Hidden -PassThru `
            -ArgumentList @('-d', $config.distro, '-u', 'root', '--exec', 'sleep', 'infinity')
        Write-Log "keep-alive wsl.exe started (pid $($keepAlive.Id))"

        $deadline = (Get-Date).AddMinutes(5)
        do {
            Start-Sleep -Seconds 3
            $docker = Invoke-InDistro 'docker info >/dev/null 2>&1 && echo docker-ok || echo docker-down'
        } until ($docker.Output -match 'docker-ok' -or (Get-Date) -gt $deadline)
        if ($docker.Output -notmatch 'docker-ok') { throw "dockerd not ready after 5 minutes: $($docker.Output)" }
        Write-Log "dockerd ready $(Get-SecondsSinceBoot) s after boot"

        $container = Invoke-InDistro $containerScript
        Write-Log "container: $($container.Output -replace "`n", ' | ')"
        if ($container.Output -notmatch 'running') { throw 'Test container is not running' }

        if ($config.networking -eq 'nat') {
            $ip = (Invoke-InDistro "hostname -I | cut -d' ' -f1").Output.Trim()
            if (-not (Test-Ipv4 $ip)) { throw "Unexpected WSL IP '$ip'" }
            Set-Content -Path (Join-Path $Spike.Data 'wsl-ip.txt') -Value $ip -Encoding ASCII
            Write-Log "WSL IP $ip"
        }

        Set-Status 'ready' "spike-whoami running on port $($config.port)"
        Write-Log "READY $(Get-SecondsSinceBoot) s after boot"

        while (-not $keepAlive.HasExited) {
            Start-Sleep -Seconds 60
            $state = (Invoke-InDistro "docker inspect -f '{{.State.Status}}' spike-whoami 2>/dev/null || echo missing").Output.Trim()
            if ($state -eq 'running') { Set-Status 'ready' "heartbeat: spike-whoami running" }
            else { Set-Status 'degraded' "spike-whoami state '$state'"; Write-Log "container state '$state'" }
        }
        Write-Log "keep-alive wsl.exe exited with code $($keepAlive.ExitCode); restarting"
    } catch {
        Write-Log "ERROR: $($_.Exception.Message)"
        Set-Status 'error' $_.Exception.Message
        if ($keepAlive -and -not $keepAlive.HasExited) { Stop-Process -Id $keepAlive.Id -Force -ErrorAction SilentlyContinue }
        Start-Sleep -Seconds 30
    }
}
