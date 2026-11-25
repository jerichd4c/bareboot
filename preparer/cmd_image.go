package main

import (
	"flag"
	"fmt"
	"os"

	"bareboot/preparer/internal/gpt"
)

// runImage implements "preparer image": writes the boot media to a file.
func runImage(args []string) error {
	fl := flag.NewFlagSet("image", flag.ExitOnError)
	out := fl.String("o", "../build/bareboot.img", "output image file")
	efi := fl.String("efi", "../build/esp/EFI/BOOT/BOOTX64.EFI", "UEFI boot program to install")
	fl.Parse(args)

	bootProgram, err := os.ReadFile(*efi)
	if err != nil {
		return fmt.Errorf("cannot read boot program (run boot\\build.ps1 first): %w", err)
	}
	if err := buildImage(*out, bootProgram); err != nil {
		return err
	}
	fmt.Printf("image written: %s (%d-byte boot program)\n", *out, len(bootProgram))
	return nil
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
