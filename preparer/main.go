// Command preparer builds the bareboot boot media.
//
//	preparer list            show the disks and which ones are safe targets
//	preparer image [flags]   write a bootable disk image (GPT + FAT32 + BOOTX64.EFI)
//	preparer write -disk ID  prepare a USB drive (administrator/root only)
//	preparer verify ...      check a prepared disk or image without writing
//
// It builds for Windows (Win32 API) and Linux (sysfs + syscalls); only the
// internal/disk package differs between the two.
package main

import (
	"fmt"
	"os"
)

const usage = `bareboot preparer

Usage:
  preparer list              list disks and whether each one can be used
  preparer image [flags]     build a bootable disk image file
  preparer write -disk ID    prepare a USB disk (administrator / root)
  preparer verify -disk ID   check a prepared disk without writing
  preparer verify -image F   check an image file

Run "preparer <command> -h" for the flags of a command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "list":
		err = runList(args)
	case "image":
		err = runImage(args)
	case "write":
		err = runWrite(args)
	case "verify":
		err = runVerify(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
