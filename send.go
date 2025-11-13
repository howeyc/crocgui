// send.go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
)

const (
	MaterialFiles = "https://github.com/zhanghai/MaterialFiles"
	MiXplorer     = "https://mixplorer.com/beta/"
	filePicker    = MiXplorer
	INSTALL       = "URL " + filePicker + " is already in the clipboard.\nInstall the app to avoid this message."
	feDel         = 0
	feBar         = 1
	feSave        = 2
	PSL           = "→"
)

func sendTabItem(a fyne.App, w fyne.Window, parent *container.AppTabs) (ti *container.TabItem) {
	index := 0
	showPage := func() {}
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	var cosED, cosHS []fyne.CanvasObject
	prog := widget.NewProgressBar()
	cosHS = append(cosHS, prog)

	topline := widget.NewLabel(lp("Pick a file to send"))

	entry := widget.NewEntry()
	cosED = append(cosED, entry)

	randomCode := utils.GetRandomName()

	entryText := os.Getenv(CROC_SECRET)
	if entryText == "" {
		entryText = randomCode
	}
	entry.SetText(entryText)

	randomCodeButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		randomCode = utils.GetRandomName()
		entry.SetText(randomCode)
	})
	cosED = append(cosED, randomCodeButton)

	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(entry.Text)
	})

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-send", a.Preferences()))
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

			now := time.Now()
			remaining := 30 - now.Second()%30
			totpLabel.SetText(totp(entry.Text))
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
							update()
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
			a.Preferences().SetBool("totp-send", b)
		})
	}

	if totpCheck.Checked {
		totpCheck.OnChanged(true)
	}

	entry.OnChanged = func(secret string) {
		os.Setenv(CROC_SECRET, secret)
		update()
	}

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)
	ready := func() (ok bool) {
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

	// fyne.Do
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
					log.Tracef("remove %s %s: %v", de, fpath, err)
					return
				}
			}
		}
		fyne.Do(func() {
			boxholder.Remove(fe)
			boxholder.Refresh()
			delete(fileentries, fpath)
		})
	}

	// nil if exists
	// fyne.Do
	addEntry := func(dst string, f func(d *widget.Button, p *widget.ProgressBar, l *widget.Label)) (newentry *fyne.Container) {
		if _, has := fileentries[dst]; has {
			log.Tracef("exists %s", dst)
			return nil
		}
		base := filepath.Base(dst)
		if fi, _ := os.Stat(dst); fi != nil && fi.IsDir() {
			base += slash
		}
		labelFile := widget.NewLabel(base)
		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if entry.Disabled() {
				log.Trace("Sending")
			} else {
				if fe, ok := fileentries[dst]; ok {
					removeEntry(dst, fe, true)
				} else {
					os.Remove(dst)
				}
			}
		})
		progFile := widget.NewProgressBar()
		newentry = container.NewHBox(
			deleteButton,
			progFile,
			labelFile,
		)

		fileentries[dst] = newentry
		fyne.Do(func() {
			if f == nil {
				progFile.Hide()
			} else {
				f(deleteButton, progFile, labelFile)
			}
			boxholder.Add(newentry)
			boxholder.Refresh()
		})
		return
	}

	// Пишу ссылку а если не удачно то кэширую
	addPath := func(src string) error {
		fi, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %v", src, err)
		}

		base := filepath.Base(src)
		dst := join(base)

		fe := addEntry(dst, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
			p.Hide()
			if fi.IsDir() {
				l.SetText(base + slash)
			}
		})

		if fe == nil {
			log.Tracef("entries %s has %s", base, src)
			return nil
		}

		if _, err := os.Stat(dst); err == nil {
			// Также и когда src==dst
			log.Tracef("cache %s has %s", dst, src)
			return nil
		}

		err = Symlink(src, dst)
		log.Tracef("symlink %s %s: %v", src, dst, err)
		if err == nil {
			return nil
		}

		go func() {
			// addPath
			log.Tracef("copyFiles: %v", copyFiles(storage.NewFileURI(src), dst, func(u fyne.URI, dstPath string) error {
				fyne.Do(func() {})
				feCopy := fe
				src := u.Path()
				if fi.IsDir() {
					// Создаю временный прогрессбар
					rel, err := filepath.Rel(join(), dstPath)
					if err != nil {
						rel = dstPath
					}
					feCopy = addEntry(dstPath, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
						l.SetText(rel)
					})
					if feCopy == nil {
						return nil
					}
				}
				CopyFileProgress(src, dstPath, feCopy, func(err error) {
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
					if feCopy != fe {
						// Удалю временный прогрессбар без удаления файла
						removeEntry(dstPath, feCopy, false)
					}
				})
				return nil
			}))
		}()

		return nil
	}

	copyFromURCProgress := func(source fyne.URIReadCloser, dst string, c *fyne.Container, onComplete func(err error)) {
		if source == nil {
			onComplete(fmt.Errorf("user cancel dialog"))
			return
		}
		if dst == "" {
			u := source.URI()
			name := uriBase(u)
			dst = join(name)
		}
		destination, err := os.Create(dst)
		if err != nil {
			source.Close()
			onComplete(fmt.Errorf("unable to create file %s error: %s", dst, err.Error()))
			return
		}

		total, err := getSize(source.URI())
		if err != nil {
			total = 1 << 30
		}
		pw, restore := NewProgressWriter(destination, total, c)

		go func() {
			_, err := io.Copy(pw, source)
			source.Close()
			destination.Close()
			restore()
			onComplete(err)
		}()
	}

	os.MkdirAll(join(), 0700)

	reload := func() {
		for _, name := range ls(join()) {
			if name != "" {
				fpath := join(name)
				if target, err := Readlink(fpath); err == nil {
					fpath = target
				} else {
					if isDir, count, _ := fileChild(fpath); isDir && count < 1 {
						log.Tracef("remove empty dir %s: %v", fpath, os.Remove(fpath))
						continue
					}
				}
				addPath(fpath)
			}
		}
		// keysToRemove := []string{}
		// for path, _ := range fileentries {
		// 	if _, err := os.Stat(path); err != nil {
		// 		keysToRemove = append(keysToRemove, path)
		// 	}
		// }

		// for _, path := range keysToRemove {
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

	reload()

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
						// source, err := storage.Reader(u)
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
			for _, src := range os.Args[1:] {
				src, err := filepath.Abs(src)
				if err != nil {
					log.Warnf("abs %s: %v", src, err)
					continue
				}
				if err := addPath(src); err != nil {
					log.Error(err.Error())
				}
			}
		}

		w.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
			if len(uris) == 0 {
				return
			}
			if entry.Disabled() {
				log.Trace("Sending")
				return
			}
			for _, uri := range uris {
				if err := addPath(uri.Path()); err != nil {
					log.Error(err.Error())
				}
			}
			showPage() //SetOnDropped
		})
	}

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
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
			fe := addEntry(dst, nil)
			if fe == nil {
				return
			}
			src := u.String()

			err := Symlink(u.Path(), dst)
			log.Tracef("symlink %s %s: %v", u.Path(), dst, err)
			if err == nil {
				return
			}

			raf := func() {}
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
					// raf()
					return
				}

				if _, err := os.Stat(dst); err != nil {
					log.Errorf("stat %s:  %v", dst, err)
					removeEntry(dst, fe, false)
				} else {
					log.Tracef("copy %s %s", src, dst)
				}
				// raf()
			})
		}, w)
	})
	cosED = append(cosED, addFileButton)

	addFolderButton := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
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
				p.Hide()
				l.SetText(name + slash)
			})
			if fe == nil {
				return
			}

			err := Symlink(u.Path(), dst)
			log.Tracef("symlink %s %s: %v", u.Path(), dst, err)
			if err == nil {
				// Десктоп
				return
			}

			log.Tracef("copyFiles error: %v", copyFiles(u, dst, func(src fyne.URI, dstPath string) error {
				fyne.Do(func() {})
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
				copyFromURCProgress(source, dstPath, feCopy, func(err error) {
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
					removeEntry(dstPath, feCopy, false)
				})
				return nil
			}))
			reload()
		}
		ShowFolderOpen(folderOpen, w)
	})
	cosED = append(cosED, addFolderButton)

	cancelChan := make(chan struct{})
	var cancelButton, mainButton *widget.Button

	removeEntrys := func(del bool) {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe, del)
		}
	}

	deleteAllButton := widget.NewButtonWithIcon("*", theme.DeleteIcon(), func() {
		if len(fileentries) > 0 {
			removeEntrys(true)
		} else {
			entry.SetText("")
		}
	})
	cosED = append(cosED, deleteAllButton)

	var reDir *widget.Button

	reDir = widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if !ready() {
			log.Error("not all files ready for send")
			return
		}
		if !recvReady() {
			log.Error("not all files ready for recv")
			return
		}

		fyne.Do(func() {
			swap = !swap
			if swap {
				reDir.SetIcon(theme.NavigateNextIcon())
				join = func(elem ...string) string {
					return filepath.Join(append([]string{tempDir, RECV}, elem...)...)
				}
			} else {
				reDir.SetIcon(theme.NavigateBackIcon())
				join = func(elem ...string) string {
					return filepath.Join(append([]string{tempDir, SEND}, elem...)...)
				}
			}
			removeEntrys(false)
			reload()
		})
	})
	cosED = append(cosED, reDir)

	mainButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		ok := len(entry.Text) > 5
		if totpCheck.Checked {
			ok = len(entry.Text) > 0
		}
		if !ok {
			log.Error("no receive code entered")
			dialog.ShowInformation(
				lp("Send"),
				lp("Enter code to download"),
				w,
			)
			return
		}

		if !ready() {
			dialog.ShowInformation(
				lp("Send"),
				lp("Pick a file to send"),
				w,
			)
			return
		}
		filepaths := []string{}
		for fpath, _ := range fileentries {
			if target, err := Readlink(fpath); err == nil {
				fpath = target
			}
			filepaths = append(filepaths, fpath)
		}
		zipfolder := a.Preferences().Bool("zip-unzip")
		cderr := os.Chdir(tempDir)
		if cderr != nil {
			log.Errorf("change to %s: %v", tempDir, cderr)
		}
		log.Trace("cd ", tempDir)
		filesInfo, emptyfolders, totalNumberFolders, serr := croc.GetFilesInfo(filepaths, zipfolder, false, []string{})

		// Посылаем если есть файлы
		if len(filepaths) < 1 || serr != nil {
			log.Error("no files ready")
			dialog.ShowInformation(
				lp("Send"),
				lp("Pick a file to send"),
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
		for _, fe := range fileentries {
			fe.Objects[feDel].Hide()
		}
		sender, err := croc.New(croc.Options{
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
			return
		}
		log.SetLevel(debugString(a))
		log.Trace("croc sender created")

		var filename string
		cancelChan = make(chan struct{})
		cancelButton.Show()
		allEnabled(false, cosED...)

		if totpCheck.Checked {
			totpProg.Hide()
		}

		doneChan := make(chan struct{})

		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer func() {
				ticker.Stop()
				fyne.Do(func() {
					// Восстанавливаю
					// mainButton.Enable()
					// prog.Hide()
					prog.SetValue(0)
					// cancelButton.Hide()
					allShow(false, cosHS...)
					// entry.Enable()
					// addFileButton.Enable()
					// addFolderButton.Enable()

					// totpCheck.Enable()
					allEnabled(true, cosED...)

					if totpCheck.Checked {
						totpProg.Show()
					} else if entry.Text == randomCode {
						randomCode = utils.GetRandomName()
						entry.SetText(randomCode)
					}
					reload()
				})
			}()

			old := 0
			oldPath := ""
			progW := NewProgressWrapper(prog)
			var TotalSent, size, totalMax int64
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
					Stop(sender)
					fyne.Do(func() {
						restart(w)
					})
					return
				case <-ticker.C:
					if sender == nil {
						return
					}
					if once && hashed(sender) {
						// Готов давать
						once = false
						for _, fi := range sender.FilesToTransfer {
							path := join(fi.Name)
							if fi.TempFile {
								path = filepath.Join(fi.FolderSource, fi.Name)
							}

							if fe := fileentries[path]; fe != nil {
								if pb := fe.Objects[feBar].(*widget.ProgressBar); pb != nil {
									pb.Max = float64(fi.Size)
								}
							} else {
								// Временный прогрессбар
								addEntry(path, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
									d.Hide()
									p.Max = float64(fi.Size)
									p.Show()
									if !fi.TempFile {
										l.SetText(fi.FolderRemote + fi.Name)
									}
								})
								// Убираем dir/
								path = join(fi.FolderRemote)
								if fi.TempFile {
									path = join(strings.TrimSuffix(fi.Name, DOTZIP))
								}

								if fr, ok := fileentries[path]; ok {
									fyne.Do(func() {
										boxholder.Remove(fr)
										delete(fileentries, path)
									})
								}
							}
							totalMax += fi.Size
						}
						fyne.Do(func() {
							toplineW.SetText(lp("Have them press the Download now"))
							prog.Show()
							for _, fe := range fileentries {
								pb := fe.Objects[feBar].(*widget.ProgressBar)
								pb.SetValue(0)
								pb.Show()
							}
						})
						progW.SetMax(totalMax)
						log.Tracef("totalMax %d", totalMax)
					}
					if sender.Step2FileInfoTransferred {
						cnum := sender.FilesToTransferCurrentNum
						if old < cnum+1 {
							old = cnum + 1
							fi := sender.FilesToTransfer[cnum]
							filename = fi.Name
							toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Sending file"), filename, cnum+1, len(sender.FilesToTransfer)))
							TotalSent += size
							size = fi.Size
							path := join(fi.Name)
							if oldPath != path {
								if fe := fileentries[oldPath]; fe != nil {
									removeEntry(oldPath, fe, true)
								}
								oldPath = path
							}
							log.Trace(path)
							if fe, ok := fileentries[path]; ok {
								fepw = NewProgressWrapper(fe.Objects[feBar].(*widget.ProgressBar))
							} else {
								fepw = NewProgressWrapper(nil)
							}
						}
						progW.SetValue(TotalSent + sender.TotalSent)
						fepw.SetValue(sender.TotalSent)
					}
				}
			}
		}()

		go func() {
			if EMULATE == 0 {
				serr = sender.Send(filesInfo, emptyfolders, totalNumberFolders)
			} else {
				log.Warnf("send %v %v %d", filesInfo, emptyfolders, totalNumberFolders)
				time.Sleep(EMULATE)
				defer func() {
					sender = nil
				}()
			}

			fyne.Do(func() {
				if serr != nil {
					if errors.Is(serr, io.EOF) {
						serr = fmt.Errorf("%s", lp("Receive cancelled."))
					}
					s := fmt.Sprintf("send: %v", serr)
					log.Error(s)
					topline.SetText(s)
				} else {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Sent file"), filename))
				}
			})
			close(doneChan)
		}()

		// +12 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
		a.Clipboard().SetContent(entry.Text)
	})
	cosED = append(cosED, mainButton)

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(cancelChan)
	})
	// cancelButton.Hide()
	cosHS = append(cosHS, cancelButton)
	allShow(false, cosHS...)

	top := container.NewVBox(
		container.NewHBox(topline,
			layout.NewSpacer(),
			addFolderButton,
			addFileButton,
			randomCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Send Code"), Widget: entry}),
		container.NewHBox(
			copyCodeButton,
			totpCheck,
			totpLabel,
			totpProg,
			layout.NewSpacer(),
			deleteAllButton,
			reDir,
		),
		mainButton,
		prog,
		cancelButton,
	)

	ti = container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
	// fyne.Do
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
	// if isMobile {
	// 	sendNotification(a, "CrocGUI", "Application closed. Tap to start it.")
	// 	w.Close()
	// 	os.Exit(0)
	// 	return
	// }
	start()
	w.Close()
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
