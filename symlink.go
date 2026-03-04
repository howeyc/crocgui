// symlink.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/schollz/logger"
)

// Symlink создает симлинк или псевдосимлинк в зависимости от CanCreateSymlinks.
func Symlink(oldname string, newname string) error {
	if isMobile || asMobile {
		return fmt.Errorf("isMobile")
	}
	if swap {
		return fmt.Errorf("swap")
	}

	base := filepath.Base(oldname)
	if strings.HasPrefix(base, STDIN) {
		return fmt.Errorf("stdin")
	}

	// Создаем директорию для новой ссылки если нужно
	dir := filepath.Dir(newname)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	if CanCreateSymlinks() {
		// Пробуем создать настоящий симлинк
		return os.Symlink(oldname, newname)
	}

	// Создаем файл-псевдосимлинк с содержимым "→oldname"
	content := PSL + filepath.FromSlash(oldname)
	return os.WriteFile(newname, []byte(content), 0644)
}

func Readlink(name string) (string, error) {
	if isMobile {
		// log.Debugf("Readlink: mobile device, skipping %s", name)
		return "", fmt.Errorf("isMobile")
	}

	fileInfo, err := os.Lstat(name)
	if err != nil {
		// log.Debugf("Readlink: Lstat failed for %s: %v", name, err)
		return "", err
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		// log.Debugf("Readlink: %s is real symlink", name)
		return os.Readlink(name)
	}

	if fileInfo.IsDir() {
		// log.Debugf("Readlink: IsDir  %s: %v", name, err)
		return "", os.ErrInvalid
	}

	// Псевдосимлинки проверяем ТОЛЬКО в каталоге SEND
	parentDir := filepath.Base(filepath.Dir(name))
	if parentDir != SEND {
		// log.Debugf("Readlink: %s not in SEND directory (parent: %s), skipping pseudosymlink check", name, parentDir)
		return "", os.ErrInvalid
	}

	// log.Debugf("Readlink: %s is in SEND directory (parent: %s), checking as pseudosymlink (size: %d bytes)", name, parentDir, fileInfo.Size())

	maxRead := len(PSL) + 4096 + 100
	if fileInfo.Size() > int64(maxRead) {
		// log.Debugf("Readlink: %s too large (%d > %d), not a pseudosymlink", name, fileInfo.Size(), maxRead)
		return "", os.ErrInvalid
	}

	file, err := os.Open(name)
	if err != nil {
		log.Debugf("Readlink: failed to open %s: %v", name, err)
		return "", err
	}
	defer file.Close()

	prefixBuffer := make([]byte, len(PSL))
	n, err := file.Read(prefixBuffer)
	if err != nil && err != io.EOF {
		log.Debugf("Readlink: failed to read prefix from %s: %v", name, err)
		return "", err
	}

	if n < len(PSL) || string(prefixBuffer) != PSL {
		// log.Debugf("Readlink: prefix mismatch or too short: got %q (%d bytes), expected %q", string(prefixBuffer), n, PSL)
		return "", os.ErrInvalid
	}

	// log.Debugf("Readlink: PSL prefix found in %s, reading target path", name)

	pathBuffer := make([]byte, maxRead-len(PSL))
	n, err = file.Read(pathBuffer)
	if err != nil && err != io.EOF {
		// log.Debugf("Readlink: failed to read target from %s: %v", name, err)
		return "", err
	}

	target := strings.TrimSpace(string(pathBuffer[:n]))
	if target == "" {
		// log.Debugf("Readlink: empty target after PSL prefix")
		return "", os.ErrInvalid
	}

	// log.Debugf("Readlink: found target %q in %s", target, name)

	target = filepath.FromSlash(target)
	// log.Debugf("Readlink: normalized target path: %q", target)

	if _, err := os.Stat(target); err != nil {
		log.Debugf("Readlink: target %q does not exist: %v", target, err)

		// if !filepath.IsAbs(target) {
		// 	absTarget := filepath.Join(filepath.Dir(name), target)
		// 	log.Debugf("Readlink: trying relative path %q", absTarget)

		// 	if _, err := os.Stat(absTarget); err == nil {
		// 		log.Debugf("Readlink: relative target %q exists", absTarget)
		// 		return target, nil
		// 	}
		// 	log.Debugf("Readlink: relative target %q also does not exist: %v", absTarget, err)
		// }
		return "", os.ErrNotExist
	}

	// log.Debugf("Readlink: successfully resolved %s -> %s", name, target)
	return target, nil
}

func isLinkDir(path string) (ok bool) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return true
	}
	if target, err := Readlink(path); err == nil {
		log.Debugf("is symlink %s to %s", path, target)
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			return true
		}
	}
	return
}

func hasFolder(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}

		file := filepath.Join(path, entry.Name())
		if target, err := Readlink(file); err == nil {
			fi, err := os.Stat(target)
			if err == nil && fi.IsDir() {
				return true
			}
		}
	}
	return false
}

func isEmptyFolder(folderPath string) (bool, error) {
	f, err := os.Open(folderPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, nil
}

func lsr0(root string) (files []string) {
	if root == "" {
		return
	}

	visited := make(map[string]bool)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return
	}

	// Собираем элементы первого уровня
	var firstLevelEntries []struct {
		name      string
		fullPath  string
		isSymlink bool
		target    string
		isDir     bool
	}

	// Читаем первый уровень
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return
	}

	// Обрабатываем каждый элемент первого уровня
	for _, entry := range entries {
		fullPath := filepath.Join(absRoot, entry.Name())

		if visited[fullPath] {
			continue
		}
		visited[fullPath] = true

		// Проверяем симлинки через Readlink
		var isSymlink bool
		var target string
		var isDir bool

		if linkTarget, err := Readlink(fullPath); err == nil {
			// Это симлинк
			isSymlink = true
			target = linkTarget
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(fullPath), target)
			}
			// Проверяем, является ли цель директорией
			if fi, err := os.Stat(target); err == nil {
				isDir = fi.IsDir()
			}
		} else {
			// Не симлинк
			isDir = entry.IsDir()
		}

		firstLevelEntries = append(firstLevelEntries, struct {
			name      string
			fullPath  string
			isSymlink bool
			target    string
			isDir     bool
		}{
			name:      entry.Name(),
			fullPath:  fullPath,
			isSymlink: isSymlink,
			target:    target,
			isDir:     isDir,
		})
	}

	// Обрабатываем каждый элемент первого уровня в порядке от os.ReadDir

	for _, firstLevelEntry := range firstLevelEntries {
		if firstLevelEntry.isSymlink {
			if firstLevelEntry.isDir {
				files = append(files, firstLevelEntry.name)
				// Симлинк на директорию - BFS обход
				target := firstLevelEntry.target

				if !visited[target] {
					visited[target] = true

					// BFS обход целевой директории
					queue := []string{target}

					for len(queue) > 0 {
						current := queue[0]
						queue = queue[1:]

						entries, err := os.ReadDir(current)
						if err != nil {
							continue
						}

						for _, e := range entries {
							eFullPath := filepath.Join(current, e.Name())

							if !visited[eFullPath] {
								visited[eFullPath] = true

								if e.IsDir() {
									queue = append(queue, eFullPath)
								} else {
									// Добавляем файл с абсолютным путем
									files = append(files, eFullPath)
								}
							}
						}
					}
				}
			} else {
				// Симлинк на файл - добавляем абсолютный путь цели
				files = append(files, firstLevelEntry.target)
			}
		} else if firstLevelEntry.isDir {
			files = append(files, firstLevelEntry.name)
			// Обычная директория - BFS обход
			queue := []string{firstLevelEntry.fullPath}

			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]

				entries, err := os.ReadDir(current)
				if err != nil {
					continue
				}

				for _, e := range entries {
					eFullPath := filepath.Join(current, e.Name())

					if !visited[eFullPath] {
						visited[eFullPath] = true

						if e.IsDir() {
							queue = append(queue, eFullPath)
						} else {
							// Добавляем файл с относительным путем
							relPath, err := filepath.Rel(absRoot, eFullPath)
							if err == nil {
								files = append(files, relPath)
							}
						}
					}
				}
			}
		} else {
			// Обычный файл в первом уровне - добавляем с относительным путем
			relPath, err := filepath.Rel(absRoot, firstLevelEntry.fullPath)
			if err == nil {
				files = append(files, relPath)
			}
		}
	}

	return files
}

func lsr2(root string) (files []string) {
	if root == "" {
		return
	}

	visited := make(map[string]bool)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return
	}

	// Проверяем корневую директорию на симлинк
	var startPath string

	if linkTarget, err := Readlink(absRoot); err == nil {
		// Корень - симлинк
		startPath = linkTarget
		if !filepath.IsAbs(startPath) {
			startPath = filepath.Join(filepath.Dir(absRoot), startPath)
		}
		// Если симлинк ведет на файл - добавляем его
		if fi, err := os.Stat(startPath); err == nil && !fi.IsDir() {
			files = append(files, absRoot)
		}
	} else {
		// Корень - не симлинк
		startPath = absRoot
	}

	// Начинаем обход с целевой директории
	queue := []string{startPath}
	visited[startPath] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(current)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			fullPath := filepath.Join(current, entry.Name())

			// Пропускаем если уже посещали
			if visited[fullPath] {
				continue
			}

			var isSymlink bool
			var target string
			var isDir bool

			// Проверяем симлинки
			if linkTarget, err := Readlink(fullPath); err == nil {
				isSymlink = true
				target = linkTarget
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(fullPath), target)
				}
				// Проверяем, является ли цель директорией
				if fi, err := os.Stat(target); err == nil {
					isDir = fi.IsDir()
				}
			} else {
				// Не симлинк
				isDir = entry.IsDir()
			}

			if isSymlink {
				if isDir {
					// Симлинк на директорию - добавляем в очередь для обхода
					if !visited[target] {
						visited[target] = true
						queue = append(queue, target)
					}
				} else {
					// Симлинк на файл - добавляем в результат
					files = append(files, fullPath)
				}
			} else if isDir {
				// Обычная директория - добавляем в очередь для обхода
				visited[fullPath] = true
				queue = append(queue, fullPath)
			} else {
				// Обычный файл - добавляем в результат
				files = append(files, fullPath)
			}
		}
	}

	return files
}

func lsr(root string) (files []string, names []string) {
	if root == "" {
		return
	}

	visited := make(map[string]bool)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return
	}

	// Собираем элементы первого уровня
	var firstLevelEntries []struct {
		name      string
		fullPath  string
		isSymlink bool
		target    string
		isDir     bool
	}

	// Читаем первый уровень
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return
	}

	// Обрабатываем каждый элемент первого уровня
	for _, entry := range entries {
		fullPath := filepath.Join(absRoot, entry.Name())

		if visited[fullPath] {
			continue
		}
		visited[fullPath] = true

		// Проверяем симлинки через Readlink
		var isSymlink bool
		var target string
		var isDir bool

		if linkTarget, err := Readlink(fullPath); err == nil {
			// Это симлинк
			isSymlink = true
			target = linkTarget
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(fullPath), target)
			}
			// Проверяем, является ли цель директорией
			if fi, err := os.Stat(target); err == nil {
				isDir = fi.IsDir()
			}
		} else {
			// Не симлинк
			isDir = entry.IsDir()
		}

		firstLevelEntries = append(firstLevelEntries, struct {
			name      string
			fullPath  string
			isSymlink bool
			target    string
			isDir     bool
		}{
			name:      entry.Name(),
			fullPath:  fullPath,
			isSymlink: isSymlink,
			target:    target,
			isDir:     isDir,
		})
	}

	// Обрабатываем каждый элемент первого уровня в порядке от os.ReadDir
	for _, firstLevelEntry := range firstLevelEntries {
		if firstLevelEntry.isSymlink {
			if firstLevelEntry.isDir {
				// Симлинк на директорию - добавляем оба пути
				files = append(files, firstLevelEntry.fullPath)
				names = append(names, firstLevelEntry.name)

				// BFS обход целевой директории
				target := firstLevelEntry.target

				if !visited[target] {
					visited[target] = true

					queue := []string{target}

					for len(queue) > 0 {
						current := queue[0]
						queue = queue[1:]

						entries, err := os.ReadDir(current)
						if err != nil {
							continue
						}

						for _, e := range entries {
							eFullPath := filepath.Join(current, e.Name())

							if !visited[eFullPath] {
								visited[eFullPath] = true

								if e.IsDir() {
									queue = append(queue, eFullPath)
								} else {
									// Добавляем файл с абсолютным путем в files
									files = append(files, eFullPath)

									// Создаем относительный путь для names
									// Относительный путь строится от корневой директории симлинка
									relPath := filepath.Join(firstLevelEntry.name, getRelativePath(target, eFullPath))
									names = append(names, filepath.FromSlash(relPath))
								}
							}
						}
					}
				}
			} else {
				// Симлинк на файл - добавляем абсолютный путь цели в files
				files = append(files, firstLevelEntry.target)
				// Для names используем имя симлинка
				names = append(names, firstLevelEntry.name)
			}
		} else if firstLevelEntry.isDir {
			// Обычная директория - добавляем оба пути
			files = append(files, firstLevelEntry.fullPath)
			names = append(names, firstLevelEntry.name)

			// BFS обход
			queue := []string{firstLevelEntry.fullPath}

			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]

				entries, err := os.ReadDir(current)
				if err != nil {
					continue
				}

				for _, e := range entries {
					eFullPath := filepath.Join(current, e.Name())

					if !visited[eFullPath] {
						visited[eFullPath] = true

						if e.IsDir() {
							queue = append(queue, eFullPath)
						} else {
							// Добавляем файл с абсолютным путем в files
							files = append(files, eFullPath)

							// Добавляем файл с относительным путем в names
							relPath, err := filepath.Rel(absRoot, eFullPath)
							if err == nil {
								names = append(names, filepath.FromSlash(relPath))
							}
						}
					}
				}
			}
		} else {
			// Обычный файл в первом уровне - добавляем оба пути
			files = append(files, firstLevelEntry.fullPath)

			relPath, err := filepath.Rel(absRoot, firstLevelEntry.fullPath)
			if err == nil {
				names = append(names, filepath.FromSlash(relPath))
			}
		}
	}

	return files, names
}

// Вспомогательная функция для получения относительного пути от базовой директории
func getRelativePath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
