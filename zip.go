// zip.go
package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unicode"

	"fyne.io/fyne/v2"
	log "github.com/schollz/logger"
)

// ZipDirectoryProgress zips the contents of source directory to destination zip file with overall progress updates in the GUI.
// It calculates the total size first and then updates progress based on bytes written to the zip file.
func ZipDirectoryProgress(destination, source string, c *fyne.Container, onComplete func(err error)) {
	go func() {
		err := zipDirectoryWithOverallProgress(destination, source, c)
		onComplete(err)
	}()
}

// zipDirectoryWithOverallProgress performs the zipping with overall progress tracking.
func zipDirectoryWithOverallProgress(destination string, source string, c *fyne.Container) (err error) {
	// 1. Check if destination already exists
	if _, statErr := os.Stat(destination); statErr == nil {
		err = fmt.Errorf("%s file already exists", destination)
		log.Error(err)
		return err
	}

	// 2. Calculate total source size
	totalSize, err := getTotalSize(source)
	if err != nil {
		log.Errorf("Error calculating total size: %v", err)
		return err
	}

	// 3. Create destination file
	file, err := os.Create(destination)
	if err != nil {
		log.Error(err)
		return err
	}
	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// 4. Create ProgressWriter for the entire archive file, using the total size
	pw, restore := NewProgressWriter(file, totalSize, c)

	zipWriter := zip.NewWriter(pw) // zipWriter now writes through the ProgressWriter
	zipWriter.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})
	defer func() {
		closeErr := zipWriter.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// 5. Walk the source directory
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error walking path %s: %v", path, err)
			return nil // Continue walking if possible
		}

		if info.Mode().IsRegular() {
			// Open the source file
			srcFile, err := os.Open(path)
			if err != nil {
				log.Errorf("Error opening file %s: %v", path, err)
				return err
			}
			defer srcFile.Close()

			// Determine the relative path for the archive
			relPath, err := filepath.Rel(source, path)
			if err != nil {
				log.Errorf("Error getting relative path for %s: %v", path, err)
				srcFile.Close()
				return err
			}
			zipPath := filepath.ToSlash(relPath)

			// Create a new entry in the zip archive with file header that preserves file times
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				log.Errorf("Error creating zip header for %s: %v", path, err)
				srcFile.Close()
				return err
			}

			// Set the name in the archive
			header.Name = zipPath

			// Set the compression method (you can change to Deflate for compression)
			header.Method = zip.Store

			// Preserve the original file modification time
			header.Modified = info.ModTime()

			// For better compatibility, also set the extended timestamp fields
			// This ensures the modification time is preserved across different zip tools
			// header.SetModTime(info.ModTime())

			// Create the file in the zip archive with the custom header
			zipEntryWriter, err := zipWriter.CreateHeader(header)
			if err != nil {
				log.Errorf("Error creating zip entry %s: %v", zipPath, err)
				srcFile.Close()
				return err
			}

			// Copy the content of the source file into the archive entry
			// io.Copy will write through zipEntryWriter, which eventually uses the ProgressWriter (pw)
			_, copyErr := io.Copy(zipEntryWriter, srcFile)
			srcFile.Close() // Explicitly close after copying

			if copyErr != nil {
				log.Errorf("Error copying file %s to zip: %v", path, copyErr)
				return copyErr
			}

			log.Tracef("Added file to archive: %s (mod time: %v)", zipPath, info.ModTime())
		}
		return nil
	})

	if err != nil {
		log.Errorf("Error during zip walk: %v", err)
		return err
	}

	// 6. Restore GUI (hides the progress bar)
	restore()
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}

// UnzipDirectoryProgress unzips the source zip file to destination directory with overall progress updates in the GUI.
// It calculates the total uncompressed size first and then updates progress based on bytes written to the destination files.
// This version uses a custom copy loop to update progress smoothly without atomic counters,
// by calling ProgressWriter.OnProgress (which uses fyne.Do) from the background goroutine.
func UnzipDirectoryProgress(destination, source string, c *fyne.Container, onComplete func(err error)) {
	go func() {
		err := unzipDirectoryWithCustomCopy(destination, source, c)
		onComplete(err)
	}()
}

// unzipDirectoryWithCustomCopy performs the unzipping with overall progress tracking using a custom copy loop.
func unzipDirectoryWithCustomCopy(destination string, source string, c *fyne.Container) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		log.Errorf("Error opening zip file %s: %v", source, err)
		return err
	}
	defer archive.Close()

	// 1. Calculate total uncompressed size of files in the archive
	var totalUncompressedSize int64
	for _, f := range archive.File {
		if !f.FileInfo().IsDir() {
			totalUncompressedSize += int64(f.UncompressedSize64)
		}
	}

	if totalUncompressedSize == 0 {
		// No files to extract
		_, restore := NewProgressWriter(io.Discard, 1, c)
		fyne.Do(func() {})
		restore()
		fmt.Fprintf(os.Stderr, "\n")
		return nil
	}

	// 2. Create ProgressWriter for overall progress, using io.Discard as dummy Writer
	// and update progress manually during file copy loops
	pw, restore := NewProgressWriter(io.Discard, totalUncompressedSize, c)
	var currentWritten int64 // Local variable, updates happen via ProgressWriter.OnProgress which uses fyne.Do

	// 3. Iterate through files in the archive
	for _, f := range archive.File {
		filePath := filepath.Join(destination, f.Name)
		fmt.Fprintf(os.Stderr, "\r\033[2K")
		fmt.Fprintf(os.Stderr, "\rUnzipping file %s", filePath)

		// Issue #593: Prevent path traversal vulnerability
		sanitizedPath := filepath.Clean(filePath)
		if strings.Contains(sanitizedPath, "..") {
			err := fmt.Errorf("invalid file path %s", sanitizedPath)
			log.Error(err)
			restore() // Restore GUI before returning error
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(sanitizedPath, os.ModePerm); err != nil {
				log.Errorf("Error creating directory %s: %v", sanitizedPath, err)
				restore() // Restore GUI before returning error
				return err
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(sanitizedPath), os.ModePerm); err != nil {
			log.Errorf("Error creating parent directory for %s: %v", sanitizedPath, err)
			restore() // Restore GUI before returning error
			return err
		}

		// Open file in archive
		fileInArchive, err := f.Open()
		if err != nil {
			log.Errorf("Error opening file in archive %s: %v", f.Name, err)
			restore() // Restore GUI before returning error
			return err
		}

		// Create (or overwrite) destination file
		dstFile, err := os.OpenFile(sanitizedPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			log.Errorf("Error creating destination file %s: %v", sanitizedPath, err)
			fileInArchive.Close()
			restore() // Restore GUI before returning error
			return err
		}

		// --- Custom copy loop for smooth progress updates ---
		buf := make([]byte, 32*1024) // Buffer for read/write
		for {
			n, readErr := fileInArchive.Read(buf)
			if n > 0 {
				// Write the chunk to the destination file
				if _, writeErr := dstFile.Write(buf[:n]); writeErr != nil {
					dstFile.Close()
					fileInArchive.Close()
					restore()
					return writeErr
				}

				// Update the local progress counter for the total archive
				// (currentWritten is updated directly in the single goroutine)
				currentWritten += int64(n)

				// Calculate overall progress
				progressFraction := float64(currentWritten) / float64(totalUncompressedSize)
				if progressFraction > 1.0 {
					progressFraction = 1.0
				}

				// Update progress in the GUI via ProgressWriter.OnProgress (which calls fyne.Do)
				// This call happens in the *background* goroutine, but OnProgress uses fyne.Do internally.
				pw.OnProgress(progressFraction)
				// Note: ProgressWriter.Written is not updated here as we are not calling pw.Write
				// and use OnProgress directly.
			}
			if readErr == io.EOF {
				break // End of file
			}
			if readErr != nil {
				dstFile.Close()
				fileInArchive.Close()
				restore()
				return readErr
			}
		}

		// Close both files after copying the current file is complete
		dstFile.Close()
		fileInArchive.Close()

		// Preserve the original file modification time from the zip entry
		if !f.Modified.IsZero() {
			// Use the modification time from the zip file header
			modTime := f.Modified
			if err := os.Chtimes(sanitizedPath, modTime, modTime); err != nil {
				log.Warnf("Failed to set modification time for %s: %v", sanitizedPath, err)
				// Continue even if setting time fails
			} else {
				log.Tracef("Set modification time for %s: %v", sanitizedPath, modTime)
			}
		} else if !f.FileInfo().ModTime().IsZero() {
			// Fallback to the file info modification time
			modTime := f.FileInfo().ModTime()
			if err := os.Chtimes(sanitizedPath, modTime, modTime); err != nil {
				log.Warnf("Failed to set modification time for %s: %v", sanitizedPath, err)
				// Continue even if setting time fails
			} else {
				log.Tracef("Set modification time for %s: %v", sanitizedPath, modTime)
			}
		}
	}

	// 4. Restore GUI (hides the progress bar)
	restore()
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}

// getTotalSize walks the source directory and sums the sizes of all regular files.
func getTotalSize(source string) (int64, error) {
	var total int64
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error walking path %s: %v", path, err)
			return nil // Skip problematic files/dirs for size calculation
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ValidFileName checks if a filename is valid
// by making sure it has no invisible characters
func ValidFileName(fname string) (err error) {
	// make sure it doesn't contain unicode or invisible characters
	for _, r := range fname {
		if !unicode.IsGraphic(r) {
			err = fmt.Errorf("non-graphical unicode: %x U+%d in '%x'", string(r), r, fname)
			return
		}
		if !unicode.IsPrint(r) {
			err = fmt.Errorf("non-printable unicode: %x U+%d in '%x'", string(r), r, fname)
			return
		}
	}
	// make sure basename does not include path separators
	_, basename := filepath.Split(fname)
	if strings.Contains(basename, string(os.PathSeparator)) {
		err = fmt.Errorf("basename cannot contain path separators: '%s'", basename)
		return
	}
	// make sure the filename is not an absolute path
	if filepath.IsAbs(fname) {
		err = fmt.Errorf("filename cannot be an absolute path: '%s'", fname)
		return
	}
	if !filepath.IsLocal(fname) {
		err = fmt.Errorf("filename must be a local path: '%s'", fname)
		return
	}
	return
}

// GetZipFileTimes возвращает информацию о времени модификации файлов в архиве
func GetZipFileTimes(zipPath string) (map[string]time.Time, error) {
	fileTimes := make(map[string]time.Time)

	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	for _, f := range archive.File {
		if !f.FileInfo().IsDir() {
			modTime := f.Modified
			if modTime.IsZero() {
				modTime = f.FileInfo().ModTime()
			}
			fileTimes[f.Name] = modTime
		}
	}

	return fileTimes, nil
}
