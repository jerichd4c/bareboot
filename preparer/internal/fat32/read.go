package fat32

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ReadFile opens the FAT32 volume in r (offset 0 = first sector of the
// partition), checks its structure and returns the contents of the file at
// path ("EFI/BOOT/BOOTX64.EFI"). It follows the same steps a firmware does
// to load the boot program: boot sector -> FAT -> directories -> cluster chain.
//
// It is written independently of Write, so it can catch mistakes in Write.
func ReadFile(r io.ReaderAt, path string) ([]byte, error) {
	v, err := openVolume(r)
	if err != nil {
		return nil, err
	}
	parts := strings.FieldsFunc(path, func(c rune) bool { return c == '/' || c == '\\' })
	if len(parts) == 0 {
		return nil, errors.New("fat32: empty path")
	}

	dir := v.rootCluster
	for i, part := range parts {
		name, err := shortName(part)
		if err != nil {
			return nil, err
		}
		data, err := v.readChain(dir, 0)
		if err != nil {
			return nil, fmt.Errorf("reading directory of %q: %w", part, err)
		}
		e, ok := findEntry(data, name)
		if !ok {
			return nil, fmt.Errorf("fat32: %q not found", strings.Join(parts[:i+1], "/"))
		}
		last := i == len(parts)-1
		if !last {
			if e.attr&attrDir == 0 {
				return nil, fmt.Errorf("fat32: %q is not a directory", part)
			}
			dir = e.cluster
			continue
		}
		if e.attr&attrDir != 0 {
			return nil, fmt.Errorf("fat32: %q is a directory", part)
		}
		return v.readChain(e.cluster, e.size)
	}
	return nil, errors.New("unreachable")
}

type volume struct {
	r           io.ReaderAt
	rootCluster uint32
	dataStart   uint32 // first sector of cluster 2
	clusters    uint32
	fat         []uint32
}

// openVolume validates the boot sector and loads the FAT, checking that
// both copies of the FAT are identical.
func openVolume(r io.ReaderAt) (*volume, error) {
	b := make([]byte, SectorSize)
	if _, err := r.ReadAt(b, 0); err != nil {
		return nil, err
	}
	u16 := func(off int) uint32 { return uint32(binary.LittleEndian.Uint16(b[off:])) }
	u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

	switch {
	case b[510] != 0x55 || b[511] != 0xAA:
		return nil, errors.New("fat32: boot sector has no 55AA signature")
	case b[0] != 0xEB && b[0] != 0xE9:
		return nil, errors.New("fat32: boot sector does not start with a jump instruction")
	case u16(11) != SectorSize:
		return nil, fmt.Errorf("fat32: %d bytes per sector, expected %d", u16(11), SectorSize)
	case b[13] != sectorsPerCluster:
		return nil, fmt.Errorf("fat32: %d sectors per cluster, expected %d", b[13], sectorsPerCluster)
	case u16(17) != 0 || u16(22) != 0 || u32(36) == 0:
		return nil, errors.New("fat32: BPB fields are not those of a FAT32 volume")
	}

	reserved, numFATs := u16(14), uint32(b[16])
	total, fatSectors := u32(32), u32(36)
	v := &volume{
		r:           r,
		rootCluster: u32(44),
		dataStart:   reserved + numFATs*fatSectors,
	}
	v.clusters = (total - v.dataStart) / sectorsPerCluster
	if v.clusters < minClusters {
		return nil, fmt.Errorf("fat32: only %d clusters, a FAT32 volume needs %d", v.clusters, minClusters)
	}

	// Load FAT #1 and compare it with every other copy.
	fatBytes := make([]byte, fatSectors*SectorSize)
	if _, err := r.ReadAt(fatBytes, int64(reserved)*SectorSize); err != nil {
		return nil, fmt.Errorf("reading FAT: %w", err)
	}
	other := make([]byte, len(fatBytes))
	for i := uint32(1); i < numFATs; i++ {
		if _, err := r.ReadAt(other, int64(reserved+i*fatSectors)*SectorSize); err != nil {
			return nil, fmt.Errorf("reading FAT #%d: %w", i+1, err)
		}
		if !bytes.Equal(fatBytes, other) {
			return nil, fmt.Errorf("fat32: FAT #%d differs from FAT #1", i+1)
		}
	}
	v.fat = make([]uint32, v.clusters+2)
	for i := range v.fat {
		v.fat[i] = binary.LittleEndian.Uint32(fatBytes[i*4:]) & 0x0FFFFFFF // top 4 bits are reserved
	}
	return v, nil
}

// readChain reads the clusters of a chain starting at first. If size > 0 the
// result is cut to size bytes (files); directories pass 0 and get all clusters.
func (v *volume) readChain(first, size uint32) ([]byte, error) {
	if size == 0 && first == 0 {
		return nil, nil // empty file
	}
	var out []byte
	cluster := make([]byte, clusterSize)
	for c, steps := first, uint32(0); ; steps++ {
		if c < rootCluster || c >= v.clusters+2 {
			return nil, fmt.Errorf("fat32: chain points to invalid cluster %d", c)
		}
		if steps > v.clusters {
			return nil, errors.New("fat32: cluster chain has a loop")
		}
		off := int64(v.dataStart+(c-rootCluster)*sectorsPerCluster) * SectorSize
		if _, err := v.r.ReadAt(cluster, off); err != nil {
			return nil, err
		}
		out = append(out, cluster...)
		next := v.fat[c]
		if next >= 0x0FFFFFF8 {
			break // end of chain
		}
		c = next
	}
	if size > 0 {
		if uint32(len(out)) < size {
			return nil, fmt.Errorf("fat32: chain holds %d bytes, file size is %d", len(out), size)
		}
		out = out[:size]
	}
	return out, nil
}

type entry struct {
	attr    byte
	cluster uint32
	size    uint32
}

// findEntry looks for an 8.3 name in the raw contents of a directory.
func findEntry(dir []byte, name [11]byte) (entry, bool) {
	for off := 0; off+dirEntrySize <= len(dir); off += dirEntrySize {
		e := dir[off : off+dirEntrySize]
		switch {
		case e[0] == 0x00:
			return entry{}, false // no more entries
		case e[0] == 0xE5, e[11] == 0x0F, e[11]&attrVolumeID != 0:
			continue // deleted entry, long-name fragment or volume label
		}
		if bytes.Equal(e[0:11], name[:]) {
			return entry{
				attr:    e[11],
				cluster: uint32(binary.LittleEndian.Uint16(e[20:]))<<16 | uint32(binary.LittleEndian.Uint16(e[26:])),
				size:    binary.LittleEndian.Uint32(e[28:]),
			}, true
		}
	}
	return entry{}, false
}
