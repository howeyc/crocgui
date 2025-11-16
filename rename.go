// rename.go
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func Rename(src, dst string) error {
	if noRename {
		return fmt.Errorf("no rename")
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}

	// Check that dst is not a subdirectory of src
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	if strings.HasPrefix(dstAbs, srcAbs+string(filepath.Separator)) {
		return errors.New("destination cannot be inside source directory")
	}

	// Try standard rename first
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else {
		return err
	}

	// If standard rename failed, use copy approach
	// if srcStat.IsDir() {
	// 	return renameDir(src, dst)
	// }

	// return renameFile(src, dst)
}

func renameDir(src, dst string) error {
	// Check if destination already exists
	if _, err := os.Stat(dst); err == nil {
		return os.ErrExist
	}

	// Create destination directory
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}

	// Copy directory contents
	if err := copyDirectory(src, dst); err != nil {
		os.RemoveAll(dst) // cleanup on error
		return err
	}

	// Remove source directory
	if err := os.RemoveAll(src); err != nil {
		return err
	}

	return nil
}

func renameFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return os.ErrExist
	}

	// Get source file info for permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Use copyFile to copy the file with original permissions
	if err := copyFile(src, dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Remove source file
	return os.Remove(src)
}

func copyDirectory(src, dst string) error {
	// Создаем целевую директорию с оригинальными правами
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	return filepath.WalkDir(src, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Пропускаем корневую директорию - она уже создана
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if dirEntry.IsDir() {
			info, err := dirEntry.Info()
			if err != nil {
				return err
			}
			// Используем Mkdir вместо MkdirAll, так как родительские директории уже созданы
			// благодаря обходу в глубину и созданию корневой директории
			return os.Mkdir(dstPath, info.Mode())
		}

		return copyFileWithMode(path, dstPath, dirEntry)
	})
}
func copyFileWithMode(src, dst string, dirEntry fs.DirEntry) error {
	info, err := dirEntry.Info()
	if err != nil {
		return err
	}
	return copyFile(src, dst, info.Mode())
}

func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst) // cleanup on error
		return err
	}

	if err := dstFile.Close(); err != nil {
		os.Remove(dst) // cleanup on error
		return err
	}

	return os.Chmod(dst, mode)
}
