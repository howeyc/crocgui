//go:generate bash -c "GOFLAGS=-ldflags=-s go install"

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/tcp"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	_ "crocgui/internal/translations"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
)

const (
	EMULATE                = time.Second * 0
	CROC_SECRET            = "CROC_SECRET"
	CROC                   = "croc"
	STDIN                  = CROC + "-stdin-"
	CROCDEBUGLOG           = CROC + "debuglog.txt"
	SCHOLLZ                = "schollz"
	TOTP                   = "TOTP-" + CROC_SECRET
	DOTZIP                 = ".zip"
	DOTTXT                 = ".txt"
	ZhangHai               = "content://me.zhanghai.android.files.file_provider/"
	Ghisler                = "content://com.ghisler.files/"
	CG                     = "crocgui"
	SEND                   = "send"
	RECV                   = "recv"
	MIME_TYPE_DIR          = "vnd.android.document/directory"
	MIME_TYPE_OCTET_STREAM = "application/octet-stream"
	ID                     = "com.github.howeyc.crocgui"
	LastFolder             = "fyne:fileDialogLastFolder"
	DEFAULT                = "default"
	NONDEFAULT             = "non-default: "
	DEFAULT_RELAY          = "croc.schollz.com"
	DEFAULT_RELAY6         = "croc6.schollz.com"
	DEFAULT_PORT           = 9009
	TRANSFERS              = 5
	DEFAULT_PASSPHRASE     = "pass123"
	REFUSING               = "refusing files"

	// cmd/c "set LOGGER=trace&crocgui.exe"
	// LOGGER=trace crocgui
	LEVEL                   = "debug"
	FORKfrom                = "howeyc"
	FORKfromVersion         = "1.11.5"
	FORKfromBuild           = 40
	FORKto                  = "abakum"
	GH                      = "github.com"
	GHP                     = FORKto + ".github.io"
	DAV                     = "dav"
	HTTP                    = "http"
	DAVS                    = "davs"
	HTTPS                   = "https"
	IO                      = HTTPS + "://" + GHP + "/croc#"
	SCAN                    = "//" + GHP + "/scan/"
	qrSize          float32 = 21 * 16
	OFF                     = "..."
	ALL                     = "0.0.0.0"
	LOCAL                   = "127.0.0.1"
)

const (
	SENDi = iota
	RECVi
	LOGi
	SETTINGSi
	ABOUTi
	LENi
)

//go:embed images/featureGraphic.png
var textlogobytes []byte

//go:embed images/icon.png
var iconData []byte // 512x512 PNG иконка

var (
	isMobile               bool
	isAndroid              bool
	ErrApplicationShutdown error
	appCtx                 context.Context
	appCancel              context.CancelFunc
	cdLock                 atomic.Int32
	uriFromIntent          = make(chan string, 100)
	textFromIntent         = make(chan string, 100)
	replacer               *strings.Replacer
	logOutput              logWriter
	atSI                   int
	notFinish              bool
	wd                     string
	OnSelectedTab          = make(map[int]func(), LENi)
	swap                   bool
	tempDir                string
	ready                  func() bool
	join                   func(...string) string
	// lastLU                 fyne.ListableURI
	slash           = string(filepath.Separator)
	ErrNilURI       = errors.New("uri is nil")
	crocRemovalFile = "croc-marked-files.txt"
	size            = fyne.NewSize(350, 700)

	// Чтоб на десктопе отладить как будто это мобильная ОС
	// cmd/c "set CROC_AS_MOBILE=1&crocgui.exe"
	// CROC_AS_MOBILE=1 crocgui
	asMobile = os.Getenv("CROC_AS_MOBILE") != ""

	// Чтоб отладить план Б при отсутствии com.android.DocumentsUI - сохранять протокол и полученные файлы в Download
	// cmd/c "set CROC_NO_DIALOGS=1&crocgui.exe"
	// CROC_NO_DIALOGS=1 crocgui
	noDialogs = os.Getenv("CROC_NO_DIALOGS") != ""

	// Чтоб перезапускать приложение при завершении передачи
	// cmd/c "set CROC_RESTART=1&crocgui.exe"
	// CROC_RESTART=1 crocgui
	noRestart = os.Getenv("CROC_RESTART") == ""

	// Чтоб на десктопе отладить копирование вместо переноса из кэша приёма
	// cmd/c "set CROC_NO_RENAME=1&crocgui.exe"
	// CROC_NO_RENAME=1 crocgui
	noRename = os.Getenv("CROC_NO_RENAME") != ""

	// Чтоб отладить cdLocked
	// cmd/c "set CROC_CD_LOCK=1&crocgui.exe"
	// CROC_CD_LOCK=1 crocgui
	longCdLock = os.Getenv("CROC_CD_LOCK") != ""

	// Чтоб отладить GUI
	// cmd/c "set CROC_DEBUG=1&crocgui.exe"
	// CROC_DEBUG=1 crocgui
	crocDebug = os.Getenv("CROC_DEBUG") != ""

	pass        = os.Getenv("CROC_PASS")
	relay4      = os.Getenv("CROC_RELAY")
	relay6      = os.Getenv("CROC_RELAY6")
	ports0      = strings.Join(makePorts(0, 0), ",")
	socks5      = os.Getenv("SOCKS5_PROXY")
	connect     = os.Getenv("HTTP_PROXY")
	code        = os.Getenv("CROC_SECRET")
	AppClosed   string
	GUI         = syscall.Stdout == 0 && syscall.Stderr == 0 //for windowsgui
	NON_BROWSER = flagActivity(
		FLAG_ACTIVITY_NEW_TASK,
		// FLAG_ACTIVITY_NO_HISTORY,
		FLAG_ACTIVITY_SINGLE_TOP,
		FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS,
		FLAG_ACTIVITY_REQUIRE_NON_BROWSER,
		FLAG_ACTIVITY_CLEAR_TOP,
		FLAG_ACTIVITY_CLEAR_TASK,
	)
	BROWSER = flagActivity(
		FLAG_ACTIVITY_NEW_TASK,
		// FLAG_ACTIVITY_NO_HISTORY,
		FLAG_ACTIVITY_SINGLE_TOP,
		// FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS,
		// FLAG_ACTIVITY_CLEAR_TOP,
		// FLAG_ACTIVITY_CLEAR_TASK,
	)
	at           *container.AppTabs
	davServer    = NewWebDAVServer()
	sleepCounter int32
	cleanups     = []func(){}
)

func main() {
	a := app.NewWithID(ID)

	wd, _ = os.Getwd()
	//tempDir = os.TempDir()
	tempDir = a.Storage().RootURI().Path()
	var (
		crocdebuglog *os.File
		err          error
	)
	if crocDebug {
		crocdebuglog, err = os.Create(filepath.Join(tempDir, CROCDEBUGLOG))
		if err == nil {
			defer func() {
				if r := recover(); r != nil {
					crocdebuglog.WriteString("PANIC: " + fmt.Sprintf("%v\n", r))
					crocdebuglog.WriteString("Stack: " + string(debug.Stack()))
					crocdebuglog.Close()
					os.Exit(1)
					return
				}
				crocdebuglog.Close()
			}()
		}
	}

	join = func(elem ...string) string {
		return filepath.FromSlash(filepath.Join(append([]string{tempDir, SEND}, elem...)...))
	}
	ErrApplicationShutdown = errors.New("application shutdown")
	appCtx, appCancel = context.WithCancel(context.Background())
	de = NewDoMonitor()
	replacer = strings.NewReplacer(
		"[trace]\t", "",
		"[debug]\t", "",
		"[info]\t", "",
		"[warn]\t", "",
		"[error]\t", "",
	)
	logOutput = newLogWriter()

	setOut := func(gui bool) {
		if crocdebuglog == nil {
			if gui {
				log.SetOutput(&logOutput)
				return
			}
			log.SetOutput(io.MultiWriter(os.Stdout, &logOutput))
		} else {
			log.SetOutput(io.MultiWriter(crocdebuglog, &logOutput))
		}
	}

	switch runtime.GOOS {
	case "android":
		isAndroid = true
		fallthrough
	case "ios":
		isMobile = true
		GUI = true
		setOut(GUI)
	case "linux":
		replacer = strings.NewReplacer(
			"\x1b[0;34;1m[trace]\t\x1b[0m", "",
			"\x1b[0;36m[debug]\t\x1b[0m", "",
			"\x1b[0;37m[info]\t\x1b[0m", "",
			"\x1b[0;33m[warn]\t\x1b[0m", "",
			"\x1b[0;31;1m[error]\t\x1b[0m", "",
		)
		if os.Getenv("DISPLAY") == "" {
			log.Error("The DISPLAY environment variable is missing")
			return
		}
		fallthrough
	default:
		setOut(GUI)
	}

	log.Info(tempDir)
	w := a.NewWindow(CROC)

	w.SetCloseIntercept(func() {
		log.Debug("CloseIntercept")
		cleanup(w)
	})

	w.SetOnClosed(func() {
		log.Debug("Сlosed")
	})

	langCode = lang.SystemLocale().String()
	log.Info(langCode)
	// Defaults
	a.Preferences().SetString("lang",
		a.Preferences().StringWithFallback("lang", langCode))
	a.Preferences().SetString("relay-address",
		a.Preferences().StringWithFallback("relay-address", DEFAULT_RELAY))
	// a.Preferences().SetString("relay-password",
	// 	a.Preferences().StringWithFallback("relay-password", DEFAULT_PASSPHRASE))
	// a.Preferences().SetString("relay-ports",
	// 	a.Preferences().StringWithFallback("relay-ports", strings.Join(makePorts(0, 8), ",")))
	// a.Preferences().SetBool("disable-local",
	// 	a.Preferences().BoolWithFallback("disable-local", true))
	a.Preferences().SetBool("testing",
		a.Preferences().BoolWithFallback("testing", true))
	// a.Preferences().SetBool("force-local",
	// 	a.Preferences().BoolWithFallback("force-local", false))
	// a.Preferences().SetBool("disable-multiplexing",
	// 	a.Preferences().BoolWithFallback("disable-multiplexing", false))
	// a.Preferences().SetBool("disable-compression",
	// 	a.Preferences().BoolWithFallback("disable-compression", false))
	a.Preferences().SetString("theme",
		a.Preferences().StringWithFallback("theme", "light"))
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

	// appTheme.color = theme.DefaultTheme()
	appTheme.size = theme.DefaultTheme()
	appTheme.fontName = DEFAULT
	// appTheme.icon = theme.DefaultTheme()

	langCode = a.Preferences().String("lang")
	langPrinter = message.NewPrinter(language.MustParse(langCode))

	setThemeColor(a.Preferences().String("theme"))
	log.SetLevel(a.Preferences().String("debug-level"))

	appTheme.fontName = a.Preferences().String("font")

	a.Settings().SetTheme(appTheme)
	atSI = a.Preferences().Int("tab")
	if atSI < 0 || atSI > 4 {
		atSI = 0
		a.Preferences().SetInt("tab", 0)
	}
	a.Preferences().SetBool("hide-logo",
		a.Preferences().BoolWithFallback("hide-logo", true))
	refreshWindow(a, w)
	w.Resize(size)
	AppClosed = lp("App closed. Tap to start.")

	w.ShowAndRun()
}

func refreshWindow(a fyne.App, w fyne.Window) {
	saveAccordionState()
	textlogores := fyne.NewStaticResource("text-logo", textlogobytes)
	textlogo := canvas.NewImageFromResource(textlogores)
	textlogo.SetMinSize(fyne.NewSize(205, 100))
	top := container.NewHBox(layout.NewSpacer(), textlogo, layout.NewSpacer())

	at = container.NewAppTabs(
		sendTabItem(a, w),
		recvTabItem(a, w),
		logTabItem(a, w),
		settingsTabItem(a, w),
		aboutTabItem(a, w),
	)

	at.OnSelected = func(tab *container.TabItem) {
		atSI = at.SelectedIndex()
		a.Preferences().SetInt("tab", atSI)
		logOutput.active(false)
		if f, ok := OnSelectedTab[atSI]; ok && f != nil {
			f()
		}
	}

	if a.Preferences().Bool("hide-logo") {
		w.SetContent(at)
	} else {
		w.SetContent(container.NewBorder(top, nil, nil, nil, at))
	}
	at.SelectIndex(atSI)
}

func ls(path string) (files []string) {
	if path == "" {
		return
	}

	des, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for _, d := range des {
		files = append(files, d.Name())
	}
	return
}

func cleanup(w fyne.Window) {
	saveAccordionState()
	appCancel()
	if err := os.Chdir(join()); err == nil {
		utils.RemoveMarkedFiles()
	}
	davServer.Stop()
	if !SleepAllowed() {
		caffeinate(0)
	}
	for _, f := range cleanups {
		if f != nil {
			f()
		}
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

func crocNew(NoRestart bool, ctx context.Context, opt croc.Options) (client *croc.Client, err error) {
	if !NoRestart {
		return croc.New(opt)
	}
	return croc.NewCtx(ctx, opt)
	// return croc.New(opt)
}

func tcpRun(NoRestart bool, ctx context.Context, debugLevel, host, port, password string, banner ...string) (err error) {
	if !NoRestart {
		return tcp.Run(debugLevel, host, port, password, banner...)
	}
	return tcp.RunCtx(ctx, debugLevel, host, port, password, banner...)
	// return tcp.Run(debugLevel, host, port, password, banner...)
}
