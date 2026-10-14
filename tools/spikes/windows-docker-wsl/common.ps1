# Shared constants and helpers for the Windows Docker/WSL spike (ADR-0002).
# Dot-sourced by setup.ps1, check.ps1 and cleanup.ps1. ASCII only: Windows PowerShell 5.1
# reads BOM-less files with the ANSI code page.

$Spike = @{
    Root           = Join-Path $env:ProgramData 'SmartClusterSpike'
    User           = 'sc-engine'
    Distro         = 'sc-engine'
    TaskPath       = '\SmartClusterSpike\'
    EngineTask     = 'Engine'
    CheckTask      = 'SystemCheck'
    ProxyTask      = 'PortProxy'
    RulePrefix     = 'SmartClusterSpike'
    WslVmCreatorId = '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}'
}
$Spike.Bin    = Join-Path $Spike.Root 'bin'
$Spike.Data   = Join-Path $Spike.Root 'data'
$Spike.DistroDir = Join-Path $Spike.Root 'distro'

$env:WSL_UTF8 = '1'
try { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch { }

function Write-Step([string]$Message) {
    Write-Host ''
    Write-Host "==> $Message" -ForegroundColor Cyan
}

# Runs a native executable without letting stderr turn into a terminating error (PS 5.1).
function Invoke-Native {
    param([string]$File, [string[]]$Arguments = @())
    $previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & $File @Arguments 2>&1 | ForEach-Object { "$_" }
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    [pscustomobject]@{ ExitCode = $code; Output = (($output | Where-Object { $_ -ne '' }) -join "`n") }
}

function Get-SecondsSinceBoot {
    $boot = (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
    [int]((Get-Date) - $boot).TotalSeconds
}

function Test-Ipv4([string]$Value) {
    $ip = $null
    ($Value -match '^\d{1,3}(\.\d{1,3}){3}$') -and [System.Net.IPAddress]::TryParse($Value, [ref]$ip)
}

function Get-LanIpv4 {
    Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { [string]$_.PrefixOrigin -in 'Dhcp', 'Manual' -and $_.IPAddress -notlike '169.254.*' -and $_.InterfaceAlias -notlike 'vEthernet*' } |
        Select-Object -ExpandProperty IPAddress
}

if (-not ('ScLsa' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;

public static class ScLsa
{
    [StructLayout(LayoutKind.Sequential)]
    private struct LSA_UNICODE_STRING { public ushort Length; public ushort MaximumLength; public IntPtr Buffer; }

    [StructLayout(LayoutKind.Sequential)]
    private struct LSA_OBJECT_ATTRIBUTES
    {
        public int Length; public IntPtr RootDirectory; public IntPtr ObjectName;
        public int Attributes; public IntPtr SecurityDescriptor; public IntPtr SecurityQualityOfService;
    }

    [DllImport("advapi32.dll")]
    private static extern uint LsaOpenPolicy(IntPtr systemName, ref LSA_OBJECT_ATTRIBUTES attributes, int access, out IntPtr handle);
    [DllImport("advapi32.dll")]
    private static extern uint LsaAddAccountRights(IntPtr handle, byte[] sid, LSA_UNICODE_STRING[] rights, int count);
    [DllImport("advapi32.dll")]
    private static extern uint LsaRemoveAccountRights(IntPtr handle, byte[] sid, bool allRights, LSA_UNICODE_STRING[] rights, int count);
    [DllImport("advapi32.dll")]
    private static extern uint LsaClose(IntPtr handle);
    [DllImport("advapi32.dll")]
    private static extern int LsaNtStatusToWinError(uint status);

    private const int PolicyAllAccess = 0x000F0FFF;
    private const uint StatusObjectNameNotFound = 0xC0000034;

    public static void Add(string account, string right) { Change(account, right, true); }
    public static void Remove(string account, string right) { Change(account, right, false); }

    private static void Change(string account, string right, bool add)
    {
        var attributes = new LSA_OBJECT_ATTRIBUTES();
        attributes.Length = Marshal.SizeOf(typeof(LSA_OBJECT_ATTRIBUTES));
        IntPtr handle;
        uint status = LsaOpenPolicy(IntPtr.Zero, ref attributes, PolicyAllAccess, out handle);
        if (status != 0) throw new Win32Exception(LsaNtStatusToWinError(status));

        var rights = new LSA_UNICODE_STRING[1];
        rights[0].Buffer = Marshal.StringToHGlobalUni(right);
        rights[0].Length = (ushort)(right.Length * 2);
        rights[0].MaximumLength = (ushort)((right.Length + 1) * 2);
        try
        {
            var sid = (SecurityIdentifier)new NTAccount(account).Translate(typeof(SecurityIdentifier));
            var sidBytes = new byte[sid.BinaryLength];
            sid.GetBinaryForm(sidBytes, 0);
            status = add
                ? LsaAddAccountRights(handle, sidBytes, rights, 1)
                : LsaRemoveAccountRights(handle, sidBytes, false, rights, 1);
            if (status != 0 && !(!add && status == StatusObjectNameNotFound))
                throw new Win32Exception(LsaNtStatusToWinError(status));
        }
        finally
        {
            Marshal.FreeHGlobal(rights[0].Buffer);
            LsaClose(handle);
        }
    }
}
'@
}
