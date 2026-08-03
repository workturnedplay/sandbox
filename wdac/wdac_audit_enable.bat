@echo off
echo this is crap, don't use it
pause
exit 1

setlocal
fltmc >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: Administrator privileges required.
    echo Re-run this script as Administrator.
    pause
    exit /b 1
)

rem echo Running as Administrator...
echo Script is running from "%~dp0"
cd /d "%~dp0"

set OUTDIR=%USERPROFILE%\wdac_audit
set XML=%OUTDIR%\wdac_audit.xml
set CIP=%OUTDIR%\wdac_audit.cip

if not exist "%OUTDIR%" mkdir "%OUTDIR%"

echo [1/4] Generating HASH-based WDAC audit policy...

powershell -NoProfile -Command "New-CIPolicy -Level Hash -FilePath '%XML%'"

if %ERRORLEVEL% neq 0 (
    echo ERROR: New-CIPolicy failed
    pause
    exit /b 1
)

echo [2/4] Setting AUDIT mode...

powershell -NoProfile -Command "Set-RuleOption -FilePath '%XML%' -Option 3"

echo [3/4] Converting XML -> CIP...

powershell -NoProfile -Command "ConvertFrom-CIPolicy '%XML%' '%CIP%'"

if %ERRORLEVEL% neq 0 (
    echo ERROR: ConvertFrom-CIPolicy failed
    pause
    exit /b 1
)

echo [4/4] Deploying policy...

where CiTool >nul 2>nul
if %ERRORLEVEL%==0 (
    CiTool.exe /update-policy "%CIP%"
) else (
    copy /Y "%CIP%" "C:\Windows\System32\CodeIntegrity\CIPolicies\Staged\wdac_audit.cip" >nul
)

echo.
echo ==========================================================
echo WDAC AUDIT POLICY INSTALLED (HASH MODE)
echo ACTIVE AFTER REBOOT
echo ==========================================================
echo.

pause

shutdown /r /t 0

endlocal