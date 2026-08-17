@echo off
setlocal EnableExtensions

:: ------------------------------------------------------------
:: Require Administrator
:: ------------------------------------------------------------
fltmc >nul 2>&1
if not "%errorlevel%"=="0" (
    echo This script must be run as Administrator.
    echo.
    pause
    exit /b 1
)

:: ------------------------------------------------------------
:: Run PowerShell payload
:: ------------------------------------------------------------
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
"$ErrorActionPreference = 'Stop'; ^
$code = @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class RestoreOwner
{
    private const uint TOKEN_QUERY = 0x0008;
    private const uint TOKEN_ADJUST_PRIVILEGES = 0x0020;
    private const uint SE_PRIVILEGE_ENABLED = 0x00000002;
    private const int ERROR_NOT_ALL_ASSIGNED = 1300;

    private const uint SE_REGISTRY_KEY = 4;
    private const uint OWNER_SECURITY_INFORMATION = 0x00000001;

    [StructLayout(LayoutKind.Sequential)]
    private struct LUID
    {
        public uint LowPart;
        public int HighPart;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct LUID_AND_ATTRIBUTES
    {
        public LUID Luid;
        public uint Attributes;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct TOKEN_PRIVILEGES
    {
        public uint PrivilegeCount;
        public LUID_AND_ATTRIBUTES Privileges;
    }

    [DllImport(""advapi32.dll"", ExactSpelling = true, SetLastError = true)]
    private static extern bool OpenProcessToken(
        IntPtr ProcessHandle,
        uint DesiredAccess,
        out IntPtr TokenHandle);

    [DllImport(""advapi32.dll"", ExactSpelling = true, SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern bool LookupPrivilegeValueW(
        string lpSystemName,
        string lpName,
        out LUID lpLuid);

    [DllImport(""advapi32.dll"", ExactSpelling = true, SetLastError = true)]
    private static extern bool AdjustTokenPrivileges(
        IntPtr TokenHandle,
        bool DisableAllPrivileges,
        ref TOKEN_PRIVILEGES NewState,
        uint BufferLength,
        IntPtr PreviousState,
        IntPtr ReturnLength);

    [DllImport(""advapi32.dll"", ExactSpelling = true, SetLastError = true)]
    private static extern uint SetNamedSecurityInfoW(
        string pObjectName,
        uint ObjectType,
        uint SecurityInfo,
        IntPtr psidOwner,
        IntPtr psidGroup,
        IntPtr pDacl,
        IntPtr pSacl);

    [DllImport(""kernel32.dll"", ExactSpelling = true, SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);

    public static void EnableRestorePrivilege()
    {
        IntPtr token = IntPtr.Zero;

        if (!OpenProcessToken(
                System.Diagnostics.Process.GetCurrentProcess().Handle,
                TOKEN_QUERY | TOKEN_ADJUST_PRIVILEGES,
                out token))
        {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                ""OpenProcessToken failed."");
        }

        try
        {
            LUID luid;

            if (!LookupPrivilegeValueW(
                    null,
                    ""SeRestorePrivilege"",
                    out luid))
            {
                throw new Win32Exception(
                    Marshal.GetLastWin32Error(),
                    ""LookupPrivilegeValueW failed."");
            }

            TOKEN_PRIVILEGES privileges = new TOKEN_PRIVILEGES
            {
                PrivilegeCount = 1,
                Privileges = new LUID_AND_ATTRIBUTES
                {
                    Luid = luid,
                    Attributes = SE_PRIVILEGE_ENABLED
                }
            };

            if (!AdjustTokenPrivileges(
                    token,
                    false,
                    ref privileges,
                    0,
                    IntPtr.Zero,
                    IntPtr.Zero))
            {
                throw new Win32Exception(
                    Marshal.GetLastWin32Error(),
                    ""AdjustTokenPrivileges failed."");
            }

            int error = Marshal.GetLastWin32Error();

            if (error == ERROR_NOT_ALL_ASSIGNED)
            {
                throw new Win32Exception(
                    error,
                    ""SeRestorePrivilege is not available in this process token."");
            }
        }
        finally
        {
            CloseHandle(token);
        }
    }

    public static void SetTrustedInstallerOwner()
    {
        const string registryPath =
            ""MACHINE\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing"";

        using (var account =
            new System.Security.Principal.NTAccount(
                ""NT SERVICE\\TrustedInstaller""))
        {
            var sid =
                (System.Security.Principal.SecurityIdentifier)
                account.Translate(
                    typeof(System.Security.Principal.SecurityIdentifier));

            byte[] sidBytes = new byte[sid.BinaryLength];
            sid.GetBinaryForm(sidBytes, 0);

            IntPtr sidPtr = Marshal.AllocHGlobal(sidBytes.Length);

            try
            {
                Marshal.Copy(sidBytes, 0, sidPtr, sidBytes.Length);

                uint result = SetNamedSecurityInfoW(
                    registryPath,
                    SE_REGISTRY_KEY,
                    OWNER_SECURITY_INFORMATION,
                    sidPtr,
                    IntPtr.Zero,
                    IntPtr.Zero,
                    IntPtr.Zero);

                if (result != 0)
                {
                    throw new Win32Exception(
                        (int)result,
                        ""SetNamedSecurityInfoW failed."");
                }
            }
            finally
            {
                Marshal.FreeHGlobal(sidPtr);
            }
        }
    }
}
'@; ^
Add-Type -TypeDefinition $code; ^
Write-Host 'Enabling SeRestorePrivilege...'; ^
[RestoreOwner]::EnableRestorePrivilege(); ^
Write-Host 'Setting owner to NT SERVICE\TrustedInstaller...'; ^
[RestoreOwner]::SetTrustedInstallerOwner(); ^
$path = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing'; ^
$owner = (Get-Acl -Path $path).Owner; ^
Write-Host ('Owner after change: ' + $owner); ^
if ($owner -cne 'NT SERVICE\TrustedInstaller') { ^
    throw ('CRITICAL: owner verification failed. Expected NT SERVICE\TrustedInstaller, got ' + $owner) ^
}; ^
Write-Host 'SUCCESS: owner restored to NT SERVICE\TrustedInstaller.'"

set "EXIT_CODE=%errorlevel%"

echo.
if "%EXIT_CODE%"=="0" (
    echo Operation completed successfully.
) else (
    echo Operation FAILED with exit code %EXIT_CODE%.
)
echo.
echo Press any key to close this window...
pause >nul

exit /b %EXIT_CODE%