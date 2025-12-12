//go:generate bash -c "GOFLAGS=-ldflags=-s go install"

package main

import (
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	_ "crocgui/internal/translations"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
)

//go:embed metadata/en-US/images/featureGraphic.png
var textlogobytes []byte

var (
	isMobile               bool
	isAndroid              bool
	ErrApplicationShutdown error
	done                   chan struct{}
	cdLock                 atomic.Int32
	uriFromIntent          = make(chan string, 100)
	textFromIntent         = make(chan string, 100)
	replacer               *strings.Replacer
	logOutput              logWriter
	atSI                   int
	notFinish              bool
	wd                     string
	OnSelectedReload       = make(map[int]func(), 2)
	swap                   bool
	tempDir                string
	ready                  func() bool
	join                   func(...string) string
	// lastLU                 fyne.ListableURI
	slash           = string(filepath.Separator)
	ErrNilURI       = errors.New("uri is nil")
	crocRemovalFile = "croc-marked-files.txt"
	ftw             fyne.Window
	size            = fyne.NewSize(350, 700)

	// Чтоб на десктопе отладить как будто это мобильная ОС
	// cmd/c "set CROC_AS_MOBILE=1&crocgui.exe"
	// CROC_AS_MOBILE=1 crocgui
	asMobile = os.Getenv("CROC_AS_MOBILE") != ""

	// Чтоб отладить план Б при отсутствии com.android.DocumentsUI - сохранять протокол и полученные файлы в Download
	// cmd/c "set CROC_NO_DIALOGS=1&crocgui.exe"
	// CROC_NO_DIALOGS=1 crocgui
	noDialogs = os.Getenv("CROC_NO_DIALOGS") != ""

	// Чтоб не перезапускать приложение при завершении передачи - почитать протокол передачи
	// cmd/c "set CROC_NO_RESTART=1&crocgui.exe"
	// CROC_NO_RESTART=1 crocgui
	noRestart = os.Getenv("CROC_NO_RESTART") != ""

	// Чтоб на десктопе отладить копирование вместо переноса из кэша приёма
	// cmd/c "set CROC_NO_RENAME=1&crocgui.exe"
	// CROC_NO_RENAME=1 crocgui
	noRename = os.Getenv("CROC_NO_RENAME") != ""

	// Чтоб отладить cdLocked
	// cmd/c "set CROC_CD_LOCK=1&crocgui.exe"
	// CROC_CD_LOCK=1 crocgui
	longCdLock = os.Getenv("CROC_CD_LOCK") != ""

	pass    = os.Getenv("CROC_PASS")
	relay4  = os.Getenv("CROC_RELAY")
	relay6  = os.Getenv("CROC_RELAY6")
	ports0  = strings.Join(makePorts(0, 0), ",")
	socks5  = os.Getenv("SOCKS5_PROXY")
	connect = os.Getenv("HTTP_PROXY")
	code    = os.Getenv("CROC_SECRET")
)

const (
	EMULATE                = time.Second * 0
	CROC_SECRET            = "CROC_SECRET"
	TOTP                   = "TOTP-" + CROC_SECRET
	DOTZIP                 = ".zip"
	ZhangHai               = "content://me.zhanghai.android.files.file_provider/"
	Ghisler                = "content://com.ghisler.files/"
	SEND                   = "crocgui-send"
	RECV                   = "crocgui-recv"
	MIME_TYPE_DIR          = "vnd.android.document/directory"
	MIME_TYPE_OCTET_STREAM = "application/octet-stream"
	ID                     = "com.github.howeyc.crocgui"
	LastFolder             = "fyne:fileDialogLastFolder"
	DEFAULT                = "default"
	NONDEFAULT             = "non-default: "
	STDIN                  = "croc-stdin-"
	DEFAULT_RELAY          = "croc.schollz.com"
	DEFAULT_RELAY6         = "croc6.schollz.com"
	DEFAULT_PORT           = 9009
	TRANSFERS              = 4
	DEFAULT_PASSPHRASE     = "pass123"
)

const (
	ZeroWidthSpace     = string(rune(0x200B) + iota) // Пробел нулевой ширины
	ZeroWidthNonJoiner                               // Не-соединитель нулевой ширины
	ZeroWidthJoiner                                  // Соединитель нулевой ширины
)

func main() {
	wd, _ = os.Getwd()
	tempDir = os.TempDir()
	join = func(elem ...string) string {
		return filepath.FromSlash(filepath.Join(append([]string{tempDir, SEND}, elem...)...))
	}
	ErrApplicationShutdown = errors.New("application shutdown")
	done = make(chan struct{})
	replacer = strings.NewReplacer(
		"[trace]\t", "",
		"[debug]\t", "",
		"[info]\t", "",
		"[warn]\t", "",
		"[error]\t", "",
	)
	logOutput = newLogWriter()

	a := app.NewWithID(ID)

	switch runtime.GOOS {
	case "android":
		isAndroid = true
		fallthrough
	case "ios":
		log.SetOutput(&logOutput)
		isMobile = true
	case "linux":
		replacer = strings.NewReplacer(
			"\x1b[0;34;1m[trace]\t\x1b[0m", "",
			"\x1b[0;36m[debug]\t\x1b[0m", "",
			"\x1b[0;37m[info]\t\x1b[0m", "",
			"\x1b[0;33m[warn]\t\x1b[0m", "",
			"\x1b[0;31;1m[error]\t\x1b[0m", "",
		)
		fallthrough
	case "freebsd", "openbsd", "netbsd":
		if os.Getenv("DISPLAY") == "" {
			log.Error("The DISPLAY environment variable is missing")
			return
		}
		fallthrough
	default:
		log.SetOutput(io.MultiWriter(os.Stdout, &logOutput))
	}

	w := a.NewWindow("croc")

	w.SetCloseIntercept(func() {
		log.Trace("CloseIntercept")
		cleanup(w)
	})

	// Defaults
	a.Preferences().SetString("lang",
		a.Preferences().StringWithFallback("lang", "en-US"))
	a.Preferences().SetString("relay-address",
		a.Preferences().StringWithFallback("relay-address", DEFAULT_RELAY))
	// a.Preferences().SetString("relay-password",
	// 	a.Preferences().StringWithFallback("relay-password", DEFAULT_PASSPHRASE))
	a.Preferences().SetString("relay-ports",
		a.Preferences().StringWithFallback("relay-ports", strings.Join(makePorts(0, 8), ",")))
	// a.Preferences().SetBool("disable-local",
	// 	a.Preferences().BoolWithFallback("disable-local", false))
	// a.Preferences().SetBool("force-local",
	// 	a.Preferences().BoolWithFallback("force-local", false))
	// a.Preferences().SetBool("disable-multiplexing",
	// 	a.Preferences().BoolWithFallback("disable-multiplexing", false))
	// a.Preferences().SetBool("disable-compression",
	// 	a.Preferences().BoolWithFallback("disable-compression", false))
	a.Preferences().SetString("theme",
		a.Preferences().StringWithFallback("theme", "system"))
	a.Preferences().SetString("font",
		a.Preferences().StringWithFallback("font", DEFAULT))
	a.Preferences().SetString("debug-level",
		a.Preferences().StringWithFallback("debug-level", "error"))
	a.Preferences().SetString("pake-curve",
		a.Preferences().StringWithFallback("pake-curve", "p256"))
	a.Preferences().SetString("croc-hash",
		a.Preferences().StringWithFallback("croc-hash", "xxhash"))
	// a.Preferences().SetBool("hide-logo",
	// 	a.Preferences().BoolWithFallback("hide-logo", false))
	a.Preferences().SetString("multicast-address",
		a.Preferences().StringWithFallback("multicast-address", "239.255.255.250"))

	// a.Preferences().SetBool("totp-send",
	// 	a.Preferences().BoolWithFallback("totp-send", false))
	// a.Preferences().SetBool("totp-recv",
	// 	a.Preferences().BoolWithFallback("totp-recv", false))
	// a.Preferences().SetBool("zip-unzip",
	// 	a.Preferences().BoolWithFallback("zip-unzip", false))
	if a.Preferences().String(relaysKey) == "" {
		saveRelays(a, getRelays(a))
		setRelayName(a, DEFAULT)
	}
	a.Preferences().SetBool("remember",
		a.Preferences().BoolWithFallback("remember", true))
	a.Preferences().SetBool("send",
		a.Preferences().BoolWithFallback("send", false))
	a.Preferences().SetBool("restore",
		a.Preferences().BoolWithFallback("restore", false))
	a.Preferences().SetString("relay6",
		a.Preferences().StringWithFallback("relay6", DEFAULT_RELAY6))
	a.Preferences().SetBool("overwrite",
		a.Preferences().BoolWithFallback("overwrite", true))

	appTheme.color = theme.DefaultTheme()
	appTheme.size = theme.DefaultTheme()
	appTheme.fontName = DEFAULT
	appTheme.icon = theme.DefaultTheme()

	langCode = a.Preferences().String("lang")
	langPrinter = message.NewPrinter(language.MustParse(langCode))

	setThemeColor(a.Preferences().String("theme"))
	log.SetLevel(a.Preferences().String("debug-level"))

	appTheme.fontName = a.Preferences().String("font")

	a.Settings().SetTheme(appTheme)

	refreshWindow(a, w)
	w.Resize(size)

	w.ShowAndRun()
}

func refreshWindow(a fyne.App, w fyne.Window) {
	saveAccordionState()
	textlogores := fyne.NewStaticResource("text-logo", textlogobytes)
	textlogo := canvas.NewImageFromResource(textlogores)
	textlogo.SetMinSize(fyne.NewSize(205, 100))
	top := container.NewHBox(layout.NewSpacer(), textlogo, layout.NewSpacer())

	at := container.NewAppTabs()
	at.Items = []*container.TabItem{
		sendTabItem(a, w, at),
		recvTabItem(a, w, at),
		logTabItem(a, w),
		settingsTabItem(a, w),
		aboutTabItem(a, w),
	}

	at.SelectIndex(atSI)
	at.OnSelected = func(tab *container.TabItem) {
		atSI = at.SelectedIndex()
		if f, ok := OnSelectedReload[atSI]; ok && f != nil {
			f()
		}
		logOutput.active(tab.Text == ZeroWidthSpace)
	}

	if a.Preferences().Bool("hide-logo") {
		w.SetContent(at)
	} else {
		w.SetContent(container.NewBorder(top, nil, nil, nil, at))
	}
}

func ls(path string) (files []string) {
	if path == "" {
		return
	}

	des, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, de := range des {
		files = append(files, de.Name())
	}
	return
}

func cleanup(w fyne.Window) {
	saveAccordionState()
	close(done)
	if err := os.Chdir(join()); err == nil {
		log.Tracef("RemoveMarkedFiles: %v", utils.RemoveMarkedFiles())
	}
	w.Close()
}

func defs(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
