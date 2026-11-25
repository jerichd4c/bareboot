package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"bareboot/preparer/internal/fat32"
	"bareboot/preparer/internal/gpt"
)

// region is one write made during preparation: where and what.
type region struct {
	off  int64
	data []byte
}

// recordingWriter passes writes on to the target and keeps a copy of each
// one, so afterwards every byte can be read back from the device and compared.
type recordingWriter struct {
	target  io.WriterAt
	regions []region
}

func (rw *recordingWriter) WriteAt(p []byte, off int64) (int, error) {
	n, err := rw.target.WriteAt(p, off)
	if n > 0 {
		rw.regions = append(rw.regions, region{off: off, data: bytes.Clone(p[:n])})
	}
	return n, err
}

// verifyWritten reads back every region from the device and compares it byte
// by byte with what was written. It answers "did the device store our data?".
func verifyWritten(r io.ReaderAt, regions []region) error {
	var total int64
	for _, reg := range regions {
		got := make([]byte, len(reg.data))
		if _, err := r.ReadAt(got, reg.off); err != nil {
			return fmt.Errorf("reading back offset %d: %w", reg.off, err)
		}
		if !bytes.Equal(got, reg.data) {
			for i := range got {
				if got[i] != reg.data[i] {
					return fmt.Errorf("data mismatch at byte %d: wrote %02X, read %02X", reg.off+int64(i), reg.data[i], got[i])
				}
			}
		}
		total += int64(len(reg.data))
	}
	check(true, fmt.Sprintf("read-back: %s in %d regions identical to what was written", formatBytes(total), len(regions)))
	return nil
}

// verifyStructure parses the media like a firmware would and checks it is
// bootable: valid GPT, an ESP, a FAT32 file system in it and our boot program
// at \EFI\BOOT\BOOTX64.EFI. It answers "is the result a correct boot disk?".
func verifyStructure(r io.ReaderAt, diskSectors uint64, bootProgram []byte) error {
	table, err := gpt.Read(r, diskSectors)
	if err != nil {
		check(false, "GPT: "+err.Error())
		return err
	}
	check(true, "GPT: protective MBR, primary and backup headers and entries valid (CRC32)")

	var esp *gpt.Partition
	for i := range table.Partitions {
		if table.Partitions[i].Type == gpt.ESPType {
			esp = &table.Partitions[i]
			break
		}
	}
	if esp == nil {
		err := errors.New("no EFI System Partition in the partition table")
		check(false, err.Error())
		return err
	}
	espSize := int64(esp.LastLBA-esp.FirstLBA+1) * gpt.SectorSize
	check(true, fmt.Sprintf("ESP: %q, LBA %d-%d (%s)", esp.Name, esp.FirstLBA, esp.LastLBA, formatBytes(espSize)))

	part := io.NewSectionReader(r, int64(esp.FirstLBA)*gpt.SectorSize, espSize)
	data, err := fat32.ReadFile(part, `EFI\BOOT\BOOTX64.EFI`)
	if err != nil {
		check(false, "FAT32: "+err.Error())
		return err
	}
	check(true, "FAT32: boot sector and both FATs valid, \\EFI\\BOOT\\BOOTX64.EFI found")

	if !bytes.Equal(data, bootProgram) {
		err := fmt.Errorf("BOOTX64.EFI on the media (%d bytes) differs from the boot program (%d bytes)", len(data), len(bootProgram))
		check(false, err.Error())
		return err
	}
	check(true, fmt.Sprintf("boot program: %d bytes, identical to the source file", len(data)))
	return nil
}

func check(ok bool, msg string) {
	mark := "OK  "
	if !ok {
		mark = "FAIL"
	}
	fmt.Printf("  [%s] %s\n", mark, msg)
}
