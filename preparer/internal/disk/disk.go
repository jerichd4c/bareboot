// Package disk finds the physical disks of the machine, describes them and
// decides whether one is a safe target for bareboot.
//
// The OS-specific part (asking Windows about each disk) lives in
// disk_windows.go; this file only holds the data and the safety rules.
package disk

import (
	"fmt"
	"strings"
)

// BusType is how the disk is connected (STORAGE_BUS_TYPE in the Windows SDK).
type BusType uint32

const BusUSB BusType = 7

var busNames = map[BusType]string{
	0: "Unknown", 1: "SCSI", 2: "ATAPI", 3: "ATA", 4: "1394", 5: "SSA",
	6: "Fibre", 7: "USB", 8: "RAID", 9: "iSCSI", 10: "SAS", 11: "SATA",
	12: "SD", 13: "MMC", 14: "Virtual", 15: "FileVirtual", 16: "Spaces",
	17: "NVMe", 18: "SCM", 19: "UFS",
}

func (b BusType) String() string {
	if name, ok := busNames[b]; ok {
		return name
	}
	return fmt.Sprintf("Bus%d", uint32(b))
}

// Disk is everything we know about one physical disk.
type Disk struct {
	Number     int    // N in \\.\PhysicalDriveN
	Path       string // device path used to open the disk
	Vendor     string // as reported by the device itself
	Product    string
	Serial     string
	Bus        BusType
	Removable  bool     // removable media flag (many USB disks report false)
	Size       int64    // bytes; 0 when there is no media (empty card reader)
	SectorSize uint32   // logical sector size in bytes
	Volumes    []string // drive letters of volumes on this disk, e.g. "E:"
	System     bool     // holds the running Windows installation
}

// Model returns vendor and product joined, or "" if the device reported neither.
func (d Disk) Model() string {
	return strings.TrimSpace(d.Vendor + " " + d.Product)
}

// Problems lists every reason why d must not be written. An empty result
// means the disk is an eligible target. minSize is the smallest usable disk.
func (d Disk) Problems(minSize int64, sectorSize uint32) []string {
	var p []string
	if d.System {
		p = append(p, "system disk (holds Windows)")
	}
	if d.Bus != BusUSB {
		p = append(p, fmt.Sprintf("not a USB disk (bus: %s)", d.Bus))
	}
	if d.Model() == "" && d.Serial == "" {
		p = append(p, "identity unknown (no model or serial number)")
	}
	switch {
	case d.Size == 0:
		p = append(p, "no media")
	case d.Size < minSize:
		p = append(p, fmt.Sprintf("too small (%s, need %s)", FormatSize(d.Size), FormatSize(minSize)))
	}
	if d.SectorSize != 0 && d.SectorSize != sectorSize {
		p = append(p, fmt.Sprintf("unsupported sector size %d (need %d)", d.SectorSize, sectorSize))
	}
	return p
}

// FormatSize prints a byte count in binary units: 14.4 GiB, 512 MiB...
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
