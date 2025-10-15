//go:build android

package main

/*
#include "notification_android.h"
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

func showCrocNotification(title, content string) {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		ctitle := C.CString(title)
		ccontent := C.CString(content)
		defer C.free(unsafe.Pointer(ctitle))
		defer C.free(unsafe.Pointer(ccontent))

		C.showCrocNotification(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			ctitle,
			ccontent,
		)

		return nil
	})
}

func sendNotification(a fyne.App, title, content string) {
	showCrocNotification(title, content)
}

func LogD(message string) {
	ctag := C.CString("croc")
	cmessage := C.CString(message)
	defer C.free(unsafe.Pointer(ctag))
	defer C.free(unsafe.Pointer(cmessage))

	C.logToAndroid(ctag, cmessage)
}
