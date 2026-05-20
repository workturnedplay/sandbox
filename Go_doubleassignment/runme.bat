@echo off
echo =========================================
echo  COMPILING SNIPPET 1 ASSEMBLY (Sinking Check)
echo =========================================
:: We redirect stderr (2) to stdout (1) so PowerShell can parse it all
go build -gcflags="-S" snippet1.go 2>&1 | powershell -NoProfile -Command "$input | Select-String 'MOVQ|CALL|TEST|J' -Context 0,0"

echo.
echo =========================================
echo  COMPILING SNIPPET 2 ASSEMBLY (Dead Store Check)
echo =========================================
go build -gcflags="-S" snippet2.go 2>&1 | powershell -NoProfile -Command "$input | Select-String 'MOVQ|CALL|TEST|J' -Context 0,0"

echo.
echo =========================================
echo  GENERATING SSA.HTML FOR SNIPPET 1
echo =========================================
:: Set the environment variable locally in the batch session (no flag needed)
set GOSSAFUNC=MainLogic
go build snippet1.go

echo Done! Open ssa.html in your browser to view the matrix.
pause