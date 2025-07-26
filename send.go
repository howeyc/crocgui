package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	log "github.com/schollz/logger"
	"golang.org/x/time/rate"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
	"github.com/schollz/pake/v3"
	"github.com/schollz/progressbar/v3"
)

func sendTabItem(a fyne.App, w fyne.Window) *container.TabItem {
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
		a.Clipboard().SetContent(sendEntry.Text)
	})

	sendDir, _ = os.MkdirTemp("", "crocgui-send")

	boxholder := container.NewVBox()
	senderScroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	if android {
		go func() {
			for {
				select {
				case <-done:
					return
				case text := <-textFromIntent:
					if text == "" {
						log.Errorf(`Received text: ""`)
						continue
					}
					log.Tracef(`Received text: "%s"`, text)
					source, err := os.CreateTemp("", "text*")
					if err != nil {
						log.Errorf("%s", err)
						continue
					}
					src := source.Name()
					_, err = source.WriteString(text)
					if err != nil {
						source.Close()
						os.Remove(src)
						log.Errorf("%s", err)
						continue
					}
					source.Close()
					if err := addPath(src, sendDir, fileentries, boxholder, sendEntry); err != nil {
						log.Errorf("%s\n", err)
					}
					os.Remove(src)
				case uriString := <-uriFromIntent:
					if uriString == "" {
						log.Errorf(`Received uri: ""`)
						continue
					}
					if _, err := url.Parse(uriString); err == nil {
						log.Tracef(`Received URI: "%s"`, uriString)
					} else {
						log.Errorf(`Received URI: "%s" error: %s`, uriString, err)
						continue
					}
					fyneURI, err := storage.ParseURI(uriString)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					listable, err := storage.CanList(fyneURI)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					if listable {
						log.Tracef("URI (%s) is dir", uriString)
						continue
					}
					can, err := storage.CanRead(fyneURI)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					if !can {
						log.Tracef("URI (%s) can't read", uriString)
						continue
					}
					src, err := url.PathUnescape(fyneURI.Path())
					if err != nil {
						log.Tracef("Decode URI (%s): %v", fyneURI.Path(), err)
						continue
					}
					dst := filepath.Join(sendDir, filepath.Base(src))
					if _, has := fileentries[dst]; has {
						log.Tracef("exists %s", src)
						continue
					}

					source, err := storage.Reader(fyneURI)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					if err := copyFromURC(source, sendDir, fileentries, boxholder, sendEntry); err != nil {
						log.Errorf("%s\n", err)
					}
				}
			}
		}()
	} else {
		if len(os.Args) > 0 {
			for _, src := range os.Args[1:] {
				if err := addPath(src, sendDir, fileentries, boxholder, sendEntry); err != nil {
					log.Errorf(err.Error())
				}
			}
		}

		w.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
			if len(uris) == 0 {
				return
			}
			for _, uri := range uris {
				if err := addPath(uri.Path(), sendDir, fileentries, boxholder, sendEntry); err != nil {
					log.Errorf(err.Error())
				}
			}
			SelectIndex(w, 0)
		})
	}

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		ShowFileOpen(func(source fyne.URIReadCloser, e error) {
			if e != nil {
				log.Errorf("Open dialog error: %s", e)
				return
			}
			if err := copyFromURC(source, sendDir, fileentries, boxholder, sendEntry); err != nil {
				log.Errorf("%s\n", err)
			}
		}, w)
	})

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
			log.Tracef("Removed file from internal cache: %s\n", fpath)
			delete(fileentries, fpath)
		}

		addFileButton.Enable()
		if sendEntry.Text == randomCode {
			randomCode = utils.GetRandomName()
			sendEntry.SetText(randomCode)
		}
		sendEntry.Enable()
	}

	sendButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		if len(sendEntry.Text) < 6 {
			log.Error("no receive code entered\n")
			dialog.ShowInformation(
				lp("Send"),
				lp("Enter code to download"),
				w,
			)
			return
		}

		// Only send if files selected
		if len(fileentries) < 1 {
			log.Error("no files selected\n")
			dialog.ShowInformation(
				lp("Send"),
				lp("Pick a file to send"),
				w,
			)
			return
		}

		// addFileButton.Hide()
		addFileButton.Disable()
		sender, err := croc.New(croc.Options{
			IsSender:         true,
			SharedSecret:     sendEntry.Text,
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
			ThrottleUpload:   a.Preferences().String("upload-throttle"),
			ZipFolder:        false,
			GitIgnore:        false,
			MulticastAddress: a.Preferences().String("multicast-address"),
			Exclude:          []string{},
		})
		if err != nil {
			log.Errorf("croc error: %s\n", err.Error())
			return
		}
		log.SetLevel(crocDebugLevel())
		log.Trace("croc sender created\n")

		var filename string
		prog.Show()

		for _, obj := range activeButtonHolder.Objects {
			activeButtonHolder.Remove(obj)
		}
		activeButtonHolder.Add(cancelButton)

		donechan := make(chan bool)
		sendnames := make(map[string]int)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()
			old := 0
			for {
				select {
				case <-ticker.C:
					if sender == nil {
						return
					}
					if sender.Step2FileInfoTransferred {
						cnum := sender.FilesToTransferCurrentNum
						fyne.Do(func() {
							if old < cnum+1 {
								old = cnum + 1
								fi := sender.FilesToTransfer[cnum]
								filename = filepath.Base(fi.Name)
								sendnames[filename] = cnum
								topline.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Sending file"), filename, cnum+1, len(sender.FilesToTransfer)))
								prog.Max = float64(fi.Size)
							}
							prog.SetValue(float64(sender.TotalSent))
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
			var filepaths []string
			for fpath := range fileentries {
				filepaths = append(filepaths, fpath)
			}
			fyne.Do(sendEntry.Disable)
			fi, emptyfolders, numFolders, ferr := croc.GetFilesInfo(filepaths, false, false, []string{})
			if ferr != nil {
				log.Errorf("file info failed: %s\n", ferr)
			}
			var serr error
			if EMULATE == 0 {
				serr = sender.Send(fi, emptyfolders, numFolders)
			} else {
				log.Warnf("Send %v %v %v\n", fi, emptyfolders, numFolders)
				time.Sleep(EMULATE)
				defer func() {
					sender = nil
				}()
			}
			donechan <- true
			fyne.Do(func() {
				if serr != nil {
					log.Errorf("Send failed: %s\n", serr)
					topline.SetText(serr.Error())
				} else {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Sent file"), filename))
				}
				resetSender()
			})
		}()
		go func() {
			select {
			case <-donechan:
				if !mobile {
					log.Tracef("A restart is better than leaving 12 goroutines leaking\n")
					fyne.Do(func() {
						restart(a)
					})
				}
				return
			case <-cancelchan:
				log.Warnf("Send cancelled. %s: %v\n", sendDir, ls(sendDir))
				Stop(sender)
				clear(sendDir)
				fyne.Do(func() {
					restart(a)
				})
			}
		}()
		// +12 go routines
		log.Warnf("NumGoroutine %d\n", runtime.NumGoroutine())
		a.Clipboard().SetContent(sendEntry.Text)
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
	})

	activeButtonHolder.Add(sendButton)

	sendTop := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer(), addFileButton, copyCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Send Code"), Widget: sendEntry}),
	)
	sendBot := container.NewVBox(
		activeButtonHolder,
		prog,
		// container.NewHBox(status, copyCodeButton),
		// debugBox,
	)

	return container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewBorder(sendTop, sendBot, nil, nil, senderScroller))
}

// Big File Dialog
func ShowFileOpen(callback func(reader fyne.URIReadCloser, err error), parent fyne.Window) {
	if mobile {
		dialog.ShowFileOpen(callback, parent)
		return
	}
	fd := dialog.NewFileOpen(callback, parent)
	fd.Resize(parent.Canvas().Size())
	fd.Show()
}

func CopyFile(src, dst string) error {
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

// Dirty refresh
func SelectIndex(window fyne.Window, index int) {
	var findTabs func(fyne.CanvasObject) *container.AppTabs
	findTabs = func(obj fyne.CanvasObject) *container.AppTabs {
		switch v := obj.(type) {
		case *container.AppTabs:
			return v
		case *fyne.Container:
			for _, child := range v.Objects {
				if tabs := findTabs(child); tabs != nil {
					return tabs
				}
			}
		}
		return nil
	}
	if tabs := findTabs(window.Content()); tabs != nil {
		tabs.SelectIndex(index)
		tabs.Refresh()
	}
}

func addPath(src, sendDir string,
	fileentries map[string]*fyne.Container,
	boxholder *fyne.Container,
	sendEntry *widget.Entry) error {
	dst := filepath.Join(sendDir, filepath.Base(src))

	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("URI (%s) %s", src, err.Error())
	} else if fi.IsDir() {
		log.Tracef("URI (%s), is dir\n", src)
		return nil
	}

	fi, err = os.Stat(dst)
	if err == nil {
		log.Tracef("URI (%s), already in internal cache %s\n", src, dst)
		return nil
	}

	if err := CopyFile(src, dst); err != nil {
		return fmt.Errorf("Unable to copy file, error: %s - %s\n", sendDir, err.Error())
	}
	log.Tracef("URI (%s), copied to internal cache %s\n", src, dst)

	if _, sterr := os.Stat(dst); sterr != nil {
		return fmt.Errorf("Stat error: %s - %s\n", dst, sterr.Error())
	}
	addEntry(dst, fileentries, boxholder, sendEntry)
	return nil
}

// For mobile Quit.
// For desktop Restart.
func restart(a fyne.App) {
	if !mobile {
		exec.Command(os.Args[0]).Start()
	}
	a.Quit()
}

type clientShadow struct {
	Options                         croc.Options
	Pake                            *pake.Pake
	Key                             []byte
	ExternalIP, ExternalIPConnected string

	// steps involved in forming relationship
	Step1ChannelSecured       bool
	Step2FileInfoTransferred  bool
	Step3RecipientRequestFile bool
	Step4FileTransferred      bool
	Step5CloseChannels        bool
	SuccessfulTransfer        bool

	// send / receive information of all files
	FilesToTransfer           []croc.FileInfo
	EmptyFoldersToTransfer    []croc.FileInfo
	TotalNumberOfContents     int
	TotalNumberFolders        int
	FilesToTransferCurrentNum int
	FilesHasFinished          map[int]struct{}
	TotalFilesIgnored         int

	// send / receive information of current file
	CurrentFile            *os.File
	CurrentFileChunkRanges []int64
	CurrentFileChunks      []int64
	CurrentFileIsClosed    bool
	LastFolder             string

	TotalSent              int64
	TotalChunksTransferred int
	chunkMap               map[uint64]struct{}
	limiter                *rate.Limiter

	// tcp connections
	conn []*comm.Comm

	bar             *progressbar.ProgressBar
	longestFilename int
	firstSend       bool

	mutex                    *sync.Mutex
	fread                    *os.File
	numfinished              int
	quit                     chan bool
	finishedNum              int
	numberOfTransferredFiles int
}

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
		log.Errorf("Stop: %v\n", err)
	}
}

func copyFromURC(source fyne.URIReadCloser, sendDir string,
	fileentries map[string]*fyne.Container,
	boxholder *fyne.Container,
	sendEntry *widget.Entry) error {
	if source == nil {
		return fmt.Errorf("User cancel dialog")
	}
	defer source.Close()
	// src := source.URI().String()
	src, err := url.PathUnescape(source.URI().Path())
	if err != nil {
		return fmt.Errorf("Decode URI (%s): %v", source.URI().Path(), err)
	}

	dst := filepath.Join(sendDir, filepath.Base(src))
	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("Unable to create file %s error: %s", dst, err.Error())
	}
	defer destination.Close()

	io.Copy(destination, source)

	log.Tracef("URI (%s), copied to internal cache %s", src, dst)

	if _, sterr := os.Stat(dst); sterr != nil {
		return fmt.Errorf("Stat file %s error: %s", dst, sterr.Error())
	}
	addEntry(dst, fileentries, boxholder, sendEntry)
	return nil
}

func addEntry(dst string,
	fileentries map[string]*fyne.Container,
	boxholder *fyne.Container,
	sendEntry *widget.Entry) {
	labelFile := widget.NewLabel(filepath.Base(dst))
	newentry := container.NewHBox(
		labelFile,
		layout.NewSpacer(),
		widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if !sendEntry.Disabled() {
				if fe, ok := fileentries[dst]; ok {
					boxholder.Remove(fe)
					os.Remove(dst)
					log.Tracef("Removed file from internal cache: %s\n", dst)
					delete(fileentries, dst)
				}
			}
		}),
	)

	fileentries[dst] = newentry
	boxholder.Add(newentry)
}
func clear(path string) {
	lsPath := ls(path)
	log.Warnf("%s: %v\n", path, lsPath)
	if len(lsPath) > 0 {
		log.Tracef("Clear %s %v\n", path, os.RemoveAll(path))
	}
}
