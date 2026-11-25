package fat32

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	dirEntrySize = 32
	attrVolumeID = 0x08
	attrDir      = 0x10
	attrArchive  = 0x20
)

// Node is a file or a directory of the tree being built.
type Node struct {
	name     [11]byte // 8.3 name as stored on disk: "BOOTX64 EFI"
	isDir    bool
	data     []byte  // file contents
	children []*Node // directory contents
	parent   *Node
	first    uint32 // first cluster, assigned during Write
}

// Mkdir creates a subdirectory and returns it.
func (n *Node) Mkdir(name string) (*Node, error) {
	return n.add(name, true, nil)
}

// AddFile creates a file with the given contents.
func (n *Node) AddFile(name string, data []byte) error {
	_, err := n.add(name, false, data)
	return err
}

func (n *Node) add(name string, isDir bool, data []byte) (*Node, error) {
	if !n.isDir {
		return nil, fmt.Errorf("fat32: %q is not a directory", n.Name())
	}
	short, err := shortName(name)
	if err != nil {
		return nil, err
	}
	for _, c := range n.children {
		if c.name == short {
			return nil, fmt.Errorf("fat32: %q already exists", name)
		}
	}
	child := &Node{name: short, isDir: isDir, data: data, parent: n}
	n.children = append(n.children, child)
	return child, nil
}

// Name returns the readable name ("BOOTX64.EFI"); the root is "/".
func (n *Node) Name() string {
	if n.parent == nil {
		return "/"
	}
	base := strings.TrimRight(string(n.name[:8]), " ")
	ext := strings.TrimRight(string(n.name[8:]), " ")
	if ext == "" {
		return base
	}
	return base + "." + ext
}

// walk visits n and then all its descendants (the root gets cluster 2 first).
func (n *Node) walk(visit func(*Node)) {
	visit(n)
	for _, c := range n.children {
		c.walk(visit)
	}
}

// content is what gets stored in the node's clusters: the file data, or the
// directory entries. Before clusters are assigned it only matters for its size.
func (n *Node) content(fs *FS) []byte {
	if !n.isDir {
		return n.data
	}
	t := fs.opts.Time
	var buf []byte
	if n.parent == nil {
		// The root directory holds the volume label as a special entry.
		buf = append(buf, dirEntry(fs.labelName(), attrVolumeID, 0, 0, t)...)
	} else {
		// Every subdirectory starts with "." (itself) and ".." (its parent;
		// cluster 0 means the root directory).
		parent := n.parent.first
		if n.parent.parent == nil {
			parent = 0
		}
		buf = append(buf, dirEntry(dotName("."), attrDir, n.first, 0, t)...)
		buf = append(buf, dirEntry(dotName(".."), attrDir, parent, 0, t)...)
	}
	for _, c := range n.children {
		attr, size := byte(attrArchive), uint32(len(c.data))
		if c.isDir {
			attr, size = attrDir, 0 // directories always have size 0
		}
		buf = append(buf, dirEntry(c.name, attr, c.first, size, t)...)
	}
	return buf
}

// dirEntry builds one 32-byte directory entry.
func dirEntry(name [11]byte, attr byte, cluster, size uint32, t time.Time) []byte {
	e := make([]byte, dirEntrySize)
	copy(e[0:11], name[:])
	e[11] = attr
	date, clock := fatDate(t), fatTime(t)
	binary.LittleEndian.PutUint16(e[14:16], clock) // creation time
	binary.LittleEndian.PutUint16(e[16:18], date)  // creation date
	binary.LittleEndian.PutUint16(e[18:20], date)  // last access date
	binary.LittleEndian.PutUint16(e[20:22], uint16(cluster>>16))
	binary.LittleEndian.PutUint16(e[22:24], clock) // last write time
	binary.LittleEndian.PutUint16(e[24:26], date)  // last write date
	binary.LittleEndian.PutUint16(e[26:28], uint16(cluster))
	binary.LittleEndian.PutUint32(e[28:32], size)
	return e
}

// shortName converts "bootx64.efi" into the on-disk 8.3 form "BOOTX64 EFI".
func shortName(name string) ([11]byte, error) {
	var out [11]byte
	for i := range out {
		out[i] = ' '
	}
	upper := strings.ToUpper(name)
	base, ext, _ := strings.Cut(upper, ".")
	if len(base) == 0 || len(base) > 8 || len(ext) > 3 || strings.Contains(ext, ".") {
		return out, fmt.Errorf("fat32: %q is not a valid 8.3 name", name)
	}
	for _, r := range base + ext {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return out, fmt.Errorf("fat32: invalid character %q in %q", r, name)
		}
	}
	copy(out[0:8], base)
	copy(out[8:11], ext)
	return out, nil
}

func dotName(s string) [11]byte {
	var out [11]byte
	copy(out[:], s+strings.Repeat(" ", 11-len(s)))
	return out
}

// fatDate packs a date as: bits 15-9 years since 1980, 8-5 month, 4-0 day.
func fatDate(t time.Time) uint16 {
	return uint16((t.Year()-1980)<<9 | int(t.Month())<<5 | t.Day())
}

// fatTime packs a time as: bits 15-11 hours, 10-5 minutes, 4-0 seconds/2.
func fatTime(t time.Time) uint16 {
	return uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
}
