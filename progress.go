// progress.go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
)

const minInterval = 200 * time.Millisecond

var ErrWriteCanceled = errors.New("write canceled")

type ProgressWrapper struct {
	*widget.ProgressBar
	lastValue float64
	lastCall  time.Time
}
type ProgressWriter struct {
	Writer       io.Writer
	Total        int64
	Written      int64
	OnProgress   func(float64)
	lastCall     time.Time
	lastProgress float64
	cancel       <-chan struct{}
}

func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	select {
	case <-pw.cancel:
		return 0, ErrWriteCanceled
	case <-done:
		return 0, ErrApplicationShutdown
	default:
	}

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
	sbShow := func() {}
	if len(c.Objects) > 3 {
		// Для вкладки Беру
		sb := c.Objects[feSave]
		fyne.Do(sb.Hide)
		sbShow = func() { fyne.Do(sb.Show) }
	}

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
		fyne.Do(func() {
			pb.Hide()
			sbShow()
		})
	}

	return pw, restore
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
				log.Tracef("Chtimes %s %v: %v", destination.Name(), t,
					os.Chtimes(destination.Name(), time.Time{}, t))
			}
		}
		restore()
		onComplete(err)
	}()
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
		// 		log.Tracef("source ModTime %s %v:%v", source.Name(), t, err)
		// 		setModTime(destination.URI(), t)
		// 		t, err = ModTime(destination.URI())
		// 		log.Tracef("destination ModTime %s %v:%v", destination.URI(), t, err)
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
