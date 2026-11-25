# Builds the complete boot media image:
#   1. boot\build.ps1  compiles the boot program (C)  -> build\esp\EFI\BOOT\BOOTX64.EFI
#   2. the preparer (Go) writes GPT + FAT32 + that file -> build\bareboot.img
$ErrorActionPreference = 'Stop'

$root  = Split-Path $PSScriptRoot -Parent
$efi   = Join-Path $root 'build\esp\EFI\BOOT\BOOTX64.EFI'
$image = Join-Path $root 'build\bareboot.img'

Write-Host '[1/2] Building boot program...'
& (Join-Path $root 'boot\build.ps1')

Write-Host '[2/2] Building disk image...'
Push-Location (Join-Path $root 'preparer')
try {
    go run . -o $image -efi $efi
    if ($LASTEXITCODE -ne 0) { throw 'Preparer failed' }
} finally {
    Pop-Location
}
