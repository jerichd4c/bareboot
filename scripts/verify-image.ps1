# Checks a disk image with independent Linux tools running in WSL, so the
# result does not depend on our own code:
#   1. sgdisk -v   GPT structure, CRC32s and backup copy
#   2. sgdisk -i   the partition is an ESP; read its position from the table
#   3. fsck.vfat   FAT32 structure of that partition
#   4. mdir        file tree inside the ESP
#   5. mtype       BOOTX64.EFI inside the image is identical to the original
#
# Requires in WSL: sudo apt install gdisk dosfstools mtools
param(
    [string]$Image = (Join-Path (Split-Path $PSScriptRoot -Parent) 'build\bareboot.img'),
    [string]$Efi   = (Join-Path (Split-Path $PSScriptRoot -Parent) 'build\esp\EFI\BOOT\BOOTX64.EFI')
)
$ErrorActionPreference = 'Stop'

$espType  = 'C12A7328-F81F-11D2-BA4B-00A0C93EC93B'
$efiPath  = '::/EFI/BOOT/BOOTX64.EFI'
$failures = 0

function Write-Result([string]$name, [bool]$ok, [string]$detail) {
    if ($ok) { Write-Host "[PASS] $name - $detail" -ForegroundColor Green }
    else     { Write-Host "[FAIL] $name - $detail" -ForegroundColor Red; $script:failures++ }
}

# Runs a Linux program in WSL without a shell (-e). Disk tools live in /usr/sbin,
# which is not in PATH for non-login sessions, so PATH is set explicitly.
function Invoke-Wsl {
    wsl -e env 'PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' MTOOLS_SKIP_CHECK=1 @args
}

# WSL expects /mnt/c/... paths
function ConvertTo-WslPath([string]$path) {
    (Invoke-Wsl wslpath -a ($path -replace '\\', '/')).Trim()
}

foreach ($f in @($Image, $Efi)) {
    if (-not (Test-Path $f)) { throw "File not found: $f (run scripts\build-image.ps1 first)" }
}
$img = ConvertTo-WslPath (Resolve-Path $Image).Path

# 1. Partition table
$out = (Invoke-Wsl sgdisk -v $img) -join "`n"
Write-Result 'GPT' ($out -match 'No problems found') 'sgdisk -v'

# 2. Partition 1 must be an ESP; its first/last sector come from the table itself
$info  = (Invoke-Wsl sgdisk -i 1 $img) -join "`n"
$isEsp = $info -match "Partition GUID code: $espType"
Write-Result 'ESP type' $isEsp 'partition 1 has the EFI System Partition GUID'
if ($info -notmatch 'First sector: (\d+)' ) { throw 'Cannot read the ESP position' }
$first = [int64]$Matches[1]
if ($info -notmatch 'Last sector: (\d+)') { throw 'Cannot read the ESP position' }
$count  = [int64]$Matches[1] - $first + 1
$offset = $first * 512

# 3. File system: fsck.vfat needs the partition alone, so copy it out with dd
$espFile = Join-Path (Split-Path $Image -Parent) 'esp-check.img'
$esp     = ConvertTo-WslPath $espFile
# 1 MiB blocks with byte offsets: with bs=512 dd makes 131072 tiny writes
# across the WSL <-> Windows file bridge, which can take minutes.
Invoke-Wsl dd "if=$img" "of=$esp" bs=1M "skip=$offset" "count=$($count * 512)" `
    'iflag=skip_bytes,count_bytes' status=none  # quoted: a bare comma makes a PowerShell array
$fsck = (Invoke-Wsl fsck.vfat -n -v $esp) -join "`n"
$fsckOk = $LASTEXITCODE -eq 0
Remove-Item $espFile -ErrorAction SilentlyContinue
Write-Result 'FAT32' $fsckOk 'fsck.vfat -n (read-only check)'
if (-not $fsckOk) { Write-Host $fsck }

# 4. File tree (mtools reads the FAT32 directly inside the image at the ESP offset)
$tree = (Invoke-Wsl mdir -/ -b -i "$img@@$offset" '::') -join "`n"
Write-Result 'Files' ($tree -match 'EFI/BOOT/BOOTX64\.EFI') '\EFI\BOOT\BOOTX64.EFI present'

# 5. Contents: hash of the file inside the image vs the compiled original
$inside   = (Invoke-Wsl sh -c "mtype -i '$img@@$offset' '$efiPath' | sha256sum").Split(' ')[0].ToUpper()
$original = (Get-FileHash $Efi -Algorithm SHA256).Hash
Write-Result 'Contents' ($inside -eq $original) "SHA-256 $($original.Substring(0, 16))..."

Write-Host ''
if ($failures -gt 0) {
    Write-Host "Image NOT verified: $failures check(s) failed." -ForegroundColor Red
    exit 1
}
Write-Host 'Image verified.' -ForegroundColor Green
