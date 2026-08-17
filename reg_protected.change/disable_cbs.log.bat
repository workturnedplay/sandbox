@echo off
setlocal EnableExtensions
rem One further point: this script is not actually a TrustedInstaller impersonation mechanism. It temporarily takes ownership as Administrators, changes the ACL, writes the value, and restores the original security descriptor. That is the mechanism the code actually implements.
rem see at the end to see which registry/value it changes!

:: -------------------------------
:: Require Administrator
:: -------------------------------
fltmc >nul 2>&1
if not "%errorlevel%"=="0" (
    echo This script must be run as Administrator.
    pause
    exit /b 1
)

:: -------------------------------
:: Execute embedded payload
:: -------------------------------
set "BATCH_PATH=%~f0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$c = (Get-Content $env:BATCH_PATH -Raw) -replace '(?s)^.*?===PS_START===\r?\n', ''; Invoke-Command -ScriptBlock ([scriptblock]::Create($c))"
set "EXIT_CODE=%errorlevel%"
echo.
echo PowerShell exited with code %EXIT_CODE%.
echo Press any key to close this window...
pause >nul
exit /b %EXIT_CODE%

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

        // ERROR_NOT_ALL_ASSIGNED means the privilege was not enabled.
        if (Marshal.GetLastWin32Error() == 1300) {
            throw new Win32Exception(
                1300,
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
    # 2. Enable privileges required to modify ownership/permissions.
    # -------------------------------------------------------------
    [TokenPrivs]::Enable('SeTakeOwnershipPrivilege')
    [TokenPrivs]::Enable('SeRestorePrivilege')

    # -------------------------------------------------------------
    # 3. Capture the complete original security descriptor.
    # -------------------------------------------------------------
    #
    # GetAccessControl() returns the Access, Owner and Group
    # portions of the security descriptor.
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

        # Capture the original owner independently so the verification
        # does not depend on a later-mutated RegistrySecurity object.
        $origOwner = $origAcl.GetOwner(
            [System.Security.Principal.NTAccount]).Value
    }
    finally {
        $keyRead.Close()
    }

    Write-Host "Original owner: $origOwner"

    $admin = New-Object System.Security.Principal.NTAccount('Administrators')

    # -------------------------------------------------------------
    # 4. Temporarily take ownership.
    # -------------------------------------------------------------
    #
    # ReadWriteSubTree is required because SetAccessControl() writes
    # the security descriptor.
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
    # 6. Write the requested registry value.
    # -------------------------------------------------------------
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
        # 7. Restore the COMPLETE original security descriptor.
        # ---------------------------------------------------------
        #
        # WRITE_OWNER is essential here. TakeOwnership only grants
        # the ability to take ownership; it is not the same as
        # explicitly requesting WRITE_OWNER access.
        #
        $restoreRights =
            [System.Security.AccessControl.RegistryRights]::TakeOwnership -bor
            [System.Security.AccessControl.RegistryRights]::WriteOwner -bor
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

        # ---------------------------------------------------------
        # 8. Verify that the original OWNER was actually restored.
        # ---------------------------------------------------------
        $keyVerify = $baseKey.OpenSubKey(
            $subKeyPath,
            [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadSubTree,
            [System.Security.AccessControl.RegistryRights]::ReadPermissions)

        if ($null -eq $keyVerify) {
            throw "CRITICAL: Could not reopen registry key to verify its restored owner: $KeyPath"
        }

        try {
            $verifyAcl = $keyVerify.GetAccessControl()
            $restoredOwner = $verifyAcl.GetOwner(
                [System.Security.Principal.NTAccount]).Value
        }
        finally {
            $keyVerify.Close()
        }

        Write-Host "Restored owner: $restoredOwner"

        if ($restoredOwner -cne $origOwner) {
            throw "CRITICAL: Registry key owner was NOT restored. Expected '$origOwner', but found '$restoredOwner'."
        }

        Write-Host "Original registry owner successfully restored."
    }
}

Set-RegistryDwordSafe `
    'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing' `
    'EnableLog' `
    0

Write-Host "Registry modification completed successfully."