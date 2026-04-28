@echo off
echo Generating loadmeinfirefox.txt, please wait...
powershell -NoProfile -Command ^
  "$w = [System.IO.StreamWriter]::new('loadmeinfirefox.txt');" ^
  "for ($i = 1; $i -le 900000; $i++) { $w.WriteLine('line {0:D7}' -f $i) };" ^
  "$w.Close();" ^
  "Write-Host 'Done.'"