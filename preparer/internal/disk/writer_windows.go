//go:build windows

package disk

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Control codes used while writing (winioctl.h).
const (
	fsctlLockVolume           = 0x00090018 // FSCTL_LOCK_VOLUME
	fsctlDismountVolume       = 0x00090020 // FSCTL_DISMOUNT_VOLUME
	ioctlDiskUpdateProperties = 0x00070140 // IOCTL_DISK_UPDATE_PROPERTIES
)

// Writer gives raw access to a whole physical disk. While it is open, every
// volume of that disk is locked and dismounted, so Windows (and any program)
// cannot touch the disk at the same time.
type Writer struct {
	disk    Disk
	file    *os.File         // \\.\PhysicalDriveN opened for read/write
	volumes []windows.Handle // locked volume handles, released on Close
}

// ElevationHint tells the user how to get the rights IsElevated checks for.
const ElevationHint = `administrator rights required: open PowerShell with "Run as administrator" and try again`

// IsElevated reports whether the process runs as administrator. Raw disk
// writes are only allowed to administrators.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// OpenForWrite locks and dismounts the volumes of d and opens the disk for
// raw reading and writing.
func OpenForWrite(d Disk) (*Writer, error) {
	w := &Writer{disk: d}

	// 1. Volumes first: since Windows Vista, writing sectors that belong to a
	//    mounted volume fails with "access denied" unless it is locked.
	for _, letter := range d.Volumes {
		h, err := lockAndDismount(letter)
		if err != nil {
			w.releaseVolumes()
			return nil, err
		}
		w.volumes = append(w.volumes, h)
	}

	// 2. The disk itself. WRITE_THROUGH asks the device to store each write
	//    before reporting success instead of keeping it in a cache.
	p, err := windows.UTF16PtrFromString(d.Path)
	if err != nil {
		w.releaseVolumes()
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		w.releaseVolumes()
		return nil, fmt.Errorf("opening %s: %w", d.Path, Explain(err))
	}
	// os.File gives us WriteAt/ReadAt (positioned I/O) and Sync (FlushFileBuffers).
	w.file = os.NewFile(uintptr(h), d.Path)
	return w, nil
}

// lockAndDismount gets exclusive use of a volume ("E:") and detaches its
// file system, so no cached data is written back over our new structures.
func lockAndDismount(letter string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(`\\.\` + letter)
	if err != nil {
		return windows.InvalidHandle, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("opening volume %s: %w", letter, Explain(err))
	}

	var returned uint32
	// Locking fails while a program has files open on the volume (an Explorer
	// window, an editor...). Retry a few times before giving up.
	for attempt := 1; ; attempt++ {
		err = windows.DeviceIoControl(h, fsctlLockVolume, nil, 0, nil, 0, &returned, nil)
		if err == nil || attempt == 5 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		windows.CloseHandle(h)
		return windows.InvalidHandle, fmt.Errorf(
			"cannot lock volume %s: close every window or program using it and try again (%w)", letter, err)
	}
	if err := windows.DeviceIoControl(h, fsctlDismountVolume, nil, 0, nil, 0, &returned, nil); err != nil {
		windows.CloseHandle(h)
		return windows.InvalidHandle, fmt.Errorf("cannot dismount volume %s: %w", letter, err)
	}
	return h, nil
}

// WriteAt writes len(p) bytes at byte offset off. Offsets and lengths must be
// multiples of the sector size: raw disks reject anything else.
func (w *Writer) WriteAt(p []byte, off int64) (int, error) {
	n, err := w.file.WriteAt(p, off)
	if err != nil {
		return n, Explain(err)
	}
	return n, nil
}

// ReadAt reads len(p) bytes at byte offset off (same alignment rules).
func (w *Writer) ReadAt(p []byte, off int64) (int, error) {
	n, err := w.file.ReadAt(p, off)
	if err != nil {
		return n, Explain(err)
	}
	return n, nil
}

// Sync forces every pending write out to the device (FlushFileBuffers).
func (w *Writer) Sync() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flushing %s: %w", w.disk.Path, Explain(err))
	}
	return nil
}

// DropCache exists for symmetry with Linux. Windows does not keep raw disk
// I/O (\\.\PhysicalDriveN) in the file cache, so reads already come from the
// device after Sync; there is nothing to discard.
func (w *Writer) DropCache() error { return DropCache(w.file) }

// DropCache does the same for any open disk device: nothing on Windows.
func DropCache(f *os.File) error { return nil }

// Close flushes, asks Windows to re-read the new partition table, closes the
// disk and finally unlocks the volumes. The first error is returned.
func (w *Writer) Close() error {
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if w.file != nil {
		keep(w.Sync())
		var returned uint32
		// Without this Windows keeps showing the old partitions until replug.
		windows.DeviceIoControl(windows.Handle(w.file.Fd()), ioctlDiskUpdateProperties,
			nil, 0, nil, 0, &returned, nil)
		keep(w.file.Close())
		w.file = nil
	}
	w.releaseVolumes()
	return first
}

func (w *Writer) releaseVolumes() {
	for _, h := range w.volumes {
		windows.CloseHandle(h) // closing the handle also releases the lock
	}
	w.volumes = nil
}

// Explain turns common Windows errors into messages that say what to do.
// The original error stays wrapped, so errors.Is still works.
func Explain(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("access denied: run as administrator and close programs using the disk (%w)", err)
	case errors.Is(err, windows.ERROR_WRITE_PROTECT):
		return fmt.Errorf("the disk is write-protected: check the lock switch of the drive (%w)", err)
	case errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED),
		errors.Is(err, windows.ERROR_NO_SUCH_DEVICE),
		errors.Is(err, windows.ERROR_FILE_NOT_FOUND),
		errors.Is(err, windows.ERROR_NOT_READY),
		errors.Is(err, windows.ERROR_NO_MEDIA_IN_DRIVE):
		return fmt.Errorf("the disk was disconnected or is not ready (%w)", err)
	case errors.Is(err, windows.ERROR_CRC),
		errors.Is(err, windows.ERROR_SECTOR_NOT_FOUND),
		errors.Is(err, windows.ERROR_IO_DEVICE),
		errors.Is(err, windows.ERROR_GEN_FAILURE):
		return fmt.Errorf("the device reported an I/O failure, the drive may be damaged (%w)", err)
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return fmt.Errorf("misaligned access (offset or size not a multiple of the sector size) (%w)", err)
	}
	return err
}
