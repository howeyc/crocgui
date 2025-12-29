//go:build unix

package main

func CanCreateSymlinks() bool {
	return !isMobile
}

func isGUIApplication() bool { return false }
