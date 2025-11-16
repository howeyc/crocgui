// recv.go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	log "github.com/schollz/logger"
)

func recvTabItem(a fyne.App, w fyne.Window, parent *container.AppTabs) (ti *container.TabItem) {
	var (
		cosED, cosSH             []fyne.CanvasObject
		addEntry                 func(dst string, f func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label)) (newentry *fyne.Container)
		cancelButton, mainButton *widget.Button
	)

	showPage := func() {}
	reload := func() {}
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()

	prog := widget.NewProgressBar()
	cosSH = append(cosSH, prog)

	topline := widget.NewLabel(lp("Wait for them before pressing Download"))
	entry := widget.NewEntryWithData(binding.BindPreferenceString("secret", a.Preferences()))
	cosED = append(cosED, entry)

	entryText := os.Getenv(CROC_SECRET)
	if entryText != "" {
		entry.SetText(entryText)
	}

	pasteCodeButton := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		entry.SetText(a.Clipboard().Content())
	})
	cosED = append(cosED, pasteCodeButton)

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-recv", a.Preferences()))
	cosED = append(cosED, totpCheck)

	totpLabel := widget.NewLabel(TOTP)
	totpProg := setupTOTP(a, entry, totpCheck, totpLabel, &entryText)

	recvDir := filepath.Join(tempDir, RECV)

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	var fileentries sync.Map

	recvReady = func() (ok bool) {
		ok = true
		fileentries.Range(func(key, value interface{}) bool {
			fe := value.(*fyne.Container)
			if fe == nil {
				ok = false
				return false
			}
			if len(fe.Objects) <= feBar {
				ok = false
				return false
			}
			if fe.Objects[feBar].Visible() {
				ok = false
				return false
			}
			return true
		})
		return ok
	}

	removeEntry := func(fpath string, fe *fyne.Container, del bool) {
		if del {
			remove := os.Remove
			de := "file"
			fi, _ := os.Stat(fpath)
			if fi != nil {
				if fi.IsDir() {
					remove = os.RemoveAll
					de = "dir"
				}

				if err := remove(fpath); err != nil {
					log.Errorf("remove %s %s: %v", de, fpath, err)
					return
				} else {
					log.Tracef("remove %s %s", de, fpath)
				}
			}
		}
		fileentries.Delete(fpath)
		fyne.Do(func() {
			boxholder.Remove(fe)
			boxholder.Refresh()
		})
	}

	ShowFileSave := func(src string, parent fyne.Window) {
		fe, ok := load(&fileentries, src)
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
				log.Errorf("folder selection: %v", err)
				fyne.Do(func() {
					topline.SetText(lp("Saved all files to") + " Download")
				})
			} else if destination == nil {
				log.Trace("folder selection canceled")
				return
			}

			if destination == nil {
				u, cl, err = ChildDownload(child)
				if err != nil {
					log.Errorf("Downloads/%s: %v", child, err)
					return
				}
				destination, err = storage.Writer(u)
				if err != nil {
					cl()
					log.Errorf("writer %s: %v", u, err)
					return
				}
			} else {
				u = destination.URI()
			}

			dst := u.Path()
			if !(isMobile || asMobile) {
				destination.Close()
				// файлпикер создаёт файл даже для каталога
				storage.Delete(u)
				fi, err := os.Stat(src)
				if err != nil {
					log.Errorf("stat %s: %v", src, err)
					return
				}
				err = Rename(src, dst)
				if err == nil {
					log.Tracef("move %s %s", src, dst)
					removeEntry(src, fe, false)
					return
				}
				log.Warnf("move %s %s: %v", src, dst, err)
				// fileSave
				root := src
				copyFiles(storage.NewFileURI(src), dst, func(u fyne.URI, dstPath string) error {
					fyne.Do(func() {})
					feCopy := fe
					src := u.Path()
					if fi.IsDir() {
						// Создаю временный прогрессбар
						rel, err := filepath.Rel(join(), src)
						if err != nil {
							rel = src
						}
						feCopy = addEntry(src, func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
							l.SetText(rel)
						})
					}
					CopyFileProgress(src, dstPath, feCopy, func(err error) {
						if err != nil {
							log.Errorf("copy %s %s: %v", src, dstPath, err)
							removeEntry(src, feCopy, false)
							return
						}

						if _, err := os.Stat(dstPath); err != nil {
							// не сохранилось
							log.Errorf("stat %s: %v", dstPath, err)
						} else {
							// сохранилось, удаляем
							log.Tracef("copy %s %s", src, dstPath)
							removeEntry(src, feCopy, true)
							if feCopy != fe {
								if os.Remove(filepath.Dir(src)) == nil {
									_, err := os.Stat(root)
									exists := err == nil
									if !exists || os.Remove(root) == nil {
										if feRoot, ok := load(&fileentries, root); ok {
											removeEntry(root, feRoot, false)
										}
									}
								}
							}
						}
					})
					return nil
				})
				return
			}

			copyToUWCProgress(destination, src, fe, func(err error) {
				cl()
				if err != nil {
					log.Errorf("copy %s %s: %v", src, dst, err)
				} else {
					log.Tracef("copy %s %s", src, dst)
					removeEntry(src, fe, true)
				}
			})
		}

		supported, err := IsSaveDialogSupported()
		if err != nil {
			log.Errorf("file picker: %v", err)
			supported = false
		}
		if !supported {
			fileSave(nil, fmt.Errorf("file picker not supported"))
			log.Trace("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(filePicker)
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
		savedialog.SetLocation(lastLU)
		notFinish = true
		savedialog.Show()
	}

	// Добавим строчку в boxholder и fileentries
	addEntry = func(dst string, f func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label)) (newentry *fyne.Container) {
		if fe, ok := load(&fileentries, dst); ok {
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
			base += slash
		}
		labelFile := widget.NewLabel(base)

		saveButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
			if isMobile || asMobile {
				// На Андроиде свернём каталог в  файл
				if isLinkDir(dst) {
					pathZip := dst + DOTZIP
					if _, err := os.Stat(pathZip); err == nil {
						log.Errorf("exists %s", pathZip)
						return
					}

					if fe := addEntry(pathZip, nil); fe != nil {
						ZipDirectoryProgress(pathZip, dst, fe, func(err error) {
							if err != nil {
								log.Errorf("zip %s %s: %v", dst, pathZip, err)
								removeEntry(pathZip, fe, true)
								return
							}
							log.Tracef("zip %s %s", dst, pathZip)

							if _, err := os.Stat(pathZip); err != nil {
								log.Errorf("stat %s: %v", pathZip, err)
								return
							}

							if feDir, ok := load(&fileentries, dst); ok {
								removeEntry(dst, feDir, true)
							}
							fyne.Do(func() {
								ShowFileSave(pathZip, w)
							})
						})
					}
					return
				}
			}
			ShowFileSave(dst, w)
		})

		deleteButton := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
			if fe, ok := load(&fileentries, dst); ok {
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

		fileentries.Store(dst, newentry)
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

	fpath := a.Preferences().String("DeleteFile")
	os.MkdirAll(recvDir, 0700)

	reload = func() {
		for _, name := range ls(recvDir) {
			if name != "" {
				path := filepath.Join(recvDir, name)
				if fpath == path {
					continue
				}
				if isLinkDir(path) {
					name += slash
				}
				if isDir, count, _ := fileChild(path); isDir && count < 1 {
					log.Tracef("remove empty dir %s: %v", path, os.Remove(path))
					continue
				}
				addEntry(path, func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
					d.Show()
					p.Hide()
					s.Show()
					l.SetText(name)
				})
			}
		}
		forEachFileEntry(&fileentries, func(path string, fe *fyne.Container) {
			if _, err := os.Stat(path); err != nil {
				removeEntry(path, fe, false)
			}
		})
	}
	OnSelectedReload[1] = reload

	for _, name := range ls(recvDir) {
		path := filepath.Join(recvDir, name)
		if name == "" {
			continue
		}
		if fpath == path {
			err := os.Remove(fpath)
			log.Tracef("Removed partially received %s: %v", fpath, err)
			if err != nil {
				continue
			}
			a.Preferences().SetString("DeleteFile", "")
		}
	}

	cancelChan := make(chan struct{})

	removeEntrys := func() {
		forEachFileEntry(&fileentries, func(fpath string, fe *fyne.Container) {
			removeEntry(fpath, fe, true)
		})
	}

	filesSave := func(lu fyne.ListableURI, err error) {
		var (
			u  fyne.URI
			cl = func() {}
		)

		if err != nil {
			log.Errorf("folder selection: %v", err)
		} else if lu == nil {
			log.Trace("folder selection canceled")
			return
		}

		forEachFileEntry(&fileentries, func(src string, fe *fyne.Container) {
			child := filepath.Base(src)
			if (isMobile || asMobile) && isLinkDir(src) {
				child += DOTZIP
			}

			if lu != nil {
				lastLU = lu
				u, cl, err = Child(lu, child)
				if err != nil {
					log.Errorf("%s/%s: %v", lu, child, err)
					u, cl, err = ChildDownload(child)
				}
			} else {
				u, cl, err = ChildDownload(child)
				if p, err := storage.Parent(u); err == nil {
					lu, err = storage.ListerForURI(p)
					if err != nil {
						lastLU = lu
					}
				}
			}
			if err != nil {
				log.Errorf("Downloads/%s: %v", child, err)
				return
			}

			dst := u.Path()
			if !(isMobile || asMobile) {
				fi, err := os.Stat(src)
				if err != nil {
					log.Errorf("stat %s: %v", src, err)
					return
				}
				err = Rename(src, dst)
				if err == nil {
					log.Tracef("move %s %s", src, dst)
					removeEntry(src, fe, true)
					fyne.Do(func() {
						if mapEmpty(&fileentries) {
							topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), filepath.Dir(dst)))
						}
					})
					return
				}
				log.Warnf("move %s %s: %v", src, dst, err)
				root := src
				copyFiles(storage.NewFileURI(root), dst, func(u fyne.URI, dstPath string) error {
					fyne.Do(func() {})
					feCopy := fe
					src := u.Path()
					if fi.IsDir() {
						// Создаю временный прогрессбар
						rel, err := filepath.Rel(join(), src)
						if err != nil {
							rel = src
						}
						feCopy = addEntry(src, func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
							l.SetText(rel)
						})
					}
					CopyFileProgress(src, dstPath, feCopy, func(err error) {
						if err != nil {
							log.Errorf("copy %s %s: %v", src, dstPath, err)
							removeEntry(src, feCopy, false)
							return
						}

						if _, err := os.Stat(dstPath); err != nil {
							// не сохранилось
							log.Errorf("stat %s: %v", dstPath, err)
						} else {
							// сохранилось, удаляем
							log.Tracef("copy %s %s", src, dstPath)
							removeEntry(src, feCopy, true)
							if feCopy != fe {
								if os.Remove(filepath.Dir(src)) == nil {
									log.Tracef("%s---------------------------------------------------------------------------------------------", src)
									_, err := os.Stat(root)
									exists := err == nil
									if !exists || os.Remove(root) == nil {
										if feRoot, ok := load(&fileentries, root); ok {
											removeEntry(root, feRoot, false)
										}
										log.Tracef("%s******************************************************************************************,root")
									}
								}
							}
						}
					})
					return nil
				})
				return
			}

			destination, err := storage.Writer(u)
			if err != nil {
				cl()
				log.Errorf("writer %s: %v", u, err)
				return
			}

			copyFrom := func(src string) {
				copyToUWCProgress(destination, src, fe, func(err error) {
					cl()
					if err != nil {
						log.Errorf("copy %s %s: %v", src, destination.URI(), err)
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("Error saving %s: %v", child, err))
						})
						return
					}
					log.Tracef("copy %s %s", src, destination.URI())
					removeEntry(src, fe, true)

					if mapEmpty(&fileentries) {
						name := uriBase(destination.URI())
						parent := strings.TrimSuffix(destination.URI().String(), name)
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), parent))
						})
					}
				})
			}
			if isLinkDir(src) {
				// На Андроиде свернём каталог в файл
				pathZip := src + DOTZIP
				if _, err := os.Stat(pathZip); err == nil {
					log.Errorf("exists %s", pathZip)
					return
				}
				ZipDirectoryProgress(pathZip, src, fe, func(err error) {
					if err != nil {
						log.Errorf("zip %s %s: %v", src, pathZip, err)
						removeEntry(pathZip, fe, true)
						return
					}

					if _, err := os.Stat(pathZip); err != nil {
						log.Errorf("stat %s: %v", pathZip, err)
						return
					}
					log.Tracef("zip %s %s", src, pathZip)

					if feDir, ok := load(&fileentries, src); ok {
						removeEntry(src, feDir, true)
					}
					copyFrom(pathZip)
				})
				return
			}
			copyFrom(src)
		})
	}

	ShowFilesSave := func() {
		if mapEmpty(&fileentries) {
			log.Error("no files to save")
			return
		}

		supported, err := IsFolderPickerSupported()
		if err != nil {
			log.Errorf("folder picker: %v", err)
			supported = false
		}
		if !supported {
			// Нет фолдерпикера
			filesSave(nil, fmt.Errorf("folder picker not supported"))
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				"",
				w,
			)
			return
		}
		ShowFolderOpen(filesSave, w)
	}

	mainButton = widget.NewButtonWithIcon(lp("Download"), theme.DownloadIcon(), func() {
		if entry.Validate() != nil {
			log.Error("no receive code entered\n")
			dialog.ShowInformation(
				lp("Download"),
				lp("Secret must be longer than 5 characters"),
				w,
			)
			return
		}

		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			secret = TOTP + secret
		}

		receiver, err := croc.New(croc.Options{
			SharedSecret:     secret,
			Debug:            debugBool(a),
			RelayAddress:     a.Preferences().String("relay-address"),
			RelayPassword:    a.Preferences().String("relay-password"),
			NoPrompt:         true,
			OnlyLocal:        a.Preferences().Bool("force-local"),
			Curve:            a.Preferences().String("pake-curve"),
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
		cancelChan = make(chan struct{})
		allShow(true, cosSH...)
		allEnabled(false, cosED...)
		if totpCheck.Checked {
			totpProg.Hide()
		}

		doneChan := make(chan struct{})
		fpath := ""

		//progress
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer func() {
				// Взял
				ticker.Stop()
				fyne.Do(func() {
					prog.SetValue(0)
					allShow(false, cosSH...)
					allEnabled(true, cosED...)
					if totpCheck.Checked {
						totpProg.Show()
					}
					showPage()
					reload()
				})
			}()

			old := 0
			oldPath := ""
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
						restart(w)
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
									receiver.FilesToTransfer[i].TempFile = strings.HasSuffix(strings.ToLower(fi.Name), DOTZIP) &&
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
										l.SetText(fi.FolderRemote + slash + fi.Name)
									}
								})
								totalMax += fi.Size
							}
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
								if fe, ok := load(&fileentries, oldPath); ok {
									fyne.Do(func() {
										fe.Objects[feDel].Show()
										fe.Objects[feBar].Hide()
										fe.Objects[feSave].Show()
									})
								}
								oldPath = path
							}
							log.Trace(path)
							if fe, ok := load(&fileentries, path); ok {
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
	cosED = append(cosED, mainButton)

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(cancelChan)
	})
	cosSH = append(cosSH, cancelButton)
	allShow(false, cosSH...)

	saveAllButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
		ShowFilesSave()
	})
	cosED = append(cosED, saveAllButton)

	deleteAllButton := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		fyne.Do(func() {
			if mapEmpty(&fileentries) {
				entry.SetText(entryText)
			} else {
				removeEntrys()
			}
		})
	})
	cosED = append(cosED, deleteAllButton)

	downloadButton := widget.NewButtonWithIcon("", theme.FolderIcon(), func() {
		if !mapEmpty(&fileentries) {
			fyne.Do(func() {
				filesSave(nil, fmt.Errorf("download"))
				topline.SetText(lp("Saved all files to") + " Download")
			})
		}
	})
	cosED = append(cosED, downloadButton)

	clearButton := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		fyne.Do(func() {
			entry.SetText(entryText)
		})
	})
	cosED = append(cosED, clearButton)

	top := container.NewVBox(
		container.NewHBox(topline,
			layout.NewSpacer(),
			pasteCodeButton,
			clearButton,
		),
		widget.NewForm(&widget.FormItem{Text: lp("Receive Code"), Widget: entry}),
		container.NewHBox(
			totpCheck,
			totpLabel,
			totpProg,
			layout.NewSpacer(),
			deleteAllButton,
			saveAllButton,
			downloadButton,
		),
		mainButton,
		prog,
		cancelButton,
	)
	ti = container.NewTabItemWithIcon(lp("Receive"), theme.DownloadIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))

	showPage = func() {
		if parent.Selected() != ti {
			fyne.Do(func() {
				parent.Select(ti)
			})
		}
	}
	return ti
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
	fd.SetLocation(lastLU)
	fd.Show()
}
