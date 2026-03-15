package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
)

const (
	LOG_LINES                = 200
	immediateUpdateThreshold = 500 * time.Millisecond
	debounceDelay            = 100 * time.Millisecond
	maxRapidUpdates          = 3
)

type logWriter struct {
	buf              *bytes.Buffer
	lastLines        []string
	lastUpdateTime   time.Time
	rapidUpdateCount int
	updateTimer      *time.Timer
	segments         []widget.RichTextSegment
	RichText         *widget.RichText
	Scroll           *container.Scroll
	isActive         bool
}

func newLogWriter() logWriter {
	lw := logWriter{
		buf:       &bytes.Buffer{},
		lastLines: make([]string, 0, LOG_LINES),
		segments:  make([]widget.RichTextSegment, 0, LOG_LINES),
		RichText:  widget.NewRichText(),
	}
	lw.RichText.Wrapping = fyne.TextWrapWord
	lw.Scroll = container.NewScroll(lw.RichText)
	return lw
}

func (lw *logWriter) active(isActive bool) {
	if !lw.isActive && isActive {
		lw.isActive = isActive
		if lw.updateTimer != nil {
			lw.updateTimer.Stop()
		}
		lw.update()
		lw.lastUpdateTime = time.Now()
		lw.rapidUpdateCount = 0
	}
	lw.isActive = isActive
}

func (lw *logWriter) appendLogLine(line string) {
	if !lw.isActive || line == "" {
		return
	}

	lw.segments = append(lw.segments, lw.createSegment(line))
	if len(lw.segments) > LOG_LINES {
		lw.segments = lw.segments[len(lw.segments)-LOG_LINES:]
	}

	lw.refresh()
}

// GUI
func (lw *logWriter) refresh() {
	lw.RichText.Segments = lw.segments
	lw.RichText.Refresh()
	// doMonitor.Bounce(lw.RichText.Refresh)
	lw.Scroll.ScrollToBottom()
}

func (lw *logWriter) createSegment(line string) widget.RichTextSegment {
	return &widget.TextSegment{
		Text: removeLevel(line),
		Style: widget.RichTextStyle{
			ColorName: getColorNameByLevel(line),
		},
	}
}

func (lw *logWriter) trimLines() {
	if len(lw.lastLines) > LOG_LINES {
		lw.lastLines = lw.lastLines[len(lw.lastLines)-LOG_LINES:]
	}
}

func (lw *logWriter) update() {
	if !lw.isActive || len(lw.lastLines) == 0 {
		return
	}

	lw.trimLines()

	if cap(lw.segments) < len(lw.lastLines) {
		lw.segments = make([]widget.RichTextSegment, len(lw.lastLines), LOG_LINES)
	} else {
		lw.segments = lw.segments[:len(lw.lastLines)]
	}

	for i, line := range lw.lastLines {
		lw.segments[i] = lw.createSegment(line)
	}

	lw.refresh()
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	n, err = lw.buf.Write(p)

	cleanData := strings.TrimSpace(string(p))
	if cleanData == "" {
		return n, err
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(cleanData, "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) == 0 {
		return n, err
	}

	// Фильтруем дубликаты
	filteredLines := make([]string, 0, len(lines))

	for _, line := range lines {
		// Проверяем, не является ли строка дубликатом последней в lastLines
		if len(lw.lastLines) > 0 && lw.lastLines[len(lw.lastLines)-1] == line {
			continue
		}
		// Проверяем, не является ли строка дубликатом последней в текущем batch
		if len(filteredLines) > 0 && filteredLines[len(filteredLines)-1] == line {
			continue
		}
		filteredLines = append(filteredLines, line)
		LogD(line)
	}

	// Добавляем отфильтрованные строки
	if len(filteredLines) > 0 {
		lw.lastLines = append(lw.lastLines, filteredLines...)
		lw.trimLines()

		if lw.isActive {
			lw.scheduleUpdate(filteredLines)
		}
	}

	return n, err
}

func (lw *logWriter) scheduleUpdate(lines []string) {
	now := time.Now()
	timeSinceLastUpdate := now.Sub(lw.lastUpdateTime)

	if lw.updateTimer != nil {
		lw.updateTimer.Stop()
	}

	needsImmediateUpdate := timeSinceLastUpdate > immediateUpdateThreshold || lw.rapidUpdateCount < maxRapidUpdates

	if needsImmediateUpdate {
		fyne.Do(func() {
			if len(lines) == 1 {
				lw.appendLogLine(lines[0])
			} else {
				lw.update()
			}
			lw.lastUpdateTime = time.Now()
			lw.rapidUpdateCount++
		})
	} else {
		lw.updateTimer = time.AfterFunc(debounceDelay, func() {
			fyne.Do(func() {
				lw.update()
				lw.lastUpdateTime = time.Now()
				lw.rapidUpdateCount = 0
			})
		})
	}
}

func debugBool(a fyne.App) bool {
	switch debugString(a) {
	case "trace", "debug":
		return true
	default:
		return false
	}
}

func debugString(a fyne.App) string {
	return a.Preferences().String("debug-level")
}

// var exportButton *widget.Button

func logTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	OnSelectedTab[LOGi] = func() { logOutput.active(true) }

	exportButton := widget.NewButtonWithIcon(lp("Export full log"), theme.ContentCopyIcon(), func() {
		log.Debug("Log copied to clipboard")
		s := logOutput.buf.String()
		a.Clipboard().SetContent(s)

		child := CROCDEBUGLOG
		fileSave := func(destination fyne.URIWriteCloser, err error) {
			var (
				u  fyne.URI
				cl = func() {}
			)
			if err != nil {
				log.Errorf("folder selection: %v", err)
			} else if destination == nil {
				log.Debug("folder selection canceled")
				return
			}

			if destination == nil {
				u, cl, err = ChildDownload(child)
				if err != nil {
					log.Errorf("append child %s to Downloads: %v", child, err)
					return
				}
				defer cl()

				destination, err = storage.Writer(u)
				if err != nil {
					log.Errorf("writer %s: %v", u, err)
					return
				}
			} else {
				u = destination.URI()
			}
			defer destination.Close()

			if _, err := logOutput.buf.WriteTo(destination); err != nil {
				log.Errorf("log saved %s: %v", u, err)
				return
			}
			log.Debugf("log saved %s", u)
		}

		supported, err := IsSaveDialogSupported()
		if err != nil {
			log.Errorf("file picker: %v", err)
			supported = false
		}
		if !supported {
			fileSave(nil, fmt.Errorf("file picker not supported"))
			log.Debug("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(filePicker)
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				INSTALL,
				w,
			)
			return
		}
		// savedialog := dialog.NewFileSave(fileSave, w)
		// savedialog.SetFileName(child)
		// savedialog.Resize(w.Canvas().Size())
		// notFinish = true
		// savedialog.Show()
		newFileSave(fileSave, w, child)
	})
	exportButton.Hide()

	debugLevelBinding := binding.BindPreferenceString("debug-level", a.Preferences())
	debugCheck := widget.NewCheck("debug", func(debug bool) {
		if debug {
			log.SetLevel(LEVEL)
			debugLevelBinding.Set(LEVEL)
		} else {
			log.SetLevel("error")
			debugLevelBinding.Set("error")
		}
		if debugBool(a) {
			exportButton.Show()
		} else {
			exportButton.Hide()
		}

		logOutput.buf.Reset()
		logOutput.lastLines = make([]string, 0, LOG_LINES)
		logOutput.segments = make([]widget.RichTextSegment, 0, LOG_LINES)
		logOutput.refresh()
	})

	debugCheck.SetChecked(debugBool(a))

	rt := logOutput.RichText
	TextWrapWord := true

	var wrapButton *widget.Button
	wrapButton = widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), func() {
		TextWrapWord = !TextWrapWord
		if TextWrapWord {
			wrapButton.SetIcon(theme.MoreVerticalIcon())
			rt.Wrapping = fyne.TextWrapWord
		} else {
			wrapButton.SetIcon(theme.MoreHorizontalIcon())
			rt.Wrapping = fyne.TextWrapOff
		}

		rt.Refresh()
	})

	top := container.NewHBox(
		debugCheck,
		layout.NewSpacer(),
		wrapButton,
		layout.NewSpacer(),
		exportButton,
	)

	content := container.NewBorder(top, nil, nil, nil, logOutput.Scroll)
	return container.NewTabItemWithIcon("", theme.DocumentIcon(), content)
}

func removeLevel(line string) string {
	return replacer.Replace(line)
}

func getColorNameByLevel(line string) fyne.ThemeColorName {
	lowerLine := strings.ToLower(line)

	switch {
	case strings.Contains(lowerLine, "[trace]"):
		return theme.ColorNameHyperlink
	case strings.Contains(lowerLine, "[debug]"):
		return theme.ColorNameForeground
	case strings.Contains(lowerLine, "[info]"):
		return theme.ColorNameForeground
	case strings.Contains(lowerLine, "[warn]"):
		return theme.ColorNameWarning
	case strings.Contains(lowerLine, "[error]"):
		return theme.ColorNameError
	default:
		return theme.ColorNameDisabled
	}
}
