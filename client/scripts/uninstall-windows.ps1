# uninstall-windows.ps1 — remove gpg-attest native messaging host from Windows
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$InstallDir = Join-Path $env:LOCALAPPDATA "gpg-attest"

# 1. Remove install directory and all contents
if (Test-Path $InstallDir) {
    Remove-Item -Recurse -Force $InstallDir
    Write-Host "Removed: $InstallDir"
}

# 2. Remove registry keys
$RegKeys = @(
    "HKCU:\Software\Mozilla\NativeMessagingHosts\org.gpg_attest.client",
    "HKCU:\Software\Google\Chrome\NativeMessagingHosts\org.gpg_attest.client",
    "HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\org.gpg_attest.client",
    "HKCU:\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\org.gpg_attest.client"
)

foreach ($key in $RegKeys) {
    Remove-Item -Force -ErrorAction SilentlyContinue $key
    Write-Host "Removed registry key: $key"
}

Write-Host "Uninstall complete."
