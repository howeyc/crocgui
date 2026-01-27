//go:build android

// exclude_from_recents_android.go
// func excludeFromRecents(){}
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

// Функция для исключения активности из недавних приложений
static void excludeFromRecents(JNIEnv* env, jobject activity) {
    jclass activity_class = NULL;
    jmethodID finishAndRemoveTask = NULL;
    jmethodID finishAffinity = NULL;
    jmethodID finishMethod = NULL;

    jclass buildVersionClass = NULL;
    jfieldID sdkIntField = NULL;
    jint sdkInt = 0;

    // Получаем версию Android SDK
    buildVersionClass = (*env)->FindClass(env, "android/os/Build$VERSION");
    if (buildVersionClass != NULL) {
        sdkIntField = (*env)->GetStaticFieldID(env, buildVersionClass, "SDK_INT", "I");
        if (sdkIntField != NULL) {
            sdkInt = (*env)->GetStaticIntField(env, buildVersionClass, sdkIntField);
        }
        (*env)->DeleteLocalRef(env, buildVersionClass);
    }

    LogD("C: Android SDK: %d", sdkInt);

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for excludeFromRecents");
        return;
    }

    if (sdkInt >= 29) {
        // Android 10+ - используем finishAndRemoveTask
        finishAndRemoveTask = (*env)->GetMethodID(env, activity_class, "finishAndRemoveTask", "()V");
        if (finishAndRemoveTask != NULL) {
            LogD("C: Using finishAndRemoveTask (Android 10+)");

            // Безопасный вызов с обработкой исключений
            (*env)->CallVoidMethod(env, activity, finishAndRemoveTask);
            if (caseException(env, "finishAndRemoveTask")) {
                LogD("C: finishAndRemoveTask failed, falling back to finish()");
                // Fallback
                finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
                if (finishMethod != NULL) {
                    (*env)->CallVoidMethod(env, activity, finishMethod);
                    caseException(env, "finish fallback");
                }
            }
        } else {
            LogD("C: finishAndRemoveTask not available");
            // Fallback to finish()
            finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
            if (finishMethod != NULL) {
                (*env)->CallVoidMethod(env, activity, finishMethod);
                caseException(env, "finish");
            }
        }
    } else {
        // Android 9 и ниже
        LogD("C: Using finish() for Android < 10");
        finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
        if (finishMethod != NULL) {
            (*env)->CallVoidMethod(env, activity, finishMethod);
            caseException(env, "finish for Android < 10");
        } else {
            LogD("C: ERROR - No finish method found!");
        }
    }

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

// excludeFromRecents исключает приложение из списка недавних
func excludeFromRecents() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.excludeFromRecents")

		C.excludeFromRecents(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.excludeFromRecents completed")
		return nil
	})
}
