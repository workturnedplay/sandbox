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
if "%EXIT_CODE%"=="0" (
    echo Operation completed successfully.
) else (
    echo Operation FAILED with exit code %EXIT_CODE%.
)
echo.
echo Press any key to close this window...
pause >nul

exit /b %EXIT_CODE%

===PS_START===
$ErrorActionPreference = 'Stop'

$code = @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class RestoreTrustedInstallerOwner
{
    private const uint TOKEN_QUERY = 0x0008;
    private const uint TOKEN_ADJUST_PRIVILEGES = 0x0020;
    private const uint SE_PRIVILEGE_ENABLED = 0x00000002;
    private const int ERROR_NOT_ALL_ASSIGNED = 1300;

    private const uint SE_REGISTRY_KEY = 4;
    private const uint OWNER_SECURITY_INFORMATION = 0x00000001;

[StructLayout(LayoutKind.Sequential)]
private struct LUID_AND_ATTRIBUTES_CHECK
{
    public LUID Luid;
    public uint Attributes;
}

[StructLayout(LayoutKind.Sequential)]
private struct PRIVILEGE_SET
{
    public uint PrivilegeCount;
    public uint Control;
    public LUID_AND_ATTRIBUTES_CHECK Privilege;
}

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


    
    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern bool MakeSelfRelativeSecurityDescriptor(
    IntPtr pAbsoluteSecurityDescriptor,
    IntPtr pSelfRelativeSecurityDescriptor,
    uint dwBufferLength,
    out uint lpdwBufferLength);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern bool OpenProcessToken(
        IntPtr ProcessHandle,
        uint DesiredAccess,
        out IntPtr TokenHandle);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern bool LookupPrivilegeValueW(
        string lpSystemName,
        string lpName,
        out LUID lpLuid);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern bool AdjustTokenPrivileges(
        IntPtr TokenHandle,
        bool DisableAllPrivileges,
        ref TOKEN_PRIVILEGES NewState,
        uint BufferLength,
        IntPtr PreviousState,
        IntPtr ReturnLength);
        
        [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern bool PrivilegeCheck(
    IntPtr ClientToken,
    ref PRIVILEGE_SET RequiredPrivileges,
    out bool pfResult);
        
        [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern bool GetTokenInformation(
    IntPtr TokenHandle,
    int TokenInformationClass,
    IntPtr TokenInformation,
    uint TokenInformationLength,
    out uint ReturnLength);

    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern uint SetNamedSecurityInfoW(
        string pObjectName,
        uint ObjectType,
        uint SecurityInfo,
        IntPtr psidOwner,
        IntPtr psidGroup,
        IntPtr pDacl,
        IntPtr pSacl);

    [DllImport("kernel32.dll", ExactSpelling = true, SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);
    
    [DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern uint RegOpenKeyExW(
    IntPtr hKey,
    string lpSubKey,
    uint ulOptions,
    uint samDesired,
    out IntPtr phkResult);

[DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern uint RegSetKeySecurity(
    IntPtr hKey,
    uint SecurityInformation,
    IntPtr pSecurityDescriptor);

[DllImport("advapi32.dll", ExactSpelling = true)]
private static extern uint RegCloseKey(
    IntPtr hKey);

[DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern bool InitializeSecurityDescriptor(
    IntPtr pSecurityDescriptor,
    uint dwRevision);

[DllImport("advapi32.dll", ExactSpelling = true, SetLastError = true)]
private static extern bool SetSecurityDescriptorOwner(
    IntPtr pSecurityDescriptor,
    IntPtr pOwner,
    bool bOwnerDefaulted);

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
                "OpenProcessToken failed.");
        }

        try
        {
            LUID luid;

            if (!LookupPrivilegeValueW(
                    null,
                    "SeRestorePrivilege",
                    out luid))
            {
                throw new Win32Exception(
                    Marshal.GetLastWin32Error(),
                    "LookupPrivilegeValueW failed.");
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
                    "AdjustTokenPrivileges failed.");
            }

            int error = Marshal.GetLastWin32Error();

            if (error == ERROR_NOT_ALL_ASSIGNED)
{
    throw new Win32Exception(
        error,
        "SeRestorePrivilege was not assigned to this process token.");
}

if (error != 0)
{
    throw new Win32Exception(
        error,
        "Unexpected error after AdjustTokenPrivileges.");
}
        }
        finally
        {
            if (token != IntPtr.Zero)
            {
                CloseHandle(token);
            }
        }
    }

public static bool IsRestorePrivilegeEnabled()
{
    IntPtr token = IntPtr.Zero;

    if (!OpenProcessToken(
            System.Diagnostics.Process.GetCurrentProcess().Handle,
            TOKEN_QUERY,
            out token))
    {
        throw new Win32Exception(
            Marshal.GetLastWin32Error(),
            "OpenProcessToken failed while checking SeRestorePrivilege.");
    }

    try
    {
        LUID luid;

        if (!LookupPrivilegeValueW(
                null,
                "SeRestorePrivilege",
                out luid))
        {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "LookupPrivilegeValueW failed while checking SeRestorePrivilege.");
        }

        PRIVILEGE_SET required = new PRIVILEGE_SET
        {
            PrivilegeCount = 1,
            Control = 1, // PRIVILEGE_SET_ALL_NECESSARY
            Privilege = new LUID_AND_ATTRIBUTES_CHECK
            {
                Luid = luid,
                Attributes = SE_PRIVILEGE_ENABLED
            }
        };

        bool enabled;

        if (!PrivilegeCheck(
                token,
                ref required,
                out enabled))
        {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                "PrivilegeCheck failed.");
        }

        return enabled;
    }
    finally
    {
        CloseHandle(token);
    }
}

public static void SetOwner(string owner)
{
    const uint HKEY_LOCAL_MACHINE = 0x80000002;
    const uint KEY_READ = 0x00020019;
    const uint WRITE_OWNER = 0x00080000;
    const uint OWNER_SECURITY_INFORMATION = 0x00000001;
    const uint SECURITY_DESCRIPTOR_REVISION = 1;

    IntPtr hKey = IntPtr.Zero;

    uint result = RegOpenKeyExW(
        new IntPtr(unchecked((int)HKEY_LOCAL_MACHINE)),
        "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing",
        0,
        KEY_READ | WRITE_OWNER,
        out hKey);

    if (result != 0)
    {
        throw new Win32Exception(
            (int)result,
            "RegOpenKeyExW failed.");
    }

    try
    {
        var account =
            new System.Security.Principal.NTAccount(owner);

        var sid =
            (System.Security.Principal.SecurityIdentifier)
            account.Translate(
                typeof(System.Security.Principal.SecurityIdentifier));

        byte[] sidBytes = new byte[sid.BinaryLength];
        sid.GetBinaryForm(sidBytes, 0);

        IntPtr sidPtr = Marshal.AllocHGlobal(sidBytes.Length);

        try
        {
            Marshal.Copy(
                sidBytes,
                0,
                sidPtr,
                sidBytes.Length);

            // Build an absolute security descriptor containing
            // the requested owner.
            IntPtr absoluteSd = Marshal.AllocHGlobal(1024);

            try
            {
                if (!InitializeSecurityDescriptor(
                        absoluteSd,
                        SECURITY_DESCRIPTOR_REVISION))
                {
                    throw new Win32Exception(
                        Marshal.GetLastWin32Error(),
                        "InitializeSecurityDescriptor failed.");
                }

                if (!SetSecurityDescriptorOwner(
                        absoluteSd,
                        sidPtr,
                        false))
                {
                    throw new Win32Exception(
                        Marshal.GetLastWin32Error(),
                        "SetSecurityDescriptorOwner failed.");
                }

                // RegSetKeySecurity expects a self-relative
                // security descriptor. First query the required size.
                uint selfRelativeSize = 0;

                MakeSelfRelativeSecurityDescriptor(
                    absoluteSd,
                    IntPtr.Zero,
                    0,
                    out selfRelativeSize);

                int sizeError = Marshal.GetLastWin32Error();

                if (selfRelativeSize == 0)
                {
                    throw new Win32Exception(
                        sizeError,
                        "Could not determine self-relative security descriptor size.");
                }

                IntPtr selfRelativeSd =
                    Marshal.AllocHGlobal((int)selfRelativeSize);

                try
                {
                    if (!MakeSelfRelativeSecurityDescriptor(
                            absoluteSd,
                            selfRelativeSd,
                            selfRelativeSize,
                            out selfRelativeSize))
                    {
                        throw new Win32Exception(
                            Marshal.GetLastWin32Error(),
                            "MakeSelfRelativeSecurityDescriptor failed.");
                    }

                    result = RegSetKeySecurity(
                        hKey,
                        OWNER_SECURITY_INFORMATION,
                        selfRelativeSd);

                    if (result != 0)
                    {
                        throw new Win32Exception(
                            (int)result,
                            "RegSetKeySecurity failed.");
                    }
                }
                finally
                {
                    Marshal.FreeHGlobal(selfRelativeSd);
                }
            }
            finally
            {
                Marshal.FreeHGlobal(absoluteSd);
            }
        }
        finally
        {
            Marshal.FreeHGlobal(sidPtr);
        }
    }
    finally
    {
        RegCloseKey(hKey);
    }
}
    
}

'@

Add-Type -TypeDefinition $code

$registryPath = 'MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing'
$psRegistryPath = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing'
$expectedOwner = 'NT SERVICE\TrustedInstaller'

Write-Host 'Enabling SeRestorePrivilege...'
[RestoreTrustedInstallerOwner]::EnableRestorePrivilege()

if (-not [RestoreTrustedInstallerOwner]::IsRestorePrivilegeEnabled()) {
    throw 'CRITICAL: SeRestorePrivilege is not enabled in the PowerShell process.'
}

Write-Host 'SeRestorePrivilege is ENABLED in the current process.'
Write-Host 'Restoring registry key owner...'

[RestoreTrustedInstallerOwner]::SetOwner(
    $expectedOwner)

Write-Host 'Verifying owner...'

$actualOwner = (Get-Acl -Path $psRegistryPath).Owner

Write-Host "Actual owner:   $actualOwner"
Write-Host "Expected owner: $expectedOwner"

if ($actualOwner -cne $expectedOwner)
{
    throw "CRITICAL: owner verification failed. Expected '$expectedOwner', but the registry reports '$actualOwner'."
}

Write-Host ''
Write-Host 'SUCCESS: Component Based Servicing is owned by NT SERVICE\TrustedInstaller.'