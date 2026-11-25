// Command preparer builds the bareboot boot media.
//
//	preparer list            show the disks and which ones are safe targets
//	preparer image [flags]   write a bootable disk image (GPT + FAT32 + BOOTX64.EFI)
package main

import (
	"fmt"
	"os"
)

const usage = `bareboot preparer

Usage:
  preparer list              list disks and whether each one can be used
  preparer image [flags]     build a bootable disk image file

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
