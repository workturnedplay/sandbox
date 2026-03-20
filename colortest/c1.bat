@echo off
:: Enable virtual terminal processing for ANSI (Windows 10+)
:: Usually enabled by default in Windows Terminal
:: If not, use: reg add HKCU\Console /v VirtualTerminalLevel /t REG_DWORD /d 1 /f

:: Escape character
set "ESC="
for /F %%A in ('echo prompt $E ^| cmd') do set "ESC=%%A"

:: Green text
echo %ESC%[32mThis is green

:: Red text
echo %ESC%[31mThis is red

:: Reset colors
echo %ESC%[0mBack to default
pause