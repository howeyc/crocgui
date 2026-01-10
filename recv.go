package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/croc"
	log "github.com/schollz/logger"
)

func recvTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()

	topline := widget.NewLabel(lp(""))

	// ============================ receive entry ============================
	recvEntry := widget.NewEntry()
	recvEntry.OnChanged = func(s string) {
		recvEntry.Text = strings.ReplaceAll(s, " ", "-")
	}
	recvEntry.SetPlaceHolder(lp("Enter code to download"))

	recvDir, _ := os.MkdirTemp("", "crocgui-recv")

	// ========================= clear receive entry =========================
	clearRecvEntry := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		recvEntry.Text = ""
		recvEntry.Refresh()
	})

	// ========================= paste receive entry =========================
	pasteRecvEntry := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		clipboard := a.Clipboard()
		recvEntry.Text = clipboard.Content()
		recvEntry.Refresh()
	})

	// ============================ receive entry container ============================
	additionalRecvEntry := container.NewHBox(clearRecvEntry, pasteRecvEntry)
	recvEntryContainer := container.NewBorder(
		nil, nil, nil, additionalRecvEntry,
		recvEntry, additionalRecvEntry,
	)

	boxholder := container.NewVBox()
	receiverScroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	debugBox := container.NewHBox(widget.NewLabel(lp("Debug log:")), layout.NewSpacer(), widget.NewButton("Export full log", func() {
		savedialog := dialog.NewFileSave(func(f fyne.URIWriteCloser, e error) {
			if f != nil {
				logoutput.buf.WriteTo(f)
				f.Close()
			}
		}, w)
		savedialog.SetFileName("crocdebuglog.txt")
		savedialog.Resize(w.Canvas().Size())
		savedialog.Show()
	}))
	debugObjects = append(debugObjects, debugBox)

	cancelchan := make(chan bool)
	activeButtonHolder := container.NewVBox()
	var cancelButton, receiveButton *widget.Button

	deleteAllFiles := func() {
		for fpath, fe := range fileentries {
			boxholder.Remove(fe)
			os.Remove(fpath)
			log.Tracef("Removed received file: %s", fpath)
			delete(fileentries, fpath)
		}
	}

	resetReceiver := func() {
		prog.Hide()
		prog.SetValue(0)
		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(receiveButton)

		topline.SetText(lp("Enter code to download"))
		recvEntry.Enable()
	}

	receiveButton = widget.NewButtonWithIcon(lp("Download"), theme.DownloadIcon(), func() {
		if len(fileentries) > 0 {
			log.Error("deleting previous files")
			fyne.Do(deleteAllFiles)
		}

		receiver, err := croc.New(croc.Options{
			IsSender:         false,
			SharedSecret:     recvEntry.Text,
			Debug:            crocDebugMode(),
			RelayAddress:     a.Preferences().String("relay-address"),
			RelayPorts:       strings.Split(a.Preferences().String("relay-ports"), ","),
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
			ZipFolder:        false,
			GitIgnore:        false,
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
		status.SetText(fmt.Sprintf("%s: %s", lp("Receive Code"), recvEntry.Text))
		prog.Show()

		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(cancelButton)

		donechan := make(chan bool)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			for {
				select {
				case <-ticker.C:
					if receiver.Step2FileInfoTransferred {
						cnum := receiver.FilesToTransferCurrentNum
						fi := receiver.FilesToTransfer[cnum]
						filename = filepath.Base(fi.Name)
						fyne.Do(func() {
							topline.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Receiving file"), filename, cnum+1, len(receiver.FilesToTransfer)))
							prog.Max = float64(fi.Size)
							prog.SetValue(float64(receiver.TotalSent))
						})
					}
				case <-donechan:
					ticker.Stop()
					return
				}
			}
		}()

		go func() {
			fyne.Do(recvEntry.Disable)
			ferr := receiver.Receive()
			donechan <- true
			if ferr != nil {
				log.Errorf("Receive failed: %s\n", ferr)
			} else {
				fyne.Do(func() {
					status.SetText(fmt.Sprintf("%s: %s", lp("Received"), filename))

					for _, fi := range receiver.FilesToTransfer {
						fpath := filepath.Join(recvDir, filepath.Base(fi.Name))
						labelFile := widget.NewLabel(filepath.Base(fpath))

						openButton := widget.NewButtonWithIcon("", theme.DocumentSaveIcon(), func() {
							ShowFileLocation(fpath, w)
						})

						deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
							if fe, ok := fileentries[fpath]; ok {
								boxholder.Remove(fe)
								os.Remove(fpath)
								log.Tracef("Removed received file: %s", fpath)
								delete(fileentries, fpath)
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
				})
			}
			fyne.Do(resetReceiver)
		}()

		go func() {
			select {
			case <-cancelchan:
				receiver.SuccessfulTransfer = true
				donechan <- true
				fyne.Do(func() {
					status.SetText(lp("Receive cancelled."))
				})
			}
			fyne.Do(resetReceiver)
		}()
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
	})

	activeButtonHolder.Add(receiveButton)

	receiveTop := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer()),
		widget.NewForm(&widget.FormItem{Text: lp("Receive Code"), Widget: recvEntryContainer, HintText: "Spaces ( ) become dash (-)"}),
	)
	receiveBot := container.NewVBox(
		activeButtonHolder,
		prog,
		container.NewHBox(status),
		debugBox,
	)

	return container.NewTabItemWithIcon(lp("Receive"), theme.DownloadIcon(),
		container.NewBorder(receiveTop, receiveBot, nil, nil, receiverScroller))
}

func ShowFileLocation(path string, parent fyne.Window) {
	savedialog := dialog.NewFileSave(func(f fyne.URIWriteCloser, e error) {
		if f != nil {
			src, err := os.Open(path)
			if err != nil {
				log.Error(err)
				return
			}
			defer src.Close()

			_, err = io.Copy(f, src)
			if err != nil {
				log.Error(err)
			}
			f.Close()
		}
	}, parent)
	savedialog.SetFileName(filepath.Base(path))
	savedialog.Resize(parent.Canvas().Size())
	savedialog.Show()
}
