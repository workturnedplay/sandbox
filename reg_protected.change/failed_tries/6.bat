@echo off
setlocal EnableExtensions

:: -------------------------------
:: Require Administrator
:: -------------------------------
fltmc >nul 2>&1
if not "%errorlevel%"=="0" (
    echo This script must be run as Administrator.
    exit /b 1
)

:: -------------------------------
:: Execute embedded payload
:: -------------------------------
set "BATCH_PATH=%~f0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$c = (Get-Content $env:BATCH_PATH -Raw) -replace '(?s)^.*?===PS_START===\r?\n', ''; Invoke-Command -ScriptBlock ([scriptblock]::Create($c))"
exit /b %errorlevel%

===PS_START===
$ErrorActionPreference = 'Stop'

$code = @'
using System;
using System.Runtime.InteropServices;

public class TokenPrivs {
    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    public static extern bool OpenProcessToken(IntPtr h, uint a, out IntPtr t);

    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    public static extern bool LookupPrivilegeValue(string s, string n, out long l);

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern bool AdjustTokenPrivileges(IntPtr t, bool d, ref TP n, uint b, IntPtr p, IntPtr r);

    [StructLayout(LayoutKind.Sequential, Pack = 1)]
    public struct TP {
        public int C;
        public long L;
        public int A;
    }

    public static void Enable(string p) {
        IntPtr t;
        // 0x0020 = TOKEN_ADJUST_PRIVILEGES, 0x0008 = TOKEN_QUERY (0x28 total)
        if (OpenProcessToken(System.Diagnostics.Process.GetCurrentProcess().Handle, 0x28, out t)) {
            TP tp = new TP { C = 1, A = 2 }; // SE_PRIVILEGE_ENABLED = 2
            if (LookupPrivilegeValue(null, p, out tp.L)) {
                AdjustTokenPrivileges(t, false, ref tp, 0, IntPtr.Zero, IntPtr.Zero);
            }
        }
    }
}
'@

Add-Type -TypeDefinition $code

function Set-RegistryDwordSafe {
    param(
        [string]$KeyPath,
        [string]$ValueName,
        [UInt32]$ValueData
    )

    $roots = @{
        'HKLM' = [Microsoft.Win32.Registry]::LocalMachine
        'HKCU' = [Microsoft.Win32.Registry]::CurrentUser
        'HKCR' = [Microsoft.Win32.Registry]::ClassesRoot
        'HKU'  = [Microsoft.Win32.Registry]::Users
        'HKCC' = [Microsoft.Win32.Registry]::CurrentConfig
    }

    $rootName = $KeyPath.Split('\')[0].ToUpper()
    if (-not $roots.ContainsKey($rootName)) {
        throw "Unsupported registry root: $rootName"
    }
    
    $baseKey = $roots[$rootName]
    $subKeyPath = $KeyPath.Substring($rootName.Length + 1)

    # -------------------------------------------------------------
    # 1. Attempt direct write (Standard permissions)
    # -------------------------------------------------------------
    try {
        $key = $baseKey.OpenSubKey($subKeyPath, $true)
        if ($null -eq $key) {
            $key = $baseKey.CreateSubKey($subKeyPath)
        }
        $key.SetValue($ValueName, $ValueData, [Microsoft.Win32.RegistryValueKind]::DWord)
        $key.Close()
        return
    } catch {
        # Direct access failed (SecurityException/UnauthorizedAccessException). 
        # Safely swallow and proceed to elevated token override.
    }

    # -------------------------------------------------------------
    # 2. Elevated TrustedInstaller override via raw .NET methods
    # -------------------------------------------------------------
    [TokenPrivs]::Enable('SeTakeOwnershipPrivilege')
    [TokenPrivs]::Enable('SeRestorePrivilege')

    # Backup original ACL and duplicate a working copy for modification
    $keyRead = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, [System.Security.AccessControl.RegistryRights]::ReadPermissions)
    $origAcl = $keyRead.GetAccessControl()
    $workAcl = $keyRead.GetAccessControl()
    $keyRead.Close()

    # Step A: Take Ownership
    $keyOwn = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, [System.Security.AccessControl.RegistryRights]::TakeOwnership)
    $admin = New-Object System.Security.Principal.NTAccount('Administrators')
    $workAcl.SetOwner($admin)
    $keyOwn.SetAccessControl($workAcl)
    $keyOwn.Close()

    # Step B: Inject FullControl DACL
    $keyPerm = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, [System.Security.AccessControl.RegistryRights]::ChangePermissions)
    $rule = New-Object System.Security.AccessControl.RegistryAccessRule($admin, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
    $workAcl.ResetAccessRule($rule)
    $keyPerm.SetAccessControl($workAcl)
    $keyPerm.Close()

    # Step C: Write the Registry Value
    try {
        $keyWrite = $baseKey.OpenSubKey($subKeyPath, $true)
        $keyWrite.SetValue($ValueName, $ValueData, [Microsoft.Win32.RegistryValueKind]::DWord)
        $keyWrite.Close()
    } finally {
        # Step D: Strictly restore original DACL and Owner (TrustedInstaller)
        # Opened atomically with both required flags to write the full security descriptor back.
        $keyRestore = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::Default, [System.Security.AccessControl.RegistryRights]"TakeOwnership, ChangePermissions")
        $keyRestore.SetAccessControl($origAcl)
        $keyRestore.Close()
    }
}

Set-RegistryDwordSafe 'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing' 'EnableLog' 0