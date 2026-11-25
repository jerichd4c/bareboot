//go:build windows

package disk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
)

// DeviceIoControl codes (winioctl.h). All of them only read information and
// are defined with FILE_ANY_ACCESS, so listing disks needs no administrator.
const (
	ioctlStorageQueryProperty       = 0x002D1400 // IOCTL_STORAGE_QUERY_PROPERTY
	ioctlDiskGetDriveGeometryEx     = 0x000700A0 // IOCTL_DISK_GET_DRIVE_GEOMETRY_EX
	ioctlVolumeGetVolumeDiskExtents = 0x00560000 // IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS

	maxDisks = 64 // PhysicalDrive numbers probed
)

// List returns every physical disk Windows exposes, ordered by number.
func List() ([]Disk, error) {
	volumes := volumesByDisk()
	systemDisks, err := systemDiskNumbers()
	if err != nil {
		// Without knowing which disk holds Windows no disk can be called safe.
		return nil, err
	}

	var disks []Disk
	for n := 0; n < maxDisks; n++ {
		d, err := query(n)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue // numbers can have gaps (a disk was unplugged)
		}
		if err != nil {
			return nil, fmt.Errorf("disk %d: %w", n, err)
		}
		d.Volumes = volumes[n]
		d.System = systemDisks[n]
		disks = append(disks, d)
	}
	return disks, nil
}

// Get returns fresh information about one disk. It is called again right
// before writing, so a disk swapped after listing is detected.
func Get(number int) (Disk, error) {
	disks, err := List()
	if err != nil {
		return Disk{}, err
	}
	for _, d := range disks {
		if d.Number == number {
			return d, nil
		}
	}
	return Disk{}, fmt.Errorf("disk %d not found (unplugged or wrong number)", number)
}

// query opens \\.\PhysicalDriveN without read/write access (metadata only)
// and asks the storage driver for its identity and geometry.
func query(n int) (Disk, error) {
	d := Disk{Number: n, Path: fmt.Sprintf(`\\.\PhysicalDrive%d`, n)}
	h, err := openDevice(d.Path)
	if err != nil {
		return d, err
	}
	defer windows.CloseHandle(h)

	if err := d.readDescriptor(h); err != nil {
		return d, fmt.Errorf("reading device descriptor: %w", err)
	}
	if err := d.readGeometry(h); err != nil {
		// Card readers without a card fail here: report "no media", not an error.
		if !errors.Is(err, windows.ERROR_NOT_READY) && !errors.Is(err, windows.ERROR_NO_MEDIA_IN_DRIVE) {
			return d, fmt.Errorf("reading geometry: %w", err)
		}
	}
	return d, nil
}

// openDevice opens a disk or volume device with access 0: enough for
// information IOCTLs, and it never blocks other programs using the device.
func openDevice(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(p, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, 0, 0)
}

func ioctl(h windows.Handle, code uint32, in []byte, out []byte) (uint32, error) {
	var returned uint32
	var inPtr *byte
	if len(in) > 0 {
		inPtr = &in[0]
	}
	err := windows.DeviceIoControl(h, code, inPtr, uint32(len(in)),
		&out[0], uint32(len(out)), &returned, nil)
	return returned, err
}

// readDescriptor fills vendor, product, serial, bus and removable flag from
// STORAGE_DEVICE_DESCRIPTOR.
func (d *Disk) readDescriptor(h windows.Handle) error {
	// STORAGE_PROPERTY_QUERY{PropertyId: StorageDeviceProperty (0),
	// QueryType: PropertyStandardQuery (0)}: 12 bytes, all zero.
	query := make([]byte, 12)
	buf := make([]byte, 1024)
	n, err := ioctl(h, ioctlStorageQueryProperty, query, buf)
	if err != nil {
		return err
	}
	buf = buf[:n]
	if len(buf) < 36 {
		return errors.New("descriptor too short")
	}
	// Layout: Version, Size (u32) | DeviceType, Modifier, RemovableMedia,
	// CommandQueueing (u8) | Vendor/Product/Revision/Serial offsets (u32) | BusType (u32)
	d.Removable = buf[10] != 0
	d.Vendor = cString(buf, binary.LittleEndian.Uint32(buf[12:16]))
	d.Product = cString(buf, binary.LittleEndian.Uint32(buf[16:20]))
	d.Serial = cString(buf, binary.LittleEndian.Uint32(buf[24:28]))
	d.Bus = BusType(binary.LittleEndian.Uint32(buf[28:32]))
	return nil
}

// readGeometry fills size and sector size from DISK_GEOMETRY_EX.
func (d *Disk) readGeometry(h windows.Handle) error {
	buf := make([]byte, 256)
	if _, err := ioctl(h, ioctlDiskGetDriveGeometryEx, nil, buf); err != nil {
		return err
	}
	// DISK_GEOMETRY: Cylinders (i64), MediaType, TracksPerCylinder,
	// SectorsPerTrack, BytesPerSector (u32) = 24 bytes; then DiskSize (i64).
	d.SectorSize = binary.LittleEndian.Uint32(buf[20:24])
	d.Size = int64(binary.LittleEndian.Uint64(buf[24:32]))
	return nil
}

// cString reads a NUL-terminated ASCII string at offset off (0 = absent).
func cString(buf []byte, off uint32) string {
	if off == 0 || int(off) >= len(buf) {
		return ""
	}
	s := buf[off:]
	if i := strings.IndexByte(string(s), 0); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(string(s))
}

// volumesByDisk maps each disk number to the drive letters of its volumes,
// asking every mounted volume which disk(s) it lives on.
func volumesByDisk() map[int][]string {
	result := map[int][]string{}
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return result
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		letter := fmt.Sprintf("%c:", 'A'+i)
		for _, n := range volumeDisks(letter) {
			result[n] = append(result[n], letter)
		}
	}
	for n := range result {
		sort.Strings(result[n])
	}
	return result
}

// volumeDisks returns the disk numbers a volume ("C:") is stored on.
// Optical drives, network shares and empty readers just return nothing.
func volumeDisks(letter string) []int {
	h, err := openDevice(`\\.\` + letter)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

	buf := make([]byte, 8+24*16) // header + up to 16 extents
	if _, err := ioctl(h, ioctlVolumeGetVolumeDiskExtents, nil, buf); err != nil {
		return nil
	}
	// VOLUME_DISK_EXTENTS: NumberOfDiskExtents (u32) + padding, then
	// DISK_EXTENT{DiskNumber (u32) + padding, StartingOffset, ExtentLength (i64)}.
	count := int(binary.LittleEndian.Uint32(buf[0:4]))
	var disks []int
	for i := 0; i < count && 8+24*(i+1) <= len(buf); i++ {
		disks = append(disks, int(binary.LittleEndian.Uint32(buf[8+24*i:])))
	}
	return disks
}

// systemDiskNumbers returns the disks that hold the running Windows
// installation (the volume of %SystemDrive%, normally C:).
func systemDiskNumbers() (map[int]bool, error) {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	numbers := volumeDisks(drive)
	if len(numbers) == 0 {
		return nil, fmt.Errorf("cannot determine which disk holds the system drive %s", drive)
	}
	result := map[int]bool{}
	for _, n := range numbers {
		result[n] = true
	}
	return result, nil
}
