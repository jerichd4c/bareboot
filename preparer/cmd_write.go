package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"bareboot/preparer/internal/disk"
	"bareboot/preparer/internal/gpt"
)

// runWrite implements "preparer write -disk ID": prepares a USB drive with the
// boot media. Every step that can destroy data is guarded:
//
//	admin check -> fresh disk info + safety rules -> summary -> typed
//	confirmation -> identity re-check -> lock/unmount -> write -> flush ->
//	read-back + structure verification
func runWrite(args []string) error {
	fl := flag.NewFlagSet("write", flag.ExitOnError)
	id := fl.String("disk", "", `disk to prepare, as shown by "preparer list" (e.g. 1 or sde)`)
	efi := fl.String("efi", defaultEFIPath(), "UEFI boot program to install")
	fl.Parse(args)

	if *id == "" {
		return errors.New(`missing -disk; run "preparer list" to see the disks`)
	}

	// 1. Raw disk access is reserved to administrators/root: fail early and clearly.
	if !disk.IsElevated() {
		return errors.New(disk.ElevationHint)
	}

	bootProgram, err := os.ReadFile(*efi)
	if err != nil {
		return fmt.Errorf("cannot read boot program (run scripts\\build-image.ps1 first): %w", err)
	}

	// 2. Current information about the target and the safety rules.
	target, err := disk.Get(*id)
	if err != nil {
		return err
	}
	if problems := target.Problems(MinDiskSize, gpt.SectorSize); len(problems) > 0 {
		return fmt.Errorf("disk %s cannot be used:\n  - %s", target.ID, strings.Join(problems, "\n  - "))
	}

	// 3. Show exactly what will be destroyed and what will be written.
	printSummary(target, len(bootProgram))

	// 4. Explicit confirmation: the user must type a phrase containing the
	//    disk name, so a stray Enter or "y" can never erase a disk.
	phrase := fmt.Sprintf("ERASE DISK %s", target.ID)
	if !confirm(phrase) {
		fmt.Println("Cancelled. Nothing was written.")
		return nil
	}

	// 5. The drive could have been swapped while we waited for the user.
	current, err := disk.Get(*id)
	if err != nil {
		return err
	}
	if !current.SameDevice(target) {
		return errors.New("the disk changed since it was shown (unplugged or replaced); nothing was written")
	}

	// 6. Write and verify. From here on Ctrl+C no longer kills the process:
	//    it is turned into a cancellation that stops cleanly between writes,
	//    closes the device and reports the drive as NOT prepared.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return writeDisk(ctx, current, bootProgram)
}

func printSummary(d disk.Disk, bootSize int) {
	fmt.Println()
	fmt.Println("Target disk")
	fmt.Printf("  Disk:        %s (%s)\n", d.ID, d.Path)
	fmt.Printf("  Model:       %s\n", orDash(d.Model()))
	fmt.Printf("  Serial:      %s\n", orDash(d.Serial))
	fmt.Printf("  Bus:         %s\n", d.Bus)
	fmt.Printf("  Capacity:    %s (%d bytes, %d-byte sectors)\n", disk.FormatSize(d.Size), d.Size, d.SectorSize)
	fmt.Printf("  Volumes:     %s\n", orDash(strings.Join(d.Volumes, " ")))
	fmt.Println()
	fmt.Println("Will be written")
	fmt.Println("  GPT partition table (protective MBR, primary and backup copies)")
	fmt.Printf("  EFI System Partition, %s, FAT32, label BAREBOOT\n", disk.FormatSize(espSectors*gpt.SectorSize))
	fmt.Printf("  \\EFI\\BOOT\\BOOTX64.EFI (%d bytes)\n", bootSize)
	fmt.Println()
	fmt.Println("WARNING: ALL DATA ON THIS DISK WILL BE LOST. Every partition and file on it")
	fmt.Println("will be destroyed. Make sure you have a backup of anything you need.")
	fmt.Println()
}

// confirm asks the user to type phrase exactly; anything else cancels.
func confirm(phrase string) bool {
	fmt.Printf("Type %q to continue, or anything else to cancel: ", phrase)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	return strings.TrimSpace(line) == phrase
}

func writeDisk(ctx context.Context, d disk.Disk, bootProgram []byte) (err error) {
	diskSectors := uint64(d.Size) / gpt.SectorSize

	// Dry run: measure how many bytes will be written, for the progress bar.
	var counter countingWriter
	if err := writeMedia(&counter, diskSectors, bootProgram); err != nil {
		return err
	}

	fmt.Printf("\nReleasing the volumes of disk %s (lock/unmount)...\n", d.ID)
	w, err := disk.OpenForWrite(d)
	if err != nil {
		return err
	}
	defer func() {
		// Close flushes and releases the volumes; its error matters too.
		if cerr := w.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			fmt.Println("\nThe preparation did NOT complete and the drive is NOT verified.")
			fmt.Println("It must not be used: fix the problem and run the command again.")
		}
	}()

	// [1/3] Write. The chain is: progress bar -> recorder -> device.
	fmt.Println("[1/3] Writing boot media...")
	rec := &recordingWriter{target: w}
	pw := &progressWriter{ctx: ctx, target: rec, total: counter.total}
	if err := writeMedia(pw, diskSectors, bootProgram); err != nil {
		fmt.Println()
		return err
	}
	fmt.Println()

	// [2/3] Complete the pending writes before reading anything back.
	fmt.Println("[2/3] Flushing pending writes to the device...")
	if err := w.Sync(); err != nil {
		return err
	}
	if err := w.DropCache(); err != nil {
		return err
	}

	// [3/3] Verify: first the raw bytes, then the structures as a firmware sees them.
	fmt.Println("[3/3] Verifying...")
	if err := verifyWritten(w, rec.regions); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}
	if err := verifyStructure(w, diskSectors, bootProgram); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	fmt.Printf("\nDone: disk %s prepared and verified (%s written).\n", d.ID, formatBytes(counter.total))
	fmt.Println("It can now boot the bareboot program on a UEFI machine (Secure Boot off).")
	return nil
}

func formatBytes(n int64) string { return disk.FormatSize(n) }
