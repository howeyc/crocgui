//go:build android

// start_activity_android.go
// func startActivity(){}
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

// Упрощенная версия - всегда использует класс текущей активности
static void startActivitySimple(JNIEnv* env, jobject activity) {
    jclass activityClass = NULL;
    jclass intentClass = NULL;
    jobject intent = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class");
        return;
    }

    intentClass = (*env)->FindClass(env, "android/content/Intent");
    if (intentClass == NULL) {
        LogD("C: ERROR - Failed to find Intent class");
        goto cleanup_activity;
    }

    // Создаем Intent для текущего класса активности
    jmethodID newIntent = (*env)->GetMethodID(env, intentClass, "<init>", "(Landroid/content/Context;Ljava/lang/Class;)V");
    if (newIntent == NULL) {
        LogD("C: ERROR - Failed to get Intent constructor");
        goto cleanup_intent;
    }

    intent = (*env)->NewObject(env, intentClass, newIntent, activity, activityClass);
    if (intent == NULL) {
        LogD("C: ERROR - Failed to create Intent");
        goto cleanup_intent;
    }

    LogD("C: Intent created successfully");

    // Устанавливаем флаги для перезапуска
    jmethodID setFlags = (*env)->GetMethodID(env, intentClass, "setFlags", "(I)Landroid/content/Intent;");
    if (setFlags != NULL) {
        (*env)->CallObjectMethod(env, intent, setFlags, 0x20000000 | 0x10000000 | 0x04000000); // SINGLE_TOP | NEW_TASK | CLEAR_TOP
        LogD("C: Flags set: SINGLE_TOP | NEW_TASK | CLEAR_TOP");
        caseException(env, "setFlags");
    }

    // Запускаем активность
    jmethodID startActivity = (*env)->GetMethodID(env, activityClass, "startActivity", "(Landroid/content/Intent;)V");
    if (startActivity != NULL) {
        LogD("C: Starting activity");
        (*env)->CallVoidMethod(env, activity, startActivity, intent);
        caseException(env, "startActivity");
        LogD("C: Activity started successfully");
    } else {
        LogD("C: ERROR - Failed to get startActivity method");
    }

    // Освобождение ресурсов
    if (intent) {
        (*env)->DeleteLocalRef(env, intent);
    }

cleanup_intent:
    if (intentClass) {
        (*env)->DeleteLocalRef(env, intentClass);
    }

cleanup_activity:
    if (activityClass) {
        (*env)->DeleteLocalRef(env, activityClass);
    }
}
*/
import "C"
import (
	"time"
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// Это просто связывает окно w
// c активностью org.golang.app.GoNativeActivity
func startActivity() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		log.Debug("Calling C.startActivitySimple")

		// Используем упрощенную версию
		C.startActivitySimple(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.startActivitySimple completed")
		return nil
	})
}

// Это просто связывает окно w
// c активностью org.golang.app.GoNativeActivity
func start() {
	// excludeFromRecents()
	finish()
	sendNotification(nil, "Croc", AppClosed)
	time.Sleep(300 * time.Millisecond)
	startActivity()
}
