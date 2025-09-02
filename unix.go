//go:build linux || darwin

package main

import (
	"os"
)

func Symlink(oldname string, newname string) error {
	return os.Symlink(oldname, newname)
}
