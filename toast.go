// toast.go
package main

import (
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
	size          fyne.Size
	timeout       time.Duration
	padding       float32
	withAnimation bool
	popup         *widget.PopUp
	as            *fyne.Animation
	ah            *fyne.Animation
}

// Константы таймаутов
const (
	ToastShort        = 3 * time.Second
	ToastLong         = 4 * time.Second
	DefaultPadding    = 20
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
	}
}

func (t *Toast) SetIcon(icon fyne.Resource) *Toast {
	t.icon = icon
	return t
}

func (t *Toast) SetText(message string) *Toast {
	t.message = message
	return t
}

func (t *Toast) SetSize(width, height float32) *Toast {
	t.size = fyne.NewSize(width, height)
	return t
}

func (t *Toast) SetTimeout(timeout time.Duration) *Toast {
	t.timeout = timeout
	return t
}

func (t *Toast) SetPadding(padding float32) *Toast {
	t.padding = padding
	return t
}

func (t *Toast) SetAnimation(on bool) *Toast {
	t.withAnimation = on
	return t
}

// Удобные методы для стандартных таймаутов
func (t *Toast) Short() *Toast {
	t.timeout = ToastShort
	return t
}

func (t *Toast) Long() *Toast {
	t.timeout = ToastLong
	return t
}

// calculateSize вычисляет размер тоста на основе содержимого
func (t *Toast) calculateSize() fyne.Size {
	minWidth := float32(120)
	maxWidth := float32(300)
	iconWidth := float32(0)
	iconHeight := float32(0)

	// Учитываем иконку
	if t.icon != nil {
		iconWidth = 24 + t.padding
		iconHeight = 24
	}

	// Оцениваем размер текста
	textWidth := float32(len(t.message)) * 8
	if textWidth < minWidth-iconWidth-t.padding*2 {
		textWidth = minWidth - iconWidth - t.padding*2
	}
	if textWidth > maxWidth-iconWidth-t.padding*2 {
		textWidth = maxWidth - iconWidth - t.padding*2
	}

	width := textWidth + iconWidth + t.padding*2
	height := float32(40)

	if iconHeight > height {
		height = iconHeight + t.padding*2
	}

	return fyne.NewSize(width, height)
}

// Hide - принудительно скрыть тост
func (t *Toast) Hide() {
	if t.as != nil {
		t.as.Stop()
	}
	if t.ah != nil {
		t.ah.Stop()
	}
	if t.popup != nil {
		t.popup.Hide()
	}
}

// showWithAnimation - показывает тост с анимацией используя fyne.Animation
func (t *Toast) showWithAnimation(startPos, endPos fyne.Position) {
	if t.popup == nil {
		return
	}

	// Создаем анимацию позиции
	t.as = fyne.NewAnimation(time.Duration(AnimationDuration), func(progress float32) {
		currentX := startPos.X + (endPos.X-startPos.X)*progress
		currentY := startPos.Y + (endPos.Y-startPos.Y)*progress
		t.popup.Move(fyne.NewPos(currentX, currentY))
	})

	t.as.Curve = fyne.AnimationLinear
	t.popup.Move(startPos)
	t.popup.Show()
	t.as.Start()
}

// hideWithAnimation - скрывает тост с анимацией
func (t *Toast) hideWithAnimation(startPos fyne.Position) {
	if t.popup == nil {
		return
	}

	canvasSize := t.window.Canvas().Size()
	endPos := fyne.NewPos(startPos.X, canvasSize.Height+50)

	t.ah = fyne.NewAnimation(time.Duration(AnimationDuration), func(progress float32) {
		currentX := startPos.X + (endPos.X-startPos.X)*progress
		currentY := startPos.Y + (endPos.Y-startPos.Y)*progress
		t.popup.Move(fyne.NewPos(currentX, currentY))
	})

	t.as.Curve = fyne.AnimationLinear
	t.ah.Start()

	go func() {
		select {
		case <-done:
			return
		case <-time.After(AnimationDuration):
			t.ah.Stop()
			if t.popup != nil {
				fyne.Do(t.popup.Hide)
			}
		}
	}()
}

func (t *Toast) Show() {
	// Автоматически вычисляем размер если не задан явно
	if t.size.Width == 0 && t.size.Height == 0 {
		t.size = t.calculateSize()
	}

	// Создаем содержимое тоста
	var content fyne.CanvasObject

	if t.icon != nil {
		icon := widget.NewIcon(t.icon)
		text := canvas.NewText(t.message, theme.Color(theme.ColorNameForeground))
		text.TextSize = 14
		text.Alignment = fyne.TextAlignCenter

		content = container.NewHBox(
			container.NewCenter(icon),
			container.NewCenter(text),
		)
	} else {
		text := canvas.NewText(t.message, theme.Color(theme.ColorNameForeground))
		text.TextSize = 14
		text.Alignment = fyne.TextAlignCenter
		content = container.NewCenter(text)
	}

	// Создаем фон
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	bg.Resize(t.size)

	// Основной контейнер с отступами
	popupContent := container.NewStack(
		bg,
		container.NewPadded(content),
	)

	t.popup = widget.NewPopUp(popupContent, t.window.Canvas())
	t.popup.Resize(t.size)

	// Начальная и конечная позиции
	canvasSize := t.window.Canvas().Size()
	startPos := fyne.NewPos(
		(canvasSize.Width-t.size.Width)/2,
		canvasSize.Height+50,
	)
	endPos := fyne.NewPos(
		(canvasSize.Width-t.size.Width)/2,
		canvasSize.Height-t.size.Height-50,
	)

	// Показываем тост с анимацией или без
	if t.withAnimation {
		t.showWithAnimation(startPos, endPos)
	} else {
		t.popup.Move(endPos)
		t.popup.Show()
	}

	go func() {
		select {
		case <-done:
			return
		case <-time.After(t.timeout):
			if t.popup != nil && t.popup.Visible() {
				fyne.Do(func() {
					if t.withAnimation {
						t.hideWithAnimation(endPos)
					} else {
						t.popup.Hide()
					}
				})
			}
		}
	}()
}
