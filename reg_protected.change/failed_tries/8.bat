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
using System.ComponentModel;
using System.Runtime.InteropServices;

public class TokenPrivs {
    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    public static extern bool OpenProcessToken(
        IntPtr h,
        uint a,
        out IntPtr t);

    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Auto)]
    public static extern bool LookupPrivilegeValue(
        string s,
        string n,
        out long l);

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern bool AdjustTokenPrivileges(
        IntPtr t,
        bool d,
        ref TP n,
        uint b,
        IntPtr p,
        IntPtr r);

    [StructLayout(LayoutKind.Sequential, Pack = 1)]
    public struct TP {
        public int C;
        public long L;
        public int A;
    }

    public static void Enable(string privilege) {
        IntPtr token;

        // TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY
        const uint TOKEN_ADJUST_PRIVILEGES = 0x0020;
        const uint TOKEN_QUERY = 0x0008;

        if (!OpenProcessToken(
                System.Diagnostics.Process.GetCurrentProcess().Handle,
                TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY,
                out token)) {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "OpenProcessToken failed.");
        }

        TP tp = new TP {
            C = 1,
            A = 2 // SE_PRIVILEGE_ENABLED
        };

        if (!LookupPrivilegeValue(null, privilege, out tp.L)) {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "LookupPrivilegeValue failed for " + privilege + ".");
        }

        if (!AdjustTokenPrivileges(
                token,
                false,
                ref tp,
                0,
                IntPtr.Zero,
                IntPtr.Zero)) {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "AdjustTokenPrivileges failed for " + privilege + ".");
        }

        // AdjustTokenPrivileges can return TRUE while GetLastError()
        // reports ERROR_NOT_ALL_ASSIGNED.
        int error = Marshal.GetLastWin32Error();
        if (error != 0) {
            throw new Win32Exception(
                error,
                "Privilege was not assigned: " + privilege + ".");
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
    # 1. Attempt direct write using normal permissions.
    # -------------------------------------------------------------
    try {
        $key = $baseKey.OpenSubKey($subKeyPath, $true)

        if ($null -eq $key) {
            $key = $baseKey.CreateSubKey($subKeyPath)
        }

        try {
            $key.SetValue(
                $ValueName,
                $ValueData,
                [Microsoft.Win32.RegistryValueKind]::DWord)
        }
        finally {
            $key.Close()
        }

        return
    }
    catch {
        # Direct access failed. Proceed with temporary ACL override.
    }

    # -------------------------------------------------------------
    # 2. Enable privileges required to modify the security descriptor.
    # -------------------------------------------------------------
    [TokenPrivs]::Enable('SeTakeOwnershipPrivilege')
    [TokenPrivs]::Enable('SeRestorePrivilege')

    # -------------------------------------------------------------
    # 3. Save the original security descriptor.
    # -------------------------------------------------------------
    #
    # ReadSubTree is sufficient because this handle is only used
    # to retrieve the existing ACL/owner.
    #
    $keyRead = $baseKey.OpenSubKey(
        $subKeyPath,
        [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadSubTree,
        [System.Security.AccessControl.RegistryRights]::ReadPermissions)

    if ($null -eq $keyRead) {
        throw "Could not open registry key for reading: $KeyPath"
    }

    try {
        $origAcl = $keyRead.GetAccessControl()
        $workAcl = $keyRead.GetAccessControl()
    }
    finally {
        $keyRead.Close()
    }

    $admin = New-Object System.Security.Principal.NTAccount('Administrators')

    # -------------------------------------------------------------
    # 4. Take ownership.
    # -------------------------------------------------------------
    #
    # ReadWriteSubTree is intentional here. SetAccessControl()
    # needs a writable registry handle.
    #
    $keyOwn = $baseKey.OpenSubKey(
        $subKeyPath,
        [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree,
        [System.Security.AccessControl.RegistryRights]::TakeOwnership)

    if ($null -eq $keyOwn) {
        throw "Could not open registry key for taking ownership: $KeyPath"
    }

    try {
        $workAcl.SetOwner($admin)
        $keyOwn.SetAccessControl($workAcl)
    }
    finally {
        $keyOwn.Close()
    }

    # -------------------------------------------------------------
    # 5. Temporarily grant Administrators Full Control.
    # -------------------------------------------------------------
    #
    # Again, ReadWriteSubTree is required because the security
    # descriptor is being modified.
    #
    $keyPerm = $baseKey.OpenSubKey(
        $subKeyPath,
        [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree,
        [System.Security.AccessControl.RegistryRights]::ChangePermissions)

    if ($null -eq $keyPerm) {
        throw "Could not open registry key for changing permissions: $KeyPath"
    }

    try {
        $rule = New-Object System.Security.AccessControl.RegistryAccessRule(
            $admin,
            'FullControl',
            'ContainerInherit,ObjectInherit',
            'None',
            'Allow')

        $workAcl.ResetAccessRule($rule)
        $keyPerm.SetAccessControl($workAcl)
    }
    finally {
        $keyPerm.Close()
    }

    # -------------------------------------------------------------
    # 6. Write the registry value.
    # -------------------------------------------------------------
    #
    # The original ACL must be restored regardless of whether the
    # actual value write succeeds.
    #
    try {
        $keyWrite = $baseKey.OpenSubKey($subKeyPath, $true)

        if ($null -eq $keyWrite) {
            throw "Could not open registry key for writing: $KeyPath"
        }

        try {
            $keyWrite.SetValue(
                $ValueName,
                $ValueData,
                [Microsoft.Win32.RegistryValueKind]::DWord)
        }
        finally {
            $keyWrite.Close()
        }
    }
    finally {
        # ---------------------------------------------------------
        # 7. Restore the complete original security descriptor.
        # ---------------------------------------------------------
        #
        # Explicitly request a writable handle and both rights
        # needed to restore ownership and permissions.
        #
        $restoreRights =
            [System.Security.AccessControl.RegistryRights]::TakeOwnership -bor
            [System.Security.AccessControl.RegistryRights]::ChangePermissions

        $keyRestore = $baseKey.OpenSubKey(
            $subKeyPath,
            [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree,
            $restoreRights)

        if ($null -eq $keyRestore) {
            throw "CRITICAL: Could not reopen registry key to restore its original security descriptor: $KeyPath"
        }

        try {
            $keyRestore.SetAccessControl($origAcl)
        }
        finally {
            $keyRestore.Close()
        }
    }
}

Set-RegistryDwordSafe `
    'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing' `
    'EnableLog' `
    0