//go:build android

// set_result_android.go
// func setResult(ok bool){}
package main

/*
#include <jni.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

static const jint RESULT_OK = -1;
static const jint RESULT_CANCELED = 0;

// Вспомогательная функция для проверки и очистки исключений
static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE; // Было исключение
    }
    return JNI_FALSE; // Не было исключение
}

// Функция для установки результата активности
static void setResult(JNIEnv* env, jobject activity, jint resultCode) {
    jclass activity_class = NULL;
    jmethodID setResult = NULL;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for setResult");
        return;
    }

    setResult = (*env)->GetMethodID(env, activity_class, "setResult", "(I)V");
    if (setResult == NULL) {
        LogD("C: ERROR - Failed to get setResult method");
        goto cleanup;
    }

    LogD("C: Setting result: %d", resultCode);

    (*env)->CallVoidMethod(env, activity, setResult, resultCode);

cleanup:
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

func setResult(ok bool) {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		var resultCode C.jint
		if ok {
			resultCode = -1 // RESULT_OK
		} else {
			resultCode = 0 // RESULT_CANCELED
		}

		log.Debugf("Calling C.setResult with code: %d", resultCode)

		C.setResult(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			resultCode,
		)

		log.Debug("C.setResult completed")
		return nil
	})
}
