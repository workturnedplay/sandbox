@echo off

::if running as admin must get back to current dir:
cd /d %~dp0

.\ppfbollocks.exe
echo ec:%ERRORLEVEL%
pause