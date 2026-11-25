package main

import (
	"fmt"
	"io"
	"time"

	"bareboot/preparer/internal/fat32"
	"bareboot/preparer/internal/gpt"
)

const (
	MiB = 1024 * 1024

	espStartLBA = 2048                      // 1 MiB alignment, the usual first partition offset
	espSectors  = 64 * MiB / gpt.SectorSize // 64 MiB EFI System Partition

	// MinDiskSize is the smallest disk that fits the ESP plus both GPT copies.
	MinDiskSize = 66 * MiB
)

// writeMedia writes the complete boot structure to w, a disk (or disk image)
// of diskSectors sectors:
//
//	GPT  ->  one EFI System Partition  ->  FAT32  ->  \EFI\BOOT\BOOTX64.EFI
func writeMedia(w io.WriterAt, diskSectors uint64, bootProgram []byte) error {
	diskID, err := gpt.NewRandomGUID()
	if err != nil {
		return err
	}
	espID, err := gpt.NewRandomGUID()
	if err != nil {
		return err
	}

	// 1. File system inside the ESP. Written before the partition table so an
	//    interrupted run leaves a disk without a valid GPT, not a broken ESP.
	fs := fat32.New(fat32.Options{
		TotalSectors:  espSectors,
		HiddenSectors: espStartLBA,
		Label:         "BAREBOOT",
		Time:          time.Now(),
	})
	efiDir, err := fs.Root().Mkdir("EFI")
	if err != nil {
		return err
	}
	bootDir, err := efiDir.Mkdir("BOOT")
	if err != nil {
		return err
	}
	// Default path every x86-64 UEFI firmware looks for on removable media.
	if err := bootDir.AddFile("BOOTX64.EFI", bootProgram); err != nil {
		return err
	}
	espWriter := io.NewOffsetWriter(w, espStartLBA*gpt.SectorSize)
	if err := fs.Write(espWriter); err != nil {
		return fmt.Errorf("formatting ESP: %w", err)
	}

	// 2. Partition table describing where the ESP is.
	table := gpt.Table{
		DiskSectors: diskSectors,
		DiskID:      diskID,
		Partitions: []gpt.Partition{{
			Type:     gpt.ESPType,
			ID:       espID,
			FirstLBA: espStartLBA,
			LastLBA:  espStartLBA + espSectors - 1,
			Name:     "EFI System Partition",
		}},
	}
	if err := table.Write(w); err != nil {
		return fmt.Errorf("writing partition table: %w", err)
	}
	return nil
}
