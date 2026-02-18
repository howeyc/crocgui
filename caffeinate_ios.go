//go:build ios

package main

/*
#include <Foundation/Foundation.h>
#include <UIKit/UIKit.h>

// Устанавливает флаг предотвращения сна
void setIdleTimerDisabled(BOOL disabled) {
    @autoreleasepool {
        [[UIApplication sharedApplication] setIdleTimerDisabled:disabled];
    }
}
*/
import "C"
import (
	"sync/atomic"

	"fyne.io/fyne/v2"
)

func caffeinate(i int32) int32 {
	old := atomic.LoadInt32(&sleepCounter)
	var newVal int32

	if i == 0 {
		atomic.StoreInt32(&sleepCounter, 0)
		newVal = 0
	} else {
		newVal = atomic.AddInt32(&sleepCounter, i)
	}

	// Управляем флагом через очередь Fyne для потокобезопасности
	fyne.Do(func() {
		if old <= 0 && newVal > 0 {
			// Включаем блокировку сна
			C.setIdleTimerDisabled(C.YES)
		} else if old > 0 && newVal <= 0 {
			// Выключаем блокировку сна
			C.setIdleTimerDisabled(C.NO)
		}
	})

	return newVal
}

func SleepAllowed() bool {
	return atomic.LoadInt32(&sleepCounter) <= 0
}
