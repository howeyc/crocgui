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

	var updateMutex sync.Mutex
	update := func() {
		updateMutex.Lock()
		defer updateMutex.Unlock()

		if !totpCheck.Checked || totpCheck.Disabled() {
			return
		}

		totpLabel.SetText(totp(entry.Text))
		now := time.Now()
		remaining := 30 - now.Second()%30
		totpProg.SetValue(float64(remaining) / 30)
	}

	totpCheck.OnChanged = func(b bool) {
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
						fyne.Do(update)
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
	}
	if totpCheck.Checked {
		totpCheck.OnChanged(true)
	}

	entry.OnChanged = func(secret string) {
		os.Setenv(CROC_SECRET, secret)
		update()
	}

	recvDir := filepath.Join(os.TempDir(), "crocgui-recv")

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	removeEntry := func(fpath string, fe *fyne.Container, del bool) {
		boxholder.Remove(fe)
		if del {
			log.Tracef("Removed received file: %s error: %v", fpath, os.Remove(fpath))
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
			if !(isMobile || copyDebug) {
				err := os.Rename(src, destination.URI().Path())
				if err == nil {
					destination.Close()
					log.Tracef("File %s moved to %s", src, destination.URI().Path())
					removeEntry(src, fe, true)
					return
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

	addEntry := func(dst string) (newentry *fyne.Container) {
		if fe, has := fileentries[dst]; has {
			log.Tracef("exists %s", dst)
			return fe
		}
		labelFile := widget.NewLabel(filepath.Base(dst))

		saveButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
			ShowFileLocation(dst, w)
		})

		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if fe, ok := fileentries[dst]; ok {
				removeEntry(dst, fe, true)
			}
		})
		progFile := widget.NewProgressBar()
		progFile.Hide()

		newentry = container.NewHBox(
			deleteButton,
			progFile,
			saveButton,
			labelFile,
		)

		fileentries[dst] = newentry
		boxholder.Add(newentry)
		return
	}

	os.MkdirAll(recvDir, 0o700)
	fpath := a.Preferences().String("DeleteFile")
	for _, name := range ls(recvDir) {
		if name != "" {
			path := filepath.Join(recvDir, name)
			if fpath == path {
				err := os.Remove(fpath)
				log.Tracef("Removed partially received file: %s error: %v", fpath, err)
				if err != nil {
					continue
				}
				a.Preferences().SetString("DeleteFile", "")
			} else {
				addEntry(path)
			}
		}
	}

	var lastSaveDir string

	cancelChan := make(chan struct{})

	var cancelButton, mainButton *widget.Button

	removeEntrys := func(del bool) {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe, del)
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

				destination, err := storage.Writer(u)
				if err != nil {
					cl()
					log.Errorf("Error creating writer from URI(%s) error: %v", u.String(), err)
					continue
				}

				if !(isMobile || copyDebug) {
					err := os.Rename(src, destination.URI().Path())
					if err == nil {
						destination.Close()
						log.Tracef("File %s moved to %s", src, destination.URI().Path())
						removeEntry(src, fe, true)
						fyne.Do(func() {
							if len(fileentries) == 0 {
								topline.SetText(fmt.Sprintf("%s %s", lp("Saved all files to"), lastSaveDir))
							}
						})
						continue
					}
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
						removeEntrys(true)
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

					for _, name := range ls(recvDir) {
						if name != "" {
							path := filepath.Join(recvDir, name)
							if fpath == path {
								continue
							}

							fe := addEntry(path)
							if pb, ok := fe.Objects[feBar].(*widget.ProgressBar); ok {
								fe.Objects[feDel].Show()
								fe.Objects[feSave].Show()
								pb.Hide()
							}
						}
					}
					refresh()
				})
			}()

			old := 0
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
							for _, fi := range receiver.FilesToTransfer {
								dst := filepath.Join(recvDir, fi.Name)
								fe := addEntry(dst)
								if pb, ok := fe.Objects[feBar].(*widget.ProgressBar); ok {
									fyne.Do(func() {
										fe.Objects[feDel].Hide()
										fe.Objects[feSave].Hide()
										pb.SetValue(0)
										pb.Max = float64(fi.Size)
										pb.Show()
									})
								}
								totalMax += fi.Size
							}
							fyne.Do(refresh)
							progW.SetMax(totalMax)
						}
						cnum := receiver.FilesToTransferCurrentNum
						if old < cnum+1 {
							old = cnum + 1
							if cnum > 0 {
								//100%
								fepw.Set100()
							}
							fi := receiver.FilesToTransfer[cnum]
							filename = fi.Name
							toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Receiving file"), filename, cnum+1, len(receiver.FilesToTransfer)))
							TotalSent += size
							size = fi.Size
							path := filepath.Join(recvDir, fi.Name)
							log.Trace(path)
							if fe, ok := fileentries[path]; ok {
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
					removeEntrys(false)
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
			removeEntrys(true)
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

// Big File Dialog
func ShowFolderOpen(callback func(fyne.ListableURI, error), parent fyne.Window) {
	if isMobile {
		notFinish = true
		dialog.ShowFolderOpen(callback, parent)
		return
	}
	fd := dialog.NewFolderOpen(callback, parent)
	fd.Show()
}

// detectMimeType определяет MIME-тип по расширению файла
func detectMimeType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	mimeTypes := map[string]string{
		".txt":  "text/plain",
		".html": "text/html",
		".htm":  "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",

		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".zip":  "application/zip",
		".rar":  "application/vnd.rar",
		".7z":   "application/x-7z-compressed",

		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",

		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".flac": "audio/flac",
		".aac":  "audio/aac",

		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".mkv":  "video/x-matroska",

		".apk": "application/vnd.android.package-archive",
	}

	if mime, exists := mimeTypes[ext]; exists {
		return mime
	}

	return "application/octet-stream"
}
