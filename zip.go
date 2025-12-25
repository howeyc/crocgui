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

// ZipDirectoryProgress создает zip-архив из исходной директории с обновлением прогресса в GUI.
// Сначала вычисляет общий размер, затем обновляет прогресс на основе записанных байт.
func ZipDirectoryProgress(destination, source string, c *fyne.Container, onComplete func(err error)) {
	go func() {
		err := zipDirectoryWithOverallProgress(destination, source, c)
		onComplete(err)
	}()
}

// zipDirectoryWithOverallProgress выполняет архивацию с отслеживанием общего прогресса.
func zipDirectoryWithOverallProgress(destination string, source string, c *fyne.Container) (err error) {
	// 1. Проверяем, существует ли файл назначения
	if _, statErr := os.Stat(destination); statErr == nil {
		err = fmt.Errorf("%s file already exists", destination)
		log.Error(err)
		return err
	}

	// 2. Вычисляем общий размер исходных данных
	totalSize, err := getTotalSize(source)
	if err != nil {
		log.Errorf("Error calculating total size: %v", err)
		return err
	}

	// 3. Создаем файл назначения
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

	// 4. Создаем ProgressWriter для всего архива, используя общий размер
	pw, restore := NewProgressWriter(file, totalSize, c)

	zipWriter := zip.NewWriter(pw) // zipWriter теперь пишет через ProgressWriter
	zipWriter.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})
	defer func() {
		closeErr := zipWriter.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// Получаем базовое имя для структуры zip (имя исходной директории)
	baseName := filepath.Base(source)

	// Отслеживаем уже добавленные директории для сохранения их времен модификации
	addedDirs := make(map[string]bool)

	// Первый проход: добавляем корневую директорию с ее временем модификации
	rootInfo, err := os.Stat(source)
	if err == nil && rootInfo.IsDir() {
		header, err := zip.FileInfoHeader(rootInfo)
		if err != nil {
			log.Error(err)
		} else {
			header.Name = baseName + "/" // Косая черта в конце указывает на директорию
			header.Method = zip.Store
			header.Modified = rootInfo.ModTime()

			_, err = zipWriter.CreateHeader(header)
			if err != nil {
				log.Error(err)
			} else {
				addedDirs[header.Name] = true
				log.Debugf("Adding %s", header.Name)
			}
		}
	}

	// 5. Обходим исходную директорию
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error walking path %s: %v", path, err)
			return nil // Продолжаем обход если возможно
		}

		// Пропускаем корневую директорию (мы уже добавили ее)
		if path == source {
			return nil
		}

		// Определяем относительный путь для архива
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			log.Errorf("Error getting relative path for %s: %v", path, err)
			return nil
		}

		// Создаем zip путь с базовой структурой имени
		zipPath := filepath.ToSlash(filepath.Join(baseName, relPath))

		if info.IsDir() {
			// Добавляем запись директории в zip с оригинальным временем модификации
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				log.Errorf("Error creating zip header for %s: %v", path, err)
				return nil
			}

			// Устанавливаем имя в архиве с косой чертой в конце для директории
			header.Name = zipPath + "/"
			header.Method = zip.Store
			header.Modified = info.ModTime()

			// Создаем директорию в zip архиве
			_, err = zipWriter.CreateHeader(header)
			if err != nil {
				log.Errorf("Error creating zip directory entry %s: %v", zipPath, err)
				return nil
			}

			addedDirs[zipPath] = true
			log.Debugf("Adding %s", zipPath+"/")
			log.Debugf("Added directory to archive: %s (mod time: %v)", zipPath, info.ModTime())
			return nil
		}

		if info.Mode().IsRegular() {
			// Обеспечиваем существование родительских директорий в zip с правильными временами
			parentDir := filepath.Dir(zipPath)
			parentsToAdd := []string{}

			// Собираем все отсутствующие родительские директории
			for parentDir != "" && parentDir != baseName {
				if !addedDirs[parentDir] {
					parentsToAdd = append([]string{parentDir}, parentsToAdd...)
				}
				parentDir = filepath.Dir(parentDir)
				if parentDir == baseName {
					break
				}
			}

			// Добавляем родительские директории в правильном порядке (от корня к листьям)
			for _, parentDir := range parentsToAdd {
				// Получаем фактическую информацию о директории для сохранения ее времени модификации
				dirPath := filepath.Join(source, strings.TrimPrefix(parentDir, baseName+"/"))
				dirInfo, err := os.Stat(dirPath)
				if err == nil {
					header := &zip.FileHeader{
						Name: parentDir + "/", // Косая черта для директории
					}
					header.SetMode(0755)
					header.Modified = dirInfo.ModTime()

					_, err := zipWriter.CreateHeader(header)
					if err != nil {
						log.Errorf("Error creating parent directory %s: %v", parentDir, err)
						return nil
					}
					addedDirs[parentDir] = true

					log.Debugf("Adding %s", parentDir+"/")
				}
			}

			// Открываем исходный файл
			srcFile, err := os.Open(path)
			if err != nil {
				log.Errorf("Error opening file %s: %v", path, err)
				return nil
			}
			defer srcFile.Close()

			// Создаем новую запись в zip архиве с заголовком файла, сохраняющим времена
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				log.Errorf("Error creating zip header for %s: %v", path, err)
				srcFile.Close()
				return nil
			}

			// Устанавливаем имя в архиве С префиксом базового имени
			header.Name = zipPath

			// Устанавливаем метод сжатия
			header.Method = zip.Deflate

			// Сохраняем оригинальное время модификации файла
			header.Modified = info.ModTime()

			// Создаем файл в zip архиве с пользовательским заголовком
			zipEntryWriter, err := zipWriter.CreateHeader(header)
			if err != nil {
				log.Errorf("Error creating zip entry %s: %v", zipPath, err)
				srcFile.Close()
				return nil
			}

			// Копируем содержимое исходного файла в запись архива
			_, copyErr := io.Copy(zipEntryWriter, srcFile)
			srcFile.Close()

			if copyErr != nil {
				log.Errorf("Error copying file %s to zip: %v", path, copyErr)
				return copyErr
			}

			log.Debugf("Added file to archive: %s (mod time: %v)", zipPath, info.ModTime())
		}
		return nil
	})

	if err != nil {
		log.Errorf("Error during zip walk: %v", err)
		return err
	}

	// 6. Восстанавливаем GUI (скрываем прогресс-бар)
	restore()
	log.Debugf("Zip creation completed")
	return nil
}

// UnzipDirectoryProgress распаковывает исходный zip-файл в директорию назначения с обновлением прогресса в GUI.
// Сначала вычисляет общий распакованный размер, затем обновляет прогресс на основе записанных байт.
func UnzipDirectoryProgress(destination, source string, c *fyne.Container, onComplete func(err error)) {
	go func() {
		err := unzipDirectoryWithCustomCopy(destination, source, c)
		onComplete(err)
	}()
}

// unzipDirectoryWithCustomCopy выполняет распаковку с отслеживанием общего прогресса с использованием пользовательского цикла копирования.
func unzipDirectoryWithCustomCopy(destination string, source string, c *fyne.Container) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		log.Errorf("Error opening zip file %s: %v", source, err)
		return err
	}
	defer archive.Close()

	// Сохраняем времена модификации для всех файлов и директорий
	modTimes := make(map[string]time.Time)

	// Первый проход: создаем структуру директорий и сохраняем времена модификации
	for _, f := range archive.File {
		filePath := filepath.Join(destination, f.Name)
		sanitizedPath := filepath.Clean(filePath)

		// Предотвращаем уязвимость обхода пути
		if strings.Contains(sanitizedPath, "..") {
			err := fmt.Errorf("invalid file path %s", sanitizedPath)
			log.Error(err)
			return err
		}

		// Сохраняем время модификации для этой записи
		modifiedTime := f.Modified
		if modifiedTime.IsZero() {
			modifiedTime = f.FileHeader.Modified
		}
		if !modifiedTime.IsZero() {
			modTimes[sanitizedPath] = modifiedTime
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(sanitizedPath, os.ModePerm); err != nil {
				log.Errorf("Error creating directory %s: %v", sanitizedPath, err)
				return err
			}
		}
	}

	// 1. Вычисляем общий распакованный размер файлов в архиве
	var totalUncompressedSize int64
	for _, f := range archive.File {
		if !f.FileInfo().IsDir() {
			totalUncompressedSize += int64(f.UncompressedSize64)
		}
	}

	if totalUncompressedSize == 0 {
		// Нет файлов для извлечения
		_, restore := NewProgressWriter(io.Discard, 1, c)
		fyne.Do(func() {})
		restore()
		log.Debugf("No files to extract")
		return nil
	}

	// 2. Создаем ProgressWriter для общего прогресса
	pw, restore := NewProgressWriter(io.Discard, totalUncompressedSize, c)
	var currentWritten int64

	// 3. Итерируемся по файлам в архиве и извлекаем их
	for _, f := range archive.File {
		filePath := filepath.Join(destination, f.Name)
		log.Debugf("Unzipping file %s", filePath)

		sanitizedPath := filepath.Clean(filePath)
		if strings.Contains(sanitizedPath, "..") {
			err := fmt.Errorf("invalid file path %s", sanitizedPath)
			log.Error(err)
			restore()
			return err
		}

		if f.FileInfo().IsDir() {
			continue // Директории уже созданы в первом проходе
		}

		// Обеспечиваем существование родительской директории
		if err := os.MkdirAll(filepath.Dir(sanitizedPath), os.ModePerm); err != nil {
			log.Errorf("Error creating parent directory for %s: %v", sanitizedPath, err)
			restore()
			return err
		}

		// Открываем файл в архиве
		fileInArchive, err := f.Open()
		if err != nil {
			log.Errorf("Error opening file in archive %s: %v", f.Name, err)
			restore()
			return err
		}

		// Создаем файл назначения
		dstFile, err := os.OpenFile(sanitizedPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			log.Errorf("Error creating destination file %s: %v", sanitizedPath, err)
			fileInArchive.Close()
			restore()
			return err
		}

		// Пользовательский цикл копирования для плавного обновления прогресса
		buf := make([]byte, 32*1024)
		for {
			n, readErr := fileInArchive.Read(buf)
			if n > 0 {
				if _, writeErr := dstFile.Write(buf[:n]); writeErr != nil {
					dstFile.Close()
					fileInArchive.Close()
					restore()
					return writeErr
				}

				currentWritten += int64(n)
				progressFraction := float64(currentWritten) / float64(totalUncompressedSize)
				if progressFraction > 1.0 {
					progressFraction = 1.0
				}

				pw.OnProgress(progressFraction)
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				dstFile.Close()
				fileInArchive.Close()
				restore()
				return readErr
			}
		}

		// Закрываем оба файла после копирования
		dstFile.Close()
		fileInArchive.Close()
	}

	// Второй проход: восстанавливаем времена модификации для ВСЕХ файлов и директорий
	log.Debugf("Restoring modification times...")
	for path, modTime := range modTimes {
		if err := os.Chtimes(path, time.Time{}, modTime); err != nil {
			log.Warnf("Failed to set modification time for %s: %v", path, err)
		} else {
			log.Debugf("Restored modification time for %s: %v", path, modTime)
		}
	}

	// 4. Восстанавливаем GUI (скрываем прогресс-бар)
	restore()
	log.Debugf("Extraction completed")
	return nil
}

// getTotalSize обходит исходную директорию и суммирует размеры всех обычных файлов.
func getTotalSize(source string) (int64, error) {
	var total int64
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("Error walking path %s: %v", path, err)
			return nil // Пропускаем проблемные файлы/директории для вычисления размера
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ValidFileName проверяет, является ли имя файла допустимым
func ValidFileName(fname string) (err error) {
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
	_, basename := filepath.Split(fname)
	if strings.Contains(basename, string(os.PathSeparator)) {
		err = fmt.Errorf("basename cannot contain path separators: '%s'", basename)
		return
	}
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
		modTime := f.Modified
		if modTime.IsZero() {
			modTime = f.FileInfo().ModTime()
		}
		fileTimes[f.Name] = modTime
	}

	return fileTimes, nil
}
