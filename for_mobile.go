//go:build android || ios

package main

import (
	"path/filepath"

	gomime "github.com/cubewise-code/go-mime"
)

// detectMimeType определяет MIME-тип по расширению файла через go-mime
func detectMimeType(fileName string) string {
	ext := filepath.Ext(fileName)
	mimeType := gomime.TypeByExtension(ext)

	if mimeType != "" {
		return mimeType
	}

	return MIME_TYPE_OCTET_STREAM
}

func CanCreateSymlinks() bool {
	return false
}
