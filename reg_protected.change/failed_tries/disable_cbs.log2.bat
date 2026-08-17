@echo off
setlocal EnableExtensions

:: -------------------------------
:: Require Administrator
:: -------------------------------
fltmc >nul 2>&1
if not "%errorlevel%"=="0" (
    echo This script must be run as Administrator.
    echo.
    pause
    exit /b 1
)

:: -------------------------------
:: Execute embedded PowerShell payload
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

public class TokenPrivs
{
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
    public struct TP
    {
        public int C;
        public long L;
        public int A;
    }

    public static void Enable(string privilege)
    {
        IntPtr token;

        const uint TOKEN_ADJUST_PRIVILEGES = 0x0020;
        const uint TOKEN_QUERY = 0x0008;

        if (!OpenProcessToken(
                System.Diagnostics.Process.GetCurrentProcess().Handle,
                TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY,
                out token))
        {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "OpenProcessToken failed.");
        }

        try
        {
            TP tp = new TP
            {
                C = 1,
                A = 2
            };

            if (!LookupPrivilegeValue(null, privilege, out tp.L))
            {
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
                    IntPtr.Zero))
            {
                throw new Win32Exception(
                    Marshal.GetLastWin32Error(),
                    "AdjustTokenPrivileges failed for " + privilege + ".");
            }

            int error = Marshal.GetLastWin32Error();

            if (error == 1300)
            {
                throw new Win32Exception(
                    error,
                    privilege + " was not assigned to this process token.");
            }
        }
        finally
        {
            CloseHandle(token);
        }
    }

    [DllImport("kernel32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);
}

public class RegistrySecurityHelper
{
    private const uint HKEY_LOCAL_MACHINE = 0x80000002;

    private const uint KEY_QUERY_VALUE = 0x0001;
    private const uint KEY_SET_VALUE = 0x0002;
    private const uint KEY_READ = 0x20019;

    private const uint READ_CONTROL = 0x00020000;
    private const uint WRITE_DAC = 0x00040000;
    private const uint WRITE_OWNER = 0x00080000;

    private const uint OWNER_SECURITY_INFORMATION = 0x00000001;

    private const uint REG_OPTION_OPEN_LINK = 0x0008;

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern uint RegOpenKeyExW(
        IntPtr hKey,
        string lpSubKey,
        uint ulOptions,
        uint samDesired,
        out IntPtr phkResult);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern uint RegCloseKey(
        IntPtr hKey);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern uint RegGetKeySecurity(
        IntPtr hKey,
        uint SecurityInformation,
        IntPtr pSecurityDescriptor,
        ref uint lpcbSecurityDescriptor);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern uint RegSetKeySecurity(
        IntPtr hKey,
        uint SecurityInformation,
        IntPtr pSecurityDescriptor);

    public static byte[] CaptureSecurityDescriptor()
    {
        IntPtr hKey = IntPtr.Zero;

        uint result = RegOpenKeyExW(
            new IntPtr(unchecked((int)HKEY_LOCAL_MACHINE)),
            "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing",
            REG_OPTION_OPEN_LINK,
            KEY_READ | READ_CONTROL,
            out hKey);

        if (result != 0)
        {
            throw new Win32Exception(
                (int)result,
                "RegOpenKeyExW failed while capturing the original security descriptor.");
        }

        try
        {
            uint size = 0;

            result = RegGetKeySecurity(
                hKey,
                0x00000001 | 0x00000002 | 0x00000004,
                IntPtr.Zero,
                ref size);

            if (size == 0)
            {
                throw new Win32Exception(
                    (int)result,
                    "RegGetKeySecurity did not return a descriptor size.");
            }

            IntPtr descriptor = Marshal.AllocHGlobal((int)size);

            try
            {
                uint actualSize = size;

                result = RegGetKeySecurity(
                    hKey,
                    0x00000001 | 0x00000002 | 0x00000004,
                    descriptor,
                    ref actualSize);

                if (result != 0)
                {
                    throw new Win32Exception(
                        (int)result,
                        "RegGetKeySecurity failed while capturing the original security descriptor.");
                }

                byte[] copy = new byte[actualSize];
                Marshal.Copy(descriptor, copy, 0, (int)actualSize);
                return copy;
            }
            finally
            {
                Marshal.FreeHGlobal(descriptor);
            }
        }
        finally
        {
            RegCloseKey(hKey);
        }
    }

    public static void RestoreSecurityDescriptor(byte[] descriptor)
    {
        IntPtr hKey = IntPtr.Zero;

        uint result = RegOpenKeyExW(
            new IntPtr(unchecked((int)HKEY_LOCAL_MACHINE)),
            "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing",
            REG_OPTION_OPEN_LINK,
            KEY_READ | READ_CONTROL | WRITE_DAC | WRITE_OWNER,
            out hKey);

        if (result != 0)
        {
            throw new Win32Exception(
                (int)result,
                "RegOpenKeyExW failed while opening the key for security restoration.");
        }

        IntPtr descriptorPtr = IntPtr.Zero;

        try
        {
            descriptorPtr = Marshal.AllocHGlobal(descriptor.Length);
            Marshal.Copy(descriptor, 0, descriptorPtr, descriptor.Length);

            result = RegSetKeySecurity(
                hKey,
                0x00000001 | 0x00000002 | 0x00000004,
                descriptorPtr);

            if (result != 0)
            {
                throw new Win32Exception(
                    (int)result,
                    "RegSetKeySecurity failed while restoring the original security descriptor.");
            }
        }
        finally
        {
            if (descriptorPtr != IntPtr.Zero)
            {
                Marshal.FreeHGlobal(descriptorPtr);
            }

            RegCloseKey(hKey);
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
            throw "Could not open registry key for writing."
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
        # Expected for the protected CBS key.
    }

    # -------------------------------------------------------------
    # 2. Enable privileges needed for temporary ownership/ACL change.
    # -------------------------------------------------------------
    [TokenPrivs]::Enable('SeTakeOwnershipPrivilege')
    [TokenPrivs]::Enable('SeRestorePrivilege')

    # -------------------------------------------------------------
    # 3. Capture the EXACT original Windows security descriptor.
    # -------------------------------------------------------------
    #
    # This is the important change from the previous implementation.
    # We save the actual self-relative descriptor returned by the
    # registry security API, rather than relying on RegistrySecurity
    # to reconstruct it during restoration.
    #
    $originalSecurityDescriptor =
        [RegistrySecurityHelper]::CaptureSecurityDescriptor()

    # Keep the original .NET ACL only for the temporary modification.
    $keyRead = $baseKey.OpenSubKey(
        $subKeyPath,
        [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadSubTree,
        [System.Security.AccessControl.RegistryRights]::ReadPermissions)

    if ($null -eq $keyRead) {
        throw "Could not open registry key for ACL manipulation: $KeyPath"
    }

    try {
        $workAcl = $keyRead.GetAccessControl()
        $origOwner = $workAcl.GetOwner(
            [System.Security.Principal.NTAccount]).Value

        Write-Host "Original owner: $origOwner"
    }
    finally {
        $keyRead.Close()
    }

    $admin = New-Object System.Security.Principal.NTAccount('Administrators')

    # -------------------------------------------------------------
    # 4. Temporarily take ownership.
    # -------------------------------------------------------------
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
    # 5. Temporarily grant Administrators FullControl.
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
        # 7. Restore the EXACT security descriptor captured above.
        # ---------------------------------------------------------
        [RegistrySecurityHelper]::RestoreSecurityDescriptor(
            $originalSecurityDescriptor)

        # ---------------------------------------------------------
        # 8. Verify the owner after restoration.
        # ---------------------------------------------------------
        $verifyKey = $baseKey.OpenSubKey(
            $subKeyPath,
            [Microsoft.Win32.RegistryKeyPermissionCheck]::ReadSubTree,
            [System.Security.AccessControl.RegistryRights]::ReadPermissions)

        if ($null -eq $verifyKey) {
            throw "CRITICAL: Could not reopen registry key after security restoration."
        }

        try {
            $verifyAcl = $verifyKey.GetAccessControl()
            $restoredOwner = $verifyAcl.GetOwner(
                [System.Security.Principal.NTAccount]).Value
        }
        finally {
            $verifyKey.Close()
        }

        Write-Host "Restored owner: $restoredOwner"

        if ($restoredOwner -cne $origOwner) {
            throw "CRITICAL: Registry key owner was NOT restored. Expected '$origOwner', but found '$restoredOwner'."
        }

        Write-Host "Original registry security descriptor restored."
    }
}

Set-RegistryDwordSafe `
    'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing' `
    'EnableLog' `
    0

Write-Host "Registry modification completed successfully."