package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"bareboot/preparer/internal/disk"
	"bareboot/preparer/internal/gpt"
)

// runList implements "preparer list": shows every physical disk with its
// identity and whether bareboot may write it.
func runList(args []string) error {
	fl := flag.NewFlagSet("list", flag.ExitOnError)
	fl.Parse(args)

	disks, err := disk.List()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DISK\tBUS\tSIZE\tMODEL\tSERIAL\tVOLUMES\tSTATUS")
	eligible := 0
	for _, d := range disks {
		status := "OK - can be used"
		if problems := d.Problems(MinDiskSize, gpt.SectorSize); len(problems) > 0 {
			status = "REJECTED: " + strings.Join(problems, "; ")
		} else {
			eligible++
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			d.Number, d.Bus, disk.FormatSize(d.Size), orDash(d.Model()), orDash(d.Serial),
			orDash(strings.Join(d.Volumes, " ")), status)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Printf("\n%d disk(s) found, %d eligible.\n", len(disks), eligible)
	if eligible == 0 {
		fmt.Println("Plug in a dedicated USB drive to prepare it.")
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
