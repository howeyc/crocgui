// +build !android

package main

import "os"

func init() {
	wd, werr := os.Getwd()
	if werr == nil {
		DEFAULT_DOWNLOAD_DIR = wd
	}
}

func fixpath(fpath string) string {
	return fpath
}
