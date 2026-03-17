// webdavlink.go
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
)

// ResolvingFileSystem implements webdav.FileSystem with symlink resolution
type ResolvingFileSystem struct {
	root string
}

// sanitizePath проверяет и очищает путь от попыток directory traversal
// Возвращает ошибку, если путь пытается выйти за пределы root через ..
func (fs *ResolvingFileSystem) sanitizePath(name string) error {
	if hasTraversal, _ := DetectPathTraversal(name); hasTraversal {
		log.Warnf("Path traversal attempt detected: %s", name)
		return os.ErrPermission
	}
	return nil
}

// isVirtualPath проверяет, является ли путь виртуальным (через псевдоссылку)
func (fs *ResolvingFileSystem) isVirtualPath(name string) bool {
	// Разбиваем путь на компоненты
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	current := fs.root

	for i, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)

		// Проверяем, является ли текущий компонент псевдоссылкой
		if _, err := Readlink(current); err == nil {
			// Если это ссылка и мы не в конце пути, то всё что дальше - виртуальное
			if i < len(parts)-1 {
				return true
			}
			// Если это ссылка и это последний компонент, то это виртуальный файл/директория
			return true
		}
	}

	// Проверяем, не является ли сам запрашиваемый путь псевдоссылкой
	fullPath := filepath.Join(fs.root, filepath.FromSlash(name))
	if _, err := Readlink(fullPath); err == nil {
		return true
	}

	return false
}

// resolvePath resolves symlinks in the entire path, including intermediate directories
func (fs *ResolvingFileSystem) resolvePath(ctx context.Context, name string) (string, error) {
	// Handle root case
	if name == "/" || name == "" {
		return fs.root, nil
	}

	parts := strings.Split(filepath.FromSlash(name), string(filepath.Separator))
	current := fs.root

	for i, part := range parts {
		if part == "" {
			continue
		}

		next := filepath.Join(current, part)

		// Check if this component is a symlink
		target, err := Readlink(next)
		if err == nil {
			// Resolve symlink
			if !filepath.IsAbs(target) {
				target = filepath.Join(current, target)
			}
			current = target

			// Append remaining parts
			remaining := filepath.Join(parts[i+1:]...)
			if remaining != "" {
				current = filepath.Join(current, remaining)
			}
			break
		}

		// Not a symlink, continue walking
		current = next
	}

	return current, nil
}

// Mkdir implements webdav.FileSystem
func (fs *ResolvingFileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	// Проверяем на directory traversal
	if err := fs.sanitizePath(name); err != nil {
		return err
	}

	// Check if this is a virtual path (read-only)
	if fs.isVirtualPath(name) {
		return os.ErrPermission
	}

	fullPath := filepath.Join(fs.root, filepath.FromSlash(name))

	// If parent is a symlink, create inside target
	parent := filepath.Dir(fullPath)
	if target, err := Readlink(parent); err == nil {
		return os.Mkdir(filepath.Join(target, filepath.Base(fullPath)), perm)
	}

	return os.Mkdir(fullPath, perm)
}

// OpenFile implements webdav.FileSystem
func (fs *ResolvingFileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	// Проверяем на directory traversal
	if err := fs.sanitizePath(name); err != nil {
		return nil, err
	}

	// Check for write operations on virtual paths
	isVirtual := fs.isVirtualPath(name)
	if isVirtual && (flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0) {
		return nil, os.ErrPermission
	}

	resolvedPath, err := fs.resolvePath(ctx, name)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(resolvedPath, flag, perm)
	if err != nil {
		return nil, err
	}

	return &ResolvingFile{
		file:         file,
		fs:           fs,
		name:         name,
		resolvedPath: resolvedPath,
		isVirtual:    isVirtual,
	}, nil
}

// RemoveAll implements webdav.FileSystem
func (fs *ResolvingFileSystem) RemoveAll(ctx context.Context, name string) error {
	// Проверяем на directory traversal
	if err := fs.sanitizePath(name); err != nil {
		return err
	}

	// Check if this is a virtual path (read-only)
	if fs.isVirtualPath(name) {
		return os.ErrPermission
	}

	resolvedPath, err := fs.resolvePath(ctx, name)
	if err != nil {
		return err
	}
	return os.RemoveAll(resolvedPath)
}

// Rename implements webdav.FileSystem
func (fs *ResolvingFileSystem) Rename(ctx context.Context, oldName, newName string) error {
	// Проверяем на directory traversal для обоих путей
	if err := fs.sanitizePath(oldName); err != nil {
		return err
	}
	if err := fs.sanitizePath(newName); err != nil {
		return err
	}

	// Check if either path is virtual (read-only)
	if fs.isVirtualPath(oldName) || fs.isVirtualPath(newName) {
		return os.ErrPermission
	}

	oldResolved, err := fs.resolvePath(ctx, oldName)
	if err != nil {
		return err
	}
	newResolved, err := fs.resolvePath(ctx, newName)
	if err != nil {
		return err
	}
	return os.Rename(oldResolved, newResolved)
}

// Stat implements webdav.FileSystem
func (fs *ResolvingFileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	// Проверяем на directory traversal
	if err := fs.sanitizePath(name); err != nil {
		return nil, err
	}

	resolvedPath, err := fs.resolvePath(ctx, name)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, err
	}

	// Check if this path is virtual (through symlinks)
	isVirtual := fs.isVirtualPath(name)

	// Always return ResolvedFileInfo with original base name
	baseName := filepath.Base(name)
	if baseName == "." || baseName == "" {
		baseName = "/"
	}

	// Add read-only flag for virtual files
	mode := info.Mode()
	if isVirtual {
		// Remove write permissions for virtual files
		mode &^= 0222 // Remove write bits for owner, group, others
	}

	return &ResolvedFileInfo{
		name:    baseName,
		size:    info.Size(),
		mode:    mode,
		modTime: info.ModTime(),
		isDir:   info.IsDir(),
		sys:     info.Sys(),
		virtual: isVirtual,
	}, nil
}

// ResolvingFile implements webdav.File with symlink resolution
type ResolvingFile struct {
	file         *os.File
	fs           *ResolvingFileSystem
	name         string
	resolvedPath string
	isVirtual    bool
}

// Readdir implements webdav.File
func (f *ResolvingFile) Readdir(count int) ([]os.FileInfo, error) {
	infos, err := f.file.Readdir(count)
	if err != nil {
		return nil, err
	}

	resolvedInfos := make([]os.FileInfo, 0, len(infos))
	for _, info := range infos {
		entryName := info.Name()
		targetPath := filepath.Join(f.resolvedPath, entryName)

		// Check if this specific entry is virtual
		isEntryVirtual := f.fs.isVirtualPath(filepath.Join(f.name, entryName))

		targetInfo, err := os.Stat(targetPath)
		if err != nil {
			targetInfo = info // fallback
		}

		// Set read-only mode for virtual entries
		mode := targetInfo.Mode()
		if isEntryVirtual {
			mode &^= 0222 // Remove write permissions
		}

		resolvedInfos = append(resolvedInfos, &ResolvedFileInfo{
			name:    entryName,
			size:    targetInfo.Size(),
			mode:    mode,
			modTime: targetInfo.ModTime(),
			isDir:   targetInfo.IsDir(),
			sys:     targetInfo.Sys(),
			virtual: isEntryVirtual,
		})
	}

	return resolvedInfos, nil
}

// Stat implements webdav.File
func (f *ResolvingFile) Stat() (os.FileInfo, error) {
	return f.fs.Stat(appCtx, f.name)
}

// Write implements webdav.File with read-only check
func (f *ResolvingFile) Write(p []byte) (int, error) {
	if f.isVirtual {
		return 0, os.ErrPermission
	}
	return f.file.Write(p)
}

// Delegate methods
func (f *ResolvingFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *ResolvingFile) Close() error {
	return f.file.Close()
}

func (f *ResolvingFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

// ResolvedFileInfo is a custom implementation of os.FileInfo
type ResolvedFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
	virtual bool
}

func (fi *ResolvedFileInfo) Name() string       { return fi.name }
func (fi *ResolvedFileInfo) Size() int64        { return fi.size }
func (fi *ResolvedFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *ResolvedFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *ResolvedFileInfo) IsDir() bool        { return fi.isDir }
func (fi *ResolvedFileInfo) Sys() interface{}   { return fi.sys }
