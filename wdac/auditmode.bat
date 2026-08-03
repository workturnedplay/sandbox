rem in powershell
echo nevermind this, delete it, and press C-c here
pause
rem 1) Generate Microsoft baseline WDAC policy (audit mode)
rem This creates a starter policy from the running system (safe baseline):
New-CIPolicy -Level Microsoft -FilePath "$env:USERPROFILE\wdac_audit.xml" -UserPEs 3
rem -Level Microsoft → trusts Microsoft-signed Windows components
rem -UserPEs 3 → includes user-mode binaries in analysis

rem 2) Convert policy to binary WDAC format
ConvertFrom-CIPolicy "$env:USERPROFILE\wdac_audit.xml" "$env:USERPROFILE\wdac_audit.cip"

