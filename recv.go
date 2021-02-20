package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v8/src/croc"
)

func recvTabItem(a fyne.App) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()

	prog := widget.NewProgressBar()
	prog.Hide()
	recvEntry := widget.NewEntry()
	topline := widget.NewLabel("Enter code to download")
	return container.NewTabItemWithIcon("Receive", theme.DownloadIcon(),
		container.NewVBox(
			topline,
			widget.NewForm(&widget.FormItem{Text: "Receive Code", Widget: recvEntry}),
			widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
				receiver, err := croc.New(croc.Options{
					IsSender:      false,
					SharedSecret:  recvEntry.Text,
					Debug:         false,
					RelayAddress:  a.Preferences().String("relay-address"),
					RelayPassword: a.Preferences().String("relay-password"),
					Stdout:        false,
					NoPrompt:      true,
					DisableLocal:  true,
				})
				if err != nil {
					log.Println("Receive setup error:", err)
				}
				prog.Show()
				donechan := make(chan bool)
				var filename string
				receivednames := make(map[string]int)
				go func() {
					ticker := time.NewTicker(time.Millisecond * 100)
					for {
						select {
						case <-ticker.C:
							if receiver.Step2FileInfoTransfered {
								cnum := receiver.FilesToTransferCurrentNum
								fi := receiver.FilesToTransfer[cnum]
								filename = filepath.Base(fi.Name)
								receivednames[filename] = cnum
								topline.SetText(fmt.Sprintf("Receiving file: %s (%d/%d)", filename, cnum+1, len(receiver.FilesToTransfer)))
								prog.Max = float64(fi.Size)
								prog.SetValue(float64(receiver.TotalSent))
							}
						case <-donechan:
							ticker.Stop()
							return
						}
					}
				}()
				cderr := os.Chdir(DEFAULT_DOWNLOAD_DIR)
				if cderr != nil {
					log.Println("Unable to change to download dir")
				}
				status.SetText("")
				rerr := receiver.Receive()
				donechan <- true
				prog.Hide()
				prog.SetValue(0)
				topline.SetText("Enter code to download")
				if rerr != nil {
					status.Text = "Receive failed: " + rerr.Error()
				} else {
					filesReceived := make([]string, len(receivednames))
					var i int
					for f := range receivednames {
						filesReceived[i] = f
						i++
					}
					sort.Slice(filesReceived, func(i, j int) bool {
						return receivednames[filesReceived[i]] < receivednames[filesReceived[j]]
					})

					plural := ""
					if len(filesReceived) > 1 {
						plural = "s"
					}
					status.Text = fmt.Sprintf("Received file%s %s", plural, strings.Join(filesReceived, ","))
				}
			}),
			prog,
			status,
		))

}
