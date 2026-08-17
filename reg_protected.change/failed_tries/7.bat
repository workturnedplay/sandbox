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

# 1. Unmanaged P/Invoke via ntdll
$ntdllStubs = @'
using System;
using System.Runtime.InteropServices;
public class TokenMgmt {
    [DllImport("ntdll.dll", SetLastError = true)]
    public static extern int RtlAdjustPrivilege(uint Privilege, bool bEnablePrivilege, bool IsThreadPrivilege, out bool PreviousValue);
    
    public static void AssertOwnershipPrivileges() {
        bool prev;
        // 9 = SeTakeOwnershipPrivilege, 18 = SeRestorePrivilege
        RtlAdjustPrivilege(9, true, false, out prev);
        RtlAdjustPrivilege(18, true, false, out prev);
    }
}
'@

Add-Type -TypeDefinition $ntdllStubs

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
    # Attempt direct write (Standard permissions)
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
        # Access denied. Proceeding to elevated token override.
    }

    # -------------------------------------------------------------
    # Elevated TrustedInstaller override via segmented handle access
    # -------------------------------------------------------------
    [TokenMgmt]::AssertOwnershipPrivileges()

    $adminAccount = [System.Security.Principal.NTAccount]"Builtin\Administrators"

    # Backup original ACL for restoration
    $keyRead = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadSubTree, [System.Security.AccessControl.RegistryRights]::ReadPermissions)
    $origAcl = $keyRead.GetAccessControl()
    $keyRead.Close()

    # Step A: Take Ownership (Acquire handle strictly for WRITE_OWNER)
    $keyWriteOwner = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree, [System.Security.AccessControl.RegistryRights]::TakeOwnership)
    $aclOwnership = $keyWriteOwner.GetAccessControl()
    $aclOwnership.SetOwner($adminAccount)
    $keyWriteOwner.SetAccessControl($aclOwnership)
    $keyWriteOwner.Close()

    # Step B: Inject DACL (Acquire handle strictly for WRITE_DAC)
    $keyWriteDac = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree, [System.Security.AccessControl.RegistryRights]::ChangePermissions)
    $aclDac = $keyWriteDac.GetAccessControl()
    $rule = New-Object System.Security.AccessControl.RegistryAccessRule(
        $adminAccount, 
        [System.Security.AccessControl.RegistryRights]::FullControl, 
        [System.Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [System.Security.AccessControl.InheritanceFlags]::ObjectInherit, 
        [System.Security.AccessControl.PropagationFlags]::None, 
        [System.Security.AccessControl.AccessControlType]::Allow
    )
    $aclDac.AddAccessRule($rule)
    $keyWriteDac.SetAccessControl($aclDac)
    $keyWriteDac.Close()

    # Step C: Write the Registry Value
    try {
        $keyWrite = $baseKey.OpenSubKey($subKeyPath, $true)
        $keyWrite.SetValue($ValueName, $ValueData, [Microsoft.Win32.RegistryValueKind]::DWord)
        $keyWrite.Close()
    } finally {
        # Step D: Restore original DACL and Owner (TrustedInstaller)
        $keyRestore = $baseKey.OpenSubKey($subKeyPath, [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadWriteSubTree, [System.Security.AccessControl.RegistryRights]::TakeOwnership -bor [System.Security.AccessControl.RegistryRights]::ChangePermissions)
        $keyRestore.SetAccessControl($origAcl)
        $keyRestore.Close()
    }
}

Set-RegistryDwordSafe 'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing' 'EnableLog' 0