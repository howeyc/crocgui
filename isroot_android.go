//go:build android

// isroot_android.go
package main

/*
#include <jni.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)
static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

static jboolean isTaskRoot(JNIEnv *env, jobject activity) {
    jclass activityClass = NULL;
    jmethodID isTaskRootMethod = NULL;
    jboolean result = JNI_FALSE;

    if (!activity) {
        LogD("isTaskRoot: activity is NULL");
        return JNI_FALSE;
    }

    activityClass = (*env)->GetObjectClass(env, activity);
    if (caseException(env, "GetObjectClass") || !activityClass) {
        return JNI_FALSE;
    }

    isTaskRootMethod = (*env)->GetMethodID(env, activityClass, "isTaskRoot", "()Z");
    if (caseException(env, "GetMethodID isTaskRoot") || !isTaskRootMethod) {
        (*env)->DeleteLocalRef(env, activityClass);
        return JNI_FALSE;
    }

    result = (*env)->CallBooleanMethod(env, activity, isTaskRootMethod);
    if (caseException(env, "CallBooleanMethod isTaskRoot")) {
        result = JNI_FALSE;
    }

    (*env)->DeleteLocalRef(env, activityClass);
    return result;
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

func IsTaskRoot() bool {
	var result bool = false

	driver.RunNative(func(ctx interface{}) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return nil
		}

		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))

		res := C.isTaskRoot(
			env,
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		result = (res == C.JNI_TRUE)
		return nil
	})

	return result
}
