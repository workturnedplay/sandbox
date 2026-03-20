@echo off

::type nul > debug.log

::if running as admin must get back to current dir:
cd /d %~dp0
echo running from: %CD%
set GODEBUG=allocfreetrace=1
.\razer-glitch-fixer.exe
:: CAPTURE THE EXIT CODE IMMEDIATELY
set EXIT_CODE=%ERRORLEVEL%

:: Check if the code is NOT 0 (errors or intentional exit codes like 5)
if %EXIT_CODE% NEQ 0 (
  echo ---- debug log file echoed below ----
  ::type debug.log
  echo ---- debug log file echoed above ----
)

echo.
echo Process exited with code: %EXIT_CODE%
pause