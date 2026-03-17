// progress.go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
)

const (
	minInterval = 200 * time.Millisecond
	KB          = 1000.0
)

var ErrWriteCanceled = errors.New("write canceled")

// ProgressState содержит статистику прогресс-бара
type ProgressState struct {
	Value     int64
	Max       int64
	Percent   float64
	Elapsed   time.Duration
	Remaining time.Duration
	SpeedBps  float64 // байт в секунду
	SpeedKBps float64 // килобайт в секунду
	SpeedMBps float64 // мегабайт в секунду
}

// LongProgressWrapper расширяет ProgressWrapper с отслеживанием времени
type LongProgressWrapper struct {
	*widget.ProgressBar
	startTime      time.Time
	lastUpdateTime time.Time
	lastValue      float64
	speedHistory   []float64
	maxHistorySize int
	lastCall       time.Time
	lastRemaining  time.Duration
}

// NewLongProgressWrapper создает новый прогресс-бар с отслеживанием статистики
func NewLongProgressWrapper(p *widget.ProgressBar) (l *LongProgressWrapper) {
	l = &LongProgressWrapper{
		ProgressBar:    p,
		lastValue:      -1,
		speedHistory:   make([]float64, 0),
		maxHistorySize: 10,
		lastRemaining:  0,
	}
	if p == nil {
		return
	}

	fyne.Do(func() {
		p.SetValue(0)
	})
	p.TextFormatter = func() string {
		return longFormatter(l)
	}
	return
}

// SetValue устанавливает значение с обновлением статистики
func (l *LongProgressWrapper) SetValue(value int64) {
	if l.ProgressBar == nil {
		return
	}

	now := time.Now()
	oldValue := l.lastValue
	newValue := float64(value)

	// Инициализация времени начала
	if l.startTime.IsZero() && newValue > 0 {
		l.startTime = now
	}

	// Расчет мгновенной скорости
	if !l.lastUpdateTime.IsZero() && newValue > oldValue && oldValue != -1 {
		timeDelta := now.Sub(l.lastUpdateTime).Seconds()
		if timeDelta > 0 {
			valueDelta := newValue - oldValue
			// Предполагаем, что значение - это байты
			instantSpeed := valueDelta / timeDelta

			// Добавляем в историю для скользящего среднего
			l.speedHistory = append(l.speedHistory, instantSpeed)
			if len(l.speedHistory) > l.maxHistorySize {
				l.speedHistory = l.speedHistory[1:]
			}
		}
	}

	l.lastValue = newValue
	l.lastUpdateTime = now

	// Обновляем UI только с интервалом minInterval
	if now.Sub(l.lastCall) >= minInterval || oldValue == -1 {
		l.lastCall = now
		fyne.Do(func() {
			l.ProgressBar.SetValue(newValue)
		})
	}
}

// State возвращает текущую статистику
func (l *LongProgressWrapper) State() ProgressState {
	state := ProgressState{
		Value: int64(l.lastValue),
		Max:   int64(l.ProgressBar.Max),
	}

	if l.ProgressBar.Max > l.ProgressBar.Min && l.lastValue > 0 {
		state.Percent = (l.lastValue - l.ProgressBar.Min) / (l.ProgressBar.Max - l.ProgressBar.Min) * 100
	}

	// Прошедшее время
	if !l.startTime.IsZero() {
		state.Elapsed = time.Since(l.startTime)

		// Средняя скорость
		if len(l.speedHistory) > 0 {
			var totalSpeed float64
			for _, speed := range l.speedHistory {
				totalSpeed += speed
			}
			state.SpeedBps = totalSpeed / float64(len(l.speedHistory))
		} else if state.Elapsed.Seconds() > 0 && l.lastValue > 0 {
			state.SpeedBps = l.lastValue / state.Elapsed.Seconds()
		}

		state.SpeedKBps = state.SpeedBps / KB
		state.SpeedMBps = state.SpeedKBps / KB

		// Оставшееся время
		if state.SpeedBps > 0 && l.lastValue < l.ProgressBar.Max {
			remainingBytes := l.ProgressBar.Max - l.lastValue
			state.Remaining = time.Duration(remainingBytes/state.SpeedBps) * time.Second
			if state.Remaining < 0 {
				state.Remaining = 0
			}
		}
	}

	return state
}

// SetMax устанавливает максимальное значение
func (l *LongProgressWrapper) SetMax(max int64) {
	if l.ProgressBar == nil {
		return
	}
	fyne.Do(l.ProgressBar.Show)
	newMax := float64(max)

	if newMax != l.ProgressBar.Max {
		l.ProgressBar.Max = newMax

		if newMax < l.lastValue || l.lastValue == -1 {
			l.lastValue = -1
		}
	}
}

// Reset сбрасывает статистику
func (l *LongProgressWrapper) Reset() {
	l.startTime = time.Time{}
	l.lastUpdateTime = time.Time{}
	l.lastValue = -1
	l.speedHistory = make([]float64, 0)
	l.lastCall = time.Time{}
	l.lastRemaining = 0

	fyne.Do(func() {
		l.ProgressBar.SetValue(0)
	})
}

type ProgressWrapper struct {
	*widget.ProgressBar
	lastValue float64
	lastCall  time.Time
}

type ProgressWriter struct {
	Writer     io.Writer
	Max        int64
	Value      int64
	OnProgress func(float64)
	lastCall   time.Time
	lastValue  float64
	cancel     <-chan struct{}
}

func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	select {
	case <-pw.cancel:
		return 0, ErrWriteCanceled
	case <-appCtx.Done():
		return 0, ErrApplicationShutdown
	default:
	}

	n, err = pw.Writer.Write(p)

	if err != nil || pw.OnProgress == nil || pw.Max <= 0 {
		return
	}

	pw.Value += int64(n)
	value := float64(pw.Value)
	if pw.lastValue < value {
		if max := float64(pw.Max); value > max {
			value = max
		}
		pw.lastValue = value
	} else {
		return
	}

	now := time.Now()
	if now.Sub(pw.lastCall) >= minInterval {
		pw.OnProgress(value)
		pw.lastCall = now
	}
	return
}

func NewProgressWriter(destination io.Writer, total int64, c *fyne.Container) (pw *ProgressWriter, restore func()) {
	db := c.Objects[feDel].(*widget.Button)
	pb := c.Objects[feBar].(*widget.ProgressBar)
	sbShow := func() {}
	if len(c.Objects) > 3 {
		// Для вкладки Беру
		sb := c.Objects[feSave]
		fyne.Do(sb.Hide)
		sbShow = func() { fyne.Do(sb.Show) }
	}

	oldOnTapped := db.OnTapped
	oldIcon := db.Icon

	cancelChan := make(chan struct{})

	pw = &ProgressWriter{
		Writer: destination,
		Max:    total,
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
		db.Icon = theme.CancelIcon()
		db.Refresh()
		setSizes(pb, total, 0)
	})

	restore = func() {
		db.OnTapped = oldOnTapped
		fyne.Do(func() {
			db.Icon = oldIcon
			db.Refresh()
			setSizes(pb, pw.Value)
			sbShow()
		})
	}

	return pw, restore
}

// Max Value
func setSizes(p *widget.ProgressBar, sizes ...int64) {
	if p == nil {
		return
	}
	max := sizes[0]
	if max > 0 {
		p.Max = float64(max)
	} else {
		p.Max = 0.1
	}
	if len(sizes) > 1 {
		p.SetValue(float64(sizes[1]))
	} else {
		p.SetValue(p.Max)
	}
}

func CopyFileProgress(src, dst string, c *fyne.Container, onComplete func(err error)) {
	fi, err := os.Stat(src)
	if err != nil {
		onComplete(err)
		return
	}

	source, err := os.Open(src)
	if err != nil {
		onComplete(err)
		return
	}
	close := func() {
		if err := source.Close(); err != nil {
			log.Errorf("close %s: %v", source.Name(), err)
		}
	}

	destination, err := os.Create(dst)
	if err != nil {
		close()
		onComplete(err)
		return
	}
	clode := func() {
		if err := destination.Close(); err != nil {
			log.Errorf("close %s: %v", destination.Name(), err)
		}
	}

	pw, restore := NewProgressWriter(destination, fi.Size(), c)

	go func() {
		_, err := io.Copy(pw, source)
		close()
		clode()
		if err == nil {
			if t, err := fileModTime(source.Name()); err == nil && !t.IsZero() {
				log.Debugf("Chtimes %s %v: %v", destination.Name(), t,
					os.Chtimes(destination.Name(), time.Time{}, t))
			}
		}
		restore()
		onComplete(err)
	}()
}

func NewProgressWrapper(p *widget.ProgressBar) *ProgressWrapper {
	if p != nil {
		fyne.Do(func() {
			p.SetValue(0)
		})
	}
	return &ProgressWrapper{
		ProgressBar: p,
		lastValue:   -1,
	}
}

func (p *ProgressWrapper) Show() {
	if p.ProgressBar != nil {
		fyne.Do(p.ProgressBar.Show)
	}
}

func (p *ProgressWrapper) Hide() {
	if p.ProgressBar != nil {
		fyne.Do(p.ProgressBar.Hide)
	}
}

func (p *ProgressWrapper) SetValue(value int64) {
	if p.ProgressBar == nil {
		return
	}
	newValue := float64(value)

	if newValue > p.lastValue || p.lastValue == -1 {
		now := time.Now()
		if now.Sub(p.lastCall) >= minInterval || p.lastValue == -1 {
			p.lastValue = newValue
			p.lastCall = now
			fyne.Do(func() {
				p.ProgressBar.SetValue(newValue)
			})
		}
	}
}

func (p *ProgressWrapper) SetMax(max int64) {
	if p.ProgressBar == nil {
		return
	}
	newMax := float64(max)

	if newMax != p.ProgressBar.Max {
		p.ProgressBar.Max = newMax

		if newMax < p.lastValue || p.lastValue == -1 {
			p.lastValue = -1
		}
	}
}

type LabelWrapper struct {
	*widget.Label
	lastText string
}

func NewLabelWrapper(label *widget.Label) *LabelWrapper {
	return &LabelWrapper{
		Label:    label,
		lastText: "",
	}
}

func (lw *LabelWrapper) SetText(text string) {
	if text != lw.lastText {
		lw.lastText = text
		fyne.Do(func() {
			lw.Label.SetText(text)
		})
	}
}

func copyToUWCProgress(destination fyne.URIWriteCloser, src string, c *fyne.Container, onComplete func(err error)) {
	if destination == nil {
		onComplete(fmt.Errorf("destination is nil (dialog closed)"))
		return
	}
	clode := func() {
		if err := destination.Close(); err != nil {
			log.Errorf("close %s: %v", destination.URI(), err)
		}
	}

	source, err := os.Open(src)
	if err != nil {
		clode()
		onComplete(fmt.Errorf("failed to open source file: %v", err))
		return
	}
	close := func() {
		if err := source.Close(); err != nil {
			log.Errorf("close %s: %v", source.Name(), err)
		}
	}

	fi, err := os.Stat(src)
	if err != nil {
		clode()
		close()
		onComplete(err)
		return
	}

	pw, restore := NewProgressWriter(destination, fi.Size(), c)

	go func() {
		_, err := io.Copy(pw, source)
		clode()
		close()
		// if err == nil {
		// 	if t, err := fileModTime(source.Name()); err == nil {
		// 		log.Debugf("source ModTime %s %v:%v", source.Name(), t, err)
		// 		setModTime(destination.URI(), t)
		// 		t, err = ModTime(destination.URI())
		// 		log.Debugf("destination ModTime %s %v:%v", destination.URI(), t, err)
		// 	}
		// }
		restore()
		onComplete(err)
	}()
}

// fileModTime стандартная реализация для обычных файлов
func fileModTime(filePath string) (time.Time, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return time.Now(), err
	}
	return fi.ModTime(), nil
}

// Обновленный textFormatter с поддержкой статистики
func longFormatter(l *LongProgressWrapper) string {
	w := l.ProgressBar
	if w.Max < 1 || w.Max == w.Min {
		return ""
	}

	state := l.State()
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}

	// Оставшиеся байты
	value := float64(w.Max - w.Value)
	unitIndex := 0
	for value >= KB {
		value /= KB
		unitIndex++
	}

	sizeStr := fmt.Sprintf("%03d%s", int(value), units[unitIndex])

	// Если скорость близка к нулю или расчет нестабилен - только объем
	if state.SpeedBps < 1 || len(l.speedHistory) < 2 {
		return sizeStr
	}
	// Есть значимая скорость - добавляем время и скорость
	speed := state.SpeedKBps
	speedUnit := "KB/s"
	if state.SpeedMBps >= 1 {
		speed = state.SpeedMBps
		speedUnit = "MB/s"
	}

	// Монотонное время
	remaining := state.Remaining
	if remaining > l.lastRemaining && l.lastRemaining > 0 {
		remaining = l.lastRemaining
	}
	if remaining < l.lastRemaining || l.lastRemaining == 0 {
		l.lastRemaining = remaining
	}

	// Форматирование времени
	var timeStr string
	if remaining.Hours() >= 1 {
		timeStr = fmt.Sprintf("%02dh", int(remaining.Hours()))
	} else if remaining.Minutes() >= 1 {
		timeStr = fmt.Sprintf("%02dm", int(remaining.Minutes()))
	} else {
		sec := int(remaining.Round(time.Second).Seconds())
		if sec < 1 {
			sec = 1
		}
		timeStr = fmt.Sprintf("%02ds", sec)
	}

	return fmt.Sprintf("%s / %s %03d%s",
		sizeStr, timeStr, int(speed), speedUnit)
}

// w.Max<1 для каталогов и отсутствующих файлов
func shortFormatter(w *widget.ProgressBar) string {
	if w.Max < 1 || w.Max == w.Min {
		// Для каталогов и отсутствующих файлов
		return "\t"
	}
	units := []string{"b", "k", "m", "g", "t", "p", "e"}
	value := w.Max
	if w.Value < w.Max {
		// Режим прогрессбара
		value -= w.Value
	}
	unitIndex := 0

	// Переходим к следующей единице когда value >= 1000
	for value >= KB &&
		//Int64.MaxValue~009e
		// unitIndex < len(units)-1 &&
		true {
		value /= KB
		unitIndex++
	}

	return fmt.Sprintf("%03d%s\t", int(value), units[unitIndex])
}
