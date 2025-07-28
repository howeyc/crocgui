package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	recvEntry := widget.NewEntry()
	recvEntry.SetPlaceHolder(lp("Enter code to download"))
	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		recvEntry.SetText(a.Clipboard().Content())
	})

	// recvDir, _ := os.MkdirTemp("", "crocgui-recv")
	recvDir := filepath.Join(os.TempDir(), "crocgui-recv")

	boxholder := container.NewVBox()
	receiverScroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	deleteFile := func(fpath string, fe *fyne.Container) {
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
					deleteFile(src, fileentries[src])
				} else {
					os.Remove(src)
				}
			}
		}, parent)
		savedialog.SetFileName(filepath.Base(src))
		savedialog.Resize(parent.Canvas().Size())
		savedialog.Show()
	}

	addEntry := func(fpath string) {
		labelFile := widget.NewLabel(filepath.Base(fpath))

		openButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
			ShowFileLocation(fpath, w)
		})

		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if fe, ok := fileentries[fpath]; ok {
				deleteFile(fpath, fe)
			}
		})

		newentry := container.NewHBox(
			labelFile,
			layout.NewSpacer(),
			openButton,
			deleteButton,
		)
		fileentries[fpath] = newentry
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
	activeButtonHolder := container.NewVBox()
	var cancelButton, receiveButton *widget.Button

	deleteAllFiles := func() {
		for fpath, fe := range fileentries {
			deleteFile(fpath, fe)
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
			prog.Show()
			prog.Max = float64(len(fileentries))
			prog.SetValue(0)

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
						deleteFile(src, fe)
					}

					fyne.Do(func() {
						prog.SetValue(prog.Value + 1)
					})
				}
				fyne.Do(func() {
					prog.Hide()
					if len(fileentries) == 0 {
						topline.SetText(fmt.Sprintf("%s: %s", lp("Saved all files to"), lastSaveDir))
					}
				})
			}()
		}, w)
	}

	resetReceiver := func() {
		prog.Hide()
		prog.SetValue(0)
		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(receiveButton)

		recvEntry.Enable()
	}

	receiveButton = widget.NewButtonWithIcon(lp("Download"), theme.DownloadIcon(), func() {
		if len(recvEntry.Text) < 6 {
			log.Error("no receive code entered")
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
					deleteAllFiles()
				} else {
					return
				}
			}, w)
		}

		receiver, err := croc.New(croc.Options{
			IsSender:         false,
			SharedSecret:     recvEntry.Text,
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
			log.Errorf("Receive setup error: %s\n", err.Error())
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
		prog.Show()

		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(cancelButton)

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
			fyne.Do(recvEntry.Disable)
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
			fyne.Do(resetReceiver)
		}()

		go func() {
			select {
			case <-donechan:
				return
			case <-cancelchan:
				log.Warnf("Receive cancelled. %s: %v\n", recvDir, ls(recvDir))
				Stop(receiver)

				fyne.Do(func() {
					resetReceiver()
				})
			}
		}()
		//  +2 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
	})

	activeButtonHolder.Add(receiveButton)

	deleteAllButton := widget.NewButtonWithIcon(lp("Delete All"), theme.DeleteIcon(), func() {
		deleteAllFiles()
	})

	saveAllButton := widget.NewButtonWithIcon(lp("Save All"), theme.FolderOpenIcon(), func() {
		saveAllFiles()
	})
	if android {
		saveAllButton.Hide()
	}

	receiveTop := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer(), copyCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Receive Code"), Widget: recvEntry}),
	)
	receiveBot := container.NewVBox(
		activeButtonHolder,
		prog,
		container.NewHBox(
			layout.NewSpacer(),
			saveAllButton,
			deleteAllButton,
		),
	)

	return container.NewTabItemWithIcon(lp("Receive"), theme.DownloadIcon(),
		container.NewBorder(receiveTop, receiveBot, nil, nil, receiverScroller))
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
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
