package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
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
	"fyne.io/fyne/v2/data/binding"
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
	entry := widget.NewEntry()
	randomCode := utils.GetRandomName()

	if secret := os.Getenv(CROC_SECRET); secret != "" {
		randomCode = secret
	}

	entry.SetText(randomCode)
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
		progFile.Hide()

		newentry = container.NewHBox(
			deleteButton,
			progFile,
			labelFile,
		)

		fileentries[dst] = newentry
		boxholder.Add(newentry)
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
			log.Tracef("URI (%s), is dir\n", src)
			return nil
		}

		fi, err = os.Stat(dst)
		if err == nil && fi.Size() == size {
			log.Tracef("URI (%s), already in internal cache %s\n", src, dst)
			addEntry(dst)
			return nil
		}

		CopyFileProgress(src, dst, fe, func(err error) {
			// onComplete
			if err != nil {
				log.Errorf("Unable to copy file, error: %s - %s\n", sendDir, err.Error())
				removeEntry(dst, fe)
				return
			}
			log.Tracef("URI (%s), copied to internal cache %s\n", src, dst)

			if _, sterr := os.Stat(dst); sterr != nil {
				log.Errorf("Stat error: %s - %s\n", dst, sterr.Error())
				removeEntry(dst, fe)
			}
		})
		return nil
	}

	copyFromURCProgress := func(source fyne.URIReadCloser, c *fyne.Container, cb func(err error)) {
		if source == nil {
			cb(fmt.Errorf("User cancel dialog"))
			return
		}
		u := source.URI()
		name := u.Name()
		log.Tracef("name %s", name)
		name = uriBase(u)
		log.Tracef("name %s", name)

		dst := filepath.Join(sendDir, name)
		destination, err := os.Create(dst)
		if err != nil {
			source.Close()
			cb(fmt.Errorf("Unable to create file %s error: %s", dst, err.Error()))
			return
		}

		pw, restore := NewProgressWriter(destination, 1<<30, c)

		go func() {
			_, err := io.Copy(pw, source)
			source.Close()
			destination.Close()
			restore()
			cb(err)
		}()
	}

	os.MkdirAll(sendDir, 0o700)
	for _, name := range ls(sendDir) {
		if name != "" {
			addEntry(filepath.Join(sendDir, name))
		}
	}

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
					if entry.Disabled() {
						log.Trace("Sending")
						continue
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
					SelectIndex(w, 0)

				case uriString := <-uriFromIntent:
					if uriString == "" {
						log.Errorf(`Received uri: ""`)
						continue
					}
					if entry.Disabled() {
						log.Trace("Sending")
						continue
					}
					if _, err := url.Parse(uriString); err == nil {
						log.Tracef(`Received URI: "%s"`, uriString)
					} else {
						log.Errorf(`Received URI: "%s" error: %s`, uriString, err)
						continue
					}
					u, err := storage.ParseURI(uriString)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					listable, err := storage.CanList(u)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					if listable {
						log.Tracef("URI (%s) is dir", uriString)
						continue
					}
					can, err := storage.CanRead(u)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					if !can {
						log.Tracef("URI (%s) can't read", uriString)
						continue
					}
					name := u.Name()
					log.Tracef("name %s", name)
					name = uriBase(u)
					log.Tracef("name %s", name)
					dst := filepath.Join(sendDir, name)
					fe := addEntry(dst)
					if fe == nil {
						continue
					}

					source, err := storage.Reader(u)
					if err != nil {
						log.Errorf("%s", err.Error())
						continue
					}
					SelectIndex(w, 0)

					src := u.String()
					copyFromURCProgress(source, fe, func(err error) {
						// onComplete
						if err != nil {
							log.Errorf("URI (%s), copied to internal cache %s error: %s\n", src, dst, err)
							removeEntry(dst, fe)
							return
						}
						log.Tracef("URI (%s), copied to internal cache %s\n", src, dst)

						if _, sterr := os.Stat(dst); sterr != nil {
							log.Errorf("Stat error: %s - %s\n", dst, sterr.Error())
							removeEntry(dst, fe)
						}
					})
				}
			}
		}()
	} else {
		if len(os.Args) > 0 {
			for _, src := range os.Args[1:] {
				if err := addPath(src); err != nil {
					log.Errorf(err.Error())
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
			SelectIndex(w, 0)
			for _, uri := range uris {
				if err := addPath(uri.Path()); err != nil {
					log.Errorf(err.Error())
				}
			}
		})
	}

	addFileButton := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
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
			name := u.Name()
			log.Tracef("name %s", name)
			name = uriBase(u)
			log.Tracef("name %s", name)
			dst := filepath.Join(sendDir, name)
			fe := addEntry(dst)
			if fe == nil {
				return
			}

			src := u.String()
			copyFromURCProgress(source, fe, func(err error) {
				// onComplete
				if err != nil {
					log.Errorf("URI (%s), copied to internal cache %s error: %s\n", src, dst, err)
					removeEntry(dst, fe)
					return
				}
				log.Tracef("URI (%s), copied to internal cache %s\n", src, dst)

				if _, sterr := os.Stat(dst); sterr != nil {
					log.Errorf("Stat error: %s - %s\n", dst, sterr.Error())
					removeEntry(dst, fe)
				}
			})
		}, w)
	})

	cancelchan := make(chan bool)
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

	reset := func() {
		mainButton.Enable()
		prog.Hide()
		prog.SetValue(0)
		cancelButton.Hide()
		removeEntrys()
		addFileButton.Enable()

		totpCheck.Enable()
		if totpCheck.Checked {
			totpProg.Show()
		} else if entry.Text == randomCode {
			randomCode = utils.GetRandomName()
			entry.SetText(randomCode)
		}

		entry.Enable()
	}

	mainButton = widget.NewButtonWithIcon(lp("Send"), theme.MailSendIcon(), func() {
		ok := len(entry.Text) > 5
		if totpCheck.Checked {
			ok = len(entry.Text) > 0
		}
		if !ok {
			log.Error("no receive code entered\n")
			dialog.ShowInformation(
				lp("Send"),
				lp("Enter code to download"),
				w,
			)
			return
		}

		ready := true
		for _, fe := range fileentries {
			//progressBar
			if fe.Objects[1].Visible() {
				ready = false
				break
			}
		}

		// Only send if files selected
		if len(fileentries) < 1 || !ready {
			log.Error("no files selected\n")
			dialog.ShowInformation(
				lp("Send"),
				lp("Pick a file to send"),
				w,
			)
			return
		}

		addFileButton.Disable()
		totpCheck.Disable()
		secret := entry.Text
		if totpCheck.Checked {
			secret = totp(entry.Text)
			totpLabel.SetText(secret)
			secret = TOTP + secret
			totpProg.Hide()
		}
		for _, fe := range fileentries {
			fe.Objects[0].Hide()
		}
		sender, err := croc.New(croc.Options{
			IsSender:         true,
			SharedSecret:     secret,
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
		mainButton.Disable()
		cancelButton.Show()

		donechan := make(chan bool)
		go func() {
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()
			old := 0
			progW := NewProgressWrapper(prog)
			var TotalSent, size int64
			totalMax := total(sendDir)
			progW.SetMax(totalMax)
			toplineW := NewLabelWrapper(topline)
			fepw := NewProgressWrapper(nil)
			once := true
			for {
				select {
				case <-done:
					close(donechan)
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
							SelectIndex(w, 0)
						})
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
							path := filepath.Join(fi.FolderSource, fi.Name)
							log.Trace(path)
							if fe, ok := fileentries[path]; ok {
								fepw = NewProgressWrapper(fe.Objects[1].(*widget.ProgressBar))
								fepw.SetMax(size)
								fepw.Show()
							} else {
								fepw = NewProgressWrapper(nil)
							}
						}
						progW.SetValue(TotalSent + sender.TotalSent)
						fepw.SetValue(sender.TotalSent)
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
			fyne.Do(entry.Disable)
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
				reset()
			})
		}()
		go func() {
			select {
			case <-done:
				return
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
				fyne.Do(func() {
					restart(a)
				})
			}
		}()
		// +12 go routines
		log.Warnf("NumGoroutine %d\n", runtime.NumGoroutine())
		a.Clipboard().SetContent(entry.Text)
	})

	cancelButton = widget.NewButtonWithIcon(lp("Cancel"), theme.CancelIcon(), func() {
		cancelchan <- true
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

	return container.NewTabItemWithIcon(lp("Send"), theme.MailSendIcon(),
		container.NewBorder(top, nil, nil, nil, scroller))
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

// For mobile Quit.
// For desktop Restart.
func restart(a fyne.App) {
	if !mobile {
		cmd := exec.Command(os.Args[0])
		cmd.Env = os.Environ()
		cmd.Start()
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
	db := c.Objects[0].(*widget.Button)
	pb := c.Objects[1].(*widget.ProgressBar)
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

	pb.SetValue(0)
	pb.Max = 1.0
	pb.Show()

	restore = func() {
		db.OnTapped = oldOnTapped
		fyne.Do(pb.Hide)
	}

	return pw, restore
}

func CopyFileProgress(src, dst string, c *fyne.Container, cb func(err error)) {
	source, err := os.Open(src)
	if err != nil {
		cb(err)
		return
	}

	fi, err := os.Stat(src)
	if err != nil {
		source.Close()
		cb(err)
		return
	}

	destination, err := os.Create(dst)
	if err != nil {
		source.Close()
		cb(err)
		return
	}

	pw, restore := NewProgressWriter(destination, fi.Size(), c)

	go func() {
		_, err := io.Copy(pw, source)
		source.Close()
		destination.Close()
		restore()
		cb(err)
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
	if pw.ProgressBar == nil {
		return
	}
	pw.SetValue(int64(pw.ProgressBar.Max))
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
	lastCall time.Time
}

func NewLabelWrapper(label *widget.Label) *LabelWrapper {
	return &LabelWrapper{
		Label:    label,
		lastText: "",
	}
}

func (lw *LabelWrapper) SetText(text string) {
	now := time.Now()

	if text != lw.lastText || now.Sub(lw.lastCall) >= minInterval {
		lw.lastText = text
		lw.lastCall = now
		fyne.Do(func() {
			lw.Label.SetText(text)
		})
	}
}

func total(dir string) int64 {
	var size int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0
	}

	return size
}

func hashed(c *croc.Client) bool {
	for _, file := range c.FilesToTransfer {
		if len(file.Hash) == 0 {
			return false
		}
	}
	return true
}
