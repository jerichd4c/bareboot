# Runs the Linux build of the preparer inside WSL as root.
#
# Usage (from any folder):
#   .\scripts\preparer.ps1 list
#   .\scripts\preparer.ps1 write -disk sde
#
# The USB drive must be attached to WSL first (once per plug-in):
#   usbipd attach --wsl --busid <BUSID>      (see "usbipd list")
# and given back to Windows afterwards:
#   usbipd detach --busid <BUSID>
#
# "wsl -u root" runs the command as Linux root: raw disk access needs it,
# like "Run as administrator" on Windows.
$ErrorActionPreference = 'Stop'

$build = Join-Path (Split-Path $PSScriptRoot -Parent) 'build'
if (-not (Test-Path (Join-Path $build 'preparer-linux'))) {
    throw 'build\preparer-linux not found (run scripts\build-image.ps1 first)'
}
wsl -u root --cd $build -e ./preparer-linux @args
exit $LASTEXITCODE
