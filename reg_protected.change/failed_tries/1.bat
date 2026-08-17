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
:: Registry key (CMD form)
:: -------------------------------
set "KEY_CMD=HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\CBS"

:: -------------------------------
:: PowerShell payload
:: -------------------------------
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "& { " ^
    "$ErrorActionPreference = 'Stop'; " ^
    "Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public class TokenPrivs { [DllImport(""advapi32.dll"", ExactSpelling=true, SetLastError=true)] public static extern bool OpenProcessToken(IntPtr h, uint a, out IntPtr t); [DllImport(""advapi32.dll"", SetLastError=true, CharSet=CharSet.Auto)] public static extern bool LookupPrivilegeValue(string s, string n, out long l); [DllImport(""advapi32.dll"", SetLastError=true)] public static extern bool AdjustTokenPrivileges(IntPtr t, bool d, ref TP n, uint b, IntPtr p, IntPtr r); [StructLayout(LayoutKind.Sequential, Pack=1)] public struct TP { public int C; public long L; public int A; } public static void Enable(string p) { IntPtr t; if (OpenProcessToken(System.Diagnostics.Process.GetCurrentProcess().Handle, 40, out t)) { TP tp = new TP { C = 1, A = 2 }; if (LookupPrivilegeValue(null, p, out tp.L)) { AdjustTokenPrivileges(t, false, ref tp, 0, IntPtr.Zero, IntPtr.Zero); } } } }'; " ^
    "function Set-RegistryDwordSafe { " ^
    "    param([string]$KeyPath, [string]$ValueName, [UInt32]$ValueData); " ^
    "    $hiveMap = @{ 'HKLM'='HKEY_LOCAL_MACHINE'; 'HKCU'='HKEY_CURRENT_USER'; 'HKCR'='HKEY_CLASSES_ROOT'; 'HKU'='HKEY_USERS'; 'HKCC'='HKEY_CURRENT_CONFIG' }; " ^
    "    if ($KeyPath -match '^(?:Registry::)?([A-Za-z0-9_]+)\\(.*)$') { " ^
    "        $h = $matches[1].ToUpper(); $s = $matches[2]; " ^
    "        if ($hiveMap.ContainsKey($h)) { $h = $hiveMap[$h] } " ^
    "        $KeyPath = ""Registry::$h\$s""; " ^
    "    } " ^
    "    if (-not (Test-Path -Path $KeyPath)) { throw ""Registry key does not exist: $KeyPath"" } " ^
    "    try { " ^
    "        Set-ItemProperty -Path $KeyPath -Name $ValueName -Value $ValueData -Type DWord; " ^
    "        return; " ^
    "    } catch [System.UnauthorizedAccessException] { " ^
    "        [TokenPrivs]::Enable('SeTakeOwnershipPrivilege'); " ^
    "        [TokenPrivs]::Enable('SeRestorePrivilege'); " ^
    "        $origAcl = Get-Acl -Path $KeyPath; " ^
    "        $workAcl = Get-Acl -Path $KeyPath; " ^
    "        $admin = New-Object System.Security.Principal.NTAccount('Administrators'); " ^
    "        $workAcl.SetOwner($admin); " ^
    "        $rule = New-Object System.Security.AccessControl.RegistryAccessRule($admin, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'); " ^
    "        $workAcl.ResetAccessRule($rule); " ^
    "        Set-Acl -Path $KeyPath -AclObject $workAcl; " ^
    "        try { " ^
    "            Set-ItemProperty -Path $KeyPath -Name $ValueName -Value $ValueData -Type DWord; " ^
    "        } finally { " ^
    "            Set-Acl -Path $KeyPath -AclObject $origAcl; " ^
    "        } " ^
    "    } " ^
    "}; " ^
    "Set-RegistryDwordSafe '%KEY_CMD%' 'EnableLog' 0; " ^
"}"