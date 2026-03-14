# install-windows.ps1 — STUB: Windows native messaging host registration
#
# Not yet implemented. When implemented, this script will:
#
#   1. Copy the binary to a suitable location (e.g. %LOCALAPPDATA%\sig-stuff\)
#   2. Write the Chrome manifest JSON to a local file and register it under:
#        HKCU\Software\Google\Chrome\NativeMessagingHosts\dev.sig_stuff.native
#      pointing to the manifest file path.
#   3. Write the Firefox manifest JSON and register it under:
#        HKCU\Software\Mozilla\NativeMessagingHosts\dev.sig_stuff.native
#
# On Windows the registry key value must be the full path to the manifest JSON file,
# not the binary itself.

Write-Error "Windows install not yet implemented."
exit 1
