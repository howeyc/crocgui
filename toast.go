// toast.go
package main

import (
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type Toast struct {
	window        fyne.Window
	message       string
	icon          fyne.Resource
	timeout       time.Duration
	padding       float32
	withAnimation bool
	popup         *widget.PopUp
	done          chan struct{}
	isHiding      atomic.Int32
}

const (
	ToastShort        = 3 * time.Second
	ToastLong         = 4 * time.Second
	DefaultPadding    = 10.0
	AnimationDuration = 300 * time.Millisecond
)

func NewToast(win fyne.Window, message string) *Toast {
	return &Toast{
		window:        win,
		message:       message,
		icon:          theme.InfoIcon(),
		timeout:       ToastShort,
		padding:       DefaultPadding,
		withAnimation: true,
		done:          make(chan struct{}),
	}
}

func (t *Toast) SetIcon(icon fyne.Resource) *Toast       { t.icon = icon; return t }
func (t *Toast) SetText(message string) *Toast           { t.message = message; return t }
func (t *Toast) SetTimeout(timeout time.Duration) *Toast { t.timeout = timeout; return t }
func (t *Toast) SetPadding(padding float32) *Toast       { t.padding = padding; return t }
func (t *Toast) SetAnimation(on bool) *Toast             { t.withAnimation = on; return t }

func (t *Toast) Short() *Toast { t.timeout = ToastShort; return t }
func (t *Toast) Long() *Toast  { t.timeout = ToastLong; return t }

func (t *Toast) buildContent() fyne.CanvasObject {
	var content fyne.CanvasObject
	text := canvas.NewText(t.message, theme.Color(theme.ColorNameForeground))
	text.TextSize = 14
	text.Alignment = fyne.TextAlignCenter
	if t.icon != nil {
		icon := widget.NewIcon(t.icon)
		content = container.NewHBox(container.NewCenter(icon), container.NewCenter(text))
	} else {
		content = container.NewCenter(text)
	}
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	popupContent := container.NewStack(bg, container.NewPadded(content))
	return popupContent
}

// Hide - принудительно скрывает тост.
func (t *Toast) Hide() {
	if t.isHiding.CompareAndSwap(0, 1) {
		select {
		case <-t.done:
			return
		default:
			close(t.done)
		}
	}
}

// showWithAnimation запускается в потоке Fyne
func (t *Toast) showWithAnimation(startPos, endPos fyne.Position) {
	t.popup.Move(startPos)
	t.popup.Show()

	anim := fyne.NewAnimation(AnimationDuration, func(progress float32) {
		currentX := startPos.X + (endPos.X-startPos.X)*progress
		currentY := startPos.Y + (endPos.Y-startPos.Y)*progress
		t.popup.Move(fyne.NewPos(currentX, currentY))
	})
	anim.Curve = fyne.AnimationEaseOut
	anim.Start()
}

// hideWithAnimation выполняет анимацию скрытия
func (t *Toast) hideWithAnimation(startPos fyne.Position) {
	canvasSize := t.window.Canvas().Size()
	endPos := fyne.NewPos(startPos.X, canvasSize.Height+50)

	anim := fyne.NewAnimation(AnimationDuration, func(progress float32) {
		currentX := startPos.X + (endPos.X-startPos.X)*progress
		currentY := startPos.Y + (endPos.Y-startPos.Y)*progress
		t.popup.Move(fyne.NewPos(currentX, currentY))
	})
	anim.Curve = fyne.AnimationEaseIn
	anim.Start()

	// Ждем завершения анимации или сигнала отмены
	go func() {
		select {
		case <-appCtx.Done():
			return
		case <-time.After(AnimationDuration + 50*time.Millisecond):
			fyne.Do(func() {
				t.popup.Hide()
				t.window.RequestFocus()
			})
		case <-t.done:
			fyne.Do(func() {
				t.popup.Hide()
				t.window.RequestFocus()
			})
		}
	}()
}

// Show отображает тост на экране.
func (t *Toast) Show() {
	if t.isHiding.Load() == 1 {
		return
	}

	popupContent := t.buildContent()
	t.popup = widget.NewPopUp(popupContent, t.window.Canvas())

	// Fyne сама вычислит размер, мы только ограничиваем ширину
	canvasSize := t.window.Canvas().Size()
	maxWidth := canvasSize.Width * 0.9

	// Получаем минимальный размер от Fyne
	contentSize := t.popup.MinSize()

	// Если ширина слишком большая - ограничиваем и пересчитываем высоту
	if contentSize.Width > maxWidth {
		contentSize.Width = maxWidth
		// Даем popup'у ограниченную ширину и вычисляем необходимую высоту
		t.popup.Resize(fyne.NewSize(maxWidth, 1000))  // Большая высота для измерения
		contentSize.Height = t.popup.MinSize().Height // Получаем актуальную высоту
	}

	// Устанавливаем финальный размер
	t.popup.Resize(contentSize)

	// Позиционируем тост
	startPos := fyne.NewPos((canvasSize.Width-contentSize.Width)/2, canvasSize.Height+50)
	endPos := fyne.NewPos((canvasSize.Width-contentSize.Width)/2, canvasSize.Height-contentSize.Height-20)

	if t.withAnimation {
		t.showWithAnimation(startPos, endPos)
	} else {
		t.popup.Move(endPos)
		t.popup.Show()
	}

	// Горутина для управления автоматическим скрытием
	go func() {
		visibleTime := t.timeout
		if t.withAnimation {
			visibleTime += AnimationDuration
		}

		select {
		case <-t.done:
			fyne.Do(func() {
				t.window.RequestFocus()
			})
			return

		case <-time.After(visibleTime):
			if t.isHiding.CompareAndSwap(0, 1) {
				fyne.Do(func() {
					if t.withAnimation {
						t.hideWithAnimation(endPos)
					} else {
						t.popup.Hide()
						t.window.RequestFocus()
					}
				})
			}
		case <-appCtx.Done():
			return
		}
	}()
}
