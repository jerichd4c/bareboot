//go:build linux

package disk

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ElevationHint tells the user how to get the rights IsElevated checks for.
const ElevationHint = "root rights required: run the command with sudo (or wsl -u root)"

// Writer gives raw access to a whole block device (/dev/sdX).
type Writer struct {
	disk Disk
	file *os.File
}

// IsElevated reports whether the process runs as root. Opening a disk
// device for writing is only allowed to root.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// OpenForWrite unmounts every file system of d and opens the device for
// raw reading and writing.
func OpenForWrite(d Disk) (*Writer, error) {
	// 1. Unmount: the kernel would otherwise keep using (and caching) the old
	//    file systems while we overwrite the sectors under them.
	for _, point := range d.Volumes {
		if err := unix.Unmount(point, 0); err != nil {
			return nil, fmt.Errorf("cannot unmount %s: close programs using it and try again: %w", point, Explain(err))
		}
	}

	// 2. Open the device.
	//    O_EXCL on a block device means "exclusive": the kernel refuses if the
	//    disk or any of its partitions is still mounted or opened exclusively.
	//    O_SYNC makes each write return only after the device stored it.
	f, err := os.OpenFile(d.Path, os.O_RDWR|unix.O_EXCL|unix.O_SYNC, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", d.Path, Explain(err))
	}
	return &Writer{disk: d, file: f}, nil
}

// WriteAt writes len(p) bytes at byte offset off (positioned write, pwrite).
func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	n, err := w.file.WriteAt(p, off)
	if err != nil {
		return n, Explain(err)
	}
	return n, nil
}

// ReadAt reads len(p) bytes at byte offset off (positioned read, pread).
func (w *Writer) ReadAt(p []byte, off int64) (int, error) {
	n, err := w.file.ReadAt(p, off)
	if err != nil {
		return n, Explain(err)
	}
	return n, nil
}

// Sync forces every pending write out to the device (fsync).
func (w *Writer) Sync() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flushing %s: %w", w.disk.Path, Explain(err))
	}
	return nil
}

// DropCache discards the kernel's cached copy of the disk (ioctl BLKFLSBUF),
// so the next reads come from the device itself. Without it a read-back
// verification could just read our own data back from RAM.
func (w *Writer) DropCache() error { return DropCache(w.file) }

// DropCache does the same for any open disk device (root only).
func DropCache(f *os.File) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKFLSBUF, 0); errno != 0 {
		return fmt.Errorf("dropping the disk cache: %w", errno)
	}
	return nil
}

// Close flushes, asks the kernel to re-read the new partition table and
// closes the device. The first error is returned.
func (w *Writer) Close() error {
	if w.file == nil {
		return nil
	}
	first := w.Sync()
	// BLKRRPART: re-read the partition table, so /dev/sdX1 appears for the new ESP.
	unix.Syscall(unix.SYS_IOCTL, w.file.Fd(), unix.BLKRRPART, 0)
	if err := w.file.Close(); err != nil && first == nil {
		first = err
	}
	w.file = nil
	return first
}

// Explain turns common errno values into messages that say what to do.
// The original error stays wrapped, so errors.Is still works.
func Explain(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return fmt.Errorf("permission denied: run as root (%w)", err)
	case errors.Is(err, unix.EBUSY):
		return fmt.Errorf("the disk is in use (mounted or opened by another program) (%w)", err)
	case errors.Is(err, unix.EROFS):
		return fmt.Errorf("the disk is write-protected: check the lock switch of the drive (%w)", err)
	case errors.Is(err, unix.ENODEV), errors.Is(err, unix.ENXIO),
		errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENOMEDIUM):
		return fmt.Errorf("the disk was disconnected or is not ready (%w)", err)
	case errors.Is(err, unix.EIO):
		return fmt.Errorf("the device reported an I/O failure, the drive may be damaged (%w)", err)
	case errors.Is(err, unix.EINVAL):
		return fmt.Errorf("misaligned access (offset or size not a multiple of the sector size) (%w)", err)
	}
	return err
}
