// send.go
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

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
	"github.com/schollz/croc/v10/src/message"
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

func sendTabItem(a fyne.App, w fyne.Window) (ti *container.TabItem) {
	var (
		cosED,
		cosSH,
		cosDAV,
		cosDAVremote []fyne.CanvasObject
		removeEntry func(fpath string, fe *fyne.Container, del bool)
		showPage    func()
		reload      func()
		treeOff     = func() {}
		scRefresh   = func() {}

		boxholder = container.NewVBox()
		scroller  = container.NewVScroll(boxholder)

		mainButton  *widget.Button
		prog        = widget.NewProgressBar()
		fileentries sync.Map
		treeButton  = widget.NewButtonWithIcon("", theme.VisibilityIcon(), nil)
	)
	var (
		addEntry func(dst string, f func(d *widget.Button, p *widget.ProgressBar,

			l *widget.Label)) (newentry *fyne.Container)
		reDir *widget.Button
	)
	var cancelButton *widget.Button
	cancelChan := make(chan struct{}, 1)
	hideCancel := func() {
		fyne.Do(func() {
			cancelButton.Hide()

			mainButton.Show()
			davServer.SetLocal(false)
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
			davServer.SetLocal(true)

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

	topline := widget.NewLabel(lp("Pick a file to send"))

	entry := widget.NewEntryWithData(binding.BindPreferenceString("secret", a.Preferences()))
	cosED = append(cosED, entry)

	entryText := code
	if entryText != "" {
		entry.SetText(entryText)
	}

	cbButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		setClipboard(a)
		if qr != nil {
			qr.Show()
		}
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
				scRefresh()
			})
		}
		if del {
			isDir := isLinkDir(path)
			cached := isCached(join(), path)
			if !cached {
				// ссылка
				link, err := linkByTarget(path, join)
				log.Debugf("link %v, err %v", link, err)
				if err != nil {
					log.Debugf("%v", err)
					boxout()
					return
				}
				path = link
				isDir = isLinkDir(path)
				del = !isDir
			}
			l := []string{path}
			remove := os.Remove
			d := "file"

			if _, err := os.Stat(path); err == nil && del {
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
							scRefresh()
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
		// log.Debugf("addEntry %s", dst)
		if _, ok := load(&fileentries, dst); ok {
			// log.Debugf("exists %s", dst)
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
			scRefresh()
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
			log.Debugf("recached %s: %v", base, os.RemoveAll(dst))
			NewToast(w, "recached "+base).Show()
		}

		wga.Add(1)
		err = Symlink(src, dst)
		log.Debugf("symlink %s %s: %v", src, dst, err)
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
			log.Debugf("copyFiles: %v",
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
							log.Debugf("copy %s %s", src, dstPath)
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
			case <-appCtx.Done():
			default:
				wg.Wait()
				log.Debugf("copyFiles done")
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
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			close()
			onComplete(fmt.Errorf("unable to create directory %s error: %s", filepath.Dir(dst), err.Error()))
			return
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
		log.Debugf("getSize %s %d: %v", source.URI(), total, err)
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
					log.Debugf("Chtimes %s %v: %v", destination.Name(), t,
						os.Chtimes(destination.Name(), time.Time{}, t))
				}
			}
			restore()
			onComplete(err)
		}()
	}

	os.MkdirAll(join(), 0700)

	reload = func() {
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

				l *widget.Label) {
				d.Show()

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
	OnSelectedTab[SENDi] = reload

	reload()

	// Функция для создания WebDAV дерева
	createWebDAVTree := func(webdavURL *url.URL) *WebDAVFileTree {
		log.Debugf("[createWebDAVTree] Creating WebDAV tree for: %s", webdavURL.String())
		ft := NewWebDAVFileTree(webdavURL)

		// Обновляем дерево - Refresh() сам проверит соединение
		ft.Refresh()

		ft.OnSelected = func(uid widget.TreeNodeID) {
			log.Debugf("[createWebDAVTree] OnSelected: %s", uid)

			var fullURLStr string
			if !strings.HasPrefix(uid, webdavURL.String()) {
				fullURLStr = fmt.Sprintf("%s://%s%s", webdavURL.Scheme, webdavURL.Host, uid)
			} else {
				fullURLStr = uid
			}

			log.Debugf("[createWebDAVTree] Opening URL: %s", fullURLStr)
			chatOpened.Store(true) // сброс: браузер открыт вручную через дерево
			time.AfterFunc(100*time.Millisecond, func() {
				OpenURL(fullURLStr)
			})
			ft.Unselect(uid)
		}

		// ft.OpenAllBranches()
		return ft
	}

	// Старуем вебдав-сервер с новыми параметрами
	updateLink := func() {}
	prev := a.Preferences().String("webdav-host")
	if !slices.Contains(hostSelectOptions(LOCAL), prev) {
		prev = hostSelectOptions(LOCAL)[0]
	}
	sCheck := widget.NewCheck("s", func(b bool) {
		a.Preferences().SetBool("webdavs", b)
		updateLink()
	})
	sCheck.SetChecked(a.Preferences().Bool("webdavs"))
	cosED = append(cosED, sCheck)

	hostSelect := NewSelect(hostSelectOptions(LOCAL), func(next string) {
		if prev != next {
			prev = next
			a.Preferences().SetString("webdav-host", prev)
			updateLink()
		}
	})
	cosED = append(cosED, hostSelect)

	link := widget.NewHyperlink("", nil)
	link.OnTapped = func() {
		s := link.URL.String()
		a.Clipboard().SetContent(s)
		NewToast(w, s).Show()
		if !(isMobile || asMobile || davServer.IsTCPForwardingActive()) && hostSelect.Selected == LOCAL {
			// для десктопов и без прокси и локально
			root := join()
			var err error
			if base := filepath.Base(root); CanCreateSymlinks() && !isWSL() || base == RECV {
				// без псевдоссылок в файловом менеджере
				err = OpenURL(root)
			} else {
				// с псевдоссылками в через вебдав
				err = OpenDAV(link.URL.String())
			}
			if err == nil {
				return
			}
			log.Errorf("OpenURL: %v", err)
		}
		// для мобильных или с псевдоссылками
		if qr != nil {
			qr.Show()
		}
	}
	port := widget.NewEntry()

	// Функция для переключения на WebDAV дерево
	switchToWebDAVTree := func() {
		if davServer.IsActive() || davServer.IsTCPForwardingActive() {
			_, ccn, proxyURL, _ := isDAV(link.URL.String())
			log.Debugf("[switchToWebDAVTree] ccn=%q proxyURL=%q", ccn, proxyURL)
			chatURL = ccn
			chatOpened.Store(false)
			// Polling goroutine: опрашивает /api/messages для auto-open
			go func() {
				// Начальный fetch — устанавливаем baseline (все текущие сообщения)
				resp, err := insecureHTTPClient.Get(proxyURL.String() + "/api/messages")
				if err != nil {
					log.Debugf("[polling] initial fetch error: %v", err)
					return
				}
				var initialMsgs []Message
				json.NewDecoder(resp.Body).Decode(&initialMsgs)
				resp.Body.Close()
				lastCount := len(initialMsgs)
				log.Debugf("[polling] baseline: %d messages", lastCount)

				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-appCtx.Done():
						return
					case <-ticker.C:
						if chatOpened.Load() {
							return // браузер уже открыт, JS возьмёт на себя
						}
						// Запрашиваем только новые сообщения с индекса lastCount
						resp, err := insecureHTTPClient.Get(fmt.Sprintf("%s/api/messages?since=%d", proxyURL.String(), lastCount))
						if err != nil {
							log.Debugf("[polling] error: %v, stopping", err)
							return // ошибка → прекращаем
						}
						var newMsgs []Message
						json.NewDecoder(resp.Body).Decode(&newMsgs)
						resp.Body.Close()
						if len(newMsgs) > 0 {
							lastCount += len(newMsgs)
							if chatOpened.CompareAndSwap(false, true) && chatURL != "" {

								log.Debugf("[polling] auto-opening browser: %s", chatURL)
								OpenURL(chatURL)
							}
							return // браузер открыт → прекращаем, JS дальше
						}
					}
				}
			}()
			scroller.Content = createWebDAVTree(proxyURL)
			de.Bounce(ti.Content.Refresh)
		}
	}

	updateLink = func() {
		addr, u := defWeb(
			DAV,
			sCheck.Checked,
			hostSelect.Selected,
			port.Text,
			"")
		link.SetText(addr)
		link.SetURL(&u)
		link.Show()

		// Обновляем адрес в TCP форвардере если активен
		davServer.UpdateForwardingAddr(addr)
		if treeButton.Icon == theme.VisibilityIcon() {
			return
		}

		if davServer.IsRemote() {
			go switchToWebDAVTree()
			return
		}

		davServer.Start(addr, join(), sCheck.Checked, hostSelect.Options...)
		time.AfterFunc(time.Second, func() {
			if !davServer.IsActive() {
				fyne.Do(func() {
					link.Hide()
					de.Bounce(ti.Content.Refresh)
				})
			} else {
				go switchToWebDAVTree()
			}
		})
	}

	port.OnSubmitted = func(s string) {
		a.Preferences().SetString("webdav-port", s)
		updateLink()
	}

	hostSelect.SetSelected(prev)
	port.SetText(a.Preferences().String("webdav-port"))
	cosED = append(cosED, port)
	cosDAV = append(cosDAV, sCheck, hostSelect, port)
	cosDAVremote = append(cosDAVremote, sCheck, hostSelect, port)

	davControl := container.NewBorder(
		nil,
		nil,
		container.NewHBox(sCheck,
			container.NewGridWrap(widget.NewLabel("\t\t\t").MinSize(), hostSelect)), link,
		container.NewGridWrap(widget.NewLabel("\t").MinSize(), port),
	)
	davControl.Hide()

	// Регистрируем callback для отслеживания состояния прокси
	davServer.SetProxyStateChangeCallback(func(enabled bool) {
		fyne.Do(func() {
			allEnabled(!enabled, cosED...)
			allEnabled(!enabled, cosSH...)
			if enabled {
				go switchToWebDAVTree()
				allEnabled(true, cosDAVremote...)
				showPage()
			} else {
				if treeButton.Icon == theme.VisibilityIcon() {
					davControl.Hide()
				}
			}
		})
	})

	// Обновляем скроллер
	scRefresh = func() {
		if treeButton.Icon == theme.VisibilityIcon() {
			de.Bounce(boxholder.Refresh)
		} else {
			switchToWebDAVTree()
		}
	}

	treeButton.OnTapped = func() {
		if treeButton.Icon == theme.VisibilityIcon() {
			treeButton.SetIcon(theme.VisibilityOffIcon())
			updateLink()
			davControl.Show()
			scRefresh()
			return
		}
		treeOff()
	}

	treeOff = func() {
		treeButton.SetIcon(theme.VisibilityIcon())
		if davServer.IsRemote() && !mainButton.Disabled() ||
			!davServer.IsRemote() && mainButton.Visible() {
			davControl.Hide()
		}
		scroller.Content = boxholder
		de.Bounce(ti.Content.Refresh)
		davServer.Stop()
	}
	cosED = append(cosED, treeButton)
	cosDAV = append(cosDAV, treeButton)
	cosDAVremote = append(cosDAVremote, treeButton)

	mainButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		fyne.Do(func() { topline.SetText("") })
		if entry.Validate() != nil {
			log.Error("no receive code entered")
			fyne.Do(func() {
				topline.SetText(lp("Secret must be longer than 5 characters"))
				NewToast(w, lp("Secret must be longer than 5 characters")).Show()
			})
			return
		}

		if !seady() || swap && !ready() {
			fyne.Do(func() {
				topline.SetText(lp("Pick a file to send"))
				NewToast(w, lp("Pick a file to send")).Show()
			})
			return
		}
		filepaths := []string{}
		allowed := []string{}
		if treeButton.Icon == theme.VisibilityIcon() {
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
			log.Debugf("filepaths %v", filepaths)
			log.Debugf("allowed %v", allowed)

			// Посылаем если есть файлы
			if len(filepaths) < 1 {
				log.Error("no files ready")
				fyne.Do(func() {
					topline.SetText(lp("Pick a file to send"))
					NewToast(w, lp("Pick a file to send")).Show()
				})
				return
			}
		}

		ZipFolder := a.Preferences().Bool("zip-unzip")

		// Пути посылаемых файлов абсолютны. Переходим в каталог только если zipfolder
		cdLocked := false
		if hasFolder(join()) && ZipFolder && treeButton.Icon == theme.VisibilityIcon() {
			log.Debug("hasFolders(join()) && zipfolder")
			if cdLock.CompareAndSwap(0, 1) {
				cdLocked = true
				log.Debug("cdLocked = true")
				if wd, _ := os.Getwd(); wd != join() {
					err := os.Chdir(join())
					log.Debugf("change to %s: %v", join(), err)
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

		// progress
		go func() {
			var (
				GitIgnore = a.Preferences().Bool("git")
				Exclude   = exclude(a.Preferences().String("exclude"))
				filesInfo = []croc.FileInfo{
					{Name: filepath.Base(os.DevNull),
						FolderSource: filepath.Dir(os.DevNull)}}
				emptyFolders       = []croc.FileInfo{}
				totalNumberFolders int
				err                error
			)

			if treeButton.Icon == theme.VisibilityIcon() {
				filesInfo, emptyFolders, totalNumberFolders, err = croc.GetFilesInfo(filepaths, ZipFolder, GitIgnore, Exclude)

				if cdLocked {
					if longCdLock {
						log.Debugf("CROC_CD_LOCK %v", longCdLock)
						time.Sleep(time.Second * 30)
					}
					cdLock.Store(0)
				}
				if err != nil {
					fyne.Do(NewToast(w, err.Error()).Show)
					return
				}
				if len(filesInfo) < 1 {
					fyne.Do(func() {
						topline.SetText(lp("Pick a file to send"))
						NewToast(w, lp("Pick a file to send")).Show()
					})
					return
				}

				// log.Debugf("filesInfo %+v %v %d: %v", filesInfo, emptyfolders, totalNumberFolders, err)

				filesInfo, emptyFolders, totalNumberFolders = filter(filesInfo, emptyFolders, totalNumberFolders, Exclude, allowed...)
				// log.Debugf("filtered filesInfo %+v %v %d", filesInfo, emptyfolders, totalNumberFolders)
				if len(filesInfo) < 1 {
					fyne.Do(func() {
						topline.SetText(lp("Pick a file to send"))
						NewToast(w, lp("Pick a file to send")).Show()
					})
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
				IsSender:     true,
				SharedSecret: secret,
				Debug:        debugBool(a),
				NoPrompt:     true,
				DisableLocal: a.Preferences().Bool("disable-local"),
				// Чтоб не было 2-х ридеров на одном порту
				NoMultiplexing:   a.Preferences().Bool("disable-multiplexing") || treeButton.Icon == theme.VisibilityOffIcon(),
				NoCompress:       a.Preferences().Bool("disable-compression"),
				OnlyLocal:        a.Preferences().Bool("force-local"),
				Curve:            a.Preferences().String("pake-curve"),
				MulticastAddress: a.Preferences().String("multicast-address"),

				HashAlgorithm:  a.Preferences().String("croc-hash"),
				ThrottleUpload: a.Preferences().String("upload-throttle"),
				Exclude:        Exclude,
				ZipFolder:      ZipFolder,
				// Overwrite:        a.Preferences().Bool("overwrite"),
				GitIgnore:        GitIgnore,
				Quiet:            GUI,
				IgnoreStdin:      GUI,
				DisableClipboard: true,
			}
			RelayPorts := ""
			opt.RelayPassword, opt.RelayAddress, opt.RelayAddress6, RelayPorts,
				comm.Socks5Proxy, comm.HttpProxy = def(a)
			opt.RelayPorts = strings.Split(RelayPorts, ",")
			opt.OnlyLocal = a.Preferences().Bool("force-local") || opt.RelayAddress == "" && opt.RelayAddress6 == ""

			var sendErr error

			log.Warnf("Restart %v", !noRestart)
			ctx, ctc := context.WithCancel(appCtx)
			client, err := crocNew(noRestart, ctx, opt)
			if err != nil {
				log.Errorf("croc: %v", err)
				fyne.Do(NewToast(w, err.Error()).Show)

				return
			}
			log.SetLevel(debugString(a))
			log.Debug("croc client created")

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
				if treeButton.Icon == theme.VisibilityOffIcon() {
					allEnabled(true, cosDAV...)
				}

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
				caffeinate(1)
				defer func() {
					// Конец
					davServer.DisableTCPForwarding()
					caffeinate(-1)
					ticker.Stop()
					fyne.Do(func() {
						mainButton.Show()
						davServer.SetLocal(false)
						if treeButton.Icon == theme.VisibilityIcon() {
							davControl.Hide()
						}
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
				toplineW.SetText(lp("Have them not press the Download yet"))
				fepw := NewProgressWrapper(nil)
				once := true
				for {
					select {
					case <-appCtx.Done():
						return
					case <-doneChan:
						if !swap && sendErr == nil {
							os.RemoveAll(join())
							os.MkdirAll(join(), 0700)
						}
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
						if once && hashed(client) {
							// Начало передачи
							once = false
							for _, fi := range client.FilesToTransfer {
								log.Debugf("fi %+v", fi)
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
							log.Debugf("totalMax %d", totalMax)
							//setClipboard(opt.SharedSecret, a)
							setClipboard(a)
							fyne.Do(func() {
								toplineW.SetText(lp("Have them press the Download now"))
								NewToast(w, lp("Have them press the Download now")).Show()
								if treeButton.Icon == theme.VisibilityIcon() {
									progW.SetMax(totalMax)
								}
							})
						}
						if client.Step1ChannelSecured {
							if treeButton.Icon == theme.VisibilityOffIcon() && davServer.IsActive() {
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
									if !swap {
										if fe, ok := load(&fileentries, oldPath); ok {
											removeEntry(oldPath, fe, true)
										}
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

			// Send
			go func() {
				if EMULATE == 0 {
					sendErr = client.Send(filesInfo, emptyFolders, totalNumberFolders)
				} else {
					log.Warnf("Send %v %v %d", filesInfo, emptyFolders, totalNumberFolders)
					time.Sleep(EMULATE)
					defer func() {
						time.Sleep(time.Millisecond * 10)
						client = nil
					}()
				}

				fyne.Do(func() {
					if sendErr != nil {
						if errors.Is(sendErr, io.EOF) ||
							errors.Is(sendErr, context.Canceled) {
							sendErr = fmt.Errorf("%s", lp("Receive cancelled."))
						}
						s := fmt.Sprintf("send: %v", sendErr)
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
		//		a.Clipboard().SetContent(entry.Text)
	}) // mainButton
	cosED = append(cosED, mainButton)

	if isAndroid {
		var (
			intentWg sync.WaitGroup
		)
		intentCtx, intentCancel := context.WithCancel(appCtx)

		oH := ""
		mH := ""
		// Если получили файл или текст то исключаем из Недавних
		// Одни файловые менеджеры это делают сами
		// другим типа totalcommander надо помогать
		// но у меня не получилось
		excludeRecents := false
		a.Lifecycle().SetOnExitedForeground(func() {
			log.Debug("ExitedForeground " + wHandle(w))
			if !notFinish && treeButton.Icon == theme.VisibilityIcon() {
				if excludeRecents {
					// Для Андроида 9 это просто finish
					// excludeFromRecents()
					finish()
				} else {
					// if md, ok := a.Driver().(mobile.Driver); ok && md != nil {
					// 	md.GoBack()
					// }
					finish()
				}
			}
		})
		a.Lifecycle().SetOnStopped(func() {
			log.Debug("Stopped " + wHandle(w))
			saveAccordionState()
		})
		a.Lifecycle().SetOnStarted(func() {
			log.Debug("Started " + wHandle(w))
			oH = wHandle(w)
		})
		a.Lifecycle().SetOnEnteredForeground(func() {
			log.Debug("EnteredForeground " + wHandle(w))
			notFinish = false
			// excludeRecents = false

			// 1. Сигнал остановки через контекст (моментально)
			intentCancel()

			// 2. Ждём завершения старой горутины
			intentWg.Wait()

			// 3. Очищаем мусор из каналов (теперь безопасно — горутина мертва)
			drainChannel(uriFromIntent)
			drainChannel(textFromIntent)

			// 4. Новый контекст для новой горутины
			intentCtx, intentCancel = context.WithCancel(appCtx)

			intentWg.Add(1)
			go func() {
				defer intentWg.Done()

				tt := time.NewTicker(time.Millisecond * 777)
				defer tt.Stop()
				for {
					select {
					case <-intentCtx.Done():
						log.Debug("intent goroutine stopped")
						return
					case <-tt.C:
						// В Андроид 9 если нажать Хоум или кнопку Недавние
						// то ни один из хуков lifecycle не сработает.
						// Если выбрать не crocgui а потом выбрать crocgui
						// то crocgui зависнет.
						// Чтоб это предотвратить ослеживаем смену хэндла окна w
						// и в этот момент открепляем и прикрепляем активность к w
						nH := wHandle(w)
						if oH != nH {
							if mH != oH {
								log.Errorf("mH %s wH %s-> nH %s", mH, oH, nH)
								finish()
								time.Sleep(time.Millisecond * 777)
								startActivity()
								return
							}
							oH = nH
						}
					case text := <-textFromIntent:
						if text == "" {
							// Ошибка обработки Намерения или Главная или из Недавних
							log.Debug("doneProcessIntent notFinish")
							notFinish = true
							return
						}
						// excludeRecents = true
						if entry.Disabled() {
							log.Debug("doneProcessIntent Sending")
							return
						}
						log.Debugf("clip\n%s", text)
						src := join(hashToFilename(text))
						if fe := addEntry(src, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
							setSizes(p, int64(len(text)))
						}); fe == nil {
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
							log.Debug("doneProcessIntent")
							return
						}

						// deepLink https://abakum.github.io/croc#
						if st, ne, as, a6, ps, pd, s5, ct, err := fromURI(uriString); err == nil {
							var _, _, _ = pd, s5, ct
							switch st {
							case "App info":
								idActions(ID, APP_OPEN_BY_DEFAULT_SETTINGS, APPLICATION_DETAILS_SETTINGS)
								return
							}
							entry.SetText(st)
							a.Preferences().SetString("new-relay", ne)
							a.Preferences().SetString("relay-address", as)
							a.Preferences().SetString("relay6", a6)
							a.Preferences().SetString("relay-ports", ps)
							// a.Preferences().SetString("relay-password", pd)
							// a.Preferences().SetString("socks5", s5)
							// a.Preferences().SetString("connect", ct)
							addCurrentRelay(a)
							return
						}
						// deepLink davX: webdavX:
						if _, ccn, _, ok := isDAV(uriString); ok {

							log.Debugf("[intent] isDAV ccn=%q, opening manually", ccn)
							if err := OpenURL(ccn); err == nil {
								chatOpened.Store(true)
							} else {
								log.Error(err)
							}
							return
						}

						excludeRecents = true
						if entry.Disabled() {
							log.Debug("Sending")
							log.Debug("doneProcessIntent")
							return
						}
						u, err := storage.ParseURI(uriString)
						if err != nil {
							log.Errorf("parse %s: %v", u, err)
							continue
						}
						log.Debugf("uri %s", u)
						log.Debugf("apiLevel %d", apiLevel())
						name := uriBase(u)
						dst := join(name)
						if u.Scheme() == "file" {
							// TotalCommander до Андроида 12
							// может посылать каталоги
							if !HasStoragePermission() {
								RequestStoragePermission()
								NewToast(w, lp("Allow access to read")+": "+name).Show()
								return
							}
							fe := addEntry(dst, nil)
							if fe == nil {
								continue
							}
							if fi, err := os.Stat(u.Path()); err == nil {
								if fi.IsDir() {
									go func() {
										var wg sync.WaitGroup
										log.Debugf("copyFiles: %v",
											copyFiles(storage.NewFileURI(u.Path()), dst, func(u fyne.URI, dstPath string) error {
												src := u.Path()
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
														log.Debugf("copy %s %s", src, dstPath)
													}
												})
												return nil
											}))
										select {
										case <-appCtx.Done():
										default:
											wg.Wait()
											log.Debugf("copyFiles done")
											showPage()
										}
									}()
									continue
								}
								CopyFileProgress(u.Path(), dst, fe, func(err error) {
									showPage()
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
								continue
							}
						}

						if IsDirectory(u) {
							continue
						}

						source, err := Reader(u)
						if err != nil {
							log.Errorf("reader: %v", err)
							continue
						}
						fe := addEntry(dst, nil)
						if fe == nil {
							continue
						}

						copyFromURCProgress(source, "", fe, func(err error) {
							showPage() //uriFromIntent
							if err != nil {
								log.Errorf("copy %s %s: %s", u, dst, err)
								removeEntry(dst, fe, true)
								return
							}

							if _, err := os.Stat(dst); err != nil {
								log.Errorf("stat %s: %v", dst, err)
								removeEntry(dst, fe, true)
							} else {
								log.Infof("copy %s %s", u, dst)
							}
						})
					}
				}
			}()
			if scannerIsBrowser {
				clipboardText := a.Clipboard().Content()
				if clipboardText != clipboardBeforeScan && strings.HasPrefix(clipboardText, IO) {
					// Браузерный сканер — шлём буфер обмена в канал
					log.Debugf("scannerIsBrowser: sending clipboard to uriFromIntent: %q", clipboardText)
					uriFromIntent <- clipboardText
				} else {
					processIntent()
				}
				scannerIsBrowser = false
			} else {
				processIntent()
			}
			mH = wHandle(w)
			// log.Debug("mainH " + mH)
			// fyne.Do(func() {
			at.OnSelected(at.Selected())
			// at.Refresh()
			// at.Selected().Content.Refresh()
			de.Bounce(ti.Content.Refresh)
			// })
		})
	} else {
		a.Lifecycle().SetOnExitedForeground(func() {
			log.Debug("ExitedForeground " + wHandle(w))
		})
		a.Lifecycle().SetOnStopped(func() {
			log.Debug("Stopped " + wHandle(w))
		})
		a.Lifecycle().SetOnStarted(func() {
			log.Debug("Started " + wHandle(w))
		})
		a.Lifecycle().SetOnEnteredForeground(func() {
			log.Debug("EnteredForeground " + wHandle(w))
			// fyne.Do(func() {
			at.OnSelected(at.Selected())
			// at.Refresh()
			de.Bounce(ti.Content.Refresh)
			// })
		})
		if !GUI {
			if stat, err := os.Stdin.Stat(); err == nil && ((stat.Mode() & os.ModeCharDevice) == 0) {
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
					case <-appCtx.Done():
					default:
						wga.Wait()
						log.Debugf("done %v", fnames)
						removeEntrys(false)
						reload()
						showPage()
					}
				}()
			}
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
				case <-appCtx.Done():
				default:
					wga.Wait()
					log.Debugf("done %v", os.Args[1:])
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
				log.Debug("Sending")
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
				case <-appCtx.Done():
				default:
					wga.Wait()
					log.Debugf("done %v", uris)
					removeEntrys(false)
					reload()
					showPage() //SetOnDropped
				}
			}()
		})
	} //isAndroid

	addClipButton := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		text := a.Clipboard().Content()
		if text == "" {
			log.Debug("empty clipboard")
			return
		}
		log.Debugf("clip\n%s", text)
		src := join(hashToFilename(text))
		if fe := addEntry(src, func(d *widget.Button, p *widget.ProgressBar, l *widget.Label) {
			setSizes(p, int64(len(text)))
		}); fe == nil {
			return
		}
		source, err := os.Create(src)
		if err != nil {
			log.Errorf("create: %v", err)
			return
		}

		_, err = source.WriteString(text)
		if err != nil {
			source.Close()
			os.Remove(src)
			log.Errorf("write: %v", err)
			return
		}

		source.Close()
	})
	cosED = append(cosED, addClipButton)
	cosDAV = append(cosDAV, addClipButton)

	addFileButton := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if supported, err := IsFilePickerSupported(); err != nil {
			log.Errorf("file picker support: %v", err)
		} else if !supported {
			log.Debugf("File picker not supported. %s", INSTALL)
			a.Clipboard().SetContent(filePicker)
			dialog.ShowInformation(
				lp("Pick a file to send"),
				INSTALL,
				w,
			)
			return
		} else {
			log.Debug("File picker is supported")
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
			log.Debugf("symlink %s %s: %v", u.Path(), dst, err)
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
					log.Debugf("copy %s %s", src, dst)
				}
			})
		}, w)
	})
	cosED = append(cosED, addFileButton)
	cosDAV = append(cosDAV, addFileButton)

	addFolderButton := widget.NewButtonWithIcon("", theme.FolderNewIcon(), func() {
		folderOpen := func(u fyne.ListableURI, e error) {
			if u == nil {
				log.Debug("folder selection canceled")
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
			log.Debugf("symlink %s %s: %v", u.Path(), dst, err)
			if err == nil {
				// Десктоп
				removeEntrys(false)
				reload()
				return
			}

			go func() {
				var wg sync.WaitGroup
				log.Debugf("copyFiles: %v",
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
								log.Debugf("copy %s %s", src, dstPath)
							}
							// Скроем временный прогрессбар без удаления файла
							// removeEntry(dstPath, feCopy, false)
						})
						return nil
					}))
				select {
				case <-appCtx.Done():
				default:
					wg.Wait()
					log.Debugf("copyFiles done")
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
	cosDAV = append(cosDAV, addFolderButton)

	reDir = widget.NewButtonWithIcon("", theme.UploadIcon(), func() {
		if treeButton.Icon == theme.VisibilityIcon() {
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
		if davServer.IsActive() {
			treeOff()
			treeButton.OnTapped()
		}
	})
	cosED = append(cosED, reDir)
	cosDAV = append(cosDAV, reDir)

	top := container.NewVBox(
		container.NewHBox(topline,
			layout.NewSpacer(),
			addClipButton,
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
			deleteAllButton,
			//

			treeButton,
			reDir,
		),
		davControl,
		mainButton,
		cancelButton,
		prog,
	)

	ti = container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
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
			de.Bounce(ti.Content.Refresh)
		})
	}

	return
}

func ShowFileOpen(callback func(fyne.URIReadCloser, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFileOpen(callback, parent)
		return
	}

	var fd *dialog.FileDialog

	if parent.FullScreen() {
		// Если уже был fullscreen - простой диалог
		fd = dialog.NewFileOpen(callback, parent)
	} else {
		// Если не было fullscreen - включаем и создаём диалог с восстановлением
		//parent.SetFullScreen(true)
		current := parent.Canvas().Size()
		parent.Resize(current.AddWidthHeight(current.Width, 0))

		fd = dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
			//parent.SetFullScreen(false)
			parent.Resize(current)
			callback(uri, err)
		}, parent)
	}

	// Настраиваем размер и показываем
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}

// For mobile os.Exit.
// For desktop Restart.
func restart(w fyne.Window) {
	if noRestart {
		return
	}
	log.Debugf("A restart is better than leaving goroutines leaking")
	start()
	cleanup(w)
	os.Exit(0)
}

func Conns(client any) ([]*comm.Comm, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Conns panic recovered: %v", r)
		}
	}()

	v := reflect.ValueOf(client)
	if v.Kind() != reflect.Ptr {
		return nil, errors.New("not a pointer")
	}

	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, errors.New("client is not a struct")
	}

	// Прямой доступ к неэкспортированному полю "conn" через unsafe
	connField := elem.FieldByName("conn")
	if !connField.IsValid() {
		return nil, errors.New("field 'conn' not found")
	}

	// Проверяем тип через отражение
	expectedType := reflect.TypeOf([]*comm.Comm{})
	if connField.Type() != expectedType {
		return nil, errors.New("field 'conn' has wrong type")
	}

	// Безопасный доступ через unsafe
	addr := connField.UnsafeAddr()
	ptr := (*[]*comm.Comm)(unsafe.Pointer(addr))
	if ptr == nil {
		return nil, errors.New("connection slice is nil")
	}

	conns := *ptr
	log.Debugf("Conns: got connections via unsafe (field 'conn'), count=%d", len(conns))
	for i, conn := range conns {
		if conn != nil && conn.Connection() != nil {
			log.Debugf("  conn[%d]: local=%v, remote=%v", i,
				conn.Connection().LocalAddr(),
				conn.Connection().RemoteAddr())
		}
	}

	return conns, nil
}

// func Stop(c any) {
func Stop(c *croc.Client) {
	conns, err := Conns(c)
	if err != nil {
		log.Errorf("Stop: %v", err)
		return
	}
	if len(conns) < 1 || conns[0] == nil {
		log.Errorf("Stop: not open")
		return
	}
	err = message.Send(conns[0], c.Key, message.Message{
		Type:    message.TypeError,
		Message: REFUSING,
	})
	log.Debug("Stop %s: %v", REFUSING, err)

	time.Sleep(time.Millisecond * 33)
	conns[0].Close()
	log.Debug("Stop close %s", conns[0].Connection().RemoteAddr())
	time.Sleep(time.Millisecond * 333)
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

const H2F = 16

func hashToFilename(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:H2F/2]) + DOTTXT
}

func validHash(path string, content ...byte) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if strings.ToLower(ext) != DOTTXT {
		return false
	}

	name := strings.TrimSuffix(base, ext)
	if len(name) != H2F {
		return false
	}

	var decoded [H2F / 2]byte
	if _, err := hex.Decode(decoded[:], []byte(name)); err != nil {
		return false
	}

	if len(content) == 0 {
		// Проверим только имя
		return true
	}

	hash := sha256.Sum256(content)
	return bytes.Equal(decoded[:], hash[:H2F/2])
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
		case *Select:
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
		de.Bounce(co.Refresh)
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
						case <-appCtx.Done():
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

func linkByTarget(target string, join func(elem ...string) string) (link string, err error) {
	l := ls(join())
	log.Debugf("ls %v", l)
	for _, name := range l {
		link = join(name)
		log.Debugf("link %s", link)
		if parent, e := Readlink(link); e == nil {
			log.Debugf("parent %s target %s", parent, target)
			if isCached(parent, target) {
				log.Debugf("return %s ", link)
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
		homedir = filepath.Join(xdgConfigHome, CROC)
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
			homedir = filepath.Join(homedir, ".config", CROC)
		}
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

func wHandle(w fyne.Window) string {
	s := fmt.Sprintf("%+v", w)
	if i := strings.Index(s, "handle:"); i != -1 {
		return strings.TrimSuffix(s[i+7:], "}")
	}
	return ""
}

// isWSL определяет, выполняется ли код в окружении WSL
func isWSL() bool {
	// WSL_DISTRO_NAME устанавливается в WSL и содержит имя дистрибутива
	return os.Getenv("WSL_DISTRO_NAME") != ""
}

func drainChannel(ch chan string) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func defWeb(sch string, s bool, h, p, path string) (host string, u url.URL) {
	port := p
	if p == "" {
		port = "8080"
	}
	u.Scheme = sch
	if s {
		u.Scheme += "s"
		if p == "" {
			port = "8443"
		}
	}
	if port == "80" {
		u.Host = h
	} else {
		u.Host = net.JoinHostPort(h, port)
	}
	host = u.Host
	u.Path = path
	return
}
