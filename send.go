package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

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
	"github.com/schollz/croc/v10/src/utils"
)

const (
	MaterialFiles = "https://github.com/zhanghai/MaterialFiles"
	INSTALL       = "URL " + MaterialFiles + " is already in the clipboard.\nInstall the app to avoid this message."
	feDel         = 0
	feBar         = 1
	feSave        = 2
)

func sendTabItem(a fyne.App, w fyne.Window, parent *container.AppTabs) (ti *container.TabItem) {
	refresh := func() {}
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel(lp("Pick a file to send"))
	entry := widget.NewEntry()
	randomCode := utils.GetRandomName()

	entryText := os.Getenv(CROC_SECRET)
	if entryText == "" {
		entryText = randomCode
	}
	entry.SetText(entryText)

	randomCodeButton := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		randomCode = utils.GetRandomName()
		entry.SetText(randomCode)
	})

	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		a.Clipboard().SetContent(entry.Text)
	})

	totpCheck := widget.NewCheckWithData("", binding.BindPreferenceBool("totp-send", a.Preferences()))
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

		now := time.Now()
		remaining := 30 - now.Second()%30
		fyne.Do(func() {
			totpLabel.SetText(totp(entry.Text))
			totpProg.SetValue(float64(remaining) / 30)
		})
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
						update()
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
		a.Preferences().SetBool("totp-send", b)
	}
	if totpCheck.Checked {
		totpCheck.OnChanged(true)
	}

	entry.OnChanged = func(secret string) {
		os.Setenv(CROC_SECRET, secret)
		update()
	}

	sendDir := filepath.Join(os.TempDir(), "crocgui-send")

	boxholder := container.NewVBox()
	scroller := container.NewVScroll(boxholder)
	fileentries := make(map[string]*fyne.Container)

	removeEntry := func(fpath string, fe *fyne.Container) {
		boxholder.Remove(fe)
		os.Remove(fpath)
		log.Tracef("Removed file from internal cache: %s", fpath)
		delete(fileentries, fpath)
	}

	// nil if exists
	addEntry := func(dst string) (newentry *fyne.Container) {
		if _, has := fileentries[dst]; has {
			log.Tracef("exists %s", dst)
			return nil
		}
		labelFile := widget.NewLabel(filepath.Base(dst))
		deleteButton := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			if entry.Disabled() {
				log.Trace("Sending")
			} else {
				if fe, ok := fileentries[dst]; ok {
					removeEntry(dst, fe)
				} else {
					os.Remove(dst)
				}
			}
		})
		progFile := widget.NewProgressBar()
		fyne.Do(progFile.Hide)

		newentry = container.NewHBox(
			deleteButton,
			progFile,
			labelFile,
		)

		fileentries[dst] = newentry
		fyne.Do(func() {
			boxholder.Add(newentry)
		})
		return
	}

	addPath := func(src string) error {
		dst := filepath.Join(sendDir, filepath.Base(src))
		fe := addEntry(dst)
		if fe == nil {
			return nil
		}

		fi, err := os.Stat(src)
		size := fi.Size()
		if err != nil {
			return fmt.Errorf("URI (%s) %s", src, err.Error())
		} else if fi.IsDir() {
			log.Tracef("URI (%s), is dir", src)
			return nil
		}

		fi, err = os.Stat(dst)
		if err == nil && fi.Size() == size {
			log.Tracef("URI (%s), already in internal cache %s", src, dst)
			addEntry(dst)
			return nil
		}

		if !(isMobile || copyDebug) {
			err := Symlink(src, dst)
			if err == nil {
				log.Tracef("Make symlink URI (%s) to internal cache %s", src, dst)
				return nil
			}
			log.Errorf("Unable make symlink URI (%s) to internal cache %s, error: %s", src, dst, err)
		}

		CopyFileProgress(src, dst, fe, func(err error) {
			if err != nil {
				log.Errorf("Unable to copy file, error: %s - %s", sendDir, err.Error())
				removeEntry(dst, fe)
				return
			}
			log.Tracef("URI (%s), copied to internal cache %s", src, dst)

			if _, sterr := os.Stat(dst); sterr != nil {
				log.Errorf("Stat error: %s - %s", dst, sterr.Error())
				removeEntry(dst, fe)
			}
		})
		return nil
	}

	copyFromURCProgress := func(source fyne.URIReadCloser, c *fyne.Container, onComplete func(err error)) {
		if source == nil {
			onComplete(fmt.Errorf("user cancel dialog"))
			return
		}
		u := source.URI()
		// name := u.Name()
		// log.Tracef("name %s", name)
		name := uriBase(u)
		// log.Tracef("name %s", name)

		dst := filepath.Join(sendDir, name)
		destination, err := os.Create(dst)
		if err != nil {
			source.Close()
			onComplete(fmt.Errorf("unable to create file %s error: %s", dst, err.Error()))
			return
		}

		pw, restore := NewProgressWriter(destination, 1<<30, c)

		go func() {
			_, err := io.Copy(pw, source)
			source.Close()
			destination.Close()
			restore()
			onComplete(err)
		}()
	}

	os.MkdirAll(sendDir, 0o700)
	for _, name := range ls(sendDir) {
		if name != "" {
			log.Trace(name)
			addEntry(filepath.Join(sendDir, name))
		}
	}

	if isAndroid {
		// doneProcessIntent := make(chan struct{})
		a.Lifecycle().SetOnExitedForeground(func() {
			log.Trace("ExitedForeground")
			if !notFinish {
				excludeFromRecents()
			}
		})
		a.Lifecycle().SetOnStopped(func() {
			log.Trace("Stopped")
		})
		a.Lifecycle().SetOnStarted(func() {
			log.Trace("Started")
		})
		a.Lifecycle().SetOnEnteredForeground(func() {
			notFinish = false
			log.Trace("EnteredForeground")
			close(uriFromIntent)
			uriFromIntent = make(chan string, 100)

			close(textFromIntent)
			textFromIntent = make(chan string, 100)
			go func() {
				for {
					select {
					case <-done:
						log.Trace("done")
						return
					case text := <-textFromIntent:
						if text == "" {
							log.Trace("doneProcessIntent")
							return
						}
						if entry.Disabled() {
							log.Trace("Sending")
							log.Trace("doneProcessIntent")
							return
						}
						log.Tracef(`Received text: "%s"`, text)
						src := filepath.Join(sendDir, "text"+hashToFilename(text))
						if fe := addEntry(src); fe == nil {
							continue
						}

						source, err := os.Create(src)
						if err != nil {
							log.Errorf("Failed to create file: %s", err)
							continue
						}

						_, err = source.WriteString(text)
						if err != nil {
							source.Close()
							os.Remove(src)
							log.Errorf("Failed to write file: %s", err)
							continue
						}

						source.Close()
						fyne.Do(refresh)

					case uriString := <-uriFromIntent:
						if uriString == "" {
							log.Trace("doneProcessIntent")
							return
						}
						if entry.Disabled() {
							log.Trace("Sending")
							log.Trace("doneProcessIntent")
							return
						}
						u, err := storage.ParseURI(uriString)
						if err != nil {
							log.Errorf("ParseURI(%s) error: %v", u, err)
							continue
						}
						log.Tracef("apiLevel %d", apiLevel())
						mimeType := MimeType(u)
						log.Tracef("URI (%s) has MimeType: %s", u, mimeType)
						if mimeType == "vnd.android.document/directory" {
							// totalcmd на 14 Андроиде
							log.Tracef("URI (%s) is dir", u)
							continue
						}
						if u.Scheme() == "file" {
							if _, err := storage.ListerForURI(u); err == nil {
								// totalcmd на 9 Андроиде
								log.Tracef("URI (%s) is dir", u)
								continue
							}
						}
						// На Андроиде 9 storage.List крэшит на схеме content как и storage.CanList

						can, err := storage.CanRead(u)
						if err != nil {
							log.Errorf("%v", err)
							continue
						}
						if !can {
							log.Tracef("URI (%s) can't read", uriString)
							continue
						}

						// name := u.Name()
						// log.Tracef("name %s", name)
						name := uriBase(u)
						// log.Tracef("name %s", name)
						dst := filepath.Join(sendDir, name)
						fe := addEntry(dst)
						if fe == nil {
							continue
						}
						source, err := storage.Reader(u)
						if err != nil {
							log.Errorf("%v", err)
							continue
						}

						fyne.Do(refresh)
						copyFromURCProgress(source, fe, func(err error) {
							if err != nil {
								log.Errorf("URI (%s), copied to internal cache %s error: %s", u, dst, err)
								removeEntry(dst, fe)
								fyne.Do(refresh)
								return
							}
							log.Tracef("URI (%s), copied to internal cache %s", u, dst)

							if fi, sterr := os.Stat(dst); sterr != nil || fi.IsDir() {
								if sterr != nil {
									log.Errorf("Stat(%s) error: %v", dst, sterr)
								}
								if fi.IsDir() {
									log.Tracef("URI (%s) is dir", u)
								}
								removeEntry(dst, fe)
								fyne.Do(refresh)
							}
						})
					}
				}
			}()
			processIntent()
		})
	} else {
		if len(os.Args) > 0 {
			for _, src := range os.Args[1:] {
				if err := addPath(src); err != nil {
					log.Error(err.Error())
				}
			}
		}

		w.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
			if len(uris) == 0 {
				return
			}
			if entry.Disabled() {
				log.Trace("Sending")
				return
			}
			for _, uri := range uris {
				if err := addPath(uri.Path()); err != nil {
					log.Error(err.Error())
				}
			}
			refresh()
		})
	}

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		if supported, err := IsFilePickerSupported(); err != nil {
			log.Errorf("Error checking file picker support: %v", err)
		} else if !supported {
			log.Trace("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(MaterialFiles)
			dialog.ShowInformation(
				lp("Pick a file to send"),
				INSTALL,
				w,
			)
			return
		} else {
			log.Trace("File picker is supported")
		}
		ShowFileOpen(func(source fyne.URIReadCloser, e error) {
			if source == nil {
				return
			}
			if e != nil {
				source.Close()
				log.Errorf("Open dialog error: %s", e)
				return
			}
			u := source.URI()
			// name := u.Name()
			// log.Tracef("name %s", name)
			name := uriBase(u)
			// log.Tracef("name %s", name)
			dst := filepath.Join(sendDir, name)
			fe := addEntry(dst)
			if fe == nil {
				return
			}
			src := u.String()

			if !(isMobile || copyDebug) {
				err := Symlink(u.Path(), dst)
				if err == nil {
					log.Tracef("Make symlink URI (%s) to internal cache %s", src, dst)
					return
				}
				log.Errorf("Unable make symlink URI (%s) to internal cache %s, error: %s", src, dst, err)
			}

			copyFromURCProgress(source, fe, func(err error) {
				if err != nil {
					log.Errorf("URI (%s), copied to internal cache %s error: %s", src, dst, err)
					removeEntry(dst, fe)
					return
				}
				log.Tracef("URI (%s), copied to internal cache %s", src, dst)

				if _, sterr := os.Stat(dst); sterr != nil {
					log.Errorf("Stat error: %s - %s", dst, sterr.Error())
					removeEntry(dst, fe)
				}
			})
		}, w)
	})

	cancelChan := make(chan struct{})
	var cancelButton, mainButton *widget.Button

	removeEntrys := func() {
		for fpath, fe := range fileentries {
			removeEntry(fpath, fe)
		}
	}

	deleteAllButton := widget.NewButtonWithIcon(lp("Delete All"), theme.DeleteIcon(), func() {
		if len(fileentries) > 0 {
			removeEntrys()
		} else {
			entry.SetText("")
		}
	})

	mainButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		ok := len(entry.Text) > 5
		if totpCheck.Checked {
			ok = len(entry.Text) > 0
		}
		if !ok {
			log.Error("no receive code entered")
			dialog.ShowInformation(
				lp("Send"),
				lp("Enter code to download"),
				w,
			)
			return
		}

		ready := true
		for _, fe := range fileentries {
			pb := fe.Objects[feBar].(*widget.ProgressBar)
			if pb.Visible() && pb.Value < pb.Max {
				ready = false
				break
			}
		}

		// Only send if files selected
		if len(fileentries) < 1 || !ready {
			log.Error("no files selected")
			dialog.ShowInformation(
				lp("Send"),
				lp("Pick a file to send"),
				w,
			)
			return
		}

		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			secret = TOTP + secret
		}
		for _, fe := range fileentries {
			fe.Objects[feDel].Hide()
		}
		sender, err := croc.New(croc.Options{
			IsSender:      true,
			SharedSecret:  secret,
			Debug:         debugBool(a),
			RelayAddress:  a.Preferences().String("relay-address"),
			RelayPorts:    strings.Split(a.Preferences().String("relay-ports"), ","),
			RelayPassword: a.Preferences().String("relay-password"),
			// Stdout:           false,
			NoPrompt:       true,
			DisableLocal:   a.Preferences().Bool("disable-local"),
			NoMultiplexing: a.Preferences().Bool("disable-multiplexing"),
			OnlyLocal:      a.Preferences().Bool("force-local"),
			NoCompress:     a.Preferences().Bool("disable-compression"),
			Curve:          a.Preferences().String("pake-curve"),
			HashAlgorithm:  a.Preferences().String("croc-hash"),
			ThrottleUpload: a.Preferences().String("upload-throttle"),
			// ZipFolder:        false,
			// GitIgnore:        false,
			MulticastAddress: a.Preferences().String("multicast-address"),
			Exclude:          []string{},
		})
		if err != nil {
			log.Errorf("croc error: %s", err.Error())
			return
		}
		log.SetLevel(debugString(a))
		log.Trace("croc sender created")

		var filename string
		mainButton.Disable()
		cancelChan = make(chan struct{})
		cancelButton.Show()
		entry.Disable()

		addFileButton.Disable()
		totpCheck.Disable()
		if totpCheck.Checked {
			totpProg.Hide()
		}
		refresh()

		doneChan := make(chan struct{})

		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer func() {
				ticker.Stop()
				fyne.Do(func() {
					mainButton.Enable()
					prog.Hide()
					prog.SetValue(0)
					cancelButton.Hide()
					entry.Enable()
					addFileButton.Enable()

					totpCheck.Enable()
					if totpCheck.Checked {
						totpProg.Show()
					} else if entry.Text == randomCode {
						randomCode = utils.GetRandomName()
						entry.SetText(randomCode)
					}
					fyne.Do(refresh)
				})
			}()

			old := 0
			progW := NewProgressWrapper(prog)
			var TotalSent, size, totalMax int64
			toplineW := NewLabelWrapper(topline)
			fepw := NewProgressWrapper(nil)
			once := true
			for {
				select {
				case <-done:
					return
				case <-doneChan:
					removeEntrys()
					log.Tracef("A restart is better than leaving 12 goroutines leaking")
					fyne.Do(func() {
						restart(a, w)
					})
					return
				case <-cancelChan:
					s := fmt.Sprintf("%s %s", lp("Send cancelled."), filename)
					log.Error(s)
					fyne.Do(func() {
						topline.SetText(s)
					})
					Stop(sender)
					fyne.Do(func() {
						restart(a, w)
					})
					return
				case <-ticker.C:
					if sender == nil {
						return
					}
					if once && hashed(sender) {
						once = false
						fyne.Do(func() {
							topline.SetText(lp("Download"))
							prog.Show()
							for _, fe := range fileentries {
								pb := fe.Objects[feBar].(*widget.ProgressBar)
								pb.SetValue(0)
								pb.Show()
							}

							fyne.Do(refresh)
						})
						for _, fi := range sender.FilesToTransfer {
							path := filepath.Join(sendDir, fi.Name)
							if fe, ok := fileentries[path]; ok {
								if pb, ok := fe.Objects[feBar].(*widget.ProgressBar); ok {
									pb.Max = float64(fi.Size)
								}
							}
							totalMax += fi.Size
						}
						progW.SetMax(totalMax)
					}
					if sender.Step2FileInfoTransferred {
						cnum := sender.FilesToTransferCurrentNum
						if old < cnum+1 {
							old = cnum + 1
							if cnum > 0 {
								//100%
								fepw.Set100()
							}
							fi := sender.FilesToTransfer[cnum]
							filename = fi.Name
							toplineW.SetText(fmt.Sprintf("%s: %s(%d/%d)", lp("Sending file"), filename, cnum+1, len(sender.FilesToTransfer)))
							TotalSent += size
							size = fi.Size
							path := filepath.Join(sendDir, fi.Name)
							log.Trace(path)
							if fe, ok := fileentries[path]; ok {
								fepw = NewProgressWrapper(fe.Objects[feBar].(*widget.ProgressBar))
							} else {
								fepw = NewProgressWrapper(nil)
							}
						}
						progW.SetValue(TotalSent + sender.TotalSent)
						fepw.SetValue(sender.TotalSent)
					}
				}
			}
		}()

		go func() {
			var filepaths []string
			for fpath := range fileentries {
				if isSymlink(fpath) {
					if target, err := os.Readlink(fpath); err == nil {
						fpath = target
					}
				}
				filepaths = append(filepaths, fpath)
			}
			fi, emptyfolders, numFolders, ferr := croc.GetFilesInfo(filepaths, false, false, []string{})
			if ferr != nil {
				log.Errorf("file info failed: %s", ferr)
			}
			var serr error
			if EMULATE == 0 {
				serr = sender.Send(fi, emptyfolders, numFolders)
			} else {
				log.Warnf("Send %v %v %v", fi, emptyfolders, numFolders)
				time.Sleep(EMULATE)
				defer func() {
					sender = nil
				}()
			}

			fyne.Do(func() {
				if serr != nil {
					if errors.Is(serr, io.EOF) {
						serr = fmt.Errorf("%s", lp("Receive cancelled."))
					}
					s := fmt.Sprintf("Send failed: %s", serr)
					log.Error(s)
					topline.SetText(s)
				} else {
					topline.SetText(fmt.Sprintf("%s: %s", lp("Sent file"), filename))
				}
			})
			close(doneChan)
		}()

		// +12 go routines
		log.Warnf("NumGoroutine %d", runtime.NumGoroutine())
		a.Clipboard().SetContent(entry.Text)
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		close(cancelChan)
	})
	cancelButton.Hide()

	top := container.NewVBox(
		container.NewHBox(topline, layout.NewSpacer(), addFileButton, randomCodeButton),
		widget.NewForm(&widget.FormItem{Text: lp("Send Code"), Widget: entry}),
		container.NewHBox(
			copyCodeButton,
			totpCheck,
			totpLabel,
			totpProg,
			layout.NewSpacer(),
			deleteAllButton,
		),
		mainButton,
		prog,
		cancelButton,
	)

	ti = container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
	refresh = func() {
		if parent.Selected() != ti {
			parent.Select(ti)
		}
		// ti.Content.Refresh()
	}
	return
}

// Big File Dialog
func ShowFileOpen(callback func(reader fyne.URIReadCloser, err error), parent fyne.Window) {
	if isMobile {
		notFinish = true
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

// For mobile os.Exit.
// For desktop Restart.
func restart(a fyne.App, w fyne.Window) {
	if noRestart {
		return
	}
	if isMobile {
		// notification := fyne.NewNotification("CrocGUI", "Application closed")
		// a.SendNotification(notification)
		sendNotification(a, "CrocGUI", "Application closed. Tap to start it.")
		w.Close()
		os.Exit(0)
		return
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = os.Environ()
	cmd.Start()
	w.Close()
	os.Exit(0)
}

// type clientShadow struct {
// 	Options                         croc.Options
// 	Pake                            *pake.Pake
// 	Key                             []byte
// 	ExternalIP, ExternalIPConnected string

// 	// steps involved in forming relationship
// 	Step1ChannelSecured       bool
// 	Step2FileInfoTransferred  bool
// 	Step3RecipientRequestFile bool
// 	Step4FileTransferred      bool
// 	Step5CloseChannels        bool
// 	SuccessfulTransfer        bool

// 	// send / receive information of all files
// 	FilesToTransfer           []croc.FileInfo
// 	EmptyFoldersToTransfer    []croc.FileInfo
// 	TotalNumberOfContents     int
// 	TotalNumberFolders        int
// 	FilesToTransferCurrentNum int
// 	FilesHasFinished          map[int]struct{}
// 	TotalFilesIgnored         int

// 	// send / receive information of current file
// 	CurrentFile            *os.File
// 	CurrentFileChunkRanges []int64
// 	CurrentFileChunks      []int64
// 	CurrentFileIsClosed    bool
// 	LastFolder             string

// 	TotalSent              int64
// 	TotalChunksTransferred int
// 	chunkMap               map[uint64]struct{}
// 	limiter                *rate.Limiter

// 	// tcp connections
// 	conn []*comm.Comm

// 	bar             *progressbar.ProgressBar
// 	longestFilename int
// 	firstSend       bool

// 	mutex                    *sync.Mutex
// 	fread                    *os.File
// 	numfinished              int
// 	quit                     chan bool
// 	finishedNum              int
// 	numberOfTransferredFiles int
// }

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
		log.Errorf("Stop: %v", err)
	}
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

type ProgressWriter struct {
	Writer       io.Writer
	Total        int64
	Written      int64
	OnProgress   func(float64)
	lastCall     time.Time
	lastProgress float64
	cancel       <-chan struct{}
	mu           sync.Mutex
}

var ErrWriteCanceled = errors.New("write canceled")

const minInterval = 200 * time.Millisecond

func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	select {
	case <-pw.cancel:
		return 0, ErrWriteCanceled
	case <-done:
		return 0, ErrApplicationShutdown
	default:
	}

	pw.mu.Lock()
	defer pw.mu.Unlock()
	n, err = pw.Writer.Write(p)

	if err != nil || pw.OnProgress == nil || pw.Total <= 0 {
		return
	}
	pw.Written += int64(n)
	progress := float64(pw.Written) / float64(pw.Total)
	if pw.lastProgress < progress {
		if progress > 1.0 {
			progress = 1.0
		}
		pw.lastProgress = progress
	} else {
		return
	}

	now := time.Now()
	if now.Sub(pw.lastCall) >= minInterval {
		pw.OnProgress(progress)
		pw.lastCall = now
	}
	return
}

func NewProgressWriter(destination io.Writer, total int64, c *fyne.Container) (pw *ProgressWriter, restore func()) {
	db := c.Objects[feDel].(*widget.Button)
	pb := c.Objects[feBar].(*widget.ProgressBar)
	oldOnTapped := db.OnTapped

	cancelChan := make(chan struct{})

	pw = &ProgressWriter{
		Writer: destination,
		Total:  total,
		cancel: cancelChan,
		OnProgress: func(p float64) {
			fyne.Do(func() { pb.SetValue(p) })
		},
	}

	db.OnTapped = func() {
		select {
		case <-cancelChan:
		default:
			close(cancelChan)
		}
	}

	fyne.Do(func() {
		pb.SetValue(0)
		pb.Max = 1.0
		pb.Show()
	})

	restore = func() {
		db.OnTapped = oldOnTapped
		fyne.Do(pb.Hide)
	}

	return pw, restore
}

func CopyFileProgress(src, dst string, c *fyne.Container, onComplete func(err error)) {
	source, err := os.Open(src)
	if err != nil {
		onComplete(err)
		return
	}

	fi, err := os.Stat(src)
	if err != nil {
		source.Close()
		onComplete(err)
		return
	}

	destination, err := os.Create(dst)
	if err != nil {
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

func hashToFilename(data string) string {
	hash := crc32.ChecksumIEEE([]byte(data))
	return fmt.Sprintf("%x", hash)
}

type ProgressWrapper struct {
	*widget.ProgressBar
	lastValue float64
	lastCall  time.Time
}

func NewProgressWrapper(bar *widget.ProgressBar) *ProgressWrapper {
	if bar != nil {
		fyne.Do(func() {
			bar.SetValue(0)
		})
	}
	return &ProgressWrapper{
		ProgressBar: bar,
		lastValue:   -1,
	}
}

func (pw *ProgressWrapper) Show() {
	if pw.ProgressBar != nil {
		fyne.Do(pw.ProgressBar.Show)
	}
}

func (pw *ProgressWrapper) Hide() {
	if pw.ProgressBar != nil {
		fyne.Do(pw.ProgressBar.Hide)
	}
}

func (pw *ProgressWrapper) Set100() {
	if pw.ProgressBar != nil {
		pw.SetValue(int64(pw.ProgressBar.Max))
	}
}

func (pw *ProgressWrapper) SetValue(value int64) {
	if pw.ProgressBar == nil {
		return
	}
	newValue := float64(value)

	if newValue > pw.lastValue || pw.lastValue == -1 {
		now := time.Now()
		if now.Sub(pw.lastCall) >= minInterval || pw.lastValue == -1 {
			pw.lastValue = newValue
			pw.lastCall = now
			fyne.Do(func() {
				pw.ProgressBar.SetValue(newValue)
			})
		}
	}
}

func (pw *ProgressWrapper) SetMax(max int64) {
	if pw.ProgressBar == nil {
		return
	}
	newMax := float64(max)

	if newMax != pw.ProgressBar.Max {
		fyne.Do(func() {
			pw.ProgressBar.Max = newMax
		})

		if newMax < pw.lastValue || pw.lastValue == -1 {
			pw.lastValue = -1
		}
	}
}

type LabelWrapper struct {
	*widget.Label
	lastText string
	// lastCall time.Time
}

func NewLabelWrapper(label *widget.Label) *LabelWrapper {
	return &LabelWrapper{
		Label:    label,
		lastText: "",
	}
}

func (lw *LabelWrapper) SetText(text string) {
	// now := time.Now()

	// if text != lw.lastText || now.Sub(lw.lastCall) >= minInterval {
	if text != lw.lastText {
		lw.lastText = text
		// lw.lastCall = now
		fyne.Do(func() {
			lw.Label.SetText(text)
		})
	}
}

func hashed(c *croc.Client) bool {
	for _, file := range c.FilesToTransfer {
		if len(file.Hash) == 0 {
			return false
		}
	}
	return true
}

func isSymlink(path string) bool {
	if isMobile {
		return false
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fileInfo.Mode()&os.ModeSymlink != 0
}
