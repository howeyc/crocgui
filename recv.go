package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
)

func recvTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	var ti *container.TabItem
	refresh := func() {}
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel("")
	entry := widget.NewEntry()

	entryText := os.Getenv(CROC_SECRET)
	if entryText != "" {
		entry.SetText(entryText)
	}
	entry.SetPlaceHolder(lp("Enter code to download"))
	pasteCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		entry.SetText(a.Clipboard().Content())
	})

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-recv", a.Preferences()))
	totpLabel := widget.NewLabel(TOTP)
	var totpChan chan struct{}

	totpStop := func() {
		if totpChan != nil {
			close(totpChan)
			totpChan = nil
		}
	}
	totpProg := widget.NewProgressBar()
	totpProg.Hide()

	update := func() {
		fyne.Do(func() {
			if !totpCheck.Checked || totpCheck.Disabled() {
				return
			}

			totpLabel.SetText(totp(entry.Text))
			now := time.Now()
			remaining := 30 - now.Second()%30
			totpProg.SetValue(float64(remaining) / 30)
		})
	}

	totpCheck.OnChanged = func(b bool) {
		fyne.Do(func() {
			totpStop()
			if b {
				if strings.HasPrefix(entry.Text, TOTP) {
					entry.SetText(entryText)
				}
				totpProg.Show()
				update()

				totpChan = make(chan struct{})
				go func() {
					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-done:
							return
						case <-ticker.C:
							update() // Уже обернуто в fyne.Do
						case <-totpChan:
							return
						}
					}
				}()
			} else {
				totpLabel.SetText(TOTP)
				totpProg.Hide()
				entryText = entry.Text
				entry.SetText(TOTP + totp(entry.Text))
			}
			a.Preferences().SetBool("totp-recv", b)
		})
	}
	if totpCheck.Checked {
		totpCheck.OnChanged(true)
	}

	entry.OnChanged = func(secret string) {
		os.Setenv(CROC_SECRET, secret)
		update() // Уже обернуто в fyne.Do
	}

	recvDir := filepath.Join(os.TempDir(), "crocgui-recv")

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	removeEntry := func(fpath string, fe *fyne.Container, del bool) {
		fyne.Do(func() {
			boxholder.Remove(fe)
			boxholder.Refresh()
		})
		if del {
			remove := os.Remove
			file := "file"
			fi, _ := os.Stat(fpath)
			if fi != nil {
				if fi.IsDir() {
					remove = os.RemoveAll
					file = "dir"
				}
				log.Tracef("Removed received %s: %s error: %v", file, fpath, remove(fpath))
			}
		}
		delete(fileentries, fpath)
	}

	ShowFileLocation := func(src string, parent fyne.Window) {
		fe, ok := fileentries[src]
		if !ok {
			return
		}
		child := filepath.Base(src)

		fileSave := func(destination fyne.URIWriteCloser, err error) {
			var (
				u  fyne.URI
				cl = func() {}
			)
			if err != nil {
				log.Errorf("NewFileSave %v", err)
				fyne.Do(func() {
					topline.SetText(lp("Saved all files to") + " Download")
				})
			} else if destination == nil {
				log.Trace("User canceled folder selection")
				return
			}

			if destination == nil {
				u, cl, err = ChildDownload(child)
				if err != nil {
					log.Errorf("append child %s to Downloads: %v", child, err)
					return
				}
				destination, err = storage.Writer(u)
				if err != nil {
					cl()
					log.Errorf("creating writer from URI(%s): %v", u, err)
					return
				}
			}
			if !(isMobile || asMobile) {
				if fi, err := os.Stat(src); err == nil && fi.IsDir() {
					destination.Close()
					os.Remove(destination.URI().Path())
					err := Rename(src, destination.URI().Path())
					if err == nil {
						log.Tracef("File %s moved to %s", src, destination.URI().Path())
						removeEntry(src, fe, true)
					} else {
						log.Warnf("File %s not moved to %s error: %v", src, destination.URI().Path(), err)
					}
					return
				}
				err := Rename(src, destination.URI().Path())
				if err == nil {
					log.Tracef("File %s moved to %s", src, destination.URI().Path())
					removeEntry(src, fe, true)
					return
				} else {
					log.Warnf("File %s not moved to %s error: %v", src, destination.URI().Path(), err)
				}
			}

			copyToUWCProgress(destination, src, fe, func(err error) {
				cl()
				if err != nil {
					log.Errorf("Error saving %s to %s error:%v", src, destination.URI().Path(), err)
				} else {
					log.Tracef("File %s saved to %s", src, destination.URI().Path())
					removeEntry(src, fe, true)
				}
			})
		}

		supported, err := IsSaveDialogSupported()
		if err != nil {
			log.Errorf("Error checking folder picker support: %v", err)
			supported = false
		}
		if !supported {
			fileSave(nil, fmt.Errorf("file picker not supported"))
			log.Trace("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(MaterialFiles)
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				INSTALL,
				w,
			)
			return
		}
		savedialog := dialog.NewFileSave(fileSave, parent)
		savedialog.SetFileName(child)
		savedialog.Resize(parent.Canvas().Size())
		notFinish = true
		savedialog.Show()
	}

	// Добавим строчку в boxholder и fileentries
	addEntry := func(dst string, f func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label)) (newentry *fyne.Container) {
		if fe := fileentries[dst]; fe != nil {
			log.Tracef("exists %s", dst)
			deleteButton := fe.Objects[feDel]
			progFile := fe.Objects[feBar]
			saveButton := fe.Objects[feSave]
			labelFile := fe.Objects[len(fe.Objects)-1]
			fyne.Do(func() {
				if f == nil {
					deleteButton.Show()
					progFile.Hide()
					saveButton.Show()
				} else {
					f(deleteButton.(*widget.Button),
						progFile.(*widget.ProgressBar),
						saveButton.(*widget.Button),
						labelFile.(*widget.Label))
				}
			})
			return fe
		}
		base := filepath.Base(dst)
		if fi, _ := os.Stat(dst); fi != nil && fi.IsDir() {
			base += "/"
		}
		labelFile := widget.NewLabel(base)

		saveButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
			ShowFileLocation(dst, w)
		})

		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if fe, ok := fileentries[dst]; ok {
				removeEntry(dst, fe, true)
			}
		})
		progFile := widget.NewProgressBar()

		newentry = container.NewHBox(
			deleteButton,
			progFile,
			saveButton,
			labelFile,
		)

		fileentries[dst] = newentry
		fyne.Do(func() {
			if f == nil {
				progFile.Hide()
			} else {
				f(deleteButton, progFile, saveButton, labelFile)
			}
			boxholder.Add(newentry)
			boxholder.Refresh()
		})
		return
	}

	os.MkdirAll(recvDir, 0o700)
	fpath := a.Preferences().String("DeleteFile")
	for _, name := range ls(recvDir) {
		path := filepath.Join(recvDir, name)
		if name == "" {
			continue
		}
		if fpath == path {
			err := os.Remove(fpath)
			log.Tracef("Removed partially received file: %s error: %v", fpath, err)
			if err != nil {
				continue
			}
			a.Preferences().SetString("DeleteFile", "")
		} else {
			if target, err := Readlink(path); err == nil {
				path = target
			}
			addEntry(path, nil)
		}
	}

	reload = func() {
		for path, fe := range fileentries {
			if _, err := os.Stat(path); err != nil {
				removeEntry(path, fe, false)
			}
		}
		for _, name := range ls(recvDir) {
			if name != "" {
				path := filepath.Join(recvDir, name)
				if fpath == path {
					continue
				}
				if isMobile || asMobile {
					// Чтоб сохранять на Андроиде свернём каталог в  файл
					if fi, _ := os.Stat(path); fi != nil && fi.IsDir() {
						if err := utils.ZipDirectory(name+".zip", path); err == nil {
							path += ".zip"
						}
					}
				}
				addEntry(path, func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
					d.Show()
					p.Hide()
					s.Show()
				})
			}
		}
	}

	var lastSaveDir string

	cancelChan := make(chan struct{})

	var cancelButton, mainButton *widget.Button

	removeEntrys := func() {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe, true)
		}
	}

	saveAllFiles := func() {
		if len(fileentries) == 0 {
			log.Error("no files to save")
			return
		}

		filesSave := func(lu fyne.ListableURI, err error) {
			var (
				u  fyne.URI
				cl = func() {}
			)

			if err != nil {
				log.Errorf("ShowFolderOpen %v", err)
			} else if lu == nil {
				log.Trace("User canceled folder selection")
				return
			}

			for src, fe := range fileentries {
				child := filepath.Base(src)

				if lu != nil {
					lastSaveDir = lu.Path()
					u, cl, err = Child(lu, child)
					if err != nil {
						log.Errorf("Error append to URI(%s) child %s error: %v", lu, child, err)
						u, cl, err = ChildDownload(child)
					}
				} else {
					u, cl, err = ChildDownload(child)
					if p, err := storage.Parent(u); err == nil {
						lastSaveDir = p.Path()
						lu, _ = storage.ListerForURI(p)
					}
				}
				if err != nil {
					log.Errorf("Error append to Downloads child %s error: %v", child, err)
					continue
				}

				if !(isMobile || asMobile) {
					err := Rename(src, u.Path())
					if err == nil {
						log.Tracef("File %s moved to %s", src, u.Path())
						removeEntry(src, fe, true)
						fyne.Do(func() {
							if len(fileentries) == 0 {
								topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lastSaveDir))
							}
						})
						continue
					} else {
						log.Warnf("File %s not moved to %s error: %v", src, u.Path(), err)
					}
				}

				destination, err := storage.Writer(u)
				if err != nil {
					cl()
					log.Errorf("creating writer from URI(%s): %v", u, err)
					continue
				}

				copyToUWCProgress(destination, src, fe, func(err error) {
					cl()
					if err != nil {
						log.Errorf("Error saving URI(%s) to %s error:%v", src, destination.URI(), err)
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("Error saving %s: %v", child, err))
						})
						return
					}
					log.Tracef("File %s saved to URI(%s)", src, destination.URI().String())
					removeEntry(src, fe, true)

					if len(fileentries) == 0 {
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lastSaveDir))
						})
					}
				})
			}
		}

		supported, err := IsFolderPickerSupported()
		if err != nil {
			log.Errorf("Error checking folder picker support: %v", err)
			supported = false
		}
		if !supported {
			filesSave(nil, fmt.Errorf("folder picker not supported"))
			log.Trace("Folder picker not supported. ", INSTALL)
			a.Clipboard().SetContent(MaterialFiles)
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				INSTALL,
				w,
			)
			return
		}
		ShowFolderOpen(filesSave, w)
	}

	mainButton = widget.NewButtonWithIcon(lp("Download"), theme.DownloadIcon(), func() {
		ok := len(entry.Text) > 5
		if totpCheck.Checked {
			ok = len(entry.Text) > 0
		}
		if !ok {
			log.Error("no receive code entered\n")
			dialog.ShowInformation(
				lp("Download"),
				lp("Enter code to download"),
				w,
			)
			return
		}
		if len(fileentries) > 0 {
			dialog.ShowConfirm(
				lp("Delete All"),
				lp("Are you sure you want to delete all received files?"), func(b bool) {
					if b {
						removeEntrys()
					}
				},
				w)
			return
		}

		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			secret = TOTP + secret
		}

		receiver, err := croc.New(croc.Options{
			// IsSender:         false,
			SharedSecret:  secret,
			Debug:         debugBool(a),
			RelayAddress:  a.Preferences().String("relay-address"),
			RelayPassword: a.Preferences().String("relay-password"),
			// Stdout:           false,
			NoPrompt: true,
			// DisableLocal:     a.Preferences().Bool("disable-local"),
			// NoMultiplexing:   a.Preferences().Bool("disable-multiplexing"),
			OnlyLocal: a.Preferences().Bool("force-local"),
			// NoCompress:       a.Preferences().Bool("disable-compression"),
			Curve: a.Preferences().String("pake-curve"),
			// HashAlgorithm:    a.Preferences().String("croc-hash"),
			Overwrite:        true,
			MulticastAddress: a.Preferences().String("multicast-address"),
			ZipFolder:        true,
		})
		if err != nil {
			log.Errorf("croc setup error: %s\n", err.Error())
			return
		}
		log.SetLevel(debugString(a))
		log.Trace("croc receiver created")

		cderr := os.Chdir(recvDir)
		if cderr != nil {
			log.Error("Unable to change to dir:", recvDir, cderr)
		}
		log.Trace("cd ", recvDir)

		var filename string
		mainButton.Disable()
		prog.Show()
		cancelChan = make(chan struct{})
		cancelButton.Show()
		entry.Disable()

		totpCheck.Disable()
		if totpCheck.Checked {
			totpProg.Hide()
		}
		refresh()

		doneChan := make(chan struct{})
		fpath := ""

		//progress
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer func() {
				ticker.Stop()
				//reset
				fyne.Do(func() {
					mainButton.Enable()
					prog.Hide()
					prog.SetValue(0)
					cancelButton.Hide()
					entry.Enable()

					totpCheck.Enable()
					if totpCheck.Checked {
						totpProg.Show()
					}

					reload()
					refresh()
				})
			}()

			old := 0
			oldPath := ""
			// oldTempFile := false
			progW := NewProgressWrapper(prog)
			toplineW := NewLabelWrapper(topline)
			var TotalSent, size, totalMax int64
			fepw := NewProgressWrapper(nil)

			once := true
			for {
				select {
				case <-done:
					return
				case <-doneChan:
					return
				case <-cancelChan:
					s := fmt.Sprintf("%s %s", lp("Receive cancelled."), filename)
					log.Error(s)
					fyne.Do(func() {
						topline.SetText(s)
					})
					a.Preferences().SetString("DeleteFile", filepath.Join(recvDir, filename))
					Stop(receiver)
					fyne.Do(func() {
						restart(a, w)
					})
					return
				case <-ticker.C:
					if receiver == nil {
						return
					}
					if receiver.Step2FileInfoTransferred {
						if once {
							once = false
							for i, fi := range receiver.FilesToTransfer {
								if isMobile || asMobile {
									// Запретим разворачивать свёрнутый каталог
									receiver.FilesToTransfer[i].TempFile = false
								} else {
									// Развернём свёрнутый каталог если включена опция zip-unzip
									receiver.FilesToTransfer[i].TempFile = strings.HasSuffix(strings.ToLower(fi.Name), ".zip") &&
										a.Preferences().Bool("zip-unzip")
								}
								dst := filepath.Join(recvDir, fi.Name)
								addEntry(dst, func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
									d.Hide()
									p.SetValue(0)
									p.Max = float64(fi.Size)
									p.Show()
									s.Hide()
									if fi.FolderRemote != "." {
										l.SetText(fi.FolderRemote + "/" + fi.Name)
									}
								})
								totalMax += fi.Size
							}
							fyne.Do(refresh)
							progW.SetMax(totalMax)
						}
						cnum := receiver.FilesToTransferCurrentNum
						if old < cnum+1 {
							old = cnum + 1
							fi := receiver.FilesToTransfer[cnum]
							filename = fi.Name
							toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Receiving file"), filename, cnum+1, len(receiver.FilesToTransfer)))
							TotalSent += size
							size = fi.Size
							path := filepath.Join(recvDir, fi.Name)
							if oldPath != path {
								if fe := fileentries[oldPath]; fe != nil {
									fyne.Do(func() {
										fe.Objects[feDel].Show()
										fe.Objects[feBar].Hide()
										fe.Objects[feSave].Show()
									})
								}
								oldPath = path
							}
							log.Trace(path)
							if fe := fileentries[path]; fe != nil {
								fepw = NewProgressWrapper(fe.Objects[feBar].(*widget.ProgressBar))
							} else {
								fepw = NewProgressWrapper(nil)
							}
						}
						progW.SetValue(TotalSent + receiver.TotalSent)
						fepw.SetValue(receiver.TotalSent)
					}
				}
			}
		}()

		// receiver.Receive
		go func() {
			var rerr error
			if EMULATE == 0 {
				rerr = receiver.Receive()
			} else {
				log.Warnf("Receive")
				time.Sleep(EMULATE)
				defer func() {
					time.Sleep(time.Millisecond * 10)
					receiver = nil
				}()
			}
			fyne.Do(func() {
				if rerr != nil {
					if errors.Is(rerr, io.EOF) {
						rerr = fmt.Errorf("%s", lp("Send cancelled."))
					}
					s := fmt.Sprintf("Receive failed: %s", rerr)
					log.Error(s)
					topline.SetText(s)
					fpath = filepath.Join(recvDir, filename)
					removeEntrys()
				} else {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Received"), filename))
				}
				a.Preferences().SetString("DeleteFile", fpath)
			})
			close(doneChan)
		}()

		//  +2 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(cancelChan)
	})
	cancelButton.Hide()

	deleteAllButton := widget.NewButtonWithIcon("*", theme.DeleteIcon(), func() { //lp("Delete All")
		if len(fileentries) > 0 {
			removeEntrys()
		} else {
			entry.SetText("")
		}
	})

	saveAllButton := widget.NewButtonWithIcon("*", theme.FolderOpenIcon(), func() { //lp("Save All")
		saveAllFiles()
	})

	top := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer(), pasteCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Receive Code"), Widget: entry}),
		container.NewHBox(
			totpCheck,
			totpLabel,
			totpProg,
			layout.NewSpacer(),
			saveAllButton,
			deleteAllButton,
		),
		mainButton,
		prog,
		cancelButton,
	)
	ti = container.NewTabItemWithIcon(lp("Receive"), theme.DownloadIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
	refresh = func() { ti.Content.Refresh() }
	return ti

}

func copyToUWCProgress(destination fyne.URIWriteCloser, src string, c *fyne.Container, onComplete func(err error)) {
	if destination == nil {
		onComplete(fmt.Errorf("destination is nil (dialog closed)"))
		return
	}

	source, err := os.Open(src)
	if err != nil {
		destination.Close()
		onComplete(fmt.Errorf("failed to open source file: %v", err))
		return
	}

	fi, err := os.Stat(src)
	if err != nil {
		destination.Close()
		source.Close()
		onComplete(err)
		return
	}

	pw, restore := NewProgressWriter(destination, fi.Size(), c)

	go func() {
		_, err := io.Copy(pw, source)
		source.Close()
		destination.Close()
		restore()
		onComplete(err)
	}()
}

// Большой диалог для десктопа
func ShowFolderOpen(callback func(fyne.ListableURI, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFolderOpen(callback, parent)
		return
	}
	fd := dialog.NewFolderOpen(callback, parent)
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}

func Rename(src, dst string) error {
	// Check if source path exists
	srcStat, err := os.Stat(src)
	if err != nil {
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
	}

	// If standard rename failed, use copy approach
	if srcStat.IsDir() {
		return renameDir(src, dst)
	}

	return renameFile(src, dst)
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
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	// Copy file content
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst) // cleanup on error
		return err
	}

	// Close destination file before chmod
	if err := dstFile.Close(); err != nil {
		os.Remove(dst) // cleanup on error
		return err
	}

	// Copy file permissions
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		os.Remove(dst) // cleanup on error
		return err
	}

	// Remove source file
	return os.Remove(src)
}

func copyDirectory0(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath, info.Mode())
	})
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
		return err
	}

	if err := dstFile.Close(); err != nil {
		return err
	}

	return os.Chmod(dst, mode)
}
