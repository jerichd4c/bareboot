# Boots a disk image in QEMU with the UEFI firmware (OVMF) shipped with QEMU.
#
# Usage:
#   .\scripts\run-qemu.ps1                          # boots build\bareboot.img in a window
#   .\scripts\run-qemu.ps1 -Image other.img
#   .\scripts\run-qemu.ps1 -display none -serial stdio   # extra arguments go to QEMU
param(
    [string]$Image = (Join-Path (Split-Path $PSScriptRoot -Parent) 'build\bareboot.img')
)
$ErrorActionPreference = 'Stop'

$build     = Join-Path (Split-Path $PSScriptRoot -Parent) 'build'
$qemuShare = 'C:\Program Files\qemu\share'
$vars      = Join-Path $build 'vars.fd'

if (-not (Test-Path $Image)) {
    throw "Image not found: $Image (run scripts\build-image.ps1 first)"
}
$Image = (Resolve-Path $Image).Path

# Writable copy of the firmware variable store (boot entries, settings)
if (-not (Test-Path $vars)) {
    Copy-Item (Join-Path $qemuShare 'edk2-i386-vars.fd') $vars
}

# pflash 0: firmware code (read-only) | pflash 1: firmware variables
# drive:    the disk image, seen by the VM as a plain hard disk
qemu-system-x86_64 -m 256M -net none $args `
    -drive "if=pflash,format=raw,readonly=on,file=$qemuShare\edk2-x86_64-code.fd" `
    -drive "if=pflash,format=raw,file=$vars" `
    -drive "format=raw,file=$Image"
