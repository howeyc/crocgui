//go:build darwin

package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <CoreFoundation/CoreFoundation.h>

static IOPMAssertionID assertionID = 0;

int preventSleep(int on) {
    if (on) {
        if (assertionID != 0) return 0; // уже включено

        CFStringRef reasonForActivity = CFSTR("crocgui transfer in progress");
        IOReturn success = IOPMAssertionCreateWithName(
            kIOPMAssertionTypeNoIdleSleep,
            kIOPMAssertionLevelOn,
            reasonForActivity,
            &assertionID
        );
        return (success == kIOReturnSuccess) ? 0 : -1;
    } else {
        if (assertionID == 0) return 0; // уже выключено

        IOReturn success = IOPMAssertionRelease(assertionID);
        assertionID = 0;
        return (success == kIOReturnSuccess) ? 0 : -1;
    }
}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"sync/atomic"

	"fyne.io/fyne/v2"
)

// caffeinate управляет энергосбережением macOS
// i:  1 - увеличить счётчик (запретить сон)
//
//	-1 - уменьшить счётчик (разрешить сон при счётчике 0)
//	 0 - сбросить счётчик (принудительно разрешить сон)
//	 n - увеличить на n (n > 0)
//	-n - уменьшить на n (n < 0)
//
// Возвращает текущее значение счётчика после операции
func caffeinate(i int32) int32 {
	old := atomic.LoadInt32(&sleepCounter)
	newVal := atomic.AddInt32(&sleepCounter, i)
	if i == 0 {
		atomic.StoreInt32(&sleepCounter, 0)
		newVal = 0
	}

	// Управление ассертом через очередь Fyne (вместо мьютекса в C)
	fyne.Do(func() {
		if old <= 0 && newVal > 0 {
			C.preventSleep(1)
		} else if old > 0 && newVal <= 0 {
			C.preventSleep(0)
		}
	})

	return newVal
}

// SleepAllowed возвращает true если сон разрешён (счётчик <= 0)
func SleepAllowed() bool {
	return atomic.LoadInt32(&sleepCounter) <= 0
}

// registered проверяет, зарегистрирована ли схема в системе macOS
func registered(scheme string) bool {
	// AppleScript напрямую спрашивает у системы обработчик для схемы
	script := fmt.Sprintf(`tell app "Finder" to get default application for url "%s://test"`, scheme)

	// Если команда успешна (код 0) - обработчик есть
	return exec.Command("osascript", "-e", script).Run() == nil
}

func CanCreateSymlinks() bool {
	return true
}
