# Boots build\esp in QEMU with the UEFI firmware (OVMF) shipped with QEMU
$ErrorActionPreference = 'Stop'

$root      = Split-Path $PSScriptRoot -Parent
$build     = Join-Path $root 'build'
$qemuShare = 'C:\Program Files\qemu\share'
$vars      = Join-Path $build 'vars.fd'

# Writable copy of the firmware variable store (boot entries, settings)
if (-not (Test-Path $vars)) {
    Copy-Item (Join-Path $qemuShare 'edk2-i386-vars.fd') $vars
}

Push-Location $build
try {
    qemu-system-x86_64 -m 256M -net none $args `
        -drive "if=pflash,format=raw,readonly=on,file=$qemuShare\edk2-x86_64-code.fd" `
        -drive "if=pflash,format=raw,file=vars.fd" `
        -drive "format=raw,file=fat:rw:esp"
} finally {
    Pop-Location
}