//go:build android

package main

/*
#include <android/log.h>
#include <stdlib.h>

void LogD(const char* message) {
	__android_log_write(ANDROID_LOG_DEBUG, "croc", message);
}
*/
import "C"
import "unsafe"

func LogD(message string) {
	cmessage := C.CString(message)
	defer C.free(unsafe.Pointer(cmessage))

	C.LogD(cmessage)
}
