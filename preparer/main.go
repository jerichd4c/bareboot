// Command preparer builds the bareboot boot media.
// For now it only writes a disk image containing an empty GPT with one ESP.
package main

import (
	"flag"
	"fmt"
	"os"

	"bareboot/preparer/internal/gpt"
)

const (
	MiB = 1024 * 1024

	espStartLBA = 2048                      // 1 MiB alignment, the usual first partition offset
	espSectors  = 64 * MiB / gpt.SectorSize // 64 MiB EFI System Partition
	imageSize   = 66 * MiB                  // ESP + room for both GPT copies
)

func main() {
	out := flag.String("o", "../build/bareboot.img", "output image file")
	flag.Parse()

	if err := buildImage(*out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("image written:", *out)
}

func buildImage(path string) (err error) {
	diskID, err := gpt.NewRandomGUID()
	if err != nil {
		return err
	}
	espID, err := gpt.NewRandomGUID()
	if err != nil {
		return err
	}

	table := gpt.Table{
		DiskSectors: imageSize / gpt.SectorSize,
		DiskID:      diskID,
		Partitions: []gpt.Partition{{
			Type:     gpt.ESPType,
			ID:       espID,
			FirstLBA: espStartLBA,
			LastLBA:  espStartLBA + espSectors - 1,
			Name:     "EFI System Partition",
		}},
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	// Give the file its final size; unwritten areas read back as zeros.
	if err := f.Truncate(imageSize); err != nil {
		return err
	}
	if err := table.Write(f); err != nil {
		return err
	}
	return f.Sync()
}
