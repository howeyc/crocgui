// recv.go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	log "github.com/schollz/logger"
)

func recvTabItem(a fyne.App, w fyne.Window) (ti *container.TabItem) {
	var (
		cosED,
		cosSH,
		cosDAV []fyne.CanvasObject
		removeEntry func(fpath string, fe *fyne.Container, del bool)
		showPage    func()
		reload      func()

		boxholder = container.NewVBox()
		scroller  = container.NewVScroll(boxholder)

		mainButton  *widget.Button
		prog        = widget.NewProgressBar()
		fileentries sync.Map
	)
	var (
		addEntry func(dst string, f func(d *widget.Button, p *widget.ProgressBar,
			s *widget.Button,
			l *widget.Label)) (newentry *fyne.Container)
		dialogFileSave func(src string, parent fyne.Window, textDialog bool)
		join           = func(elem ...string) string {
			return filepath.FromSlash(filepath.Join(append([]string{tempDir, RECV}, elem...)...))
		}
		ShowFilesSave func()
		filesSave     func(lu fyne.ListableURI, err error)
	)
	var cancelButton *widget.Button
	cancelChan := make(chan struct{}, 1)
	hideCancel := func() {
		fyne.Do(func() {
			cancelButton.Hide()
			mainButton.Show()
		})
		select {
		case <-cancelChan:
		default:
			close(cancelChan)
		}
	}
	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), hideCancel)
	showCancel := func() {
		select {
		case <-cancelChan:
		default:
			close(cancelChan)
		}
		cancelChan = make(chan struct{})
		fyne.Do(func() {
			mainButton.Hide()
			cancelButton.Show()
		})
	}
	cosSH = append(cosSH, prog, cancelButton)
	allShow(false, cosSH...)

	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()

	topline := widget.NewLabel(lp("Wait for them before pressing Download"))

	entry := widget.NewEntryWithData(binding.BindPreferenceString("secret", a.Preferences()))
	cosED = append(cosED, entry)

	entryText := code
	if entryText != "" {
		entry.SetText(entryText)
	}

	qrButton := widget.NewButtonWithIcon("", theme.ViewFullScreenIcon(), func() {
		if qr != nil {
			qr.scanner()
		}
	})

	cbButton := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		cc := a.Clipboard().Content()
		if st, ne, as, a6, ps, pd, s5, ct, err := fromURI(cc); err == nil {
			var _, _, _ = pd, s5, ct
			entry.SetText(st)
			a.Preferences().SetString("new-relay", ne)
			a.Preferences().SetString("relay-address", as)
			a.Preferences().SetString("relay6", a6)
			a.Preferences().SetString("relay-ports", ps)
			// a.Preferences().SetString("relay-password", pd)
			// a.Preferences().SetString("socks5", s5)
			// a.Preferences().SetString("connect", ct)
			addCurrentRelay(a)
			cc = fmt.Sprintf("%s: %s", lp("Relay"), ne)
		} else {
			if _, ccn, _, ok := isDAV(cc); ok {
				// Открываем в браузере
				if err := OpenURL(ccn); err == nil {
					return
				} else {
					log.Error(err)
				}
			}
			entry.SetText(cc)
		}
		NewToast(w, cc).Show()
	})

	cosED = append(cosED, cbButton)
	secretButton := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		a.Clipboard().SetContent("")
		fyne.Do(func() {
			entry.SetText(entryText)
		})
	})
	cosED = append(cosED, secretButton)

	totpCheck := widget.NewCheck("", nil)

	cosED = append(cosED, totpCheck)

	totpLabel := widget.NewLabel(TOTP)
	totpProg := setupTOTP(a, entry, totpCheck, totpLabel, &entryText,
		"totp-recv")

	removeEntrys := func(del bool) {
		if !del {
			fileentries.Clear()
			fyne.Do(func() {
				boxholder.RemoveAll()
			})
			return
		}
		forEachFileEntry(&fileentries, func(fpath string, fe *fyne.Container) {
			removeEntry(fpath, fe, del)
		})
	}

	deleteAllButton := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		fyne.Do(func() {
			if mapEmpty(&fileentries) {
				entry.SetText(entryText)
			} else {
				removeEntrys(true)
			}
		})
	})
	cosED = append(cosED, deleteAllButton)
	cosDAV = append(cosDAV, deleteAllButton)

	ready = func() (ok bool) {
		ok = true
		fileentries.Range(func(key, value interface{}) bool {
			fe := value.(*fyne.Container)
			if fe == nil {
				ok = false
				return ok
			}
			if len(fe.Objects) <= feBar {
				ok = false
				return ok
			}
			bar(fe, false, func(w *widget.ProgressBar) {
				ok = w.Value == w.Max
			})
			return ok
		})
		return ok
	}

	removeEntry = func(path string, fe *fyne.Container, del bool) {
		key := path
		boxout := func() {
			if fe == nil {
				ok := false
				fe, ok = load(&fileentries, key)
				if !ok {
					return
				}
			}
			fileentries.Delete(key)
			fyne.Do(func() {
				boxholder.Remove(fe)
				de.Bounce(boxholder.Refresh)
			})
		}
		if del {
			isDir := isLinkDir(path)
			l := []string{path}
			remove := os.Remove
			d := "file"

			if _, err := os.Stat(path); err == nil {
				if isDir {
					remove = os.RemoveAll
					d = "dir"
					l = append(l, lsr2(path)...)
					log.Debugf("remove dirs %v", l)
				}

				if err := remove(path); err != nil {
					log.Errorf("remove %s %s: %v", d, path, err)
					return
				} else {
					log.Debugf("remove %s %s", d, path)
					if isDir {
						fyne.Do(func() {
							forEachFileEntry(&fileentries, func(sub string, fe *fyne.Container) {
								if slices.Contains(l, sub) {
									log.Debugf("remove %s", sub)
									fileentries.Delete(sub)
									boxholder.Remove(fe)
								}
							})
							de.Bounce(boxholder.Refresh)
						})
						return
					}
				}
			}
		}
		boxout()
	} //removeEntry

	// Добавим строчку в boxholder и fileentries
	addEntry = func(dst string, f func(d *widget.Button, p *widget.ProgressBar,
		s *widget.Button,
		l *widget.Label)) (newentry *fyne.Container) {
		dst = filepath.FromSlash(dst)
		// log.Debugf("addEntry %s", dst)
		if fe, ok := load(&fileentries, dst); ok {
			// log.Debugf("exists %s", dst)
			deleteButton := fe.Objects[feDel]
			progFile := fe.Objects[feBar]
			saveButton := fe.Objects[feSave]
			labelFile := fe.Objects[len(fe.Objects)-1]
			fyne.Do(func() {
				if f == nil {
					deleteButton.Show()
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

		var size int64
		if fi, err := os.Stat(dst); err == nil {
			if fi.IsDir() {
				base += slash
			} else {
				size = fi.Size()
			}
		}
		labelFile := widget.NewLabel(base)
		icon := theme.ContentRemoveIcon()
		iconSB := theme.DocumentSaveIcon()
		clipString := ""
		if validHash(dst) {
			clip, err := os.ReadFile(dst)
			if err == nil && validHash(base, clip...) {
				clipString = string(clip)
				iconSB = theme.ContentCopyIcon()
			}
		}
		saveButton := widget.NewButtonWithIcon("", iconSB, func() {
			if clipString != "" {
				a.Clipboard().SetContent(clipString)
			}
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
							log.Debugf("zip %s %s", dst, pathZip)

							if _, err := os.Stat(pathZip); err != nil {
								log.Errorf("stat %s: %v", pathZip, err)
								return
							}

							if feDir, ok := load(&fileentries, dst); ok {
								removeEntry(dst, feDir, true)
							}
							fyne.Do(func() {
								dialogFileSave(pathZip, w, false)
							})
						})
					}
					return
				}
			}
			dialogFileSave(dst, w, clipString != "")
		}) //saveButton

		deleteButton := widget.NewButtonWithIcon("", icon, func() {
			removeEntry(dst, newentry, true)
		})

		progFile := widget.NewProgressBar()
		progFile.TextFormatter = func() string {
			return shortFormatter(progFile)
		}

		newentry = container.NewHBox(
			deleteButton,
			progFile,
			saveButton,
			labelFile,
		)

		fileentries.Store(dst, newentry)
		fyne.Do(func() {
			setSizes(progFile, size)
			if f != nil {
				f(deleteButton, progFile,
					saveButton,
					labelFile)
			}
			boxholder.Add(newentry)
			de.Bounce(boxholder.Refresh)
		})
		return
	} //addEntry

	//
	dialogFileSave = func(src string, parent fyne.Window, textDialog bool) {
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
				log.Debug("folder selection canceled")
				if textDialog {
					qr.showTextDialog()
				}
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
			if isMobile || asMobile {
				copyToUWCProgress(destination, src, fe, func(err error) {
					cl()
					if err != nil {
						log.Errorf("copy %s %s: %v", src, dst, err)
					} else {
						log.Debugf("copy %s %s", src, dst)
						removeEntry(src, fe, true)
					}
				})
				return
			}
			// Десктоп
			destination.Close()
			// файлпикер создаёт файл
			storage.Delete(u)
			fi, err := os.Stat(src)
			if err != nil {
				log.Errorf("stat %s: %v", src, err)
				return
			}
			err = Rename(src, dst)
			if err == nil {
				log.Debugf("move %s %s", src, dst)
				removeEntry(src, fe, false)
				// fyne.Do(func() {
				// 	log.Debugf("fileTreeShow %s", u)
				// 	fileTreeShow(u, a)
				// })
				return
			}
			log.Warnf("move %s %s: %v", src, dst, err)
			// fileSave
			root := src
			go func() {
				log.Debugf("copyFiles: %v",
					copyFiles(storage.NewFileURI(src), dst, func(u fyne.URI, dstPath string) error {
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
								log.Debugf("copy %s %s", src, dstPath)
								removeEntry(src, feCopy, true)
								if feCopy != fe {
									if os.Remove(filepath.Dir(src)) == nil {
										_, err := os.Stat(root)
										exists := err == nil
										if !exists || os.Remove(root) == nil {
											// Финал
											if feRoot, ok := load(&fileentries, root); ok {
												removeEntry(root, feRoot, false)
											}
											// fyne.Do(func() {
											// 	u := storage.NewFileURI(filepath.Dir(dstPath))
											// 	log.Debugf("fileTreeShow %s", u)
											// 	fileTreeShow(u, a)
											// })
										}
									}
								}
							}
						})
						return nil
					}))
			}()
		} //fileSave

		supported, err := IsSaveDialogSupported()
		if err != nil {
			log.Errorf("file picker: %v", err)
			supported = false
		}
		if !supported {
			fileSave(nil, fmt.Errorf("file picker not supported"))
			log.Debug("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(filePicker)
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				INSTALL,
				w,
			)
			return
		}
		// savedialog := dialog.NewFileSave(fileSave, parent)
		// savedialog.SetFileName(child)
		// savedialog.Resize(parent.Canvas().Size())
		// notFinish = true
		// savedialog.Show()
		newFileSave(fileSave, parent, child)
	} //ShowFileSave

	fpath := a.Preferences().String("DeleteFile")
	os.MkdirAll(join(), 0700)

	reload = func() {
		if fi, err := os.Stat(fpath); err == nil && !fi.IsDir() {
			err := os.Remove(fpath)
			log.Debugf("Removed partially received %s: %v", fpath, err)
			if err == nil || os.IsNotExist(err) {
				fpath = ""
				a.Preferences().SetString("DeleteFile", "")
			}
		}
		exists := make(map[string]bool)
		inZip := make(map[string]bool)
		prefixs := []string{}
		p, n := lsr(join())
		// log.Debugf("lsr %v", n)
		for _, path := range p {
			ext := filepath.Ext(path)
			if strings.ToLower(ext) == DOTZIP {
				dir := strings.TrimSuffix(path, ext)
				prefixs = append(prefixs, dir+slash)
			}
		}
		// log.Debugf("prefixs %v", prefixs)
	loop:
		for i, path := range p {
			name := n[i]
			if name == "" ||
				path == fpath || // если не удалось удалить то не показываю
				name == crocRemovalFile {
				continue
			}
			for _, prefix := range prefixs {
				if strings.HasPrefix(path, prefix) {
					inZip[path] = true
					continue loop
				}
			}
			exists[path] = true
			addEntry(path, func(d *widget.Button, p *widget.ProgressBar,
				s *widget.Button,
				l *widget.Label) {
				d.Show()
				s.Show()
				if isLinkDir(path) {
					name += slash
				}
				l.SetText(name)
			})
		}

		forEachFileEntry(&fileentries, func(path string, fe *fyne.Container) {
			switch {
			case exists[path]:
				return
			case inZip[path]:
				log.Debugf("removeEntry zipped %s", path)
			default:
				if _, err := os.Stat(path); err != nil {
					log.Debugf("removeEntry %s: %v", path, err)
				} else {
					return
				}
			}
			removeEntry(path, fe, true)
		})
	}
	OnSelectedTab[RECVi] = reload
	mainButton = widget.NewButtonWithIcon(lp("Download"), theme.DownloadIcon(), func() {
		fyne.Do(func() { topline.SetText("") })
		if davServer.IsLocal() {
			log.Error("WebDav IsLocal")
			fyne.Do(func() {
				topline.SetText("WebDav IsLocal")
				NewToast(w, "WebDav IsLocal").Show()
			})
			return
		}
		if entry.Validate() != nil {
			log.Error("no receive code entered")
			fyne.Do(func() {
				topline.SetText(lp("Secret must be longer than 5 characters"))
				NewToast(w, lp("Secret must be longer than 5 characters")).Show()
			})
			return
		}

		if !cdLock.CompareAndSwap(0, 1) {
			NewToast(w, lp("Cancel")+" "+lp("Send")).Show()
			return
		}
		// cdLocked
		if wd, _ := os.Getwd(); wd != join() {
			err := os.Chdir(join())
			log.Debugf("change to %s: %v", join(), err)
			if err != nil {
				log.Errorf("croc: %v", err)
				NewToast(w, err.Error()).Show()
				cdLock.Store(0)
				return
			}
		}
		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			secret = TOTP + secret
		}

		opt := croc.Options{
			SharedSecret: secret,
			Debug:        debugBool(a),
			NoPrompt:     true,
			DisableLocal: a.Preferences().Bool("disable-local"),
			// Чтоб не было 2-х ридеров на одном порту
			NoMultiplexing:   a.Preferences().Bool("disable-multiplexing") || davServer.IsActive() || davServer.IsTCPForwardingActive(),
			NoCompress:       a.Preferences().Bool("disable-compression"),
			OnlyLocal:        a.Preferences().Bool("force-local"),
			Curve:            a.Preferences().String("pake-curve"),
			Overwrite:        a.Preferences().Bool("overwrite"),
			MulticastAddress: a.Preferences().String("multicast-address"),
			TestFlag:         a.Preferences().Bool("testing"),
			Quiet:            GUI,
			IgnoreStdin:      GUI,
		}
		opt.RelayPassword, opt.RelayAddress, opt.RelayAddress6, _,
			comm.Socks5Proxy, comm.HttpProxy = def(a)

		switch {
		case strings.HasPrefix(opt.RelayAddress, "0"):
			// Подключаемся напрямую к отправителю
			// --ip
			opt.IP = strings.TrimPrefix(opt.RelayAddress, "0")
			fallthrough
		case opt.OnlyLocal:
			opt.RelayAddress = ""
			opt.RelayAddress6 = ""
			// Не у кого запрашивать локальный адрес
			opt.TestFlag = false
		}

		ctx, ctc := context.WithCancel(appCtx)
		client, err := crocNew(noRestart, ctx, opt)
		if err != nil {
			log.Errorf("croc: %v", err)
			NewToast(w, err.Error()).Show()
			cdLock.Store(0)
			return
		}
		log.SetLevel(debugString(a))
		log.Debug("croc client created")

		if a.Preferences().Bool("remember") {
			p := NewPreferences(a.Preferences())
			p.SetString("relay", opt.RelayAddress)
			a.Preferences().SetBool("send", false)
			saveConfig(p, opt, false)
		}

		var filename string
		showCancel()
		allEnabled(false, cosED...)
		if davServer.IsActive() || davServer.IsTCPForwardingActive() {
			allEnabled(true, cosDAV...)
		}

		if totpCheck.Checked {
			totpProg.Hide()
		}

		doneChan := make(chan struct{})
		fpath := ""

		// progress
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			caffeinate(1)
			defer func() {
				// Конец
				davServer.DisableTCPForwarding()
				caffeinate(-1)
				ticker.Stop()
				if longCdLock {
					log.Debugf("CROC_CD_LOCK %v", longCdLock)
					time.Sleep(time.Second * 30)
				}
				cdLock.Store(0)
				fyne.Do(func() {
					mainButton.Show()
					allShow(false, cosSH...)
					allEnabled(true, cosED...)
					if totpCheck.Checked {
						totpProg.Show()
					}
					removeEntrys(false)
					reload()
					showPage()
					log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
				})
			}()

			old := 0
			oldPath := ""
			var TotalSent, size, totalMax int64
			progW := NewLongProgressWrapper(prog)
			toplineW := NewLabelWrapper(topline)

			fepw := NewProgressWrapper(nil)
			once := true
			for {
				select {
				case <-appCtx.Done():
					return
				case <-doneChan:
					return
				case <-cancelChan:
					s := fmt.Sprintf("%s %s", lp("Receive cancelled."), filename)
					log.Error(s)
					fyne.Do(func() {
						topline.SetText(s)
					})
					a.Preferences().SetString("DeleteFile", join(filename))
					if noRestart {
						ctc()
					} else {
						Stop(client)
						fyne.Do(func() {
							restart(w)
						})
					}
					return
				case <-ticker.C:
					if client == nil {
						return
					}
					if client.Step1ChannelSecured {
						if davServer.IsActive() {
							if davServer.IsTCPForwardingActive() {
								continue
							}
							err := davServer.EnableTCPForwarding(client)
							if err != nil {
								log.Errorf("failed to enable port forwarding: %v", err)
								return
							}
							log.Infof("enabled port forwarding")
						}
					}
					if client.Step2FileInfoTransferred {
						if once {
							// Начало приёма
							once = false
							toplineW.SetText(lp("Receiving file"))

							for i, fi := range client.FilesToTransfer {
								if isMobile || asMobile {
									// Запретим разворачивать свёрнутый каталог
									client.FilesToTransfer[i].TempFile = false
								} else {
									// Развернём свёрнутый каталог если включена опция zip-unzip
									client.FilesToTransfer[i].TempFile = strings.HasSuffix(strings.ToLower(fi.Name), DOTZIP) &&
										a.Preferences().Bool("zip-unzip")
								}
								addEntry(join(trimDotSlash(fi)), func(d *widget.Button, p *widget.ProgressBar, s *widget.Button, l *widget.Label) {
									d.Hide()
									setSizes(p, fi.Size, 0)
									s.Hide()
									l.SetText(trimDotSlash(fi))
								}) //addEntry
								totalMax += fi.Size
							}
							progW.SetMax(totalMax)
							log.Debugf("totalMax %d", totalMax)
						}
						cnum := client.FilesToTransferCurrentNum
						if old < cnum+1 {
							old = cnum + 1
							fi := client.FilesToTransfer[cnum]
							filename = trimDotSlash(fi)
							toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Receiving file"), filename, cnum+1, len(client.FilesToTransfer)))
							TotalSent += size
							size = fi.Size
							path := join(filename)
							if oldPath != path {
								if fe, ok := load(&fileentries, oldPath); ok {
									fyne.Do(func() {
										fe.Objects[feDel].Show()
										bar(fe, false, func(w *widget.ProgressBar) {
											w.SetValue(w.Max)
										})
										fe.Objects[feSave].Show()
									})
								}
								oldPath = path
							}
							log.Debug(path)
							if fe, ok := load(&fileentries, path); ok {
								fepw = NewProgressWrapper(fe.Objects[feBar].(*widget.ProgressBar))
							} else {
								fepw = NewProgressWrapper(nil)
							}
						}
						progW.SetValue(TotalSent + client.TotalSent)
						fepw.SetValue(client.TotalSent)
					}
				}
			}
		}() // progress

		// Receive
		go func() {
			var recvErr error
			if EMULATE == 0 {
				recvErr = client.Receive()
			} else {
				log.Warnf("Receive")
				time.Sleep(EMULATE)
				defer func() {
					time.Sleep(time.Millisecond * 10)
					client = nil
				}()
			}

			fyne.Do(func() {
				if recvErr != nil {
					if errors.Is(recvErr, io.EOF) ||
						errors.Is(recvErr, context.Canceled) {
						recvErr = fmt.Errorf("%s", lp("Send cancelled."))
					}
					s := fmt.Sprintf("receive: %s", recvErr)
					log.Error(s)
					topline.SetText(s)
					fpath = join(filename)
				} else {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Received"), filename))
				}
				a.Preferences().SetString("DeleteFile", fpath)
			})
			close(doneChan)
		}() // Receive
		//  +2 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
	}) // mainButton
	cosED = append(cosED, mainButton)

	saveAllButton := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		ShowFilesSave()
	})
	cosED = append(cosED, saveAllButton)
	cosDAV = append(cosDAV, saveAllButton)

	downloadButton := widget.NewButtonWithIcon("", theme.FolderIcon(), func() {
		if !mapEmpty(&fileentries) {
			fyne.Do(func() {
				filesSave(nil, fmt.Errorf("download"))
				topline.SetText(lp("Saved all files to") + " Download")
			})
		}
	})
	cosED = append(cosED, downloadButton)
	cosDAV = append(cosDAV, downloadButton)

	filesSave = func(lu fyne.ListableURI, err error) {
		var (
			u  fyne.URI
			cl = func() {}
		)

		if err != nil {
			log.Errorf("folder selection: %v", err)
		} else if lu == nil {
			log.Debug("folder selection canceled")
			return
		}

		forEachFileEntry(&fileentries, func(src string, fe *fyne.Container) {
			child := filepath.Base(src)
			if (isMobile || asMobile) && isLinkDir(src) {
				child += DOTZIP
			}

			if lu != nil {
				// lastLU = lu
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
						// lastLU = lu
						a.Preferences().SetString(LastFolder, lu.String())
					}
				}
			}
			if err != nil {
				log.Errorf("Downloads/%s: %v", child, err)
				return
			}

			dst := u.Path()
			if !(isMobile || asMobile) {
				// Десктоп
				fi, err := os.Stat(src)
				if err != nil {
					log.Errorf("stat %s: %v", src, err)
					return
				}
				err = Rename(src, dst)
				if err == nil {
					log.Debugf("move %s %s", src, dst)
					removeEntry(src, fe, true)
					fyne.Do(func() {
						if mapEmpty(&fileentries) {
							// dir := filepath.Dir(dst)
							topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lu.Path()))
						}
					})
					return
				}
				log.Warnf("move %s %s: %v", src, dst, err)
				root := src
				go func() {
					log.Debugf("copyFiles: %v",
						copyFiles(storage.NewFileURI(root), dst, func(u fyne.URI, dstPath string) error {
							// fyne.Do(func() {})
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
									log.Debugf("copy %s %s", src, dstPath)
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
						}))
				}()
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
					log.Debugf("copy %s %s", src, destination.URI())
					removeEntry(src, fe, true)

					if mapEmpty(&fileentries) {
						// Финиш
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
					log.Debugf("zip %s %s", src, pathZip)

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

	ShowFilesSave = func() {
		if mapEmpty(&fileentries) {
			log.Error("no files to save")
			NewToast(w, "no files to save").Show()
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
			NewToast(w, lp("Saved all files to")+" Download").Show()
			return
		}
		ShowFolderOpen(filesSave, w)
	}

	top := container.NewVBox(
		topline,
		container.NewBorder(
			nil, nil,
			container.NewHBox(qrButton, cbButton, secretButton),
			nil,
			entry,
		),
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
		if at == nil {
			return
		}
		ps := at.Selected()
		if ps == nil || ti == nil {
			return
		}
		fyne.Do(func() {
			if ps != ti {
				at.Select(ti)
				ps := at.Selected()
				if ps == nil {
					return
				}
			}
			ps.Content.Refresh()
		})
	}

	return
}

// Большой диалог для десктопа
func ShowFolderOpen0(callback func(fyne.ListableURI, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFolderOpen(callback, parent)
		return
	}
	fd := dialog.NewFolderOpen(callback, parent)
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}
func ShowFolderOpen(callback func(fyne.ListableURI, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFolderOpen(callback, parent)
		return
	}

	var fd *dialog.FileDialog

	if parent.FullScreen() {
		// Если уже был fullscreen - простой диалог
		fd = dialog.NewFolderOpen(callback, parent)
	} else {
		// Если не было fullscreen - включаем и создаём диалог с восстановлением
		//parent.SetFullScreen(true)
		current := parent.Canvas().Size()
		parent.Resize(current.AddWidthHeight(current.Width, 0))

		fd = dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			//parent.SetFullScreen(false)
			parent.Resize(current)
			callback(uri, err)
		}, parent)
	}

	// Настраиваем размер и показываем
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}

func newFileSave(callback func(fyne.URIWriteCloser, error), parent fyne.Window, fileName string) (fd *dialog.FileDialog) {
	if isMobile {
		notFinish = true
		fd = dialog.NewFileSave(callback, parent)
		fd.SetFileName(fileName)
		fd.Show()
		return
	}

	if !parent.FullScreen() {
		//parent.SetFullScreen(true)
		current := parent.Canvas().Size()
		parent.Resize(current.AddWidthHeight(current.Width, 0))

		fd = dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			//parent.SetFullScreen(false)
			parent.Resize(current)
			callback(uri, err)
		}, parent)
	} else {
		fd = dialog.NewFileSave(callback, parent)
	}

	fd.Resize(parent.Canvas().Size())
	fd.SetFileName(fileName)
	fd.Show()

	return
}
