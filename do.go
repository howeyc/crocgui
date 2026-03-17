// do.go
package main

import (
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	log "github.com/schollz/logger"
)

const gapTimeout = 100 * time.Millisecond

type DoMonitor struct {
	requests chan func()
}

func NewDoMonitor() *DoMonitor {
	dm := &DoMonitor{
		requests: make(chan func(), 100),
	}

	go dm.monitor()
	return dm
}

func (dm *DoMonitor) monitor() {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	pending := make(map[uintptr]func())

	for {
		select {
		case <-appCtx.Done():
			return

		case fn := <-dm.requests:
			var iface interface{} = fn
			id := *(*uintptr)(unsafe.Pointer(&iface))
			pending[id] = fn

			// Гарантированно останавливаем и освобождаем таймер
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(gapTimeout)

		case <-timer.C:
			if len(pending) > 0 {
				dm.executeAll(pending)
				pending = make(map[uintptr]func())
			}
		}
	}
}

func (dm *DoMonitor) executeAll(pending map[uintptr]func()) {
	if len(pending) == 0 {
		return
	}

	fyne.Do(func() {
		for _, fn := range pending {
			log.Debugf("executeAll %p", fn)
			fn()
		}
	})
}

func (dm *DoMonitor) Bounce(fn func()) {
	// log.Debugf("Bounce %v", fn)
	if dm == nil {
		fyne.Do(fn)
		return
	}
	select {
	case dm.requests <- fn:
	default:
		// Канал переполнен - выполняем немедленно
		fyne.Do(fn)
	}
}

var de *DoMonitor
