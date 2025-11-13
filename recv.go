// recv.go
package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
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
	log "github.com/schollz/logger"
)

func recvTabItem(a fyne.App, w fyne.Window, parent *container.AppTabs) *container.TabItem {
	index := 1
	var ti *container.TabItem
	showPage := func() {}
	reload := func() {}
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	var cosED, cosSH []fyne.CanvasObject
	prog := widget.NewProgressBar()
	cosSH = append(cosSH, prog)

	topline := widget.NewLabel(lp("Wait for them before pressing Download"))
	entry := widget.NewEntry()
	cosED = append(cosED, entry)

	entryText := os.Getenv(CROC_SECRET)
	if entryText != "" {
		entry.SetText(entryText)
	}
	entry.SetPlaceHolder(lp("Enter code to download"))
	pasteCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		entry.SetText(a.Clipboard().Content())
	})
	cosED = append(cosED, pasteCodeButton)

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-recv", a.Preferences()))
	cosED = append(cosED, totpCheck)

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

	recvDir := filepath.Join(tempDir, RECV)

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)
	recvReady = func() (ok bool) {
		for _, fe := range fileentries {
			if fe == nil {
				return
			}
			if len(fe.Objects) <= feBar {
				return
			}
			if fe.Objects[feBar].Visible() {
				return
			}
		}
		return true
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
		fyne.Do(func() {
			boxholder.Remove(fe)
			boxholder.Refresh()
			delete(fileentries, fpath)
		})
	}
	var addEntry func(dst string, f func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label)) (newentry *fyne.Container)

	ShowFileSave := func(src string, parent fyne.Window) {
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
				// os.Remove(dst)
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
							if des, _ := os.ReadDir(filepath.Dir(src)); len(des) == 0 {
								// удаляю родителя
								reload()
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
						if false {
							// if err := utils.ZipDirectory(pathZip, dst); err == nil {
							// 	log.Tracef("zipped %s->%s error: %v", dst, pathZip, os.RemoveAll(dst))
							// 	if feDir, ok := fileentries[dst]; ok {
							// 		removeEntry(dst, feDir, true)
							// 	}
							// 	ShowFileLocation(pathZip, w)
							// }
						} else {
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

								if feDir, ok := fileentries[dst]; ok {
									removeEntry(dst, feDir, true)
								}
								fyne.Do(func() {
									ShowFileSave(pathZip, w)
								})
							})
						}
					}
					return
				}
			}
			ShowFileSave(dst, w)
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
		// feKeys := []string{}
		// for path, _ := range fileentries {
		// 	if _, err := os.Stat(path); err != nil {
		// 		feKeys = append(feKeys, path)
		// 	}
		// }

		// for _, path := range feKeys {
		// 	if fe, exists := fileentries[path]; exists {
		// 		removeEntry(path, fe, false)
		// 	}
		// }
		// delEntrys := make(map[string]*fyne.Container, len(fileentries))
		// for path, fe := range fileentries {
		// 	if _, err := os.Stat(path); err != nil {
		// 		delEntrys[path] = fe
		// 	}
		// }

		// for path, fe := range delEntrys {
		// 	removeEntry(path, fe, false)
		// }
		for path, fe := range maps.Clone(fileentries) {
			if _, err := os.Stat(path); err != nil {
				removeEntry(path, fe, false)
			}
		}
	}
	OnSelectedReload[index] = reload

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

	var cancelButton, mainButton *widget.Button

	removeEntrys := func() {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe, true)
		}
	}

	ShowFilesSave := func() {
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
				log.Errorf("folder selection: %v", err)
			} else if lu == nil {
				log.Trace("folder selection canceled")
				return
			}

			for src, fe := range maps.Clone(fileentries) {
				child := filepath.Base(src)
				if (isMobile || asMobile) && isLinkDir(src) {
					child += DOTZIP
				}

				if lu != nil {
					lastLU = lu
					// lastSaveDir = lu.Path()
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
					continue
				}

				dst := u.Path()
				if !(isMobile || asMobile) {
					fi, err := os.Stat(src)
					if err != nil {
						log.Errorf("stat %s: %v", src, err)
						continue
					}
					err = Rename(src, u.Path())
					if err == nil {
						log.Tracef("move %s %s", src, dst)
						removeEntry(src, fe, true)
						fyne.Do(func() {
							if len(fileentries) == 0 {
								topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lastLU))
							}
						})
						continue
					}
					log.Warnf("move %s %s: %v", src, dst, err)
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
								if des, _ := os.ReadDir(filepath.Dir(src)); len(des) == 0 {
									// удаляю родителя
									reload()
								}
							}
						})
						return nil
					})
					continue
				}

				destination, err := storage.Writer(u)
				if err != nil {
					cl()
					log.Errorf("writer %s: %v", u, err)
					continue
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

						if len(fileentries) == 0 {
							fyne.Do(func() {
								topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lastLU))
							})
						}
					})
				}
				if isLinkDir(src) {
					// На Андроиде свернём каталог в файл
					pathZip := src + DOTZIP
					if _, err := os.Stat(pathZip); err == nil {
						log.Errorf("exists %s", pathZip)
						continue
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

						if feDir, ok := fileentries[src]; ok {
							removeEntry(src, feDir, true)
						}
						copyFrom(pathZip)
					})
					continue
				}
				copyFrom(src)
			}
		}

		supported, err := IsFolderPickerSupported()
		if err != nil {
			log.Errorf("folder picker: %v", err)
			supported = false
		}
		if !supported {
			// Нет фолдерпикера
			supported, err := IsSaveDialogSupported()
			if err != nil {
				log.Errorf("file picker: %v", err)
				supported = false
			}
			if !supported {
				// Нет файлпикера
				filesSave(nil, fmt.Errorf("folder picker not supported"))
				log.Trace("File picker not supported. ", INSTALL)
				a.Clipboard().SetContent(filePicker)
				dialog.ShowInformation(
					lp("Saved all files to")+" Download",
					INSTALL,
					w,
				)
				return
			}
			// Использую файлпикер вместо фолдерпикера
			savedialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil {
					log.Errorf("file picker: %v", err)
					return
				}
				if writer == nil {
					log.Errorf("file picker canceled")
					return
				}
				u := writer.URI()
				writer.Close()
				log.Tracef("remove %s: %v", u, storage.Delete(u))
				lu, err := Parent(u)
				if err != nil {
					log.Errorf("parent: %v", err)
					filesSave(nil, fmt.Errorf("folder picker not supported"))
					return
				}
				lastLU = lu
				filesSave(lu, nil)
			}, w)
			savedialog.Resize(w.Canvas().Size())
			savedialog.SetLocation(lastLU)
			savedialog.SetConfirmText("Select Folder. File name will be ignored ")
			savedialog.SetFileName("_")
			notFinish = true
			savedialog.Show()
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
					// topline.SetText(lp("Wait for them before pressing Download"))
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
							// showPage()
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
	cosED = append(cosED, mainButton)

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(cancelChan)
	})
	cosSH = append(cosSH, cancelButton)
	allShow(false, cosSH...)

	saveAllButton := widget.NewButtonWithIcon("*", theme.FolderOpenIcon(), func() { //lp("Save All")
		ShowFilesSave()
	})
	cosED = append(cosED, saveAllButton)

	deleteAllButton := widget.NewButtonWithIcon("*", theme.DeleteIcon(), func() { //lp("Delete All")
		if len(fileentries) > 0 {
			removeEntrys()
		} else {
			entry.SetText("")
		}
	})
	cosED = append(cosED, deleteAllButton)

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
