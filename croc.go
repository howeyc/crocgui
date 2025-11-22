package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
	. "github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
)

// helper function to walk each subfolder and parses against an ignore file.
// returns a hashmap Key: Absolute filepath, Value: boolean (true=ignore)
func gitWalk(dir string, gitObj *ignore.GitIgnore, files map[string]bool) {
	var ignoredDir bool
	var current string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if isChild(current, path) && ignoredDir {
			files[path] = true
			return nil
		}
		if info.IsDir() && filepath.Base(path) == filepath.Base(dir) {
			ignoredDir = false // Skip applying ignore rules for root directory
			return nil
		}
		if gitObj.MatchesPath(info.Name()) {
			files[path] = true
			ignoredDir = true
			current = path
			return nil
		} else {
			files[path] = false
			ignoredDir = false
			return nil
		}
	})
	if err != nil {
		log.Errorf("filepath error")
	}
}

func isChild(parentPath, childPath string) bool {
	relPath, err := filepath.Rel(parentPath, childPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(relPath, "..")
}

// This function retrieves the important file information
// for every file that will be transferred
func getFilesInfo(fnames []string, zipfolder bool, ignoreGit bool, exclusions []string) (filesInfo []FileInfo, emptyFolders []FileInfo, totalNumberFolders int, err error) {
	// fnames: the relative/absolute paths of files/folders that will be transferred
	totalNumberFolders = 0
	var paths []string
	for _, fname := range fnames {
		// Support wildcard
		if strings.Contains(fname, "*") {
			matches, errGlob := filepath.Glob(fname)
			if errGlob != nil {
				err = errGlob
				return
			}
			paths = append(paths, matches...)
			continue
		} else {
			paths = append(paths, fname)
		}
	}
	ignoredPaths := make(map[string]bool)
	if ignoreGit {
		wd, wdErr := os.Stat(".gitignore")
		if wdErr == nil {
			gitIgnore, gitErr := ignore.CompileIgnoreFile(wd.Name())
			if gitErr == nil {
				for _, path := range paths {
					abs, absErr := filepath.Abs(path)
					if absErr != nil {
						err = absErr
						return
					}
					if gitIgnore.MatchesPath(path) {
						ignoredPaths[abs] = true
					}
				}
			}
		}
		for _, path := range paths {
			abs, absErr := filepath.Abs(path)
			if absErr != nil {
				err = absErr
				return
			}
			file, fileErr := os.Stat(path)
			if fileErr == nil && file.IsDir() {
				_, subErr := os.Stat(filepath.Join(path, ".gitignore"))
				if subErr == nil {
					gitObj, gitObjErr := ignore.CompileIgnoreFile(filepath.Join(path, ".gitignore"))
					if gitObjErr != nil {
						err = gitObjErr
						return
					}
					gitWalk(abs, gitObj, ignoredPaths)
				}
			}
		}
	}
	for _, fpath := range paths {
		stat, errStat := os.Lstat(fpath)

		if errStat != nil {
			err = errStat
			return
		}

		absPath, errAbs := filepath.Abs(fpath)

		if errAbs != nil {
			err = errAbs
			return
		}
		if stat.IsDir() && zipfolder {
			if fpath[len(fpath)-1:] != "/" {
				fpath += "/"
			}
			fpath = filepath.Dir(fpath)
			dest := filepath.Base(fpath) + ".zip"
			ZipDirectory(dest, fpath)
			utils.MarkFileForRemoval(dest)
			stat, errStat = os.Lstat(dest)
			if errStat != nil {
				err = errStat
				return
			}
			absPath, errAbs = filepath.Abs(dest)
			if errAbs != nil {
				err = errAbs
				return
			}

			fInfo := FileInfo{
				Name:         stat.Name(),
				FolderRemote: "./",
				FolderSource: filepath.Dir(absPath),
				Size:         stat.Size(),
				ModTime:      stat.ModTime(),
				Mode:         stat.Mode(),
				TempFile:     true,
				IsIgnored:    ignoredPaths[absPath],
			}
			if fInfo.IsIgnored {
				continue
			}
			filesInfo = append(filesInfo, fInfo)
			continue
		}

		if stat.IsDir() {
			err = filepath.Walk(absPath,
				func(pathName string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					absPathWithSeparator := filepath.Dir(absPath)
					if !strings.HasSuffix(absPathWithSeparator, string(os.PathSeparator)) {
						absPathWithSeparator += string(os.PathSeparator)
					}
					if strings.HasSuffix(absPathWithSeparator, string(os.PathSeparator)+string(os.PathSeparator)) {
						absPathWithSeparator = strings.TrimSuffix(absPathWithSeparator, string(os.PathSeparator))
					}
					remoteFolder := strings.TrimPrefix(filepath.Dir(pathName), absPathWithSeparator)
					if !info.IsDir() {
						fInfo := FileInfo{
							Name:         info.Name(),
							FolderRemote: strings.ReplaceAll(remoteFolder, string(os.PathSeparator), "/") + "/",
							FolderSource: filepath.Dir(pathName),
							Size:         info.Size(),
							ModTime:      info.ModTime(),
							Mode:         info.Mode(),
							TempFile:     false,
							IsIgnored:    ignoredPaths[pathName],
						}
						if fInfo.IsIgnored && ignoreGit {
							return nil
						} else {
							filesInfo = append(filesInfo, fInfo)
						}
					} else {
						if ignoredPaths[pathName] {
							return filepath.SkipDir
						}
						isEmptyFolder, _ := isEmptyFolder(pathName)
						totalNumberFolders++
						if isEmptyFolder {
							emptyFolders = append(emptyFolders, FileInfo{
								// Name: info.Name(),
								FolderRemote: strings.ReplaceAll(strings.TrimPrefix(pathName,
									filepath.Dir(absPath)+string(os.PathSeparator)), string(os.PathSeparator), "/") + "/",
							})
						}
					}
					return nil
				})
			if err != nil {
				return
			}

		} else {
			fInfo := FileInfo{
				Name:         stat.Name(),
				FolderRemote: "./",
				FolderSource: filepath.Dir(absPath),
				Size:         stat.Size(),
				ModTime:      stat.ModTime(),
				Mode:         stat.Mode(),
				TempFile:     false,
				IsIgnored:    ignoredPaths[absPath],
			}
			if fInfo.IsIgnored && ignoreGit {
				continue
			} else {
				filesInfo = append(filesInfo, fInfo)
			}
		}
	}
	return
}

func ZipDirectory(destination string, source string) (err error) {
	if _, err = os.Stat(destination); err == nil {
		log.Errorf("%s file already exists!\n", destination)
		return fmt.Errorf("file already exists: %s", destination)
	}

	// Check if source directory exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		log.Errorf("Source directory does not exist: %s", source)
		return fmt.Errorf("source directory does not exist: %s", source)
	}

	fmt.Fprintf(os.Stderr, "Zipping %s to %s\n", source, destination)
	file, err := os.Create(destination)
	if err != nil {
		log.Error(err)
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	// no compression because croc does its compression on the fly
	writer.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})
	defer writer.Close()

	// Get base name for zip structure
	baseName := strings.TrimSuffix(filepath.Base(destination), ".zip")

	// First pass: add the root directory with its modification time
	rootInfo, err := os.Stat(source)
	if err == nil && rootInfo.IsDir() {
		header, err := zip.FileInfoHeader(rootInfo)
		if err != nil {
			log.Error(err)
		} else {
			header.Name = baseName + "/" // Trailing slash indicates directory
			header.Method = zip.Store
			header.Modified = rootInfo.ModTime()

			_, err = writer.CreateHeader(header)
			if err != nil {
				log.Error(err)
			} else {
				fmt.Fprintf(os.Stderr, "\r\033[2K")
				fmt.Fprintf(os.Stderr, "\rAdding %s", baseName+"/")
			}
		}
	}

	// Second pass: add all other directories and files
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error(err)
			return nil
		}

		// Skip root directory (we already added it)
		if path == source {
			return nil
		}

		// Calculate relative path from source directory
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			log.Error(err)
			return nil
		}

		// Create zip path with base name structure
		zipPath := filepath.Join(baseName, relPath)
		zipPath = filepath.ToSlash(zipPath)

		if info.IsDir() {
			// Add directory entry to zip with original modification time
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				log.Error(err)
				return nil
			}
			header.Name = zipPath + "/" // Trailing slash indicates directory
			header.Method = zip.Store
			// Preserve the original modification time
			header.Modified = info.ModTime()

			_, err = writer.CreateHeader(header)
			if err != nil {
				log.Error(err)
				return nil
			}

			fmt.Fprintf(os.Stderr, "\r\033[2K")
			fmt.Fprintf(os.Stderr, "\rAdding %s", zipPath+"/")
			return nil
		}

		if info.Mode().IsRegular() {
			f1, err := os.Open(path)
			if err != nil {
				log.Error(err)
				return nil
			}
			defer f1.Close()

			// Create file header with modified time
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				log.Error(err)
				return nil
			}
			header.Name = zipPath
			header.Method = zip.Deflate

			w1, err := writer.CreateHeader(header)
			if err != nil {
				log.Error(err)
				return nil
			}

			if _, err := io.Copy(w1, f1); err != nil {
				log.Error(err)
				return nil
			}

			fmt.Fprintf(os.Stderr, "\r\033[2K")
			fmt.Fprintf(os.Stderr, "\rAdding %s", zipPath)
		}
		return nil
	})

	if err != nil {
		log.Error(err)
		return fmt.Errorf("error during directory walk: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\n")
	return nil
}
