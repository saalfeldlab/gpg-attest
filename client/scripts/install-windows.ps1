# install-windows.ps1 — install gpg-attest native messaging host on Windows
# Usage: .\install-windows.ps1 [path\to\gpg-attest-windows-amd64.exe]
#   Defaults to .\build\gpg-attest-windows-amd64.exe if no argument given.
param(
    [string]$BinaryPath = ".\build\gpg-attest-windows-amd64.exe"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not (Test-Path $BinaryPath)) {
    Write-Error "Binary not found at $BinaryPath`nRun 'make cross' first."
    exit 1
}

$InstallDir  = Join-Path $env:LOCALAPPDATA "gpg-attest"
$InstallPath = Join-Path $InstallDir "gpg-attest.exe"

# 1. Create install directory and copy binary
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Force $BinaryPath $InstallPath
if (-not (Test-Path $InstallPath)) {
    Write-Error "Failed to copy binary to $InstallPath"
    exit 1
}
Write-Host "Installed binary: $InstallPath"

# Use forward slashes in JSON paths — accepted by Chrome, Firefox, and Edge
$InstallPathJson = $InstallPath.Replace('\', '/')

# 2. Build and write Firefox manifest
$FirefoxManifestPath = Join-Path $InstallDir "org.gpg_attest.client.firefox.json"
$FirefoxManifest = [ordered]@{
    name               = "org.gpg_attest.client"
    description        = "gpg-attest native messaging host: signs content via GnuPG"
    path               = $InstallPathJson
    type               = "stdio"
    allowed_extensions = @("attestension@gpg-attest.org")
}
$FirefoxManifest | ConvertTo-Json | Set-Content -Encoding UTF8 $FirefoxManifestPath
Write-Host "Wrote manifest:  $FirefoxManifestPath"

# 3. Build and write Chrome/Edge/Brave manifest
$ChromeManifestPath = Join-Path $InstallDir "org.gpg_attest.client.chrome.json"
$ChromeManifest = [ordered]@{
    name            = "org.gpg_attest.client"
    description     = "gpg-attest native messaging host: signs content via GnuPG"
    path            = $InstallPathJson
    type            = "stdio"
    allowed_origins = @("chrome-extension://PLACEHOLDER_CHROME_EXTENSION_ID/")
}
$ChromeManifest | ConvertTo-Json | Set-Content -Encoding UTF8 $ChromeManifestPath
Write-Host "Wrote manifest:  $ChromeManifestPath"

# 4. Register manifests in the registry
$RegEntries = @(
    @{ Key = "HKCU:\Software\Mozilla\NativeMessagingHosts\org.gpg_attest.client";             Value = $FirefoxManifestPath },
    @{ Key = "HKCU:\Software\Google\Chrome\NativeMessagingHosts\org.gpg_attest.client";       Value = $ChromeManifestPath  },
    @{ Key = "HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\org.gpg_attest.client";      Value = $ChromeManifestPath  },
    @{ Key = "HKCU:\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\org.gpg_attest.client"; Value = $ChromeManifestPath }
)

foreach ($entry in $RegEntries) {
    New-Item -Force -Path $entry.Key | Out-Null
    Set-ItemProperty -Path $entry.Key -Name "(default)" -Value $entry.Value
    Write-Host "Registered:      $($entry.Key)"
}

Write-Host ""
Write-Host "Done. Reload your browser extension to pick up the new host."
