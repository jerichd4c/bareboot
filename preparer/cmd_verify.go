package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"bareboot/preparer/internal/disk"
	"bareboot/preparer/internal/gpt"
)

// runVerify implements "preparer verify": checks, without writing anything,
// that a disk or an image file is a correct bareboot boot media.
func runVerify(args []string) error {
	fl := flag.NewFlagSet("verify", flag.ExitOnError)
	id := fl.String("disk", "", "disk to check, as shown by \"preparer list\"")
	image := fl.String("image", "", "image file to check instead of a disk")
	efi := fl.String("efi", defaultEFIPath(), "boot program the media must contain")
	fl.Parse(args)

	if (*id == "") == (*image == "") {
		return errors.New("give exactly one of -disk or -image")
	}
	bootProgram, err := os.ReadFile(*efi)
	if err != nil {
		return fmt.Errorf("cannot read boot program: %w", err)
	}

	var f *os.File
	var size int64
	if *image != "" {
		if f, err = os.Open(*image); err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		size = info.Size()
		fmt.Printf("Verifying image %s (%s)\n", *image, formatBytes(size))
	} else {
		if !disk.IsElevated() {
			return errors.New(disk.ElevationHint)
		}
		d, err := disk.Get(*id)
		if err != nil {
			return err
		}
		// Read-only: nothing is unmounted or locked.
		if f, err = os.Open(d.Path); err != nil {
			return fmt.Errorf("opening %s: %w", d.Path, disk.Explain(err))
		}
		if err := disk.DropCache(f); err != nil {
			f.Close()
			return err
		}
		size = d.Size
		fmt.Printf("Verifying disk %s (%s, %s)\n", d.ID, orDash(d.Model()), formatBytes(size))
	}
	defer f.Close()

	if err := verifyStructure(f, uint64(size)/gpt.SectorSize, bootProgram); err != nil {
		return fmt.Errorf("NOT a valid bareboot media: %w", err)
	}
	fmt.Println("\nValid bareboot boot media.")
	return nil
}
