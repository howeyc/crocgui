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
	"path"
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

	entryText := code
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
			return shortFormatter(progFile)
		}

		newentry = container.NewHBox(
			deleteButton,
			progFile,

			labelFile,
		)

		fileentries.Store(dst, newentry)
		fyne.Do(func() {
			setSizes(progFile, size)
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
		// log.Tracef("lsr %v", n)
		for _, path := range p {
			ext := filepath.Ext(path)
			if strings.ToLower(ext) == DOTZIP {
				dir := strings.TrimSuffix(path, ext)
				prefixs = append(prefixs, dir+slash)
			}
		}
		// log.Tracef("prefixs %v", prefixs)
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
				processedPath := path
				if target, err := Readlink(path); err == nil {
					processedPath = target
				}

				// Логика для filepaths (каталоги и файлы в корне)
				if strings.HasSuffix(l.Text, slash) || !strings.Contains(l.Text, slash) {
					filepaths = append(filepaths, processedPath)
				}

				// Логика для allowed (все файлы, но не каталоги)
				if !strings.HasSuffix(l.Text, slash) {
					allowed = append(allowed, processedPath)
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

		ZipFolder := a.Preferences().Bool("zip-unzip")

		// Пути посылаемых файлов абсолютны. Переходим в каталог только если zipfolder
		cdLocked := false
		if hasFolder(join()) && ZipFolder {
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
			GitIgnore := a.Preferences().Bool("git")
			Exclude := exclude(a.Preferences().String("exclude"))
			filesInfo, emptyfolders, totalNumberFolders, err := croc.GetFilesInfo(filepaths, ZipFolder, GitIgnore, Exclude)

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

			// log.Tracef("filesInfo %+v %v %d: %v", filesInfo, emptyfolders, totalNumberFolders, err)

			filesInfo, emptyfolders, totalNumberFolders = filter(filesInfo, emptyfolders, totalNumberFolders, Exclude, allowed...)
			// log.Tracef("filtered filesInfo %+v %v %d", filesInfo, emptyfolders, totalNumberFolders)
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

			opt := croc.Options{
				IsSender:         true,
				SharedSecret:     secret,
				Debug:            debugBool(a),
				NoPrompt:         true,
				DisableLocal:     a.Preferences().Bool("disable-local"),
				NoMultiplexing:   a.Preferences().Bool("disable-multiplexing"),
				NoCompress:       a.Preferences().Bool("disable-compression"),
				Curve:            a.Preferences().String("pake-curve"),
				HashAlgorithm:    a.Preferences().String("croc-hash"),
				ThrottleUpload:   a.Preferences().String("upload-throttle"),
				MulticastAddress: a.Preferences().String("multicast-address"),
				Exclude:          Exclude,
				ZipFolder:        ZipFolder,
				Overwrite:        a.Preferences().Bool("overwrite"),
				GitIgnore:        GitIgnore,
			}
			RelayPorts := ""
			opt.RelayPassword, opt.RelayAddress, opt.RelayAddress6, RelayPorts,
				comm.Socks5Proxy, comm.HttpProxy = def(a)
			opt.RelayPorts = strings.Split(RelayPorts, ",")
			opt.OnlyLocal = a.Preferences().Bool("force-local") || opt.RelayAddress == "" && opt.RelayAddress6 == ""

			client, err := croc.New(opt)
			if err != nil {
				log.Errorf("croc: %v", err)
				fyne.Do(NewToast(w, err.Error()).Show)

				return
			}
			log.SetLevel(debugString(a))
			log.Trace("croc client created")

			if a.Preferences().Bool("remember") {
				p := NewPreferences(a.Preferences())
				p.SetString("relay", opt.RelayAddress)
				a.Preferences().SetBool("send", true)
				saveConfig(p, opt, true)
			}

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
						// prog.SetValue(0)
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
				progW := NewLongProgressWrapper(prog)
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
							for _, fi := range client.FilesToTransfer {
								log.Tracef("fi %+v", fi)
								path := filepath.Join(fi.FolderSource, fi.Name)

								if fe, ok := load(&fileentries, path); ok {
									button(fe, true, feDel, func(d *widget.Button) {
										d.Hide()
									})
									bar(fe, true, func(p *widget.ProgressBar) {
										setSizes(p, fi.Size, 0)
									})
								} else {
									addEntry(path, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
										d.Hide()
										setSizes(p, fi.Size, 0)

										if !fi.TempFile {
											l.SetText(trimDotSlash(fi))
										}
									}) //addEntry
								}

								totalMax += fi.Size
							}
							log.Tracef("totalMax %d", totalMax)
							fyne.Do(func() {
								toplineW.SetText(lp("Have them press the Download now"))
								NewToast(w, lp("Have them press the Download now")).Show()
								progW.SetMax(totalMax)
							})
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
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// cat file|crocgui
			go func() {
				fnames, err := getStdin()
				if err != nil {
					return
				}
				utils.MarkFileForRemoval(fnames[0])
				var wga sync.WaitGroup
				for _, src := range fnames {
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
					log.Tracef("done %v", fnames)
					removeEntrys(false)
					reload()
					showPage()
				}
			}()
		}

		if len(os.Args) > 1 {
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
	// fd.SetLocation(lastLU)
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
		case *fyne.Container:
			allEnabled(enabled, w.Objects...)
		case *widget.Select:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}

		case *widget.Entry:
			w.TextStyle.Italic = !enabled
			if !enabled && w.OnChanged == nil {
				w.OnChanged = func(string) {
					if w.TextStyle.Italic {
						w.Undo()
					}
				}
			}
		case *widget.Check:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}
		case *widget.Label:
			w.TextStyle.Italic = !enabled
		case *widget.RadioGroup:
			if enabled {
				w.Enable()
			} else {
				w.Disable()
			}
		}
		co.Refresh()
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
		if entry.TextStyle.Italic {
			entry.Undo()
			return
		}
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
	cb(fmt.Errorf(DEFAULT))
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

func filter(filesInfo, emptyFoldersToTransfer []croc.FileInfo, totalNumberFolders int, exclusions []string, allowed ...string) ([]croc.FileInfo, []croc.FileInfo, int) {
	// Вспомогательная функция для проверки исключений
	shouldExclude := func(f croc.FileInfo) bool {
		if len(exclusions) == 0 {
			return false
		}
		fullPath := path.Join(strings.ToLower(f.FolderRemote), strings.ToLower(f.Name))
		for _, exclusion := range exclusions {
			if strings.Contains(fullPath, exclusion) {
				return true
			}
		}
		return false
	}

	// Вспомогательная функция для проверки разрешений
	isAllowed := func(f croc.FileInfo) bool {
		if len(allowed) == 0 {
			return true
		}
		sourcePath := filepath.Join(f.FolderSource, f.Name)
		for _, a := range allowed {
			if a == sourcePath {
				return true
			}
		}
		return false
	}

	// Функция фильтрации счета папок
	countUniqueFolders := func(files, emptyFolders []croc.FileInfo) int {
		folderMap := make(map[string]bool)
		for _, f := range files {
			folderMap[f.FolderRemote] = true
		}
		for _, f := range emptyFolders {
			folderMap[f.FolderRemote] = true
		}
		return len(folderMap)
	}

	// Применяем фильтры к filesInfo
	filteredFiles := make([]croc.FileInfo, 0, len(filesInfo))
	for _, f := range filesInfo {
		if !shouldExclude(f) && isAllowed(f) {
			filteredFiles = append(filteredFiles, f)
		}
	}

	// Применяем фильтры к emptyFoldersToTransfer
	filteredEmpty := make([]croc.FileInfo, 0, len(emptyFoldersToTransfer))
	for _, f := range emptyFoldersToTransfer {
		if !shouldExclude(f) && isAllowed(f) {
			filteredEmpty = append(filteredEmpty, f)
		}
	}

	// Считаем уникальные папки
	total := countUniqueFolders(filteredFiles, filteredEmpty)

	return filteredFiles, filteredEmpty, total
}

func exclude(e string) (exclusions []string) {
	for _, v := range strings.Split(e, ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			exclusions = append(exclusions, v)
		}
	}
	return
}

func getStdin() (fnames []string, err error) {
	f, err := os.CreateTemp("", STDIN+"*")
	if err != nil {
		return nil, err
	}
	fileName := f.Name()
	defer f.Close()

	_, err = io.Copy(f, os.Stdin)
	if err != nil {
		return nil, err
	}

	if err := f.Close(); err != nil {
		return nil, err
	}

	return []string{fileName}, nil
}

func GetConfigDir(requireValidPath bool) (homedir string, err error) {
	if envHomedir, isSet := os.LookupEnv("CROC_CONFIG_DIR"); isSet {
		homedir = envHomedir
	} else if xdgConfigHome, isSet := os.LookupEnv("XDG_CONFIG_HOME"); isSet {
		homedir = filepath.Join(xdgConfigHome, "croc")
	} else {
		if isMobile || asMobile {
			homedir = tempDir
		} else {
			homedir, err = os.UserHomeDir()
			if err != nil {
				if !requireValidPath {
					err = nil
					homedir = ""
				}
				return
			}
		}
		homedir = filepath.Join(homedir, ".config", "croc")
	}

	if requireValidPath {
		if _, err = os.Stat(homedir); os.IsNotExist(err) {
			err = os.MkdirAll(homedir, 0o700)
		}
	}
	return
}

func def(a fyne.App) (p, r, r6, ps, s, h string) {
	p = defs(pass, a.Preferences().String("relay-password"), DEFAULT_PASSPHRASE)
	if p != DEFAULT_PASSPHRASE && !(isMobile || asMobile) {
		p = determinePass(p)
	}
	r = defs(relay4, a.Preferences().String("relay-address"))
	r6 = defs(relay6, a.Preferences().String("relay6"))
	ps = defs(a.Preferences().String("relay-ports"), ports0)
	s = defs(socks5, a.Preferences().String("socks5"))
	h = defs(connect, a.Preferences().String("connect"))
	return
}
