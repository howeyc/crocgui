//go:build android && !linux

package main

/*
#include <jni.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

// Глобальная ссылка на WakeLock, чтобы мы могли вызвать release на том же объекте
static jobject globalWakeLock = NULL;

static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

static void acquireWakeLock(JNIEnv* env, jobject activity) {
    if (globalWakeLock != NULL) return;

    jclass activity_class = (*env)->GetObjectClass(env, activity);
    jmethodID getSystemServiceMethod = (*env)->GetMethodID(env, activity_class,
        "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");

    jclass contextClass = (*env)->FindClass(env, "android/content/Context");
    jfieldID powerServiceField = (*env)->GetStaticFieldID(env, contextClass, "POWER_SERVICE", "Ljava/lang/String;");
    jstring powerServiceString = (*env)->GetStaticObjectField(env, contextClass, powerServiceField);

    jobject powerManager = (*env)->CallObjectMethod(env, activity, getSystemServiceMethod, powerServiceString);
    jclass powerManager_class = (*env)->GetObjectClass(env, powerManager);

    // PARTIAL_WAKE_LOCK = 1
    jmethodID newWakeLockMethod = (*env)->GetMethodID(env, powerManager_class,
        "newWakeLock", "(ILjava/lang/String;)Landroid/os/PowerManager$WakeLock;");

    jstring tag = (*env)->NewStringUTF(env, "crocgui:transfer");
    jobject localWakeLock = (*env)->CallObjectMethod(env, powerManager, newWakeLockMethod, 1, tag);
    (*env)->DeleteLocalRef(env, tag);

    if (caseException(env, "newWakeLock") || localWakeLock == NULL) return;

    // СОХРАНЯЕМ КАК GLOBAL REF
    globalWakeLock = (*env)->NewGlobalRef(env, localWakeLock);

    jclass wakeLock_class = (*env)->GetObjectClass(env, globalWakeLock);
    jmethodID acquireMethod = (*env)->GetMethodID(env, wakeLock_class, "acquire", "()V");

    if (acquireMethod != NULL) {
        (*env)->CallVoidMethod(env, globalWakeLock, acquireMethod);
        LogD("WakeLock Global Acquired");
    }
}

static void releaseWakeLock(JNIEnv* env, jobject activity) {
    if (globalWakeLock == NULL) return;

    jclass wakeLock_class = (*env)->GetObjectClass(env, globalWakeLock);
    jmethodID releaseMethod = (*env)->GetMethodID(env, wakeLock_class, "release", "()V");

    if (releaseMethod != NULL) {
        (*env)->CallVoidMethod(env, globalWakeLock, releaseMethod);
        LogD("WakeLock Global Released");
    }

    (*env)->DeleteGlobalRef(env, globalWakeLock);
    globalWakeLock = NULL;
}
*/
import "C"
import (
	"sync/atomic"
	"unsafe"

	"fyne.io/fyne/v2/driver"
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

	// В Android driver.RunNative гарантирует выполнение в потоке JNI,
	// так что дополнительный fyne.Do не обязателен, но допустим.
	if old <= 0 && newVal > 0 {
		acquireWakeLock()
	} else if old > 0 && newVal <= 0 {
		releaseWakeLock()
	}

	return newVal
}

func SleepAllowed() bool {
	return atomic.LoadInt32(&sleepCounter) <= 0
}

func acquireWakeLock() {
	driver.RunNative(func(ctx interface{}) error {
		if ac, ok := ctx.(*driver.AndroidContext); ok {
			C.acquireWakeLock((*C.JNIEnv)(unsafe.Pointer(ac.Env)), (C.jobject)(unsafe.Pointer(ac.Ctx)))
		}
		return nil
	})
}

func releaseWakeLock() {
	driver.RunNative(func(ctx interface{}) error {
		if ac, ok := ctx.(*driver.AndroidContext); ok {
			C.releaseWakeLock((*C.JNIEnv)(unsafe.Pointer(ac.Env)), (C.jobject)(unsafe.Pointer(ac.Ctx)))
		}
		return nil
	})
}
