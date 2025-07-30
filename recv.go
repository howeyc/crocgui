package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel("")
	entry := widget.NewEntry()

	if secret := os.Getenv("CROC_SECRET"); secret != "" {
		entry.SetText(secret)
	}
	entry.SetPlaceHolder(lp("Enter code to download"))
	pasteCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		entry.SetText(a.Clipboard().Content())
	})

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-recv", a.Preferences()))
	totpLabel := widget.NewLabel("TOTP")
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

		if !totpCheck.Checked {
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
			totpProg.Show()
			update()

			totpChan = make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						fyne.Do(func() {
							update()
						})
					case <-totpChan:
						return
					}
				}
			}()
		} else {
			totpLabel.SetText("TOTP")
			totpProg.Hide()
		}
		a.Preferences().SetBool("totp-recv", b)
	}
	if totpCheck.Checked {
		totpCheck.OnChanged(true)
	}

	entry.OnChanged = func(secret string) {
		os.Setenv("CROC_SECRET", secret)
		update()
	}

	recvDir := filepath.Join(os.TempDir(), "crocgui-recv")

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	removeEntry := func(fpath string, fe *fyne.Container) {
		boxholder.Remove(fe)
		os.Remove(fpath)
		log.Tracef("Removed received file: %s", fpath)
		delete(fileentries, fpath)
	}

	ShowFileLocation := func(src string, parent fyne.Window) {
		savedialog := dialog.NewFileSave(func(destination fyne.URIWriteCloser, e error) {
			if err := copyToUWC(destination, src); err != nil {
				log.Error("%s\n", err)
			} else {
				if _, ok := fileentries[src]; ok {
					removeEntry(src, fileentries[src])
				} else {
					os.Remove(src)
				}
			}
		}, parent)
		savedialog.SetFileName(filepath.Base(src))
		savedialog.Resize(parent.Canvas().Size())
		savedialog.Show()
	}

	addEntry := func(dst string) {
		labelFile := widget.NewLabel(filepath.Base(dst))

		openButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
			ShowFileLocation(dst, w)
		})

		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if fe, ok := fileentries[dst]; ok {
				removeEntry(dst, fe)
			}
		})

		newentry := container.NewHBox(
			labelFile,
			layout.NewSpacer(),
			openButton,
			deleteButton,
		)

		fileentries[dst] = newentry
		boxholder.Add(newentry)
	}

	os.MkdirAll(recvDir, 0o700)
	for _, name := range ls(recvDir) {
		if name != "" {
			addEntry(filepath.Join(recvDir, name))
		}
	}

	var lastSaveDir string

	cancelchan := make(chan bool)
	var cancelButton, mainButton *widget.Button

	removeEntrys := func() {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe)
		}
	}

	saveAllFiles := func() {
		if len(fileentries) == 0 {
			log.Error("no files to save")
			return
		}

		ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				log.Errorf("Error selecting folder: %v", err)
				return
			}
			if uri == nil {
				log.Error("User canceled folder selection")
				return
			}

			lastSaveDir = uri.Path()

			go func() {
				for src, fe := range fileentries {
					filename := filepath.Base(src)
					dstURI, _ := storage.Child(uri, filename)

					w, err := storage.Writer(dstURI)
					if err != nil {
						log.Errorf("Error creating writer for %s: %v", filename, err)
						continue
					}

					if err := copyToUWC(w, src); err != nil {
						log.Errorf("Error saving %s: %v", filename, err)
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("Error saving %s: %v", filename, err))
						})
					} else {
						log.Tracef("File %s saved to %s", filename, dstURI.String())
						removeEntry(src, fe)
					}

				}
				fyne.Do(func() {
					if len(fileentries) == 0 {
						topline.SetText(fmt.Sprintf("%s: %s", lp("Saved all files to"), lastSaveDir))
					}
				})
			}()
		}, w)
	}

	reset := func() {
		mainButton.Enable()
		prog.Hide()
		prog.SetValue(0)
		cancelButton.Hide()

		totpCheck.Enable()
		if totpCheck.Checked {
			totpProg.Show()
		}

		entry.Enable()
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
			dialog.ShowConfirm(lp("Delete All"), lp("Are you sure you want to delete all received files?"), func(b bool) {
				if b {
					removeEntrys()
				}
			}, w)
			return
		}

		totpCheck.Disable()
		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			totpProg.Hide()
		}

		receiver, err := croc.New(croc.Options{
			IsSender:         false,
			SharedSecret:     secret,
			Debug:            crocDebugMode(),
			RelayAddress:     a.Preferences().String("relay-address"),
			RelayPassword:    a.Preferences().String("relay-password"),
			Stdout:           false,
			NoPrompt:         true,
			DisableLocal:     a.Preferences().Bool("disable-local"),
			NoMultiplexing:   a.Preferences().Bool("disable-multiplexing"),
			OnlyLocal:        a.Preferences().Bool("force-local"),
			NoCompress:       a.Preferences().Bool("disable-compression"),
			Curve:            a.Preferences().String("pake-curve"),
			HashAlgorithm:    a.Preferences().String("croc-hash"),
			Overwrite:        true,
			MulticastAddress: a.Preferences().String("multicast-address"),
		})
		if err != nil {
			log.Errorf("croc setup error: %s\n", err.Error())
			return
		}
		log.SetLevel(crocDebugLevel())
		log.Trace("croc receiver created")

		cderr := os.Chdir(recvDir)
		if cderr != nil {
			log.Error("Unable to change to dir:", recvDir, cderr)
		}
		log.Trace("cd", recvDir)

		var filename string
		mainButton.Disable()
		prog.Show()
		cancelButton.Show()

		donechan := make(chan bool)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()
			old := 0
			for {
				select {
				case <-ticker.C:
					if receiver == nil {
						return
					}
					if receiver.Step2FileInfoTransferred {
						cnum := receiver.FilesToTransferCurrentNum
						fyne.Do(func() {
							if old < cnum+1 {
								old = cnum + 1
								fi := receiver.FilesToTransfer[cnum]
								filename = filepath.Base(fi.Name)
								topline.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Receiving file"), filename, cnum+1, len(receiver.FilesToTransfer)))
								prog.Max = float64(fi.Size)
							}
							prog.SetValue(float64(receiver.TotalSent))
						})
					}
				case <-donechan:
					return
				case <-cancelchan:
					return
				}
			}
		}()

		go func() {
			fyne.Do(entry.Disable)
			var rerr error
			if EMULATE == 0 {
				rerr = receiver.Receive()
			} else {
				log.Warnf("Receive\n")
				time.Sleep(EMULATE)
				defer func() {
					time.Sleep(time.Millisecond * 10)
					receiver = nil
				}()
			}
			donechan <- true
			if rerr != nil {
				log.Errorf("Receive failed: %s\n", rerr)
				fyne.Do(func() {
					topline.SetText(rerr.Error())
				})
			} else {
				fyne.Do(func() {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Received"), filename))

					for _, fi := range receiver.FilesToTransfer {
						fpath := filepath.Join(recvDir, filepath.Base(fi.Name))
						addEntry(fpath)
					}
				})
			}
			fyne.Do(reset)
		}()

		go func() {
			select {
			case <-donechan:
				return
			case <-cancelchan:
				log.Warnf("Receive cancelled. %s: %v\n", recvDir, ls(recvDir))
				Stop(receiver)

				fyne.Do(func() {
					reset()
				})
			}
		}()
		//  +2 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
	})
	cancelButton.Hide()

	deleteAllButton := widget.NewButtonWithIcon(lp("Delete All"), theme.DeleteIcon(), func() {
		if len(fileentries) > 0 {
			removeEntrys()
		} else {
			entry.SetText("")
		}
	})

	saveAllButton := widget.NewButtonWithIcon(lp("Save All"), theme.FolderOpenIcon(), func() {
		saveAllFiles()
	})
	if android {
		saveAllButton.Hide()
	}

	top := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer(), pasteCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Receive Code"), Widget: entry}),
		container.NewHBox(
			totpLabel,
			totpCheck,
			totpProg,
			layout.NewSpacer(),
			saveAllButton,
			deleteAllButton,
		),
		mainButton,
		prog,
		cancelButton,
	)

	return container.NewTabItemWithIcon(lp("Receive"), theme.DownloadIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
}

func copyToUWC(destination fyne.URIWriteCloser, src string) error {
	if destination == nil {
		return fmt.Errorf("destination is nil (dialog closed)")
	}
	defer destination.Close()
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer source.Close()

	if _, err = io.Copy(destination, source); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	return nil
}

// Big File Dialog
func ShowFolderOpen(callback func(fyne.ListableURI, error), parent fyne.Window) {
	if mobile {
		dialog.ShowFolderOpen(callback, parent)
		return
	}
	fd := dialog.NewFolderOpen(callback, parent)
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}
