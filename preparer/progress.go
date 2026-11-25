package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// countingWriter accepts writes without storing them and only adds up their
// size. Running writeMedia against it first tells us how many bytes the real
// run will write, so the progress bar can show a percentage.
type countingWriter struct {
	total int64
}

func (c *countingWriter) WriteAt(p []byte, _ int64) (int, error) {
	c.total += int64(len(p))
	return len(p), nil
}

// progressWriter passes every write to the real target and redraws a bar.
type progressWriter struct {
	target io.WriterAt
	total  int64
	done   int64
	mu     sync.Mutex
}

func (pw *progressWriter) WriteAt(p []byte, off int64) (int, error) {
	n, err := pw.target.WriteAt(p, off)
	pw.mu.Lock()
	pw.done += int64(n)
	pw.draw()
	pw.mu.Unlock()
	return n, err
}

// draw prints "[#########-----------]  45%  512.0 KiB / 1.1 MiB" on one line.
func (pw *progressWriter) draw() {
	const width = 30
	pct := float64(pw.done) / float64(pw.total)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * width)
	fmt.Printf("\r  [%s%s] %3.0f%%  %s / %s   ",
		strings.Repeat("#", filled), strings.Repeat("-", width-filled),
		pct*100, formatBytes(pw.done), formatBytes(pw.total))
}
