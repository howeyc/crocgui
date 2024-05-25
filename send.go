package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/schollz/logger"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
)

func sendTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	logInfo := widget.NewLabelWithData(logbinding)
	logInfo.Wrapping = fyne.TextWrapWord

	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel(lp("Pick a file to send"))
	randomCode := utils.GetRandomName()
	sendEntry := widget.NewEntry()
	sendEntry.SetText(randomCode)
	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(sendEntry.Text)
	})
	copyCodeButton.Hide()

	sendDir, _ := os.MkdirTemp("", "crocgui-send")

	boxholder := container.NewVBox()
	senderScroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	sendentryChanged := func() {
		if entrylen := len(fileentries); entrylen < 6 {
			senderScroller.SetMinSize(fyne.NewSize(100, 42*float32(entrylen)))
		}
	}

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, e error) {
			if e != nil {
				log.Errorf("Open dialog error: %s", e.Error())
				return
			}
			if f != nil {
				nfile, oerr := os.Create(filepath.Join(sendDir, f.URI().Name()))
				if oerr != nil {
					log.Errorf("Unable to copy file, error: %s - %s\n", sendDir, oerr.Error())
					return
				}
				io.Copy(nfile, f)
				nfile.Close()
				fpath := nfile.Name()
				log.Tracef("Android URI (%s), copied to internal cache %s", f.URI().String(), nfile.Name())

				_, sterr := os.Stat(fpath)
				if sterr != nil {
					log.Errorf("Stat error: %s - %s\n", fpath, sterr.Error())
					return
				}
				labelFile := widget.NewLabel(filepath.Base(fpath))
				newentry := container.NewHBox(labelFile, layout.NewSpacer(), widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
					// Can only add/remove if not currently attempting a send
					if !sendEntry.Disabled() {
						if fe, ok := fileentries[fpath]; ok {
							boxholder.Remove(fe)
							os.Remove(fpath)
							log.Tracef("Removed file from internal cache: %s", fpath)
							delete(fileentries, fpath)
							sendentryChanged()
						}
					}
				}))
				fileentries[fpath] = newentry
				boxholder.Add(newentry)
				sendentryChanged()
			}
		}, w)
	})

	debugBox := container.NewHBox(widget.NewLabel(lp("Debug log:")), layout.NewSpacer(), widget.NewButton("Export full log", func() {
		savedialog := dialog.NewFileSave(func(f fyne.URIWriteCloser, e error) {
			if f != nil {
				logoutput.buf.WriteTo(f)
				f.Close()
			}
		}, w)
		savedialog.SetFileName("crocdebuglog.txt")
		savedialog.Show()
	}))
	debugObjects = append(debugObjects, debugBox)

	cancelchan := make(chan bool)
	activeButtonHolder := container.NewVBox()
	var cancelButton, sendButton *widget.Button

	resetSender := func() {
		prog.Hide()
		prog.SetValue(0)
		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(sendButton)

		for fpath, fe := range fileentries {
			boxholder.Remove(fe)
			os.Remove(fpath)
			log.Tracef("Removed file from internal cache: %s", fpath)
			delete(fileentries, fpath)
		}

		topline.SetText(lp("Pick a file to send"))
		addFileButton.Show()
		if sendEntry.Text == randomCode {
			randomCode = utils.GetRandomName()
			sendEntry.SetText(randomCode)
		}
		copyCodeButton.Hide()
		sendEntry.Enable()
	}

	sendButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		// Only send if files selected
		if len(fileentries) < 1 {
			log.Error("no files selected")
			return
		}

		addFileButton.Hide()
		sender, err := croc.New(croc.Options{
			IsSender:       true,
			SharedSecret:   sendEntry.Text,
			Debug:          crocDebugMode(),
			RelayAddress:   a.Preferences().String("relay-address"),
			RelayPorts:     strings.Split(a.Preferences().String("relay-ports"), ","),
			RelayPassword:  a.Preferences().String("relay-password"),
			Stdout:         false,
			NoPrompt:       true,
			DisableLocal:   a.Preferences().Bool("disable-local"),
			NoMultiplexing: a.Preferences().Bool("disable-multiplexing"),
			OnlyLocal:      a.Preferences().Bool("force-local"),
			NoCompress:     a.Preferences().Bool("disable-compression"),
			Curve:          a.Preferences().String("pake-curve"),
			HashAlgorithm:  a.Preferences().String("croc-hash"),
			ThrottleUpload: a.Preferences().String("upload-throttle"),
			ZipFolder:      false,
			GitIgnore:      false,
		})
		if err != nil {
			log.Errorf("croc error: %s\n", err.Error())
			return
		}
		log.SetLevel(crocDebugLevel())
		log.Trace("croc sender created")

		var filename string
		status.SetText(fmt.Sprintf("%s: %s", lp("Receive Code"), sendEntry.Text))
		copyCodeButton.Show()
		prog.Show()

		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(cancelButton)

		donechan := make(chan bool)
		sendnames := make(map[string]int)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			for {
				select {
				case <-ticker.C:
					if sender.Step2FileInfoTransferred {
						cnum := sender.FilesToTransferCurrentNum
						fi := sender.FilesToTransfer[cnum]
						filename = filepath.Base(fi.Name)
						sendnames[filename] = cnum
						topline.SetText(fmt.Sprintf("%s: (%d/%d)", lp("Sending file"), filename, cnum+1, len(sender.FilesToTransfer)))
						prog.Max = float64(fi.Size)
						prog.SetValue(float64(sender.TotalSent))
					}
				case <-donechan:
					ticker.Stop()
					return
				}
			}
		}()
		go func() {
			var filepaths []string
			for fpath := range fileentries {
				filepaths = append(filepaths, fpath)
			}
			sendEntry.Disable()
			fi, emptyfolders, numFolders, ferr := croc.GetFilesInfo(filepaths, false, false)
			if ferr != nil {
				log.Errorf("file info failed: %s\n", ferr)
			}
			serr := sender.Send(fi, emptyfolders, numFolders)
			donechan <- true
			if serr != nil {
				log.Errorf("Send failed: %s\n", serr)
			} else {
				status.SetText(fmt.Sprintf("%s %s", lp("Sent file"), filename))
			}
			resetSender()
		}()
		go func() {
			select {
			case <-cancelchan:
				donechan <- true
				status.SetText(lp("Send cancelled."))
			}
			resetSender()
		}()
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
	})

	activeButtonHolder.Add(sendButton)

	return container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewVBox(
			container.NewHBox(topline, layout.NewSpacer(), addFileButton),
			widget.NewForm(&widget.FormItem{Text: lp("Send Code"), Widget: sendEntry}),
			senderScroller,
			activeButtonHolder,
			prog,
			container.NewHBox(status, copyCodeButton),
			debugBox,
			logInfo,
		))
}
