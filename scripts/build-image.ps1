# Builds the complete boot media image:
#   1. boot\build.ps1  compiles the boot program (C)      -> build\esp\EFI\BOOT\BOOTX64.EFI
#   2. go build        compiles the preparer tool (Go)    -> build\preparer.exe
#   3. preparer image  writes GPT + FAT32 + that file     -> build\bareboot.img
$ErrorActionPreference = 'Stop'

$root     = Split-Path $PSScriptRoot -Parent
$efi      = Join-Path $root 'build\esp\EFI\BOOT\BOOTX64.EFI'
$image    = Join-Path $root 'build\bareboot.img'
$preparer = Join-Path $root 'build\preparer.exe'

Write-Host '[1/3] Building boot program...'
& (Join-Path $root 'boot\build.ps1')

# -s -w strips the symbol table and debug info. Besides making the binary
# smaller, Windows Smart App Control blocks the unstripped build of this tool
# (an unsigned program that opens raw disks), so always use this script.
Write-Host '[2/3] Building preparer...'
Push-Location (Join-Path $root 'preparer')
try {
    go build -ldflags '-s -w' -o $preparer .
    if ($LASTEXITCODE -ne 0) { throw 'Preparer build failed' }
} finally {
    Pop-Location
}

Write-Host '[3/3] Building disk image...'
& $preparer image -o $image -efi $efi
if ($LASTEXITCODE -ne 0) { throw 'Preparer failed' }
