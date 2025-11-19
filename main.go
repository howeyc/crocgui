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
	recvReady              func() bool
	join                   func(...string) string
	lastLU                 fyne.ListableURI
	slash                  = string(filepath.Separator)
	ErrNilURI              = errors.New("uri is nil")
	crocRemovalFile        = "croc-marked-files.txt"
	ftw                    fyne.Window
	size                   = fyne.NewSize(370, 740)
)

const (
	EMULATE                = time.Second * 0
	CROC_SECRET            = "CROC_SECRET"
	TOTP                   = "TOTP-" + CROC_SECRET
	ZeroWidthSpace         = "\u200B" // Пробел нулевой ширины
	ZeroWidthNonJoiner     = "\u200C" // Не-соединитель нулевой ширины
	ZeroWidthJoiner        = "\u200D" // Соединитель нулевой ширины
	DOTZIP                 = ".zip"
	ZhangHai               = "content://me.zhanghai.android.files.file_provider/"
	Ghisler                = "content://com.ghisler.files/"
	SEND                   = "crocgui-send"
	RECV                   = "crocgui-recv"
	MIME_TYPE_DIR          = "vnd.android.document/directory"
	MIME_TYPE_OCTET_STREAM = "application/octet-stream"

	// Чтоб на десктопе отладить как будто это мобильная ОС.
	asMobile = false
	// Чтоб на десктопе или Андроиде 9- отладить план Б при отсутствии com.android.DocumentsUI на мобильной ОС сохранять протокол и полученные файлы в Загрузки.
	noDialogDebug = false
	// Чтоб не перезапускать приложение при завершении передачи
	noRestart = false
	// Чтоб на десктопе отладить копирование вместо переноса из кэша приёма
	noRename = false
)

func main() {
	wd, _ = os.Getwd()
	tempDir = os.TempDir()
	join = func(elem ...string) string {
		return filepath.Join(append([]string{tempDir, SEND}, elem...)...)
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

	a := app.NewWithID("com.github.howeyc.crocgui")

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
	a.Preferences().SetString("lang", a.Preferences().StringWithFallback("lang", "en-US"))
	a.Preferences().SetString("relay-address", a.Preferences().StringWithFallback("relay-address", "croc.schollz.com:9009"))
	a.Preferences().SetString("relay-password", a.Preferences().StringWithFallback("relay-password", "pass123"))
	a.Preferences().SetString("relay-ports", a.Preferences().StringWithFallback("relay-ports", "9009,9010,9011,9012,9013,9014,9015,9016,9017"))
	a.Preferences().SetBool("disable-local", a.Preferences().BoolWithFallback("disable-local", false))
	a.Preferences().SetBool("force-local", a.Preferences().BoolWithFallback("force-local", false))
	a.Preferences().SetBool("disable-multiplexing", a.Preferences().BoolWithFallback("disable-multiplexing", false))
	a.Preferences().SetBool("disable-compression", a.Preferences().BoolWithFallback("disable-compression", false))
	a.Preferences().SetString("theme", a.Preferences().StringWithFallback("theme", "system"))
	a.Preferences().SetString("font", a.Preferences().StringWithFallback("font", "default"))
	a.Preferences().SetString("debug-level", a.Preferences().StringWithFallback("debug-level", "error"))
	a.Preferences().SetString("pake-curve", a.Preferences().StringWithFallback("pake-curve", "p256"))
	a.Preferences().SetString("croc-hash", a.Preferences().StringWithFallback("croc-hash", "xxhash"))
	a.Preferences().SetBool("hide-logo", a.Preferences().BoolWithFallback("hide-logo", false))
	a.Preferences().SetString("multicast-address", a.Preferences().StringWithFallback("multicast-address", "239.255.255.250"))

	a.Preferences().SetBool("totp-send", a.Preferences().BoolWithFallback("totp-send", false))
	a.Preferences().SetBool("totp-recv", a.Preferences().BoolWithFallback("totp-recv", false))
	a.Preferences().SetBool("zip-unzip", a.Preferences().BoolWithFallback("zip-unzip", true))

	appTheme.color = theme.DefaultTheme()
	appTheme.size = theme.DefaultTheme()
	appTheme.fontName = "default"
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
	close(done)
	if err := os.Chdir(join()); err == nil {
		log.Tracef("RemoveMarkedFiles: %v", utils.RemoveMarkedFiles())
	}
	w.Close()
}
