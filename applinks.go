package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/tcp"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
	"github.com/skip2/go-qrcode"
)

// Флаги активности (Activity Flags)
const (
	// Если установлен, новая activity не сохраняется в стеке истории.
	// Как только пользователь уходит от неё, activity завершается.
	FLAG_ACTIVITY_NO_HISTORY = 0x40000000

	// Если установлен, activity не будет запущена, если она уже работает
	// на вершине стека истории.
	FLAG_ACTIVITY_SINGLE_TOP = 0x20000000

	// Если установлен, эта activity станет началом новой задачи
	// в этом стеке истории.
	FLAG_ACTIVITY_NEW_TASK = 0x10000000

	// Используется для создания новой задачи и запуска activity в нее.
	// Всегда используется вместе с FLAG_ACTIVITY_NEW_DOCUMENT или FLAG_ACTIVITY_NEW_TASK.
	FLAG_ACTIVITY_MULTIPLE_TASK = 0x08000000

	// Если установлен, и запускаемая activity уже работает в текущей задаче,
	// то вместо запуска нового экземпляра все другие activity поверх неё
	// будут закрыты.
	FLAG_ACTIVITY_CLEAR_TOP = 0x04000000

	// Если установлен и этот intent используется для запуска новой activity
	// из существующей, то цель ответа существующей activity будет
	// передана новой activity.
	FLAG_ACTIVITY_FORWARD_RESULT = 0x02000000

	// Если установлен и этот intent используется для запуска новой activity
	// из существующей, текущая activity не будет считаться верхней
	// для решения о доставке нового intent.
	FLAG_ACTIVITY_PREVIOUS_IS_TOP = 0x01000000

	// Если установлен, новая activity не сохраняется в списке недавно
	// запущенных activities.
	FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS = 0x00800000

	// Этот флаг обычно не устанавливается кодом приложения, а устанавливается
	// системой, как описано в документации launchMode для singleTask.
	FLAG_ACTIVITY_BROUGHT_TO_FRONT = 0x00400000

	// Если установлен, и эта activity либо запускается в новой задаче,
	// либо выводится наверх существующей задачи, то она будет запущена
	// как главная дверь задачи.
	FLAG_ACTIVITY_RESET_TASK_IF_NEEDED = 0x00200000

	// Этот флаг устанавливается системой, если эта activity запускается из истории.
	FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY = 0x00100000

	// Устаревший флаг, начиная с API 21 работает идентично
	// FLAG_ACTIVITY_NEW_DOCUMENT.
	FLAG_ACTIVITY_CLEAR_WHEN_TASK_RESET = 0x00080000

	// Используется для открытия документа в новой задаче, корнем которой
	// является activity, запущенная этим Intent.
	FLAG_ACTIVITY_NEW_DOCUMENT = FLAG_ACTIVITY_CLEAR_WHEN_TASK_RESET

	// Если установлен, этот флаг предотвратит нормальный вызов
	// onUserLeaveHint у текущей передней activity перед её приостановкой.
	FLAG_ACTIVITY_NO_USER_ACTION = 0x00040000

	// Если установлен в Intent, переданном в startActivity(),
	// этот флаг вызовет перемещение запускаемой activity на вершину
	// стека истории её задачи, если она уже работает.
	FLAG_ACTIVITY_REORDER_TO_FRONT = 0x00020000

	// Если установлен в Intent, переданном в startActivity(),
	// этот флаг предотвратит применение системой анимации перехода
	// activity к следующему состоянию activity.
	FLAG_ACTIVITY_NO_ANIMATION = 0x00010000

	// Если установлен в Intent, переданном в startActivity(),
	// этот флаг вызовет очистку любой существующей задачи, связанной
	// с activity, перед запуском activity.
	FLAG_ACTIVITY_CLEAR_TASK = 0x00008000

	// Если установлен в Intent, переданном в startActivity(),
	// этот флаг поместит новую задачу поверх текущей домашней задачи.
	FLAG_ACTIVITY_TASK_ON_HOME = 0x00004000

	// Позволяет документу, созданному FLAG_ACTIVITY_NEW_DOCUMENT,
	// оставаться в списке недавних задач после закрытия пользователем.
	FLAG_ACTIVITY_RETAIN_IN_RECENTS = 0x00002000

	// Этот флаг используется только для разделенного многозадачного режима.
	// Новая activity будет отображаться рядом с запускающей её activity.
	FLAG_ACTIVITY_LAUNCH_ADJACENT = 0x00001000

	// Если установлен в Intent, переданном в startActivity(),
	// этот флаг попытается запустить мгновенное приложение, если на устройстве
	// нет полного приложения, которое уже может обработать intent.
	FLAG_ACTIVITY_MATCH_EXTERNAL = 0x00000800

	// Если установлен в intent, переданном в startActivity(),
	// этот флаг запустит intent только если он разрешается в результат,
	// который не является браузером.
	FLAG_ACTIVITY_REQUIRE_NON_BROWSER = 0x00000400

	// Если установлен в intent, переданном в startActivity(),
	// этот флаг запустит intent только если он разрешается в единственный результат.
	FLAG_ACTIVITY_REQUIRE_DEFAULT = 0x00000200
)

// Другие флаги Intent
const (
	// Если установлен, получатель этого Intent получит разрешение на
	// выполнение операций чтения URI в данных Intent.
	FLAG_GRANT_READ_URI_PERMISSION = 0x00000001

	// Если установлен, получатель этого Intent получит разрешение на
	// выполнение операций записи URI в данных Intent.
	FLAG_GRANT_WRITE_URI_PERMISSION = 0x00000002

	// Может быть установлен вызывающим для указания, что этот Intent исходит
	// из фоновой операции, а не от прямого взаимодействия с пользователем.
	FLAG_FROM_BACKGROUND = 0x00000004

	// Флаг для отладки: при установке, во время разрешения этого intent
	// будут выводиться сообщения журнала.
	FLAG_DEBUG_LOG_RESOLUTION = 0x00000008

	// Если установлен, этот intent не будет соответствовать компонентам в пакетах,
	// которые в данный момент остановлены.
	FLAG_EXCLUDE_STOPPED_PACKAGES = 0x00000010

	// Если установлен, этот intent всегда будет соответствовать компонентам в пакетах,
	// которые в данный момент остановлены.
	FLAG_INCLUDE_STOPPED_PACKAGES = 0x00000020

	// В сочетании с FLAG_GRANT_READ_URI_PERMISSION и/или
	// FLAG_GRANT_WRITE_URI_PERMISSION, предоставление прав доступа к URI может быть
	// сохранено после перезагрузки устройства.
	FLAG_GRANT_PERSISTABLE_URI_PERMISSION = 0x00000040

	// В сочетании с FLAG_GRANT_READ_URI_PERMISSION и/или
	// FLAG_GRANT_WRITE_URI_PERMISSION, предоставление прав доступа к URI
	// применяется к любому URI, который является префиксным совпадением.
	FLAG_GRANT_PREFIX_URI_PERMISSION = 0x00000080

	// Флаг для автоматического сопоставления intents на основе их осведомленности
	// о Direct Boot и текущего состояния пользователя.
	FLAG_DIRECT_BOOT_AUTO = 0x00000100

	// Устаревший алиас для FLAG_DIRECT_BOOT_AUTO
	FLAG_DEBUG_TRIAGED_MISSING = FLAG_DIRECT_BOOT_AUTO

	// Внутренний флаг, указывающий, что эфемерные приложения не должны
	// учитываться при разрешении intent.
	FLAG_IGNORE_EPHEMERAL = 0x80000000

	APPLICATION_DETAILS_SETTINGS = "android.settings.APPLICATION_DETAILS_SETTINGS"
	APP_OPEN_BY_DEFAULT_SETTINGS = "android.settings.APP_OPEN_BY_DEFAULT_SETTINGS"
)

func flagActivity(flags ...int) string {
	var result int = 0

	for _, flag := range flags {
		result |= flag
	}

	return fmt.Sprintf("0x%08X", result)
}

func fromURI(u string) (st, ne, as, a6, ps, pd, s5, ct string, err error) {
	if len(u) <= len(IO) || !strings.HasPrefix(u, IO) {
		err = fmt.Errorf("not IO")
		return
	}

	fragment := strings.TrimPrefix(u, IO)
	log.Debug(fragment)
	decoded, err := base64.RawURLEncoding.DecodeString(fragment)
	log.Debug(string(decoded))
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(fragment)
		log.Error(string(decoded))
		if err != nil {
			return
		}
	}

	str := strings.TrimRight(string(decoded), "\n")
	//log.Debug(str)
	ss := strings.Split(str, "\n")
	for i, s := range ss {
		switch i {
		case 0:
			st = s
		case 1:
			ne = s
		case 2:
			as = s
		case 3:
			a6 = s
		case 4:
			ps = s
		case 5:
			pd = s
		case 6:
			s5 = s
		case 7:
			ct = s
		default:
			return
		}
	}
	log.Debugf("%v", ss)

	return
}

func isDAV(cc string) (schemes []string, ccNew string, u *url.URL, ok bool) {
	var err error
	u, err = url.Parse(cc)
	if err != nil {
		return
	}
	switch u.Scheme {
	case DAV, "webdav":
		u.Scheme = HTTP
		schemes = []string{HTTP, DAV, "webdav"}
		ccNew = u.String()
		ok = true
		return
	case DAVS, "webdavs":
		u.Scheme = HTTPS
		schemes = []string{HTTPS, DAVS, "webdavs"}
		ccNew = u.String()
		ok = true
		return
	}
	return
}

// st, ne, as, a6, ps, pd, s5, ct
func toURI(ss ...string) (u string) {
	for i, s := range ss {
		if i > 7 {
			break
		}
		s = strings.ReplaceAll(s, "\n", "")
		u += strings.TrimSpace(s) + "\n"
	}
	u = strings.TrimRight(u, "\n")
	log.Debug(IO + u)
	return IO + base64.RawURLEncoding.EncodeToString([]byte(u))
}

// Show открывает секцию QR в аккордеоне
func (qr *QR) Show() {
	if at != nil && len(at.Items) > 3 {
		at.SelectIndex(3)
	}
	if accordion != nil && qr.accordionItem != nil {
		accordion.CloseAll()
		for i, item := range accordion.Items {
			if item == qr.accordionItem {
				accordion.Open(i)
				break
			}
		}
	}
}

// generateQR создает QR-код из текста
func (qr *QR) generateQR() {
	if qr.currentText == "" {
		qr.currentText = fmt.Sprintf("%s://%s/%s/%s/releases", HTTPS, GH, FORKto, CG)
	}

	// Генерируем QR-код
	pngData, err := qrcode.Encode(qr.currentText, qrcode.High, int(qrSize))
	if err != nil {
		log.Error(err)
		qr.qrImage.Resource = theme.BrokenImageIcon()
	} else {
		qrResource := fyne.NewStaticResource("qr.png", pngData)
		qr.qrImage.Resource = qrResource
	}
	qr.qrImage.Refresh()
}

// updateLink обновляет гиперссылку
func (qr *QR) updateLink() {
	displayText := qr.currentText
	if len(displayText) > 80 {
		displayText = displayText[:80] + "…"
	}

	qr.link.SetText(displayText)

	// Устанавливаем обработчик в зависимости от типа
	if qr.isIO {
		qr.link.OnTapped = qr.handleIOTapped
		qr.link.SetURLFromString(qr.currentText)
	} else if u, err := url.Parse(qr.currentText); err == nil && u.Scheme != "" && u.Host != "" {
		qr.link.SetURL(u)
		qr.link.OnTapped = func() {
			err := OpenDAV(qr.currentText)
			log.Debugf("OpenDAV %s: %v", qr.currentText, err)
		}
	} else {
		qr.link.SetURL(nil)
		qr.link.OnTapped = qr.showTextDialog
	}
}

// makeScannerSettings создает настройки сканера для Android
func (qr *QR) makeScannerSettings() fyne.CanvasObject {
	if !(isAndroid || asMobile) {
		return nil // Возвращаем nil, если не Android
	}

	// Нажмите Deep Link выше для пробы.
	labelT := widget.NewLabel(lp("Click the Deep Link above to test"))
	labelT.Wrapping = fyne.TextWrapWord
	labelT.Alignment = fyne.TextAlignCenter

	// Если откроется браузер:
	labelL := widget.NewLabel(lp("If a browser opens then:"))
	labelL.Wrapping = fyne.TextWrapWord
	labelL.Alignment = fyne.TextAlignTrailing

	// Разрешить
	setupButton := widget.NewButtonWithIcon(lp("Allow")+"\n"+GHP, theme.SettingsIcon(), func() {
		idActions(ID, APP_OPEN_BY_DEFAULT_SETTINGS, APPLICATION_DETAILS_SETTINGS)
	})

	return container.NewVBox(
		container.NewBorder(
			labelT,
			nil,
			nil,
			setupButton,
			labelL,
		),
		widget.NewSeparator(),
	)
}

// makeQRInstructions создает инструкции по использованию QR
func (qr *QR) makeQRInstructions() fyne.CanvasObject {

	labelB := container.NewHBox(
		layout.NewSpacer(),
		widget.NewLabel(lp("Use")),
		widget.NewIcon(theme.ViewFullScreenIcon()),
		widget.NewLabel(lp("on page")),
		widget.NewIcon(theme.DownloadIcon()),
		widget.NewLabel(lp("to")),
	)

	labelB.Add(layout.NewSpacer())

	vBox := container.NewVBox(labelB)
	if isAndroid || asMobile {
		optScan := []string{
			"Default html5-QRcode",
			"miUI Xiaomi",
			"Samsung",
			"OPlus",

			"BinaryEye",
			"Lens Google",
			"ZXing",

			"Chrome Google",
			"Via",
			"sBrowser Samsung",

			"Opera mini",
			"Microsoft",
			"Firefox",
		}
		scanSelect := widget.NewSelect(optScan, func(sel string) {
			qr.app.Preferences().SetString("scanner", sel)
		})

		selScan := qr.app.Preferences().String("scanner")
		if !slices.Contains(optScan, selScan) {
			selScan = optScan[0]
			qr.app.Preferences().SetString("scanner", selScan)
		}
		scanSelect.SetSelected(selScan)

		qrButton := widget.NewButton(lp("Scan QRs")+" "+lp("with:"), func() {
			qr.scanner()
		})

		vBox.Add(container.NewBorder(
			nil,
			nil,
			qrButton,
			nil,
			scanSelect,
		))
	} else {
		qrButton := widget.NewButton(lp("Scan QRs"), func() {
			qr.scanner()
		})
		vBox.Add(qrButton)
	}
	return vBox
}

func (qr *QR) scanner() {
	if !(isAndroid || asMobile) {
		s := HTTPS + ":" + SCAN
		err := OpenURL(s)
		log.Debugf("%s: %v", s, err)
		return
	}
	// view 0x23000003
	// send 0x1b080001
	intents := []*Intent{
		// html5-qrcode
		&Intent{Categories: []string{CATEGORY_DEFAULT, CATEGORY_BROWSABLE}},
		// XIAOMI / REDMI / POCO
		&Intent{Action: "miui.intent.action.scanner"},
		// SAMSUNG
		&Intent{Action: "com.samsung.android.app.opticalreader.SCAN"},
		// OPPO / REALME / ONEPLUS (ColorOS/Oplus)
		&Intent{Component: "com.oplus.scanner/.ScanActivity"},
		// BINARY EYE
		&Intent{Data: "//scan/", Scheme: "binaryeye"},
		&Intent{Data: "//details?id=de.markusfisch.android.binaryeye", Scheme: "market"},
		// Google Lens
		&Intent{Component: "com.google.ar.lens/com.google.vr.apps.ornament.app.lens.LensLauncherActivity"},
		&Intent{Data: "//details?id=com.google.ar.lens", Scheme: "market"},
		// ZXING
		&Intent{Action: "com.google.zxing.client.android.SCAN"},
		&Intent{Package: "com.android.chrome", Categories: []string{CATEGORY_BROWSABLE}},
		&Intent{Package: "mark.via.gp", Categories: []string{CATEGORY_BROWSABLE}},
		&Intent{Package: "com.sec.android.app.sbrowser", Categories: []string{CATEGORY_BROWSABLE}},
		&Intent{Package: "com.opera.mini.native", Categories: []string{CATEGORY_BROWSABLE}},
		&Intent{Package: "com.microsoft.emmx", Categories: []string{CATEGORY_BROWSABLE}},
		&Intent{Package: "org.mozilla.firefox", Categories: []string{CATEGORY_BROWSABLE}},
	}

	notFinish = false
	contains := false
	sel := qr.app.Preferences().String("scanner")
	key := strings.ToLower(strings.Fields(sel)[0])

	scannerIsBrowser = false
	clipboardBeforeScan = qr.app.Clipboard().Content()

	for _, i := range intents {
		isB := slices.Contains(i.Categories, CATEGORY_BROWSABLE)
		i.Flags = NON_BROWSER
		if isB {
			i.Flags = BROWSER
			i.Scheme = HTTPS
			i.Data = SCAN
		}
		s := i.String()
		// Пропускаем до выбранного
		// остальные это фолбэки
		switch {
		case contains:
		case strings.Contains(strings.ToLower(s), key):
			contains = true
		default:
			continue
		}

		log.Debugf("%s", s)
		if err := OpenURL(s); err == nil {
			if isB {
				scannerIsBrowser = true
			}
			log.Debugf("find^^^")
			return
		}
	}

	fyne.Do(func() {
		qr.Show()
	})
}

func setClipboard(a fyne.App) (text string) {
	code := a.Preferences().String("secret")
	name := a.Preferences().String("new-relay")
	if name == "" {
		name = relayName(a)
	}
	relayAddress := a.Preferences().String("relay-address")
	if host := a.Preferences().String("host"); host != "" && host != "..." {
		// Если включен локальный посредник
		log.Debugf("host %s", host)
		relayAddress = host
		if publicIP, err := utils.PublicIP(); err == nil {
			log.Debugf("public IP %s", publicIP)
			ports := a.Preferences().String("relay-ports")
			if ports == "" {
				ports = strconv.Itoa(DEFAULT_PORT)
			} else {
				ports = strings.Split(ports, ",")[0]
			}
			address := net.JoinHostPort(publicIP, ports)
			if err := tcp.PingServer(address); err == nil {
				// Если виден снаружи
				relayAddress = publicIP
				log.Infof("croc IP %s", relayAddress)
			} else {
				// Оставляем локальный IP
				log.Debugf("could not ping: %+v", err)
			}
		}
	}
	text = toURI(
		code,
		name,
		relayAddress,
		a.Preferences().String("relay6"),
		a.Preferences().String("relay-ports"),
		// a.Preferences().String("relay-password"),
		// a.Preferences().String("socks5"),
		// a.Preferences().String("connect"),
	)
	a.Clipboard().SetContent(text)
	return
}

// Intent represents an abstract description of an operation to be performed.
// All fields are defined as strings to simplify the Go implementation.
type Intent struct {
	// Action - The general action to be performed (e.g., ACTION_VIEW, ACTION_EDIT)
	Action string

	// Data - The data to operate on, expressed as a URI string
	Data string

	// Type - Explicit MIME type of the intent data
	Type string

	// Package - Target package name for the intent
	Package string

	// Component - Explicit component name (e.g., "com.example/.MainActivity")
	Component string

	// Categories - Set of categories for the intent
	Categories []string

	// Flags - Launch flags for the intent (stored as string but represents integer flags)
	Flags string

	// Scheme - URI scheme part
	Scheme string

	// SourceBounds - Rectangle bounds as string (e.g., "0,0,100,100")
	SourceBounds string

	// Identifier - Intent identifier
	Identifier string

	// Extras - Additional data as key-value pairs
	Extras map[string]ExtraValue

	// Selector - Optional selector intent
	Selector *Intent
}

// ExtraValue represents a typed extra value in the intent
type ExtraValue struct {
	Type  string // Type: "S", "B", "i", "l", "f", "d", "b", "c", "s"
	Value string // String representation of the value
}

// Common intent actions (from Android Intent class)
const (
	ACTION_VIEW   = "android.intent.action.VIEW"
	ACTION_EDIT   = "android.intent.action.EDIT"
	ACTION_MAIN   = "android.intent.action.MAIN"
	ACTION_DIAL   = "android.intent.action.DIAL"
	ACTION_CALL   = "android.intent.action.CALL"
	ACTION_SEND   = "android.intent.action.SEND"
	ACTION_SEARCH = "android.intent.action.SEARCH"
)

// Common intent categories
const (
	CATEGORY_DEFAULT     = "android.intent.category.DEFAULT"
	CATEGORY_BROWSABLE   = "android.intent.category.BROWSABLE"
	CATEGORY_LAUNCHER    = "android.intent.category.LAUNCHER"
	CATEGORY_HOME        = "android.intent.category.HOME"
	CATEGORY_ALTERNATIVE = "android.intent.category.ALTERNATIVE"
)

// NewIntent creates a new Intent with default VIEW action
func NewIntent() *Intent {
	return &Intent{
		Action:     ACTION_VIEW,
		Extras:     make(map[string]ExtraValue),
		Categories: []string{},
	}
}

// SetAction sets the action for the intent
func (i *Intent) SetAction(action string) *Intent {
	i.Action = action
	return i
}

// SetData sets the data URI for the intent
func (i *Intent) SetData(data string) *Intent {
	i.Data = data
	return i
}

// SetType sets the MIME type for the intent
func (i *Intent) SetType(typeStr string) *Intent {
	i.Type = typeStr
	return i
}

// SetPackage sets the target package name
func (i *Intent) SetPackage(pkg string) *Intent {
	i.Package = pkg
	return i
}

// SetComponent sets the component name
func (i *Intent) SetComponent(component string) *Intent {
	i.Component = component
	return i
}

// AddCategory adds a category to the intent
func (i *Intent) AddCategory(category string) *Intent {
	i.Categories = append(i.Categories, category)
	return i
}

// SetFlags sets the launch flags (as string representation of integer)
func (i *Intent) SetFlags(flags string) *Intent {
	i.Flags = flags
	return i
}

// SetScheme sets the URI scheme
func (i *Intent) SetScheme(scheme string) *Intent {
	i.Scheme = scheme
	return i
}

// PutExtra adds an extra value to the intent
func (i *Intent) PutExtra(key string, value ExtraValue) *Intent {
	if i.Extras == nil {
		i.Extras = make(map[string]ExtraValue)
	}
	i.Extras[key] = value
	return i
}

// PutStringExtra adds a string extra
func (i *Intent) PutStringExtra(key, value string) *Intent {
	return i.PutExtra(key, ExtraValue{Type: "S", Value: value})
}

// PutIntExtra adds an integer extra
func (i *Intent) PutIntExtra(key string, value int) *Intent {
	return i.PutExtra(key, ExtraValue{Type: "i", Value: fmt.Sprintf("%d", value)})
}

// PutBoolExtra adds a boolean extra
func (i *Intent) PutBoolExtra(key string, value bool) *Intent {
	return i.PutExtra(key, ExtraValue{Type: "B", Value: fmt.Sprintf("%t", value)})
}

// String returns the intent as a string in the format: "intent:#Intent;package=my;end"
// This mimics the Android Intent.toUri(URI_INTENT_SCHEME) format
func (i *Intent) String() string {
	var parts []string

	// Start with intent scheme
	result := "intent:"

	// Add data part if present
	if i.Data != "" {
		result += i.Data
	}

	// Add Intent parameters section
	result += "#Intent;"

	// Add action
	if i.Action != "" && i.Action != ACTION_VIEW {
		parts = append(parts, fmt.Sprintf("action=%s", i.Action))
	}

	// Add categories
	for _, category := range i.Categories {
		parts = append(parts, fmt.Sprintf("category=%s", category))
	}

	// Add type
	if i.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%s", i.Type))
	}

	// Add identifier
	if i.Identifier != "" {
		parts = append(parts, fmt.Sprintf("identifier=%s", i.Identifier))
	}

	// Add launch flags
	if i.Flags != "" {
		parts = append(parts, fmt.Sprintf("launchFlags=%s", i.Flags))
	}

	// Add package
	if i.Package != "" {
		parts = append(parts, fmt.Sprintf("package=%s", i.Package))
	}

	// Add component
	if i.Component != "" {
		parts = append(parts, fmt.Sprintf("component=%s", i.Component))
	}

	// Add scheme
	if i.Scheme != "" {
		parts = append(parts, fmt.Sprintf("scheme=%s", i.Scheme))
	}

	// Add source bounds
	if i.SourceBounds != "" {
		parts = append(parts, fmt.Sprintf("sourceBounds=%s", i.SourceBounds))
	}

	// Add extras
	if i.Extras != nil {
		for key, extra := range i.Extras {
			// Determine the type prefix based on the ExtraValue.Type
			typePrefix := ""
			switch extra.Type {
			case "S":
				typePrefix = "S."
			case "B":
				typePrefix = "B."
			case "b":
				typePrefix = "b."
			case "c":
				typePrefix = "c."
			case "d":
				typePrefix = "d."
			case "f":
				typePrefix = "f."
			case "i":
				typePrefix = "i."
			case "l":
				typePrefix = "l."
			case "s":
				typePrefix = "s."
			default:
				typePrefix = "S." // Default to string
			}
			parts = append(parts, fmt.Sprintf("%s%s=%s", typePrefix, key, extra.Value))
		}
	}

	// Add selector if present (simplified - in real implementation would be more complex)
	if i.Selector != nil {
		parts = append(parts, "SEL")
		// Note: Selector would have its own parameters in a full implementation
	}

	// Combine all parts
	if len(parts) > 0 {
		result += strings.Join(parts, ";") + ";"
	}

	// End marker
	result += "end"

	return result
}

// ParseUri parses a URI string into an Intent (simplified version)
// This is a basic implementation based on the Java parseUriInternal method
func ParseUri(uri string) (*Intent, error) {
	intent := NewIntent()

	// Check for android-app scheme
	if strings.HasPrefix(uri, "android-app:") {
		// Simplified handling for android-app scheme
		intent.SetPackage(strings.TrimPrefix(uri, "android-app://"))
		intent.SetAction(ACTION_MAIN)
		return intent, nil
	}

	// Check for intent scheme
	if strings.HasPrefix(uri, "intent:") {
		// Find the #Intent; part
		hashIndex := strings.LastIndex(uri, "#")
		if hashIndex == -1 {
			// Simple intent URI without parameters
			dataPart := strings.TrimPrefix(uri, "intent:")
			if dataPart != "" {
				intent.SetData(dataPart)
			}
			return intent, nil
		}

		// Parse intent with parameters
		dataPart := uri[:hashIndex]
		if dataPart != "" && dataPart != "intent:" {
			intent.SetData(strings.TrimPrefix(dataPart, "intent:"))
		}

		// Parse parameters (simplified)
		paramPart := uri[hashIndex:]
		if strings.HasPrefix(paramPart, "#Intent;") {
			// Remove #Intent; and ;end
			paramPart = strings.TrimPrefix(paramPart, "#Intent;")
			paramPart = strings.TrimSuffix(paramPart, ";end")

			// Split parameters
			params := strings.Split(paramPart, ";")
			for _, param := range params {
				if param == "" {
					continue
				}

				// Parse key-value pair
				parts := strings.SplitN(param, "=", 2)
				if len(parts) != 2 {
					continue
				}

				key := parts[0]
				value := parts[1]

				// Handle different parameter types
				switch key {
				case "action":
					intent.SetAction(value)
				case "package":
					intent.SetPackage(value)
				case "component":
					intent.SetComponent(value)
				case "type":
					intent.SetType(value)
				case "scheme":
					intent.SetScheme(value)
				case "launchFlags":
					intent.SetFlags(value)
				case "category":
					intent.AddCategory(value)
				default:
					// Handle extras (simplified)
					if len(key) > 2 && key[1] == '.' {
						// This is an extra (e.g., "S.name", "i.id")
						extraType := string(key[0])
						extraKey := key[2:]
						intent.PutExtra(extraKey, ExtraValue{Type: extraType, Value: value})
					}
				}
			}
		}
	} else {
		// Regular URI - treat as VIEW action
		intent.SetData(uri)
	}

	return intent, nil
}

func idActions(id string, actions ...string) {
	intent := &Intent{
		Data:   id,
		Scheme: "package",
		Flags: flagActivity(
			FLAG_ACTIVITY_SINGLE_TOP,
			FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS,
		),
	}
	notFinish = true
	for _, i := range actions {
		intent.SetAction(i)
		if err := OpenURL(intent.String()); err == nil {
			log.Debug(intent)
			return
		}
	}
	notFinish = false
}

var (
	qr *QR

	scannerIsBrowser    bool
	clipboardBeforeScan string
)

// QR управляет всей секцией QR в настройках
type QR struct {
	app    fyne.App
	window fyne.Window

	// Виджеты
	qrImage      *canvas.Image
	link         *widget.Hyperlink
	scannerSetup fyne.CanvasObject
	instructions fyne.CanvasObject
	separator    *widget.Separator

	// Контейнеры
	mainContainer *fyne.Container
	accordionItem *widget.AccordionItem

	// Состояние
	isIO        bool
	currentText string
}

// NewQR создает новую секцию QR
func NewQR(a fyne.App, w fyne.Window) *QR {
	qr := &QR{
		app:    a,
		window: w,
		isIO:   false,
	}

	// Создаем виджеты
	qr.qrImage = canvas.NewImageFromResource(theme.BrokenImageIcon())
	qr.qrImage.SetMinSize(fyne.NewSize(qrSize, qrSize))
	qr.qrImage.FillMode = canvas.ImageFillContain

	qr.link = widget.NewHyperlink("", nil)
	qr.link.Wrapping = fyne.TextWrapBreak
	qr.separator = widget.NewSeparator()

	qr.scannerSetup = qr.makeScannerSettings()
	qr.instructions = qr.makeQRInstructions()

	// Создаем основной контейнер
	qr.mainContainer = container.NewVBox(
		container.NewCenter(qr.qrImage),
		qr.link,
		qr.separator,
	)

	// Добавляем опциональные виджеты
	if qr.scannerSetup != nil {
		qr.mainContainer.Add(qr.scannerSetup)
	}
	if qr.instructions != nil {
		qr.mainContainer.Add(qr.instructions)
	}

	// Создаем элемент аккордеона
	qr.accordionItem = widget.NewAccordionItem("QR", qr.mainContainer)

	// Первоначальное обновление
	fyne.Do(qr.UpdateFromClipboard)

	return qr
}

// initWidgets создает и настраивает виджеты
func (qr *QR) initWidgets() {
	// Создаем пустой QR-код
	qr.qrImage = canvas.NewImageFromResource(theme.BrokenImageIcon())
	qr.qrImage.SetMinSize(fyne.NewSize(qrSize, qrSize))

	// Создаем ссылку
	qr.link = widget.NewHyperlink("", nil)
	qr.link.Wrapping = fyne.TextWrapBreak

	// Создаем разделитель
	qr.separator = widget.NewSeparator()

	// Создаем дополнительные виджеты
	qr.scannerSetup = qr.makeScannerSettings()
	qr.instructions = qr.makeQRInstructions()
}

// buildLayout собирает контейнеры
func (qr *QR) buildLayout() {
	qr.mainContainer = container.NewVBox(
		container.NewCenter(qr.qrImage), //NewWithoutLayout
		qr.link,
		qr.separator,
	)

	// Добавляем опциональные виджеты
	if qr.scannerSetup != nil {
		qr.mainContainer.Add(qr.scannerSetup)
	}
	if qr.instructions != nil {
		qr.mainContainer.Add(qr.instructions)
	}

	// Создаем элемент аккордеона
	qr.accordionItem = widget.NewAccordionItem("QR", qr.mainContainer)
}

// UpdateFromClipboard обновляет секцию из содержимого буфера обмена
func (qr *QR) UpdateFromClipboard() {
	text := qr.app.Clipboard().Content()
	qr.currentText = text

	// Определяем тип контента
	qr.isIO = strings.HasPrefix(text, IO)

	// Генерируем новый QR-код
	qr.generateQR()
	qr.updateLink()
	qr.updateVisibility()

	qr.mainContainer.Refresh()
}

// handleIOTapped обрабатывает нажатие на IO-ссылку
func (qr *QR) handleIOTapped() {
	if !(isAndroid || asMobile) {
		if err := OpenURL(qr.currentText); err == nil {
			NewToast(qr.window, "OK").Show()
		}
		return
	}

	intent := &Intent{
		Data:   strings.TrimPrefix(qr.link.URL.String(), qr.link.URL.Scheme+":"),
		Scheme: qr.link.URL.Scheme,
		Flags: flagActivity(
			FLAG_ACTIVITY_SINGLE_TOP,
			FLAG_ACTIVITY_REQUIRE_NON_BROWSER,
		),
	}

	notFinish = true
	if err := OpenURL(intent.String()); err == nil {
		NewToast(qr.window, "OK").Show()
		return
	}

	intent.SetFlags(BROWSER)
	if err := OpenURL(intent.String()); err != nil {
		qr.app.SendNotification(fyne.NewNotification("Error", err.Error()))
	}
	notFinish = false
}

// updateVisibility управляет видимостью виджетов
func (qr *QR) updateVisibility() {
	if qr.scannerSetup != nil {
		if qr.isIO {
			qr.scannerSetup.Show()
		} else {
			qr.scannerSetup.Hide()
		}
	}
}

// SetClipboard устанавливает контент в буфер обмена и обновляет секцию
func (qr *QR) SetClipboard(text string) string {
	qr.app.Clipboard().SetContent(text)
	qr.UpdateFromClipboard()
	return text
}

// GetAccordionItem возвращает элемент аккордеона
func (qr *QR) GetAccordionItem() *widget.AccordionItem {
	return qr.accordionItem
}

func (qr *QR) showTextDialog() {
	textEntry := widget.NewMultiLineEntry()
	textEntry.SetText(qr.currentText)
	textEntry.Wrapping = fyne.TextWrapWord
	TextWrapWord := true

	var wrapButton *widget.Button
	wrapButton = widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), func() {
		TextWrapWord = !TextWrapWord
		if TextWrapWord {
			wrapButton.SetIcon(theme.MoreVerticalIcon())
			textEntry.Wrapping = fyne.TextWrapWord
		} else {
			wrapButton.SetIcon(theme.MoreHorizontalIcon())
			textEntry.Wrapping = fyne.TextWrapOff
		}

		textEntry.Refresh()
	})

	dialog := dialog.NewCustom(
		"Clipboard",
		"OK",
		container.NewBorder(
			wrapButton,
			nil, nil, nil,
			container.NewVScroll(textEntry),
		),
		qr.window,
	)
	if !(isAndroid || asMobile || qr.window.FullScreen()) {
		current := qr.window.Canvas().Size()
		qr.window.Resize(current.AddWidthHeight(current.Width, 0))
		dialog.SetOnClosed(func() {
			qr.window.Resize(current)
		})
	}
	dialog.Resize(qr.window.Canvas().Size())

	dialog.Show()
}
