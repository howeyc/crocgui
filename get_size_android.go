//go:build android

// get_size_android.go
// func getSize(uri fyne.URI) (size int64, err error) {return 0, nil}
package main

/*
#include <jni.h>
#include <string.h>
#include <android/log.h>
#include <stdlib.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

// Вспомогательная функция для проверки и очистки исключений
static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

// getSize пытается получить размер файла
static jlong getSize(JNIEnv* env, jobject activity, const char* uriStr) {
    jlong size = -1;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass openableColumnsClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jstring juriStr = NULL;
    jstring sizeCol = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getSize: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getSize: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getSize: Failed to get contentResolver");
        goto cleanup;
    }

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getSize: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getSize: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getSize: Failed to parse URI");
        goto cleanup;
    }

    openableColumnsClass = (*env)->FindClass(env, "android/provider/OpenableColumns");
    if (openableColumnsClass == NULL) {
        LogD("getSize: Failed to find OpenableColumns class");
        goto cleanup;
    }

    jfieldID sizeFieldID = (*env)->GetStaticFieldID(env, openableColumnsClass, "SIZE", "Ljava/lang/String;");
    if (sizeFieldID == NULL) {
        LogD("getSize: Failed to get SIZE field ID");
        goto cleanup;
    }

    sizeCol = (*env)->GetStaticObjectField(env, openableColumnsClass, sizeFieldID);

    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getSize: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getSize: Failed to get query method");
        goto cleanup;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getSize: Failed to find String class");
        goto cleanup;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    (*env)->SetObjectArrayElement(env, projection, 0, sizeCol);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);
    if (caseException(env, "query for size") || cursor == NULL) {
        LogD("getSize: SIZE query returned NULL cursor");
        goto cleanup;
    }

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getSize: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getLong = (*env)->GetMethodID(env, cursorClass, "getLong", "(I)J");

    if (moveToFirst == NULL || getLong == NULL) {
        LogD("getSize: Failed to get cursor methods");
        goto cleanup;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        size = (*env)->CallLongMethod(env, cursor, getLong, 0);
        if (caseException(env, "getLong for size")) {
            size = -1;
        } else {
            LogD("getSize: Got size: %lld", (long long)size);
        }
    } else {
        LogD("getSize: No size available");
    }

cleanup:
    if (cursor) {
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in getSize");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (sizeCol) (*env)->DeleteLocalRef(env, sizeCol);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (openableColumnsClass) (*env)->DeleteLocalRef(env, openableColumnsClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass) (*env)->DeleteLocalRef(env, cursorClass);

    return size;
}
*/
import "C"
import (
	"fmt"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// getSize возвращает размер файла в байтах
func getSize(uri fyne.URI) (size int64, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cSize := C.getSize(env, activity, uriStr)
		size = int64(cSize)

		if size == -1 {
			size = 0
			err = fmt.Errorf("failed to get size")
		}

		return nil
	})

	return size, err
}
