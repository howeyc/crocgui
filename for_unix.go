//go:build unix

package main

func CanCreateSymlinks() bool {
	return !isMobile
}
