package main

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v8/src/croc"
)

func recvTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()

	recvDir, _ := os.MkdirTemp("", "crocgui-recv")

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
					IsSender:       false,
					SharedSecret:   recvEntry.Text,
					Debug:          false,
					RelayAddress:   a.Preferences().String("relay-address"),
					RelayPassword:  a.Preferences().String("relay-password"),
					Stdout:         false,
					NoPrompt:       true,
					DisableLocal:   a.Preferences().Bool("disable-local"),
					NoMultiplexing: a.Preferences().Bool("disable-multiplexing"),
					OnlyLocal:      a.Preferences().Bool("force-local"),
					NoCompress:     a.Preferences().Bool("disable-compression"),
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
				cderr := os.Chdir(recvDir)
				if cderr != nil {
					log.Println("Unable to change to dir:", recvDir, cderr)
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
					status.SetText(fmt.Sprintf("Received file%s %s", plural, strings.Join(filesReceived, ",")))
					filepath.Walk(recvDir, func(path string, info fs.FileInfo, err error) error {
						if err != nil {
							return err
						}
						if !info.IsDir() {
							var diagwg sync.WaitGroup
							diagwg.Add(1)
							savedialog := dialog.NewFileSave(func(f fyne.URIWriteCloser, e error) {
								var ofile io.WriteCloser
								var oerr error
								ofile = f
								oerr = e
								if oerr != nil {
									status.SetText(oerr.Error())
									return
								}
								ifile, ierr := os.Open(path)
								if ierr != nil {
									status.SetText(ierr.Error())
									return
								}
								io.Copy(ofile, ifile)
								ifile.Close()
								ofile.Close()
								os.Remove(path)
								diagwg.Done()
							}, w)
							savedialog.SetFileName(filepath.Base(path))
							savedialog.Show()
							diagwg.Wait()
						}
						return nil
					})
				}
			}),
			prog,
			status,
		))

}
