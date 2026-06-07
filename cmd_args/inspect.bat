@echo off
setlocal EnableDelayedExpansion

echo ============================================
echo           ARGUMENT INSPECTOR
echo ============================================
echo Full raw string (%%*): %*
echo --------------------------------------------

set /a count=0

:loop
if "%~1" NEQ "" (
    set /a count+=1
    echo Argument !count!: [%1]
    shift
    goto :loop
)

echo --------------------------------------------
echo Total Argument Count: !count!
echo ============================================
pause