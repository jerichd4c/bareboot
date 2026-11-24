// Package gpt writes a GUID Partition Table (UEFI Specification, chapter 5):
// a protective MBR, a primary header and entry array at the start of the
// disk, and a backup copy of both at the end.
package gpt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"unicode/utf16"
)

const (
	SectorSize = 512

	headerSize    = 92  // bytes actually used in the header sector
	entrySize     = 128 // bytes per partition entry
	numEntries    = 128 // standard size of the entry array
	entriesBytes  = entrySize * numEntries
	entrySectors  = entriesBytes / SectorSize // 32
	maxNameChars  = 36                        // UTF-16 code units in an entry name
	revision1_0   = 0x00010000
	signature     = "EFI PART"
	mbrTypeGPT    = 0xEE             // partition type of the protective MBR entry
	firstUsable   = 2 + entrySectors // LBA 0 MBR, LBA 1 header, LBA 2-33 entries
	backupReserve = entrySectors + 1 // backup entries + backup header at the end
)

// ESPType is the partition type GUID of an EFI System Partition.
var ESPType = MustParseGUID("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

// Partition describes one entry of the table. LBAs are inclusive.
type Partition struct {
	Type     GUID
	ID       GUID
	FirstLBA uint64
	LastLBA  uint64
	Name     string
}

// Table is a complete GPT for a disk of DiskSectors sectors.
type Table struct {
	DiskSectors uint64
	DiskID      GUID
	Partitions  []Partition
}

// FirstUsableLBA is the first sector a partition may use.
func FirstUsableLBA() uint64 { return firstUsable }

// LastUsableLBA is the last sector a partition may use on a disk of the given size.
func LastUsableLBA(diskSectors uint64) uint64 { return diskSectors - 1 - backupReserve }

// Write stores the protective MBR, the primary GPT and the backup GPT in w.
// w is usually an image file or a physical disk opened for writing.
func (t *Table) Write(w io.WriterAt) error {
	if err := t.validate(); err != nil {
		return err
	}
	lastLBA := t.DiskSectors - 1

	entries := t.entryArray()
	entriesCRC := crc32.ChecksumIEEE(entries)

	primary := t.header(1, lastLBA, 2, entriesCRC)
	backup := t.header(lastLBA, 1, lastLBA-entrySectors, entriesCRC)

	// Order matters little for an image, but for a real disk we write the
	// backup first and the primary last, so a half-written table is invalid.
	writes := []struct {
		lba  uint64
		data []byte
		what string
	}{
		{lastLBA - entrySectors, entries, "backup partition entries"},
		{lastLBA, backup, "backup GPT header"},
		{2, entries, "primary partition entries"},
		{1, primary, "primary GPT header"},
		{0, t.protectiveMBR(), "protective MBR"},
	}
	for _, wr := range writes {
		if _, err := w.WriteAt(wr.data, int64(wr.lba)*SectorSize); err != nil {
			return fmt.Errorf("writing %s: %w", wr.what, err)
		}
	}
	return nil
}

func (t *Table) validate() error {
	if len(t.Partitions) == 0 || len(t.Partitions) > numEntries {
		return errors.New("gpt: invalid number of partitions")
	}
	last := LastUsableLBA(t.DiskSectors)
	if t.DiskSectors <= firstUsable+backupReserve {
		return errors.New("gpt: disk too small")
	}
	for i, p := range t.Partitions {
		if p.FirstLBA < firstUsable || p.LastLBA > last || p.FirstLBA > p.LastLBA {
			return fmt.Errorf("gpt: partition %d (LBA %d-%d) outside usable area %d-%d",
				i+1, p.FirstLBA, p.LastLBA, firstUsable, last)
		}
		if len(utf16.Encode([]rune(p.Name))) > maxNameChars {
			return fmt.Errorf("gpt: partition %d name longer than %d characters", i+1, maxNameChars)
		}
	}
	return nil
}

// protectiveMBR builds LBA 0: a legacy MBR with a single 0xEE partition
// covering the disk, so old tools see the disk as "in use".
func (t *Table) protectiveMBR() []byte {
	mbr := make([]byte, SectorSize)
	e := mbr[446:462] // first of the four 16-byte MBR partition entries

	size := t.DiskSectors - 1
	if size > 0xFFFFFFFF {
		size = 0xFFFFFFFF // MBR fields are 32-bit
	}
	e[0] = 0x00                                           // not bootable
	e[1], e[2], e[3] = 0x00, 0x02, 0x00                   // CHS start (cylinder 0, head 0, sector 2)
	e[4] = mbrTypeGPT                                     // partition type
	e[5], e[6], e[7] = 0xFF, 0xFF, 0xFF                   // CHS end: "beyond CHS range"
	binary.LittleEndian.PutUint32(e[8:12], 1)             // starting LBA
	binary.LittleEndian.PutUint32(e[12:16], uint32(size)) // size in sectors

	mbr[510], mbr[511] = 0x55, 0xAA // boot signature
	return mbr
}

// entryArray builds the 128 x 128-byte partition entry array.
func (t *Table) entryArray() []byte {
	buf := make([]byte, entriesBytes)
	for i, p := range t.Partitions {
		e := buf[i*entrySize : (i+1)*entrySize]
		copy(e[0:16], p.Type[:])
		copy(e[16:32], p.ID[:])
		binary.LittleEndian.PutUint64(e[32:40], p.FirstLBA)
		binary.LittleEndian.PutUint64(e[40:48], p.LastLBA)
		binary.LittleEndian.PutUint64(e[48:56], 0) // attributes
		for j, c := range utf16.Encode([]rune(p.Name)) {
			binary.LittleEndian.PutUint16(e[56+2*j:], c)
		}
	}
	return buf
}

// header builds one GPT header sector. The primary and the backup differ only
// in where they live (myLBA), where the other copy is (altLBA) and where
// their entry array starts.
func (t *Table) header(myLBA, altLBA, entriesLBA uint64, entriesCRC uint32) []byte {
	h := make([]byte, SectorSize)
	copy(h[0:8], signature)
	binary.LittleEndian.PutUint32(h[8:12], revision1_0)
	binary.LittleEndian.PutUint32(h[12:16], headerSize)
	// h[16:20] header CRC32, filled in last; h[20:24] reserved (zero)
	binary.LittleEndian.PutUint64(h[24:32], myLBA)
	binary.LittleEndian.PutUint64(h[32:40], altLBA)
	binary.LittleEndian.PutUint64(h[40:48], firstUsable)
	binary.LittleEndian.PutUint64(h[48:56], LastUsableLBA(t.DiskSectors))
	copy(h[56:72], t.DiskID[:])
	binary.LittleEndian.PutUint64(h[72:80], entriesLBA)
	binary.LittleEndian.PutUint32(h[80:84], numEntries)
	binary.LittleEndian.PutUint32(h[84:88], entrySize)
	binary.LittleEndian.PutUint32(h[88:92], entriesCRC)

	// The header CRC covers the first 92 bytes with the CRC field set to zero.
	binary.LittleEndian.PutUint32(h[16:20], crc32.ChecksumIEEE(h[:headerSize]))
	return h
}
