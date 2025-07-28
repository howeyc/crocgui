//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static char* GetFileName(JNIEnv* env, jobject activity, const char* uriStr) {
    // 1. Get ContentResolver
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);

    // 2. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) return NULL;

    // 3. Query ContentResolver
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

    // Create projection array
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring displayName = (*env)->NewStringUTF(env, "_display_name");
    (*env)->SetObjectArrayElement(env, projection, 0, displayName);
    (*env)->DeleteLocalRef(env, displayName);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (cursor == NULL) return NULL;

    // 4. Get file name
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

    char* result = NULL;
    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        jstring name = (*env)->CallObjectMethod(env, cursor, getString, 0);
        if (name != NULL) {
            const char* utfStr = (*env)->GetStringUTFChars(env, name, NULL);
            result = strdup(utfStr);
            (*env)->ReleaseStringUTFChars(env, name, utfStr);
            (*env)->DeleteLocalRef(env, name);
        }
    }

    (*env)->DeleteLocalRef(env, cursor);
    return result;
}
*/
import "C"
import (
	"net/url"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

func uriBase(uri fyne.URI) string {
	var fileName string

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cName := C.GetFileName(env, activity, uriStr)
		if cName != nil {
			fileName = C.GoString(cName)
			C.free(unsafe.Pointer(cName))
		}
		return nil
	})

	if fileName == "" {
		return base(uri.Path())
	}

	return fileName
}

func base(path string) string {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		decoded = strings.ReplaceAll(path, "%2F", "/")
	}

	lastSlash := strings.LastIndex(decoded, "/")
	if lastSlash < 0 {
		return replace(decoded)
	}

	return replace(decoded[lastSlash+1:])
}

func replace(s string) string {
	return strings.NewReplacer(
		"?", "_",
		":", "_",
	).Replace(s)
}
