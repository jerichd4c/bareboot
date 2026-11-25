# Builds the complete boot media image:
#   1. boot\build.ps1   compiles the boot program (C)         -> build\esp\EFI\BOOT\BOOTX64.EFI
#   2. go build         compiles the preparer for Linux/WSL   -> build\preparer-linux
#                       and for Windows                       -> build\preparer.exe
#   3. preparer image   writes GPT + FAT32 + that file        -> build\bareboot.img
#
# The image is built by the Linux binary inside WSL: on this machine Windows
# Smart App Control blocks the (unsigned) Windows build of the preparer.
$ErrorActionPreference = 'Stop'

$root  = Split-Path $PSScriptRoot -Parent
$build = Join-Path $root 'build'

Write-Host '[1/3] Building boot program...'
& (Join-Path $root 'boot\build.ps1')

Write-Host '[2/3] Building preparer (Linux and Windows)...'
Push-Location (Join-Path $root 'preparer')
try {
    # Cross-compilation: Go builds a Linux ELF binary from Windows by itself.
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'
    go build -ldflags '-s -w' -o (Join-Path $build 'preparer-linux') .
    if ($LASTEXITCODE -ne 0) { throw 'Linux build failed' }

    $env:GOOS = 'windows'
    go build -ldflags '-s -w' -o (Join-Path $build 'preparer.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

Write-Host '[3/3] Building disk image...'
# Default paths are relative to the executable: build/bareboot.img, build/esp/...
wsl --cd $build -e ./preparer-linux image
if ($LASTEXITCODE -ne 0) { throw 'Preparer failed' }
