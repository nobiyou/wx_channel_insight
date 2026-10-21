@echo off
setlocal
set "PACKAGE_ROOT=%~dp0..\.."
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PACKAGE_ROOT%\scripts\windows\stop-insight-preview.ps1" -PackageRoot "%PACKAGE_ROOT%"
pause
endlocal
