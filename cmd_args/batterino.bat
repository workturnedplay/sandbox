@echo off
setlocal EnableDelayedExpansion

echo =========================================
echo 1. The Raw Input (%%*) looks like:
echo "%*"
call inspect.bat %*
echo =========================================

echo 2. The String Replacement Method:
set "ALL_ARGS=%*"
set "CLEANED_ARGS=!ALL_ARGS:--resized =!"
echo "!CLEANED_ARGS!"
call inspect.bat !CLEANED_ARGS!
echo =========================================

echo 3. The Loop Method (Rebuilding Piece-by-Piece):
set "LOOP_ARGS="
shift
:loop
if "%~1" NEQ "" (
    set "LOOP_ARGS=!LOOP_ARGS! %1"
    shift
    goto :loop
)
echo "!LOOP_ARGS!"
call inspect.bat !LOOP_ARGS!
echo =========================================
pause