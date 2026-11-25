# Builds the UEFI boot program into build\esp\EFI\BOOT\BOOTX64.EFI
$ErrorActionPreference = 'Stop'

$root  = Split-Path $PSScriptRoot -Parent
$build = Join-Path $root 'build'
$esp   = Join-Path $build 'esp\EFI\BOOT'
New-Item -ItemType Directory -Force $esp | Out-Null

$src = Join-Path $PSScriptRoot 'main.c'
$obj = Join-Path $build 'main.o'
$efi = Join-Path $esp 'BOOTX64.EFI'

clang -target x86_64-unknown-windows -ffreestanding -fshort-wchar -mno-red-zone `
      -mno-stack-arg-probe -fno-stack-protector -Wall -Wextra -O2 `
      -c $src -o $obj
if ($LASTEXITCODE -ne 0) { throw 'Compilation failed' }

# /Brepro: reproducible build. Without it the linker writes the current time
# into the PE header, so every build is a different file and a prepared USB
# could never be compared with the compiled program.
lld-link -subsystem:efi_application -entry:efi_main -nodefaultlib /Brepro "-out:$efi" $obj
if ($LASTEXITCODE -ne 0) { throw 'Linking failed' }

Write-Host "OK: $efi"