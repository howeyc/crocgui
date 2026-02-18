//go:build android

package main

/*
#include <jni.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)
#define LogE(...) __android_log_print(ANDROID_LOG_ERROR, "croc", __VA_ARGS__)

static jobject globalWakeLock = NULL;

static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogE("Exception in %s", context);
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
    if (caseException(env, "GetMethodID getSystemService")) return;

    jclass contextClass = (*env)->FindClass(env, "android/content/Context");
    if (caseException(env, "FindClass Context")) return;

    jfieldID powerServiceField = (*env)->GetStaticFieldID(env, contextClass, "POWER_SERVICE", "Ljava/lang/String;");
    if (caseException(env, "GetStaticFieldID POWER_SERVICE")) return;

    jstring powerServiceString = (*env)->GetStaticObjectField(env, contextClass, powerServiceField);
    if (caseException(env, "GetStaticObjectField POWER_SERVICE")) return;

    jobject powerManager = (*env)->CallObjectMethod(env, activity, getSystemServiceMethod, powerServiceString);
    if (caseException(env, "CallObjectMethod getSystemService") || powerManager == NULL) return;

    jclass powerManager_class = (*env)->GetObjectClass(env, powerManager);
    if (caseException(env, "GetObjectClass PowerManager")) return;

    jmethodID newWakeLockMethod = (*env)->GetMethodID(env, powerManager_class,
        "newWakeLock", "(ILjava/lang/String;)Landroid/os/PowerManager$WakeLock;");
    if (caseException(env, "GetMethodID newWakeLock")) return;

    jstring tag = (*env)->NewStringUTF(env, "crocgui:transfer");
    if (caseException(env, "NewStringUTF")) return;

    jobject localWakeLock = (*env)->CallObjectMethod(env, powerManager, newWakeLockMethod, 1, tag);
    (*env)->DeleteLocalRef(env, tag);

    if (caseException(env, "newWakeLock") || localWakeLock == NULL) return;

    globalWakeLock = (*env)->NewGlobalRef(env, localWakeLock);
    if (globalWakeLock == NULL) {
        LogE("Failed to create global reference");
        return;
    }

    jclass wakeLock_class = (*env)->GetObjectClass(env, globalWakeLock);
    if (caseException(env, "GetObjectClass WakeLock")) return;

    jmethodID acquireMethod = (*env)->GetMethodID(env, wakeLock_class, "acquire", "()V");
    if (caseException(env, "GetMethodID acquire")) return;

    if (acquireMethod != NULL) {
        (*env)->CallVoidMethod(env, globalWakeLock, acquireMethod);
        if (caseException(env, "CallVoidMethod acquire")) return;
        LogD("WakeLock acquired");
    }
}

static void releaseWakeLock(JNIEnv* env, jobject activity) {
    if (globalWakeLock == NULL) return;

    jclass wakeLock_class = (*env)->GetObjectClass(env, globalWakeLock);
    if (caseException(env, "GetObjectClass WakeLock")) return;

    jmethodID releaseMethod = (*env)->GetMethodID(env, wakeLock_class, "release", "()V");
    if (caseException(env, "GetMethodID release")) return;

    if (releaseMethod != NULL) {
        (*env)->CallVoidMethod(env, globalWakeLock, releaseMethod);
        if (caseException(env, "CallVoidMethod release")) return;
        LogD("WakeLock released");
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
