//go:build android

// finish_android.go
// func finish(){}
package main

/*
#include <jni.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

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

// Функция для завершения активности
static void finish(JNIEnv* env, jobject activity) {
    jclass activity_class = NULL;
    jmethodID finishMethod = NULL;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for finish");
        return;
    }

    finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
    if (finishMethod == NULL) {
        LogD("C: ERROR - Failed to get finish method");
        goto cleanup;
    }

    LogD("C: Finishing activity");
    (*env)->CallVoidMethod(env, activity, finishMethod);

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

// Это не закрывает приложение a и не закрывает окно w
// Просто открепляет окно w от активности  org.golang.app.GoNativeActivity
func finish() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.finish")

		C.finish(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.finish completed")
		return nil
	})
}
