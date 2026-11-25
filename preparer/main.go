// Command preparer builds the bareboot boot media.
// For now it writes a bootable disk image: GPT + ESP (FAT32) + BOOTX64.EFI.
package main

import (
	"flag"
	"fmt"
	"os"

	"bareboot/preparer/internal/gpt"
)

func main() {
	out := flag.String("o", "../build/bareboot.img", "output image file")
	efi := flag.String("efi", "../build/esp/EFI/BOOT/BOOTX64.EFI", "UEFI boot program to install")
	flag.Parse()

	bootProgram, err := os.ReadFile(*efi)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot read boot program (run boot\\build.ps1 first):", err)
		os.Exit(1)
	}

	if err := buildImage(*out, bootProgram); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("image written: %s (%d-byte boot program)\n", *out, len(bootProgram))
}

func buildImage(path string, bootProgram []byte) (err error) {
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
	if err := f.Truncate(MinDiskSize); err != nil {
		return err
	}
	if err := writeMedia(f, MinDiskSize/gpt.SectorSize, bootProgram); err != nil {
		return err
	}
	return f.Sync()
}
