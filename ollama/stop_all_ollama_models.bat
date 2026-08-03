@echo off
echo Note: 'ollama serve' should be running, not ollama app or otherwise auto-ran! else will get stuck!
rem ------------------------------------------------------------
rem  stop_all_ollama_models.bat
rem  Stops every model currently loaded into Ollama.
rem ------------------------------------------------------------
setlocal EnableDelayedExpansion

echo ---------------------------------------------
echo Stopping all loaded Ollama models...
echo ---------------------------------------------

rem Skip the header line (NAME …) and grab the first column (the model name)
for /f "skip=1 tokens=1" %%M in ('ollama ps') do (
    echo Stopping model: %%M
    ollama stop "%%M"
)

echo ---------------------------------------------
echo All models stopped.
echo ---------------------------------------------
endlocal

pause
ollama ps
pause