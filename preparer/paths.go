package main

import (
	"os"
	"path/filepath"
)

// Default file locations are relative to the executable, not to the current
// directory, so "build\preparer.exe" works the same from any folder:
//
//	build\preparer.exe
//	build\bareboot.img
//	build\esp\EFI\BOOT\BOOTX64.EFI
func buildDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func defaultEFIPath() string {
	return filepath.Join(buildDir(), "esp", "EFI", "BOOT", "BOOTX64.EFI")
}

func defaultImagePath() string {
	return filepath.Join(buildDir(), "bareboot.img")
}
