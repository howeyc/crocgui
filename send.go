package main

import (
	"fmt"
	"log"
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
	"github.com/schollz/croc/v8/src/croc"
	"github.com/schollz/croc/v8/src/utils"
)

func sendTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel("Pick a file to send")
	var currentCode string
	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		if currentCode != "" {
			w.Clipboard().SetContent(currentCode)
		}
	})
	copyCodeButton.Hide()

	boxholder := container.NewVBox()
	fileentries := make(map[string]*fyne.Container)

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, e error) {
			if e != nil {
				return
			}
			if f != nil {
				fpath := fixpath(f.URI().Path())
				_, sterr := os.Stat(fpath)
				if sterr != nil {
					status.SetText(fmt.Sprintf("Stat error: %s - %s", fpath, sterr.Error()))
					return
				}
				labelFile := widget.NewLabel(filepath.Base(fpath))
				newentry := container.NewHBox(labelFile, layout.NewSpacer(), widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
					if fe, ok := fileentries[fpath]; ok {
						boxholder.Remove(fe)
						delete(fileentries, fpath)
					}
				}))
				fileentries[fpath] = newentry
				boxholder.Add(newentry)
			}
		}, w)
	})

	return container.NewTabItemWithIcon("Send", theme.MailSendIcon(),
		container.NewVBox(
			container.NewHBox(topline, layout.NewSpacer(), addFileButton),
			boxholder,
			widget.NewButtonWithIcon("Send", theme.MailSendIcon(), func() {
				addFileButton.Hide()
				randomName := utils.GetRandomName()
				sender, err := croc.New(croc.Options{
					IsSender:       true,
					SharedSecret:   randomName,
					Debug:          false,
					RelayAddress:   a.Preferences().String("relay-address"),
					RelayPorts:     strings.Split(a.Preferences().String("relay-ports"), ","),
					RelayPassword:  a.Preferences().String("relay-password"),
					Stdout:         false,
					NoPrompt:       true,
					DisableLocal:   a.Preferences().Bool("disable-local"),
					NoMultiplexing: a.Preferences().Bool("disable-multiplexing"),
					OnlyLocal:      a.Preferences().Bool("force-local"),
					NoCompress:     a.Preferences().Bool("disable-compression"),
				})
				if err != nil {
					status.SetText("croc error: " + err.Error())
					return
				}
				var filename string
				status.SetText("Receive Code: " + randomName)
				currentCode = randomName
				copyCodeButton.Show()
				prog.Show()
				donechan := make(chan bool)
				sendnames := make(map[string]int)
				go func() {
					ticker := time.NewTicker(time.Millisecond * 100)
					for {
						select {
						case <-ticker.C:
							if sender.Step2FileInfoTransfered {
								cnum := sender.FilesToTransferCurrentNum
								fi := sender.FilesToTransfer[cnum]
								filename = filepath.Base(fi.Name)
								sendnames[filename] = cnum
								topline.SetText(fmt.Sprintf("Sending file: %s (%d/%d)", filename, cnum+1, len(sender.FilesToTransfer)))
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
					serr := sender.Send(croc.TransferOptions{
						PathToFiles: filepaths,
					})
					donechan <- true
					prog.Hide()
					prog.SetValue(0)
					for _, fpath := range filepaths {
						if fe, ok := fileentries[fpath]; ok {
							boxholder.Remove(fe)
							delete(fileentries, fpath)
						}
					}
					topline.SetText("Pick a file to send")
					addFileButton.Show()
					if serr != nil {
						log.Println("Send failed:", serr)
					} else {
						status.SetText(fmt.Sprintf("Sent file %s", filename))
					}
					currentCode = ""
					copyCodeButton.Hide()
				}()
			}),
			prog,
			container.NewHBox(status, copyCodeButton),
		))
}
