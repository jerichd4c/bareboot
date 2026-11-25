package gpt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"unicode/utf16"
)

// Read parses the GPT of a disk of diskSectors sectors and checks it the way
// a firmware would: protective MBR, both headers (signature, CRC32, where they
// point) and both entry arrays (CRC32, identical copies).
//
// It is written independently of Write, so it can catch mistakes in Write.
func Read(r io.ReaderAt, diskSectors uint64) (*Table, error) {
	if diskSectors <= firstUsable+backupReserve {
		return nil, errors.New("gpt: disk too small")
	}
	lastLBA := diskSectors - 1

	if err := checkProtectiveMBR(r); err != nil {
		return nil, err
	}
	primary, err := readHeader(r, 1)
	if err != nil {
		return nil, fmt.Errorf("primary header: %w", err)
	}
	backup, err := readHeader(r, lastLBA)
	if err != nil {
		return nil, fmt.Errorf("backup header: %w", err)
	}

	// Each copy must say where it is and where the other one is.
	if primary.myLBA != 1 || primary.altLBA != lastLBA {
		return nil, fmt.Errorf("gpt: primary header points to LBA %d/%d, expected 1/%d", primary.myLBA, primary.altLBA, lastLBA)
	}
	if backup.myLBA != lastLBA || backup.altLBA != 1 {
		return nil, fmt.Errorf("gpt: backup header points to LBA %d/%d, expected %d/1", backup.myLBA, backup.altLBA, lastLBA)
	}
	// Apart from those fields, the two copies must describe the same table.
	if primary.firstUsable != backup.firstUsable || primary.lastUsable != backup.lastUsable ||
		primary.diskID != backup.diskID || primary.numEntries != backup.numEntries ||
		primary.entrySize != backup.entrySize || primary.entriesCRC != backup.entriesCRC {
		return nil, errors.New("gpt: primary and backup headers differ")
	}
	if primary.entrySize != entrySize || primary.numEntries != numEntries {
		return nil, fmt.Errorf("gpt: unsupported entry array (%d x %d bytes)", primary.numEntries, primary.entrySize)
	}

	entries, err := readEntries(r, primary)
	if err != nil {
		return nil, fmt.Errorf("primary entries: %w", err)
	}
	backupEntries, err := readEntries(r, backup)
	if err != nil {
		return nil, fmt.Errorf("backup entries: %w", err)
	}
	if !bytes.Equal(entries, backupEntries) {
		return nil, errors.New("gpt: primary and backup entry arrays differ")
	}

	t := &Table{DiskSectors: diskSectors, DiskID: primary.diskID}
	for i := 0; i < numEntries; i++ {
		e := entries[i*entrySize : (i+1)*entrySize]
		var p Partition
		copy(p.Type[:], e[0:16])
		if p.Type == (GUID{}) {
			continue // unused entry
		}
		copy(p.ID[:], e[16:32])
		p.FirstLBA = binary.LittleEndian.Uint64(e[32:40])
		p.LastLBA = binary.LittleEndian.Uint64(e[40:48])
		p.Name = decodeName(e[56:128])
		if p.FirstLBA < primary.firstUsable || p.LastLBA > primary.lastUsable || p.FirstLBA > p.LastLBA {
			return nil, fmt.Errorf("gpt: partition %d (LBA %d-%d) outside usable area", i+1, p.FirstLBA, p.LastLBA)
		}
		t.Partitions = append(t.Partitions, p)
	}
	return t, nil
}

type header struct {
	myLBA, altLBA           uint64
	firstUsable, lastUsable uint64
	entriesLBA              uint64
	numEntries, entrySize   uint32
	entriesCRC              uint32
	diskID                  GUID
}

func readHeader(r io.ReaderAt, lba uint64) (header, error) {
	var h header
	buf := make([]byte, SectorSize)
	if _, err := r.ReadAt(buf, int64(lba)*SectorSize); err != nil {
		return h, err
	}
	if string(buf[0:8]) != signature {
		return h, errors.New(`signature "EFI PART" not found`)
	}
	if size := binary.LittleEndian.Uint32(buf[12:16]); size != headerSize {
		return h, fmt.Errorf("unexpected header size %d", size)
	}
	// Recompute the CRC with its own field set to zero, as the spec says.
	stored := binary.LittleEndian.Uint32(buf[16:20])
	binary.LittleEndian.PutUint32(buf[16:20], 0)
	if crc := crc32.ChecksumIEEE(buf[:headerSize]); crc != stored {
		return h, fmt.Errorf("header CRC32 mismatch (stored %08X, computed %08X)", stored, crc)
	}
	h.myLBA = binary.LittleEndian.Uint64(buf[24:32])
	h.altLBA = binary.LittleEndian.Uint64(buf[32:40])
	h.firstUsable = binary.LittleEndian.Uint64(buf[40:48])
	h.lastUsable = binary.LittleEndian.Uint64(buf[48:56])
	copy(h.diskID[:], buf[56:72])
	h.entriesLBA = binary.LittleEndian.Uint64(buf[72:80])
	h.numEntries = binary.LittleEndian.Uint32(buf[80:84])
	h.entrySize = binary.LittleEndian.Uint32(buf[84:88])
	h.entriesCRC = binary.LittleEndian.Uint32(buf[88:92])
	return h, nil
}

func readEntries(r io.ReaderAt, h header) ([]byte, error) {
	buf := make([]byte, int(h.numEntries)*int(h.entrySize))
	if _, err := r.ReadAt(buf, int64(h.entriesLBA)*SectorSize); err != nil {
		return nil, err
	}
	if crc := crc32.ChecksumIEEE(buf); crc != h.entriesCRC {
		return nil, fmt.Errorf("entry array CRC32 mismatch (stored %08X, computed %08X)", h.entriesCRC, crc)
	}
	return buf, nil
}

func checkProtectiveMBR(r io.ReaderAt) error {
	mbr := make([]byte, SectorSize)
	if _, err := r.ReadAt(mbr, 0); err != nil {
		return err
	}
	if mbr[510] != 0x55 || mbr[511] != 0xAA {
		return errors.New("gpt: protective MBR has no 55AA signature")
	}
	if mbr[446+4] != mbrTypeGPT {
		return fmt.Errorf("gpt: protective MBR partition type is %02X, expected EE", mbr[446+4])
	}
	return nil
}

func decodeName(b []byte) string {
	u := make([]uint16, 0, maxNameChars)
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
