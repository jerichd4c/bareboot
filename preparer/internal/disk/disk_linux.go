//go:build linux

package disk

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// On Linux the kernel publishes every block device under /sys (sysfs) as
// plain text files, so listing disks is reading files, not calling ioctls:
//
//	/sys/block/sde/size                       size in 512-byte units
//	/sys/block/sde/queue/logical_block_size   sector size
//	/sys/block/sde/device/vendor, model       identity reported by the device
//	/sys/block/sde -> ../devices/.../usb2/... the real path tells the bus

// List returns every physical disk the kernel exposes, ordered by name.
func List() ([]Disk, error) {
	mounts, err := mountsByDisk()
	if err != nil {
		return nil, err
	}
	systemDisks, err := systemDiskNames(mounts)
	if err != nil {
		// Without knowing which disk holds the OS no disk can be called safe.
		return nil, err
	}

	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, fmt.Errorf("reading /sys/block: %w", err)
	}
	var disks []Disk
	for _, e := range entries {
		name := e.Name()
		// Only real devices have a "device" link; this skips loop, ram, zram, dm-*.
		if _, err := os.Stat(filepath.Join("/sys/block", name, "device")); err != nil {
			continue
		}
		d := query(name)
		d.Volumes = mounts[name].points
		d.System = systemDisks[name]
		disks = append(disks, d)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].ID < disks[j].ID })
	return disks, nil
}

func query(name string) Disk {
	sys := filepath.Join("/sys/block", name)
	d := Disk{ID: name, Path: "/dev/" + name}

	// "size" is always counted in 512-byte units, whatever the real sector size.
	if sectors, err := readInt(filepath.Join(sys, "size")); err == nil {
		d.Size = sectors * 512
	}
	if ss, err := readInt(filepath.Join(sys, "queue", "logical_block_size")); err == nil {
		d.SectorSize = uint32(ss)
	}
	d.Vendor = readText(filepath.Join(sys, "device", "vendor"))
	d.Product = readText(filepath.Join(sys, "device", "model"))
	d.Removable = readText(filepath.Join(sys, "removable")) == "1"

	real, _ := filepath.EvalSymlinks(sys)
	d.Bus = busFromPath(real)
	if d.Bus == BusUSB {
		d.Serial = usbSerial(real)
	}
	return d
}

// busFromPath deduces the bus from the device's position in the hardware
// tree, e.g. /sys/devices/pci0000:00/.../usb2/2-1/.../block/sde -> USB.
func busFromPath(p string) BusType {
	switch {
	case strings.Contains(p, "/usb"):
		return BusUSB
	case strings.Contains(p, "/nvme"):
		return BusNVMe
	case strings.Contains(p, "/ata"):
		return BusSATA
	case strings.Contains(p, "/virtio"), strings.Contains(p, "VMBUS"):
		return BusVirtual // virtual disks of a VM, e.g. the disks of WSL itself
	case strings.Contains(p, "/host"):
		return BusSCSI
	}
	return BusUnknown
}

// usbSerial walks up from the block device to the USB device node (the first
// parent with an "idVendor" file) and reads its serial number. It must stop
// there: further up is the USB host controller, whose "serial" file holds the
// controller's name, not the drive's. Many cheap drives have no serial at all.
func usbSerial(p string) string {
	for dir := p; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "idVendor")); err == nil {
			return readText(filepath.Join(dir, "serial"))
		}
	}
	return ""
}

// mountInfo is what is mounted from one disk (from any of its partitions).
type mountInfo struct {
	points []string // mount points, e.g. /media/usb
}

// mountsByDisk reads /proc/self/mountinfo and groups mount points by the
// whole disk they come from (a partition sde1 belongs to disk sde).
func mountsByDisk() (map[string]mountInfo, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, fmt.Errorf("reading mount table: %w", err)
	}
	defer f.Close()

	result := map[string]mountInfo{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// Fields: id parent major:minor root mountpoint ...
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		diskName := diskOfDevice(fields[2])
		if diskName == "" {
			continue // not a block device (proc, tmpfs, network...)
		}
		m := result[diskName]
		m.points = append(m.points, unescapeMount(fields[4]))
		result[diskName] = m
	}
	return result, sc.Err()
}

// diskOfDevice turns "8:65" (major:minor) into the whole-disk name "sde".
// /sys/dev/block/8:65 links to .../block/sde/sde1; a partition has a
// "partition" file and its parent directory is the disk.
func diskOfDevice(majMin string) string {
	real, err := filepath.EvalSymlinks(filepath.Join("/sys/dev/block", majMin))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(real, "partition")); err == nil {
		return filepath.Base(filepath.Dir(real))
	}
	return filepath.Base(real)
}

// systemDiskNames returns the disks holding the root file system "/".
func systemDiskNames(mounts map[string]mountInfo) (map[string]bool, error) {
	result := map[string]bool{}
	for name, m := range mounts {
		for _, p := range m.points {
			if p == "/" {
				result[name] = true
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("cannot determine which disk holds the root file system")
	}
	return result, nil
}

// unescapeMount decodes the octal escapes mountinfo uses (\040 is a space).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readInt(path string) (int64, error) {
	return strconv.ParseInt(readText(path), 10, 64)
}
