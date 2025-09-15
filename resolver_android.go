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

static jboolean IsDirectoryUri(JNIEnv* env, jobject activity, const char* uriStr) {
    jboolean isDirectory = JNI_FALSE;

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

    if (uri == NULL) {
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    // 3. Query for MIME type
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

    // Projection for MIME type
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);
    (*env)->DeleteLocalRef(env, mimeTypeCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (cursor == NULL) {
        return JNI_FALSE;
    }

    // 4. Check MIME type
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        jstring mimeType = (*env)->CallObjectMethod(env, cursor, getString, 0);
        if (mimeType != NULL) {
            const char* mimeTypeStr = (*env)->GetStringUTFChars(env, mimeType, NULL);

            // Check if MIME type indicates a directory
            if (strcmp(mimeTypeStr, "vnd.android.document/directory") == 0) {
                isDirectory = JNI_TRUE;
            }

            (*env)->ReleaseStringUTFChars(env, mimeType, mimeTypeStr);
            (*env)->DeleteLocalRef(env, mimeType);
        }
    }

    (*env)->DeleteLocalRef(env, cursor);
    return isDirectory;
}

static jboolean CanListDirectory(JNIEnv* env, jobject activity, const char* uriStr) {
    // 1. First check if it's a directory
    // if (!IsDirectoryUri(env, activity, uriStr)) {
    //     return JNI_FALSE;
    // }

    jboolean canList = JNI_FALSE;

    // 2. Get ContentResolver (только если это директория)
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);

    // 3. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) {
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    // 4. Try to list children using DocumentsContract
    jclass documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    // 5. Try to build child documents URI
    jmethodID buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");

    if (buildChildDocumentsUriMethod != NULL) {
        jobject childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
            buildChildDocumentsUriMethod, uri, NULL);

        if (childUri != NULL) {
            canList = JNI_TRUE;
            (*env)->DeleteLocalRef(env, childUri);
        }
    }

    // 6. Alternative approach: try to query
    if (!canList) {
        jmethodID queryMethod = (*env)->GetMethodID(env, (*env)->GetObjectClass(env, contentResolver),
            "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

        if (queryMethod != NULL) {
            jclass stringClass = (*env)->FindClass(env, "java/lang/String");
            jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
            jstring colName = (*env)->NewStringUTF(env, "document_id");
            (*env)->SetObjectArrayElement(env, projection, 0, colName);
            (*env)->DeleteLocalRef(env, colName);

            jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
                uri, projection, NULL, NULL, NULL);

            if (cursor != NULL) {
                canList = JNI_TRUE;
                (*env)->DeleteLocalRef(env, cursor);
            }
            (*env)->DeleteLocalRef(env, projection);
        }
    }

    // Cleanup
    (*env)->DeleteLocalRef(env, documentsContractClass);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    return canList;
}

static char* getMimeTypeFromUri(JNIEnv* env, jobject activity, const char* uriStr) {
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

    if (uri == NULL) {
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    // 3. Query for MIME type
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

    // Projection for MIME type
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);
    (*env)->DeleteLocalRef(env, mimeTypeCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    char* result = NULL;

    if (cursor != NULL) {
        // 4. Get MIME type
        jclass cursorClass = (*env)->GetObjectClass(env, cursor);
        jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
        jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

        if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
            jstring mimeType = (*env)->CallObjectMethod(env, cursor, getString, 0);
            if (mimeType != NULL) {
                const char* mimeTypeStr = (*env)->GetStringUTFChars(env, mimeType, NULL);
                result = strdup(mimeTypeStr);
                (*env)->ReleaseStringUTFChars(env, mimeType, mimeTypeStr);
                (*env)->DeleteLocalRef(env, mimeType);
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }

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
	"fyne.io/fyne/v2/storage"
)

// CanList проверяет, можно ли получить список файлов в указанном URI
func CanList(u fyne.URI) (bool, error) {
	if u == nil {
		return false, nil
	}

	if apiLevel() > 28 {
		return storage.CanList(u)

	}
	var canList C.jboolean = C.JNI_FALSE

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(u.String())
		defer C.free(unsafe.Pointer(uriStr))

		// Используем нативную функцию для проверки возможности листинга
		canList = C.CanListDirectory(env, activity, uriStr)
		return nil
	})

	// Явно преобразуем jboolean (uint8) в bool
	return canList == C.JNI_TRUE, nil
}

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

// Проверка существования файла через ContentResolver
func fileExists(uri fyne.URI) (exists bool) {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cName := C.GetFileName(env, activity, uriStr)
		if cName != nil {
			exists = true
			C.free(unsafe.Pointer(cName))
		}
		return nil
	})

	return
}

// IsDirectory проверяет, является ли URI директорией
func IsDirectory(uri fyne.URI) bool {
	if uri == nil {
		return false
	}

	var isDirectory C.jboolean = C.JNI_FALSE

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		isDirectory = C.IsDirectoryUri(env, activity, uriStr)
		return nil
	})

	return isDirectory == C.JNI_TRUE
}

// MimeType возвращает MIME-тип для указанного URI через Android ContentResolver
func MimeType(u fyne.URI) string {
	if u == nil {
		return ""
	}

	uri := u.String()
	if uri == "" {
		return ""
	}

	var mimeType string
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		// Конвертируем Go string в C string
		curi := C.CString(uri)
		defer C.free(unsafe.Pointer(curi))

		// Вызываем нативную функцию для получения MIME-типа
		cmimeType := C.getMimeTypeFromUri(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			curi,
		)

		if cmimeType != nil {
			mimeType = C.GoString(cmimeType)
			C.free(unsafe.Pointer(cmimeType))
		}
		return nil
	})

	return mimeType
}
