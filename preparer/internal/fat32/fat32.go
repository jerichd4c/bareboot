// Package fat32 builds a FAT32 file system from an in-memory directory tree
// and writes it to a partition (Microsoft "FAT: General Overview of On-Disk
// Format"). Only what a boot partition needs is supported: 8.3 names, no
// long file names, everything written in a single pass.
package fat32

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	SectorSize        = 512
	sectorsPerCluster = 1
	clusterSize       = SectorSize * sectorsPerCluster
	reservedSectors   = 32 // boot sector, FSInfo, backups; 32 is the standard for FAT32
	numFATs           = 2  // the FAT is stored twice for redundancy
	fsInfoSector      = 1
	backupBootSector  = 6
	rootCluster       = 2     // first data cluster; clusters 0 and 1 don't exist
	minClusters       = 65525 // fewer clusters would make it FAT16 by definition
	endOfChain        = 0x0FFFFFFF
	mediaFixed        = 0xF8
)

// Options describes the partition the file system is created in.
type Options struct {
	TotalSectors  uint32    // partition size in sectors
	HiddenSectors uint32    // LBA where the partition starts on the disk
	Label         string    // volume label, up to 11 characters
	VolumeID      uint32    // serial number shown by "dir" on Windows
	Time          time.Time // timestamp for every file and directory
}

// FS is a file system being built in memory.
type FS struct {
	opts Options
	root *Node
}

// New starts an empty file system.
func New(opts Options) *FS {
	return &FS{opts: opts, root: &Node{isDir: true}}
}

// Root returns the root directory.
func (fs *FS) Root() *Node { return fs.root }

// geometry is where each region of the volume starts.
type geometry struct {
	fatSectors uint32 // size of one FAT
	clusters   uint32 // number of data clusters
	dataStart  uint32 // first sector of cluster 2
}

func computeGeometry(total uint32) (geometry, error) {
	// The FAT size depends on the number of clusters, and the number of
	// clusters depends on how much space the FATs take: iterate until stable.
	fatSectors := uint32(1)
	for {
		clusters := (total - reservedSectors - numFATs*fatSectors) / sectorsPerCluster
		need := ceilDiv((clusters+2)*4, SectorSize) // 4 bytes per entry, +2 reserved entries
		if need <= fatSectors {
			break
		}
		fatSectors = need
	}
	g := geometry{
		fatSectors: fatSectors,
		dataStart:  reservedSectors + numFATs*fatSectors,
	}
	g.clusters = (total - g.dataStart) / sectorsPerCluster
	if g.clusters < minClusters {
		return g, fmt.Errorf("fat32: partition too small (%d clusters, need %d)", g.clusters, minClusters)
	}
	return g, nil
}

// Write formats the partition and stores every file and directory.
// w must address the partition, so offset 0 is its first sector.
func (fs *FS) Write(w io.WriterAt) error {
	g, err := computeGeometry(fs.opts.TotalSectors)
	if err != nil {
		return err
	}

	// 1. Assign clusters to every node and build the allocation table.
	fat := make([]uint32, g.clusters+2)
	fat[0] = 0x0FFFFF00 | mediaFixed // entry 0 repeats the media byte
	fat[1] = endOfChain              // entry 1 is reserved
	next := uint32(rootCluster)
	var nodes []*Node
	var allocErr error
	fs.root.walk(func(n *Node) {
		count := ceilDiv(uint32(len(n.content(fs))), clusterSize)
		if n.isDir && count == 0 {
			count = 1 // a directory always owns at least one cluster
		}
		if count == 0 {
			return // empty file: first cluster stays 0
		}
		if next+count > g.clusters+2 {
			allocErr = fmt.Errorf("fat32: not enough space for %q", n.Name())
			return
		}
		n.first = next
		for c := next; c < next+count-1; c++ {
			fat[c] = c + 1 // each cluster points to the next one
		}
		fat[next+count-1] = endOfChain
		next += count
		nodes = append(nodes, n)
	})
	if allocErr != nil {
		return allocErr
	}

	// 2. Data first, metadata last: if writing stops halfway the volume has
	//    no valid boot sector instead of one pointing at garbage.
	for _, n := range nodes {
		data := n.content(fs) // directories are serialized now that clusters are known
		padded := make([]byte, ceilDiv(uint32(len(data)), clusterSize)*clusterSize)
		copy(padded, data) // the rest stays zero, so old disk contents never leak in
		off := int64(g.dataStart+(n.first-rootCluster)*sectorsPerCluster) * SectorSize
		if _, err := w.WriteAt(padded, off); err != nil {
			return fmt.Errorf("writing %q: %w", n.Name(), err)
		}
	}

	fatBytes := make([]byte, g.fatSectors*SectorSize)
	for i, v := range fat {
		binary.LittleEndian.PutUint32(fatBytes[i*4:], v)
	}
	for i := uint32(0); i < numFATs; i++ {
		off := int64(reservedSectors+i*g.fatSectors) * SectorSize
		if _, err := w.WriteAt(fatBytes, off); err != nil {
			return fmt.Errorf("writing FAT #%d: %w", i+1, err)
		}
	}

	freeClusters := g.clusters - (next - rootCluster)
	boot := fs.bootSector(g)
	info := fsInfo(freeClusters, next)
	for _, s := range []struct {
		sector uint32
		data   []byte
		what   string
	}{
		{backupBootSector + 1, info, "backup FSInfo"},
		{backupBootSector, boot, "backup boot sector"},
		{fsInfoSector, info, "FSInfo"},
		{0, boot, "boot sector"},
	} {
		if _, err := w.WriteAt(s.data, int64(s.sector)*SectorSize); err != nil {
			return fmt.Errorf("writing %s: %w", s.what, err)
		}
	}
	return nil
}

// bootSector builds sector 0 of the volume: the BIOS Parameter Block (BPB)
// that describes the layout of the file system.
func (fs *FS) bootSector(g geometry) []byte {
	b := make([]byte, SectorSize)
	b[0], b[1], b[2] = 0xEB, 0x58, 0x90 // x86 jump over the BPB (required signature)
	copy(b[3:11], "BAREBOOT")           // OEM name
	binary.LittleEndian.PutUint16(b[11:13], SectorSize)
	b[13] = sectorsPerCluster
	binary.LittleEndian.PutUint16(b[14:16], reservedSectors)
	b[16] = numFATs
	// b[17:19] root entries and b[19:21] 16-bit total sectors: 0 on FAT32
	b[21] = mediaFixed
	// b[22:24] 16-bit FAT size: 0 on FAT32
	binary.LittleEndian.PutUint16(b[24:26], 63)  // sectors per track (legacy CHS)
	binary.LittleEndian.PutUint16(b[26:28], 255) // number of heads (legacy CHS)
	binary.LittleEndian.PutUint32(b[28:32], fs.opts.HiddenSectors)
	binary.LittleEndian.PutUint32(b[32:36], fs.opts.TotalSectors)

	// FAT32 extended BPB
	binary.LittleEndian.PutUint32(b[36:40], g.fatSectors)
	// b[40:42] flags (both FATs mirrored), b[42:44] version 0.0
	binary.LittleEndian.PutUint32(b[44:48], rootCluster)
	binary.LittleEndian.PutUint16(b[48:50], fsInfoSector)
	binary.LittleEndian.PutUint16(b[50:52], backupBootSector)
	b[64] = 0x80 // drive number
	b[66] = 0x29 // extended boot signature: the next three fields are valid
	binary.LittleEndian.PutUint32(b[67:71], fs.volumeID())
	label := fs.labelName()
	copy(b[71:82], label[:])
	copy(b[82:90], "FAT32   ") // informative only, never used to detect the type

	b[510], b[511] = 0x55, 0xAA
	return b
}

// fsInfo builds the FSInfo sector: a hint of free space so the OS doesn't
// have to scan the whole FAT.
func fsInfo(freeClusters, nextFree uint32) []byte {
	b := make([]byte, SectorSize)
	binary.LittleEndian.PutUint32(b[0:4], 0x41615252)     // "RRaA" lead signature
	binary.LittleEndian.PutUint32(b[484:488], 0x61417272) // "rrAa" structure signature
	binary.LittleEndian.PutUint32(b[488:492], freeClusters)
	binary.LittleEndian.PutUint32(b[492:496], nextFree)
	binary.LittleEndian.PutUint32(b[508:512], 0xAA550000) // trail signature
	return b
}

func (fs *FS) volumeID() uint32 {
	if fs.opts.VolumeID != 0 {
		return fs.opts.VolumeID
	}
	return uint32(fs.opts.Time.Unix())
}

// labelName returns the label padded to 11 characters ("NO NAME" if empty).
func (fs *FS) labelName() [11]byte {
	var l [11]byte
	copy(l[:], "NO NAME    ")
	if fs.opts.Label != "" {
		copy(l[:], fmt.Sprintf("%-11.11s", fs.opts.Label))
	}
	return l
}

func ceilDiv(a, b uint32) uint32 { return (a + b - 1) / b }
