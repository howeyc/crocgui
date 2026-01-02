//go:build unix

package main

func CanCreateSymlinks() bool {
	return !isMobile
}

// func isWindowsGUI() bool { return false }
