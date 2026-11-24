package gpt

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// GUID is a 128-bit identifier stored in the on-disk (mixed-endian) layout:
// the first three fields are little-endian, the last two are big-endian.
type GUID [16]byte

// ParseGUID converts the text form "C12A7328-F81F-11D2-BA4B-00A0C93EC93B"
// into its on-disk byte layout.
func ParseGUID(s string) (GUID, error) {
	var g GUID
	raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(raw) != 16 {
		return g, fmt.Errorf("invalid GUID %q", s)
	}
	// Fields 1-3 are little-endian on disk, so their bytes are reversed.
	binary.LittleEndian.PutUint32(g[0:4], binary.BigEndian.Uint32(raw[0:4]))
	binary.LittleEndian.PutUint16(g[4:6], binary.BigEndian.Uint16(raw[4:6]))
	binary.LittleEndian.PutUint16(g[6:8], binary.BigEndian.Uint16(raw[6:8]))
	copy(g[8:16], raw[8:16])
	return g, nil
}

// MustParseGUID is ParseGUID for constants known to be valid.
func MustParseGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

// NewRandomGUID returns a random (version 4) GUID.
func NewRandomGUID() (GUID, error) {
	var g GUID
	if _, err := rand.Read(g[:]); err != nil {
		return g, err
	}
	g[7] = (g[7] & 0x0F) | 0x40 // version 4 (high nibble of field 3, little-endian)
	g[8] = (g[8] & 0x3F) | 0x80 // RFC 4122 variant
	return g, nil
}

// String returns the usual text form of the GUID.
func (g GUID) String() string {
	return fmt.Sprintf("%08X-%04X-%04X-%X-%X",
		binary.LittleEndian.Uint32(g[0:4]),
		binary.LittleEndian.Uint16(g[4:6]),
		binary.LittleEndian.Uint16(g[6:8]),
		g[8:10], g[10:16])
}
