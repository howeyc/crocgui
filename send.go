// send.go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	log "github.com/schollz/logger"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xw "fyne.io/x/fyne/widget"
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
)

const (
	MaterialFiles = "https://github.com/zhanghai/MaterialFiles"
	MiXplorer     = "https://mixplorer.com/beta/"
	filePicker    = MiXplorer
	INSTALL       = "URL " + filePicker + " is already in the clipboard.\nInstall the app to avoid this message."
	PSL           = "→"
)

const (
	feDel = iota
	feBar
	feSave
)

func sendTabItem(a fyne.App, w fyne.Window, parent *container.AppTabs) (ti *container.TabItem) {
	var (
		cosED, cosSH []fyne.CanvasObject
		removeEntry  func(fpath string, fe *fyne.Container, del bool)
		showPage     func()
		reload       func()

		boxholder = container.NewVBox()
		scroller  = container.NewVScroll(boxholder)

		mainButton  *widget.Button
		prog        = widget.NewProgressBar()
		fileentries sync.Map
	)
	var (
		addEntry func(dst string, f func(d *widget.Button, p *widget.ProgressBar,

			l *widget.Label)) (newentry *fyne.Container)
		reDir *widget.Button
	)
	var cancelButton *widget.Button
	cancelChan := make(chan struct{}, 1)
	hideCancel := func() {
		fyne.Do(cancelButton.Hide)
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
		fyne.Do(cancelButton.Show)
	}
	cosSH = append(cosSH, prog, cancelButton)
	allShow(false, cosSH...)

	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()

	topline := widget.NewLabel(lp("Pick a file to send"))

	entry := widget.NewEntryWithData(binding.BindPreferenceString("secret", a.Preferences()))
	cosED = append(cosED, entry)

	entryText := os.Getenv(CROC_SECRET)
	if entryText != "" {
		entry.SetText(entryText)
	}

	cbButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(entry.Text)
	})

	secretButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		fyne.Do(func() {
			entry.SetText(utils.GetRandomName())
		})
	})
	cosED = append(cosED, secretButton)

	totpCheck := widget.NewCheck("", nil)
	cosED = append(cosED, totpCheck)

	totpLabel := widget.NewLabel(TOTP)
	totpProg := setupTOTP(a, entry, totpCheck, totpLabel, &entryText,
		"totp-send")

	removeEntrys := func(del bool) {
		if !del {
			fileentries.Clear()
			fyne.Do(func() {
				boxholder.RemoveAll()
				if ftw != nil {
					ftw.Close()
				}
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

	seady := func() (ok bool) {
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
				doMonitor.DoRequest(boxholder.Refresh)
				if ftw != nil {
					ftw.Close()
				}
			})
		}
		if del {
			isDir := isLinkDir(path)
			cached := isCached(join(), path)
			if !cached {
				// ссылка
				link, err := linkByTarget(path, join)
				log.Tracef("link %v, err %v", link, err)
				if err != nil {
					log.Tracef("%v", err)
					boxout()
					return
				}
				path = link
				isDir = isLinkDir(path)
				del = !isDir
			}
			l := []string{path}
			remove := os.Remove
			de := "file"

			if _, err := os.Stat(path); err == nil && del {
				if isDir {
					remove = os.RemoveAll
					de = "dir"
					l = append(l, lsr2(path)...)
					log.Tracef("remove dirs %v", l)
				}

				if err := remove(path); err != nil {
					log.Errorf("remove %s %s: %v", de, path, err)
					return
				} else {
					log.Tracef("remove %s %s", de, path)
					if isDir {
						fyne.Do(func() {
							forEachFileEntry(&fileentries, func(sub string, fe *fyne.Container) {
								if slices.Contains(l, sub) {
									log.Tracef("remove %s", sub)
									fileentries.Delete(sub)
									boxholder.Remove(fe)
								}
							})
							doMonitor.DoRequest(boxholder.Refresh)
							if ftw != nil {
								ftw.Close()
							}
						})
						return
					}
				}
			}
		}
		boxout()
	} //removeEntry

	// nil if exists
	addEntry = func(dst string, f func(d *widget.Button, p *widget.ProgressBar,

		l *widget.Label)) (newentry *fyne.Container) {
		dst = filepath.FromSlash(dst)
		// log.Tracef("addEntry %s", dst)
		if _, ok := load(&fileentries, dst); ok {
			log.Tracef("exists %s", dst)
			return nil
		}
		base := filepath.Base(dst)

		var size int64
		if fi, err := os.Stat(dst); err == nil {
			if isLinkDir(dst) {
				base += slash
			} else {
				size = fi.Size()
			}
		}
		labelFile := widget.NewLabel(base)
		icon := theme.ContentRemoveIcon()
		if isCached(join(), dst) {
			if _, err := Readlink(dst); err == nil {
				icon = theme.MoreHorizontalIcon()
			}
		} else {
			icon = theme.MoreHorizontalIcon()
		}

		deleteButton := widget.NewButtonWithIcon("", icon, func() {
			removeEntry(dst, newentry, true)
		})

		progFile := widget.NewProgressBar()
		progFile.TextFormatter = func() string {
			return textFormatter(progFile)
		}

		newentry = container.NewHBox(
			deleteButton,
			progFile,

			labelFile,
		)

		fileentries.Store(dst, newentry)
		fyne.Do(func() {
			if size > 0 {
				progFile.Max = float64(size)
			} else {
				progFile.Max = 0.1
			}
			progFile.SetValue(progFile.Max)
			if f != nil {
				f(deleteButton, progFile,

					labelFile)
			}
			boxholder.Add(newentry)
			doMonitor.DoRequest(boxholder.Refresh)
			if ftw != nil {
				ftw.Close()
			}
		})
		return
	} //addEntry

	// Пишу ссылку а если не удачно то кэширую
	addPath := func(src string, wga *sync.WaitGroup) error {
		_, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %v", src, err)
		}

		base := filepath.Base(src)
		dst := join(base)
		if len(join()) < len(dst) && isCached(join(), dst) {
		} else {
			return fmt.Errorf("not in cach %s", dst)
		}
		// fe := addEntry(dst, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
		// 	p.Hide()
		// 	if fi.IsDir() {
		// 		l.SetText(base + slash)
		// 	}
		// })

		// if fe == nil {
		// 	log.Tracef("entry %s has %s", base, src)
		// 	return nil
		// }

		if _, err := os.Stat(dst); err == nil {
			log.Tracef("recached %s: %v", base, os.RemoveAll(dst))
			NewToast(w, "recached "+base).Show()
		}

		wga.Add(1)
		err = Symlink(src, dst)
		log.Tracef("symlink %s %s: %v", src, dst, err)
		if err == nil {
			// Сортирую
			// removeEntrys(false)
			// reload()
			wga.Done()
			return nil
		}

		go func() {
			// addPath
			var wg sync.WaitGroup
			log.Tracef("copyFiles: %v",
				copyFiles(storage.NewFileURI(src), dst, func(u fyne.URI, dstPath string) error {
					// feCopy := fe
					src := u.Path()
					// if fi.IsDir() {
					// Создаю временный прогрессбар
					rel, err := filepath.Rel(join(), dstPath)
					if err != nil {
						rel = dstPath
					}
					feCopy := addEntry(dstPath, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
						l.SetText(rel)
					})
					if feCopy == nil {
						return nil
					}
					// }
					wg.Add(1)
					CopyFileProgress(src, dstPath, feCopy, func(err error) {
						defer wg.Done()
						if err != nil {
							log.Errorf("copy %s %s: %v", src, dstPath, err)
							removeEntry(dstPath, feCopy, true)
							return
						}

						if _, err := os.Stat(dstPath); err != nil {
							// не закэшировал
							log.Errorf("stat %s: %v", dstPath, err)
						} else {
							// закэшировал
							log.Tracef("copy %s %s", src, dstPath)
						}
						// if feCopy != fe {
						// 	// Удалю временный прогрессбар без удаления файла
						// 	// removeEntry(dstPath, feCopy, false)
						// 	removeEntry(src, fe, false)
						// }
					})
					return nil
				}))
			select {
			case <-done:
			default:
				wg.Wait()
				log.Tracef("copyFiles done")
				// Сортирую
				// removeEntrys(false)
				// reload()
				wga.Done()
			}
		}()

		return nil
	} //addPath

	copyFromURCProgress := func(source fyne.URIReadCloser, dst string, c *fyne.Container, onComplete func(err error)) {
		if source == nil {
			onComplete(fmt.Errorf("user cancel dialog"))
			return
		}
		close := func() {
			if err := source.Close(); err != nil {
				log.Errorf("close %s: %v", source.URI(), err)
			}
		}

		if dst == "" {
			u := source.URI()
			name := uriBase(u)
			dst = join(name)
		}
		destination, err := os.Create(dst)
		if err != nil {
			close()
			onComplete(fmt.Errorf("unable to create file %s error: %s", dst, err.Error()))
			return
		}
		clode := func() {
			if err := destination.Close(); err != nil {
				log.Errorf("close %s: %v", destination.Name(), err)
			}
		}

		total, err := getSize(source.URI())
		log.Tracef("getSize %s %d: %v", source.URI(), total, err)
		if err != nil {
			total = 1 << 30
		}
		pw, restore := NewProgressWriter(destination, total, c)

		go func() {
			_, err := io.Copy(pw, source)
			close()
			clode()
			if err == nil {
				if t, err := ModTime(source.URI()); err == nil && !t.IsZero() {
					log.Tracef("Chtimes %s %v: %v", destination.Name(), t,
						os.Chtimes(destination.Name(), time.Time{}, t))
				}
			}
			restore()
			onComplete(err)
		}()
	}

	os.MkdirAll(join(), 0700)

	reload = func() {
		prefixs := []string{}
		p, n := lsr(join())
		log.Tracef("lsr %v", n)
		for _, path := range p {
			ext := filepath.Ext(path)
			if strings.ToLower(ext) == DOTZIP {
				dir := strings.TrimSuffix(path, ext)
				prefixs = append(prefixs, dir+slash)
			}
		}
		log.Tracef("prefixs %v", prefixs)
	loop:
		for i, path := range p {
			name := n[i]
			if name == "" ||

				name == crocRemovalFile {
				continue
			}
			for _, prefix := range prefixs {
				if strings.HasPrefix(path, prefix) {
					log.Tracef("continue %s %s", path, prefix)
					continue loop
				}
			}
			addEntry(path, func(d *widget.Button, p *widget.ProgressBar,

				l *widget.Label) {
				d.Show()
				// p.Hide()

				if isLinkDir(path) {
					name += slash
				}
				l.SetText(name)
			})
		}

		forEachFileEntry(&fileentries, func(path string, fe *fyne.Container) {
			if _, err := os.Stat(path); err != nil {
				log.Tracef("removeEntry %s", path)
				removeEntry(path, fe, false)
			}
		})
	}
	OnSelectedReload[0] = reload

	reload()
	mainButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		if entry.Validate() != nil {
			log.Error("no receive code entered")
			NewToast(w, lp("Secret must be longer than 5 characters")).Show()
			return
		}

		if !seady() || swap && !ready() {
			NewToast(w, lp("Pick a file to send")).Show()
			return
		}
		filepaths := []string{}
		allowed := []string{}
		fileentries.Range(func(key, value interface{}) bool {
			path := key.(string)
			fe := value.(*fyne.Container)
			label(fe, false, func(l *widget.Label) {
				if strings.HasSuffix(l.Text, slash) || // каталоги
					!strings.Contains(l.Text, slash) { // файлы в корне
					if target, err := Readlink(path); err == nil {
						path = target
					}
					filepaths = append(filepaths, path)
				} else {
					allowed = append(allowed, path)
				}
			})

			return true
		})
		log.Tracef("filepaths %v", filepaths)
		log.Tracef("allowed %v", allowed)

		// Посылаем если есть файлы
		if len(filepaths) < 1 {
			log.Error("no files ready")
			NewToast(w, lp("Pick a file to send")).Show()
			return
		}

		zipfolder := a.Preferences().Bool("zip-unzip")

		// Пути посылаемых файлов абсолютны. Переходим в каталог только если zipfolder
		cdLocked := false
		if hasFolder(join()) && zipfolder {
			log.Trace("hasFolders(join()) && zipfolder")
			if cdLock.CompareAndSwap(0, 1) {
				cdLocked = true
				log.Trace("cdLocked = true")
				if wd, _ := os.Getwd(); wd != join() {
					err := os.Chdir(join())
					log.Tracef("change to %s: %v", join(), err)
					if err != nil {
						NewToast(w, err.Error()).Show()
						cdLock.Store(0)
						return
					}
				}
			} else {
				NewToast(w, lp("Cancel")+" "+lp("Download")).Show()
				return
			}
		}
		//
		go func() {
			filesInfo, emptyfolders, totalNumberFolders, err := croc.GetFilesInfo(filepaths, zipfolder, false, nil)

			if cdLocked {
				if longCdLock {
					time.Sleep(time.Second * 30)
				}
				cdLock.Store(0)
			}
			if err != nil {
				fyne.Do(NewToast(w, err.Error()).Show)
				return
			}
			if len(filesInfo) < 1 {
				fyne.Do(NewToast(w, lp("Pick a file to send")).Show)
				return
			}

			if len(allowed) > 0 {
				filtered := []croc.FileInfo{}
				allowedSet := make(map[string]struct{})

				for _, a := range allowed {
					allowedSet[a] = struct{}{}
				}
				// log.Tracef("allowedSet %v", allowedSet)

				for _, fi := range filesInfo {
					if _, ok := allowedSet[filepath.Join(fi.FolderSource, fi.Name)]; ok {
						filtered = append(filtered, fi)
					}
				}
				filesInfo = filtered[:]
			}
			// log.Tracef("filtered filesInfo %+v: %v", filesInfo, err)
			if len(filesInfo) < 1 {
				fyne.Do(NewToast(w, lp("Pick a file to send")).Show)
				return
			}

			secret := entry.Text
			if totpCheck.Checked {
				secret = totp(entry.Text)
				totpLabel.SetText(secret)
				secret = TOTP + secret
			}

			client, err := croc.New(croc.Options{
				IsSender:         true,
				SharedSecret:     secret,
				Debug:            debugBool(a),
				RelayAddress:     a.Preferences().String("relay-address"),
				RelayPorts:       strings.Split(a.Preferences().String("relay-ports"), ","),
				RelayPassword:    a.Preferences().String("relay-password"),
				NoPrompt:         true,
				DisableLocal:     a.Preferences().Bool("disable-local"),
				NoMultiplexing:   a.Preferences().Bool("disable-multiplexing"),
				OnlyLocal:        a.Preferences().Bool("force-local"),
				NoCompress:       a.Preferences().Bool("disable-compression"),
				Curve:            a.Preferences().String("pake-curve"),
				HashAlgorithm:    a.Preferences().String("croc-hash"),
				ThrottleUpload:   a.Preferences().String("upload-throttle"),
				MulticastAddress: a.Preferences().String("multicast-address"),
				Exclude:          []string{},
				ZipFolder:        zipfolder,
			})
			if err != nil {
				log.Errorf("croc: %v", err)
				fyne.Do(NewToast(w, err.Error()).Show)

				return
			}
			log.SetLevel(debugString(a))
			log.Trace("croc client created")

			var filename string
			showCancel()
			fyne.Do(func() {

				allEnabled(false, cosED...)

				if totpCheck.Checked {
					totpProg.Hide()
				}
				// Скрываю кнопки Удалить
				fileentries.Range(func(key, value interface{}) bool {
					fe := value.(*fyne.Container)
					fe.Objects[feDel].Hide()
					return true
				})
			})

			doneChan := make(chan struct{})

			// progress
			go func() {
				ticker := time.NewTicker(time.Millisecond * 100)
				defer func() {
					// Конец
					ticker.Stop()
					fyne.Do(func() {
						prog.SetValue(0)
						allShow(false, cosSH...)
						allEnabled(true, cosED...)
						if totpCheck.Checked {
							totpProg.Show()
						}

						reload()
					})
				}()

				old := 0
				oldPath := ""
				var TotalSent, size, totalMax int64
				progW := NewProgressWrapper(prog)
				toplineW := NewLabelWrapper(topline)
				toplineW.SetText(lp("Have them not press the Download yet"))
				fepw := NewProgressWrapper(nil)
				once := true
				for {
					select {
					case <-done:
						return
					case <-doneChan:
						if !swap {
							os.RemoveAll(join())
						}
						log.Tracef("A restart is better than leaving 12 goroutines leaking")
						fyne.Do(func() {
							restart(w)
						})
						return
					case <-cancelChan:
						s := fmt.Sprintf("%s %s", lp("Send cancelled."), filename)
						log.Error(s)
						fyne.Do(func() {
							topline.SetText(s)
						})

						Stop(client)
						fyne.Do(func() {
							restart(w)
						})
						return
					case <-ticker.C:
						if client == nil {
							return
						}
						if once && hashed(client) {
							// Начало передачи
							once = false
							fyne.Do(func() {
								toplineW.SetText(lp("Have them press the Download now"))
								NewToast(w, lp("Have them press the Download now")).Show()
								prog.Show()
							})
							for _, fi := range client.FilesToTransfer {
								log.Tracef("fi %+v", fi)
								path := filepath.Join(fi.FolderSource, fi.Name)

								if fe, ok := load(&fileentries, path); ok {
									button(fe, true, feDel, func(w *widget.Button) { w.Hide() })
									bar(fe, true, func(w *widget.ProgressBar) {
										w.SetValue(0)
										w.Max = float64(fi.Size)
										w.Show()
									})
								} else {
									// Временный прогрессбар
									addEntry(path, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
										d.Hide()
										p.SetValue(0)
										p.Max = float64(fi.Size)
										// p.Show()

										if !fi.TempFile {
											l.SetText(trimDotSlash(fi))
										}
									}) //addEntry
								}

								totalMax += fi.Size
							}
							progW.SetMax(totalMax)
							log.Tracef("totalMax %d", totalMax)
						}
						if client.Step2FileInfoTransferred {
							cnum := client.FilesToTransferCurrentNum
							if old < cnum+1 {
								old = cnum + 1
								fi := client.FilesToTransfer[cnum]
								filename = trimDotSlash(fi)
								toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Sending file"), filename, cnum+1, len(client.FilesToTransfer)))
								TotalSent += size
								size = fi.Size
								// path := join(fi.Name)
								path := filepath.Join(fi.FolderSource, fi.Name)
								if oldPath != path {
									if fe, ok := load(&fileentries, oldPath); ok {
										removeEntry(oldPath, fe, true)
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
							progW.SetValue(TotalSent + client.TotalSent)
							fepw.SetValue(client.TotalSent)
						}
					}
				}
			}() // progress

			// Send
			go func() {
				var err error
				if EMULATE == 0 {
					err = client.Send(filesInfo, emptyfolders, totalNumberFolders)
				} else {
					log.Warnf("Send %v %v %d", filesInfo, emptyfolders, totalNumberFolders)
					time.Sleep(EMULATE)
					defer func() {
						time.Sleep(time.Millisecond * 10)
						client = nil
					}()
				}

				fyne.Do(func() {
					if err != nil {
						if errors.Is(err, io.EOF) {
							err = fmt.Errorf("%s", lp("Receive cancelled."))
						}
						s := fmt.Sprintf("send: %v", err)
						log.Error(s)
						topline.SetText(s)
						//

					} else {
						topline.SetText(fmt.Sprintf("%s: %s", lp("Sent file"), filename))
					}

				})
				close(doneChan)
			}() // Send
		}() //go
		// +12 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
		a.Clipboard().SetContent(entry.Text)
	}) // mainButton
	cosED = append(cosED, mainButton)

	treeButton := widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		ft := fileTreeShow(storage.NewFileURI(join()), a)
		if ft != nil {
			ft.OnSelected = func(uid widget.TreeNodeID) {
				selected(uid, func(err error) {
					log.Tracef("selected %v: %v", uid, err)
				})
			}
		}
	})
	cosED = append(cosED, treeButton)

	if isAndroid {
		a.Lifecycle().SetOnExitedForeground(func() {
			log.Trace("ExitedForeground")
			if !notFinish {
				excludeFromRecents()
			}
		})
		a.Lifecycle().SetOnStopped(func() {
			log.Trace("Stopped")
		})
		a.Lifecycle().SetOnStarted(func() {
			log.Trace("Started")
		})
		a.Lifecycle().SetOnEnteredForeground(func() {
			notFinish = false
			log.Trace("EnteredForeground")
			close(uriFromIntent)
			uriFromIntent = make(chan string, 100)

			close(textFromIntent)
			textFromIntent = make(chan string, 100)
			go func() {
				for {
					select {
					case <-done:
						log.Trace("done")
						return
					case text := <-textFromIntent:
						if text == "" {
							log.Trace("doneProcessIntent notFinish")
							notFinish = true
							return
						}
						if entry.Disabled() {
							log.Trace("doneProcessIntent Sending")
							return
						}
						log.Tracef(`text "%s"`, text)
						src := join("text" + hashToFilename(text))
						if fe := addEntry(src, nil); fe == nil {
							continue
						}

						source, err := os.Create(src)
						if err != nil {
							log.Errorf("create: %v", err)
							continue
						}

						_, err = source.WriteString(text)
						if err != nil {
							source.Close()
							os.Remove(src)
							log.Errorf("write: %v", err)
							continue
						}

						source.Close()
						showPage() //textFromIntent

					case uriString := <-uriFromIntent:
						if uriString == "" {
							log.Trace("doneProcessIntent")
							return
						}
						if entry.Disabled() {
							log.Trace("Sending")
							log.Trace("doneProcessIntent")
							return
						}
						u, err := storage.ParseURI(uriString)
						if err != nil {
							log.Errorf("parse %s: %v", u, err)
							continue
						}
						log.Tracef("uri %s", u)
						log.Tracef("apiLevel %d", apiLevel())

						if IsDirectory(u) {
							continue
						}
						name := uriBase(u)
						dst := join(name)
						source, err := Reader(u)
						if err != nil {
							log.Errorf("reader: %v", err)
							continue
						}
						fe := addEntry(dst, nil)
						if fe == nil {
							continue
						}

						showPage() //uriFromIntent
						copyFromURCProgress(source, "", fe, func(err error) {
							if err != nil {
								log.Errorf("copy %s %s: %s", u, dst, err)
								removeEntry(dst, fe, true)
								return
							}

							if _, err := os.Stat(dst); err != nil {
								log.Errorf("stat %s: %v", dst, err)
								removeEntry(dst, fe, true)
							} else {
								log.Errorf("copy %s %s", u, dst)
							}
						})
					}
				}
			}()
			processIntent()
		})
	} else {
		if len(os.Args) > 0 {
			go func() {
				var wga sync.WaitGroup
				for _, src := range os.Args[1:] {
					src, err := filepath.Abs(src)
					if err != nil {
						log.Warnf("abs %s: %v", src, err)
						continue
					}
					if err := addPath(src, &wga); err != nil {
						log.Error(err.Error())
					}
				}
				select {
				case <-done:
				default:
					wga.Wait()
					log.Tracef("done %v", os.Args[1:])
					removeEntrys(false)
					reload()
					showPage()
				}
			}()
		}

		w.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
			if len(uris) == 0 {
				return
			}
			if entry.Disabled() {
				log.Trace("Sending")
				return
			}
			// reload()
			go func() {
				var wga sync.WaitGroup
				for _, uri := range uris {
					if err := addPath(uri.Path(), &wga); err != nil {
						log.Error(err.Error())
					}
				}
				select {
				case <-done:
				default:
					wga.Wait()
					log.Tracef("done %v", uris)
					removeEntrys(false)
					reload()
					showPage() //SetOnDropped
				}
			}()
		})
	} //isAndroid

	addFileButton := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if supported, err := IsFilePickerSupported(); err != nil {
			log.Errorf("file picker support: %v", err)
		} else if !supported {
			log.Tracef("File picker not supported. %s", INSTALL)
			a.Clipboard().SetContent(filePicker)
			dialog.ShowInformation(
				lp("Pick a file to send"),
				INSTALL,
				w,
			)
			return
		} else {
			log.Trace("File picker is supported")
		}
		ShowFileOpen(func(source fyne.URIReadCloser, e error) {
			if source == nil {
				return
			}
			if e != nil {
				source.Close()
				log.Errorf("file dialog: %v", e)
				return
			}
			u := source.URI()
			name := uriBase(u)
			dst := join(name)
			// fe := addEntry(dst, nil)
			// if fe == nil {
			// 	return
			// }
			src := u.String()

			err := Symlink(u.Path(), dst)
			log.Tracef("symlink %s %s: %v", u.Path(), dst, err)
			if err == nil {
				addEntry(u.Path(), nil)
				// button(fe, feDel, func(b *widget.Button) {
				// 	b.SetIcon(theme.MoreHorizontalIcon())
				// })
				return
			}

			fe := addEntry(dst, nil)
			if fe == nil {
				return
			}

			raf := func() {
			}
			if apiLevel() < 29 && strings.HasPrefix(src, ZhangHai) {
				raf = func() {
					fyne.Do(func() {
						restart(w)
					})
				}
			}
			copyFromURCProgress(source, "", fe, func(err error) {
				defer raf()
				if err != nil {
					log.Errorf("copy %s %s: %v", src, dst, err)
					removeEntry(dst, fe, true)
					return
				}

				if _, err := os.Stat(dst); err != nil {
					log.Errorf("stat %s:  %v", dst, err)
					removeEntry(dst, fe, false)
				} else {
					log.Tracef("copy %s %s", src, dst)
				}
			})
		}, w)
	})
	cosED = append(cosED, addFileButton)

	addFolderButton := widget.NewButtonWithIcon("", theme.FolderNewIcon(), func() {
		folderOpen := func(u fyne.ListableURI, e error) {
			if u == nil {
				log.Trace("folder selection canceled")
				return
			}
			if e != nil {
				log.Errorf("folder selection: %s", e)
				return
			}
			name := uriBase(u)
			dst := join(name)
			fe := addEntry(dst, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
				// p.Hide()
				l.SetText(name + slash)
			})
			if fe == nil {
				return
			}

			err := Symlink(u.Path(), dst)
			log.Tracef("symlink %s %s: %v", u.Path(), dst, err)
			if err == nil {
				// Десктоп
				removeEntrys(false)
				reload()
				return
			}

			go func() {
				var wg sync.WaitGroup
				log.Tracef("copyFiles: %v",
					copyFiles(u, dst, func(src fyne.URI, dstPath string) error {
						// fyne.Do(func() {})
						// Покажем временный прогрессбар
						rel, err := filepath.Rel(join(), dstPath)
						if err != nil {
							rel = dstPath
						}
						feCopy := addEntry(dstPath, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
							l.SetText(rel)
						})
						if feCopy == nil {
							return nil
						}
						source, err := Reader(src)
						if err != nil {
							return fmt.Errorf("reader %s: %v", src, err)
						}
						wg.Add(1)
						copyFromURCProgress(source, dstPath, feCopy, func(err error) {
							defer wg.Done()
							if err != nil {
								log.Errorf("copy %s %s: %v", src, dstPath, err)
								removeEntry(dstPath, feCopy, true)
								return
							}

							if _, err := os.Stat(dstPath); err != nil {
								log.Errorf("stat %s: %v", dstPath, err)
							} else {
								log.Tracef("copy %s %s", src, dstPath)
							}
							// Скроем временный прогрессбар без удаления файла
							// removeEntry(dstPath, feCopy, false)
						})
						return nil
					}))
				select {
				case <-done:
				default:
					wg.Wait()
					log.Tracef("copyFiles done")
					// Сортирую
					removeEntrys(false)
					reload()
				}
			}()
			// reload()
		}
		ShowFolderOpen(folderOpen, w)
	})
	cosED = append(cosED, addFolderButton)

	reDir = widget.NewButtonWithIcon("", theme.UploadIcon(), func() {
		if !seady() {
			log.Error("not all files ready for send")
			NewToast(w, "not all files ready for send").Show()
			return
		}
		if !ready() {
			log.Error("not all files ready for recv")
			NewToast(w, "not all files ready for recv").Show()
			return
		}

		swap = !swap
		if swap {
			reDir.SetIcon(theme.MailForwardIcon())
			join = func(elem ...string) string {
				return filepath.FromSlash(filepath.Join(append([]string{tempDir, RECV}, elem...)...))
			}
		} else {
			reDir.SetIcon(theme.UploadIcon())
			join = func(elem ...string) string {
				return filepath.FromSlash(filepath.Join(append([]string{tempDir, SEND}, elem...)...))
			}
		}
		removeEntrys(false)
		reload()
	})
	cosED = append(cosED, reDir)

	top := container.NewVBox(
		container.NewHBox(topline,
			layout.NewSpacer(),
			addFileButton,
			addFolderButton,
		),
		container.NewBorder(
			nil, nil,
			container.NewHBox(secretButton, cbButton),
			nil,
			entry,
		),
		container.NewHBox(
			totpCheck,
			totpLabel,
			totpProg,
			layout.NewSpacer(),
			treeButton,
			deleteAllButton,
			//

			reDir,
		),
		mainButton,
		prog,
		cancelButton,
	)

	ti = container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
	showPage = func() {
		if parent.Selected() != ti {
			fyne.Do(func() {
				parent.Select(ti)
			})
		}
	}

	return
}

// Большой диалог для десктопа
func ShowFileOpen(callback func(fyne.URIReadCloser, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFileOpen(callback, parent)
		return
	}
	fd := dialog.NewFileOpen(callback, parent)
	fd.Resize(parent.Canvas().Size())
	fd.SetLocation(lastLU)
	fd.Show()
}

// For mobile os.Exit.
// For desktop Restart.
func restart(w fyne.Window) {
	if noRestart {
		return
	}
	start()
	cleanup(w)
	os.Exit(0)
}

// type clientShadow struct {
// 	Options                         croc.Options
// 	Pake                            *pake.Pake
// 	Key                             []byte
// 	ExternalIP, ExternalIPConnected string

// 	// steps involved in forming relationship
// 	Step1ChannelSecured       bool
// 	Step2FileInfoTransferred  bool
// 	Step3RecipientRequestFile bool
// 	Step4FileTransferred      bool
// 	Step5CloseChannels        bool
// 	SuccessfulTransfer        bool

// 	// send / receive information of all files
// 	FilesToTransfer           []croc.FileInfo
// 	EmptyFoldersToTransfer    []croc.FileInfo
// 	TotalNumberOfContents     int
// 	TotalNumberFolders        int
// 	FilesToTransferCurrentNum int
// 	FilesHasFinished          map[int]struct{}
// 	TotalFilesIgnored         int

// 	// send / receive information of current file
// 	CurrentFile            *os.File
// 	CurrentFileChunkRanges []int64
// 	CurrentFileChunks      []int64
// 	CurrentFileIsClosed    bool
// 	LastFolder             string

// 	TotalSent              int64
// 	TotalChunksTransferred int
// 	chunkMap               map[uint64]struct{}
// 	limiter                *rate.Limiter

// 	// tcp connections
// 	conn []*comm.Comm

// 	bar             *progressbar.ProgressBar
// 	longestFilename int
// 	firstSend       bool

// 	mutex                    *sync.Mutex
// 	fread                    *os.File
// 	numfinished              int
// 	quit                     chan bool
// 	finishedNum              int
// 	numberOfTransferredFiles int
// }

func Conns(client interface{}) ([]*comm.Comm, error) {
	defer func() { recover() }()

	v := reflect.ValueOf(client)
	if v.Kind() != reflect.Ptr {
		return nil, errors.New("not a pointer")
	}

	field := v.Elem().FieldByName("conn")
	if !field.IsValid() {
		return nil, errors.New("no such field")
	}

	return field.Interface().([]*comm.Comm), nil
}

func Stop(client interface{}) {
	conns, err := Conns(client)
	if err == nil {
		if len(conns) > 0 {
			conns[0].Close()
			time.Sleep(time.Millisecond * 333)
		}
	} else {
		log.Errorf("stop: %v", err)
	}
}

func totp(secret string) string {
	key := []byte(secret)
	epoch := time.Now().Unix() / 30
	message := make([]byte, 8)
	for i := 0; i < 8; i++ {
		message[7-i] = byte(epoch >> (8 * i))
	}

	hash := hmac.New(sha256.New, key)
	hash.Write(message)
	hmacHash := hash.Sum(nil)
	offset := int(hmacHash[len(hmacHash)-1] & 0xf)
	code := int32(hmacHash[offset]&0x7f)<<24 |
		int32(hmacHash[offset+1]&0xff)<<16 |
		int32(hmacHash[offset+2]&0xff)<<8 |
		int32(hmacHash[offset+3]&0xff)

	otp := code % int32(math.Pow10(6))
	return fmt.Sprintf("%06d", otp)
}

func hashToFilename(data string) string {
	hash := crc32.ChecksumIEEE([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func hashed(c *croc.Client) bool {
	if len(c.FilesToTransfer) == 0 {
		return false
	}
	for _, file := range c.FilesToTransfer {
		if len(file.Hash) == 0 {
			return false
		}
	}
	return true
}

func allEnabled(enabled bool, cos ...fyne.CanvasObject) {
	for _, co := range cos {
		switch w := co.(type) {
		case *widget.Button:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}
		case *widget.Entry:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}
		case *widget.Check:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}
		}
	}
}

func allShow(show bool, cos ...fyne.CanvasObject) {
	for _, co := range cos {
		if show {
			co.Show()
		} else {
			co.Hide()
		}
	}
}

func setupTOTP(a fyne.App, entry *widget.Entry, totpCheck *widget.Check, totpLabel *widget.Label, entryText *string, bind string) *widget.ProgressBar {
	totpCheck.Bind(binding.BindPreferenceBool(bind, a.Preferences()))

	var totpChan chan struct{}
	entry.Validator = func(s string) error {
		if totpCheck.Checked || len(entry.Text) > 5 {
			return nil
		}
		return errors.New(lp("Secret must be longer than 5 characters"))
	}
	totpProg := widget.NewProgressBar()
	totpProg.Hide()

	update := func() {
		fyne.Do(func() {
			if !totpCheck.Checked || totpCheck.Disabled() {
				return
			}

			now := time.Now()
			remaining := 30 - now.Second()%30
			totpLabel.SetText(totp(entry.Text))
			totpProg.SetValue(float64(remaining) / 30)
		})
	}

	totpCheck.OnChanged = func(b bool) {
		a.Preferences().SetBool(bind, b)
		fyne.Do(func() {
			entry.SetValidationError(entry.Validate())
			// Останавливаем предыдущую горутину
			if totpChan != nil {
				close(totpChan)
				totpChan = nil
			}

			if b {
				if strings.HasPrefix(entry.Text, TOTP) {
					entry.SetText(*entryText)
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
							update()
						case <-totpChan:
							return
						}
					}
				}()
			} else {
				totpLabel.SetText(TOTP)
				totpProg.Hide()
				*entryText = entry.Text
				entry.SetText(TOTP + totp(entry.Text))
			}
		})
	}

	entry.OnChanged = func(secret string) {
		os.Setenv(CROC_SECRET, secret)
		update()
	}

	return totpProg
}

func mapEmpty(m *sync.Map) (empty bool) {
	empty = true
	m.Range(func(_, _ interface{}) bool {
		empty = false
		return false
	})
	return
}

func forEachFileEntry(fileentries *sync.Map, fn func(path string, fe *fyne.Container)) {
	tempMap := make(map[string]*fyne.Container)
	fileentries.Range(func(key, value interface{}) bool {
		tempMap[key.(string)] = value.(*fyne.Container)
		return true
	})

	for path, fe := range tempMap {
		fn(path, fe)
	}
}

func load(fileentries *sync.Map, path string) (*fyne.Container, bool) {
	path = filepath.FromSlash(path)
	if fe, ok := fileentries.Load(path); ok {
		if container, ok := fe.(*fyne.Container); ok {
			return container, true
		}
	}
	return nil, false
}

func fileTreeShow(uri fyne.URI, a fyne.App) (ft *xw.FileTree) {

	if uri == nil {
		log.Errorf("uri is nul")
		return
	}

	ft = xw.NewFileTree(uri)
	if ft == nil {
		log.Errorf("file tree is nul")
		return
	}

	if ftw != nil {
		ftw.Close()
	}

	ftw = a.NewWindow(uri.Path())
	ft.OpenAllBranches()

	ftw.SetContent(ft)

	ftw.Resize(size)
	ftw.Show()
	return
}

// .zip распаковать
// dir упаковать
func selected(uid widget.TreeNodeID, cb func(err error)) {
	if cb == nil {
		cb = func(err error) {}
	}

	u, err := storage.ParseURI(uid)
	if err != nil {
		cb(err)
		return
	}
	root := ftw.Title()
	switch ext := strings.ToLower(u.Extension()); ext {
	case DOTZIP:
		log.Tracef("unZip %v %s", u.Path(), root)
		cb(nil)
		return
	}
	cb(fmt.Errorf("default"))
}

func linkByTarget(target string, join func(elem ...string) string) (link string, err error) {
	l := ls(join())
	log.Tracef("ls %v", l)
	for _, name := range l {
		link = join(name)
		log.Tracef("link %s", link)
		if parent, e := Readlink(link); e == nil {
			log.Tracef("parent %s target %s", parent, target)
			if isCached(parent, target) {
				log.Tracef("return %s ", link)
				return
			}
		}
	}
	return "", fmt.Errorf("linkByTarget: not found link")
}

func isCached(parent, child string) bool {
	if parent == "" ||
		child == "" ||
		len(parent) > len(child) ||
		!filepath.IsAbs(parent) ||
		!filepath.IsAbs(child) {
		return false
	}
	parent = strings.ToUpper(parent[:1]) + parent[1:]
	child = strings.ToUpper(child[:1]) + child[1:]

	// Нормализуем разделители и очищаем пути
	parent = filepath.Clean(filepath.FromSlash(parent))
	child = filepath.Clean(filepath.FromSlash(child))

	if parent == child {
		return true
	}

	// Просто проверяем что child начинается с parent + разделитель
	prefix := parent + string(filepath.Separator)
	return strings.HasPrefix(child, prefix)
}

// feDel
// feSave
func button(fe *fyne.Container, do bool, i int, f func(*widget.Button)) {
	if fe == nil || len(fe.Objects) <= i {
		return
	}

	if w, ok := fe.Objects[i].(*widget.Button); ok {
		if do {
			fyne.Do(func() { f(w) })
		} else {
			f(w)
		}
	}
}

func bar(fe *fyne.Container, do bool, f func(*widget.ProgressBar)) {
	if fe == nil || len(fe.Objects) <= feBar {
		return
	}

	if w, ok := fe.Objects[feBar].(*widget.ProgressBar); ok {
		if do {
			fyne.Do(func() { f(w) })
		} else {
			f(w)
		}
	}
}

func label(fe *fyne.Container, do bool, f func(*widget.Label)) {
	if fe == nil || len(fe.Objects) < 1 {
		return
	}
	if w, ok := fe.Objects[len(fe.Objects)-1].(*widget.Label); ok {
		if do {
			fyne.Do(func() { f(w) })
		} else {
			f(w)
		}
	}
}

func trimDotSlash(fi croc.FileInfo) (s string) {
	s = filepath.Join(fi.FolderRemote, fi.Name)
	s = filepath.Clean(s)
	s = filepath.FromSlash(s)
	return
}
