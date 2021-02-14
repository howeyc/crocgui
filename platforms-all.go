// +build !android

package main

const DEFAULT_DOWNLOAD_DIR = "."

func fixpath(fpath string) string {
	return fpath
}
