@echo off
SETLOCAL

:: 1. Get GOROOT dynamically if it's not in your system PATH
for /f "delims=" %%i in ('go env GOROOT') do set "GOPATH_ROOT=%%i"

echo Generating certificates...
:: We use quotes because GOROOT often contains spaces (e.g., C:\Program Files\Go)
go run "%GOPATH_ROOT%\src\crypto\tls\generate_cert.go" --host localhost

if %ERRORLEVEL% NEQ 0 (
    echo Failed to generate certificates.
    pause
    exit /b
)


pause