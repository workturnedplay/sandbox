@echo off
setlocal enabledelayedexpansion

rem WDAC = “can this code run?”
rem NTFS = “can this file be read/written?”

REM ===== CONFIG =====
set RUST_BIN=%USERPROFILE%\.rustup\toolchains
set MSYS_BIN=C:\msys64\ucrt64\bin
set OUTDIR=%USERPROFILE%\wdac_policy
set XML=%OUTDIR%\wdac.xml
set BIN=%OUTDIR%\wdac.bin

REM ===== CREATE OUTPUT DIR =====
if not exist "%OUTDIR%" mkdir "%OUTDIR%"

echo [1/4] Generating hash-based WDAC policy...

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "New-CIPolicy -Level Hash -FilePath '%XML%' -ScanPath '%MSYS_BIN%'" 

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "New-CIPolicy -Level Hash -FilePath '%XML%' -ScanPath '%RUST_BIN%' -Append"

rem echo [2/4] Adding extra critical binaries (optional explicit hashing)...

rem powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  rem "Get-ChildItem '%MSYS_BIN%' -Filter *.exe | ForEach-Object { Add-SignerRule -FilePath $_.FullName -PolicyPath '%XML%' -ErrorAction SilentlyContinue }"

echo [3/4] Converting policy to binary...

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "ConvertFrom-CIPolicy '%XML%' '%BIN%'"

echo [4/4] Installing WDAC policy (staging for reboot)...

REM WARNING: this overwrites current SIPolicy (standard WDAC deployment method)
copy /Y "%BIN%" "C:\Windows\System32\CodeIntegrity\SIPolicy.p7b" >nul

echo.
echo DONE.
echo Policy staged. REBOOT REQUIRED to activate WDAC changes.
echo.

pause
endlocal