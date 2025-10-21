//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

void LogD(const char* message);

static char* GetFileName(JNIEnv* env, jobject activity, const char* uriStr) {
    // 1. Get ContentResolver
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class in GetFileName");
        return NULL;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("C: ERROR - Failed to get getContentResolver method");
        (*env)->DeleteLocalRef(env, activityClass);
        return NULL;
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (contentResolver == NULL) {
        LogD("C: ERROR - contentResolver is NULL");
        (*env)->DeleteLocalRef(env, activityClass);
        return NULL;
    }

    // 2. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("C: ERROR - Failed to find Uri class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("C: ERROR - Failed to get parse method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return NULL;
    }

    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) {
        LogD("C: ERROR - Failed to parse URI");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return NULL;
    }

    // 3. Query ContentResolver
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("C: ERROR - Failed to get query method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return NULL;
    }

    // Create projection array
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("C: ERROR - Failed to find String class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return NULL;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring displayName = (*env)->NewStringUTF(env, "_display_name");
    (*env)->SetObjectArrayElement(env, projection, 0, displayName);
    (*env)->DeleteLocalRef(env, displayName);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, uriClass);

    if (cursor == NULL) {
        LogD("C: ERROR - cursor is NULL");
        return NULL;
    }

    // 4. Get file name
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    if (moveToFirst == NULL) {
        LogD("C: ERROR - Failed to get moveToFirst method");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    if (getString == NULL) {
        LogD("C: ERROR - Failed to get getString method");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

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
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class in IsDirectoryUri");
        return JNI_FALSE;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("C: ERROR - Failed to get getContentResolver method");
        (*env)->DeleteLocalRef(env, activityClass);
        return JNI_FALSE;
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (contentResolver == NULL) {
        LogD("C: ERROR - contentResolver is NULL");
        (*env)->DeleteLocalRef(env, activityClass);
        return JNI_FALSE;
    }

    // 2. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("C: ERROR - Failed to find Uri class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("C: ERROR - Failed to get parse method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return JNI_FALSE;
    }

    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) {
        LogD("C: ERROR - Failed to parse URI");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return JNI_FALSE;
    }

    // 3. Query for MIME type
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("C: ERROR - Failed to get query method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return JNI_FALSE;
    }

    // Projection for MIME type
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("C: ERROR - Failed to find String class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return JNI_FALSE;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);
    (*env)->DeleteLocalRef(env, mimeTypeCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, uriClass);

    if (cursor == NULL) {
        LogD("C: ERROR - cursor is NULL");
        return JNI_FALSE;
    }

    // 4. Check MIME type
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    if (moveToFirst == NULL) {
        LogD("C: ERROR - Failed to get moveToFirst method");
        (*env)->DeleteLocalRef(env, cursor);
        return JNI_FALSE;
    }

    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    if (getString == NULL) {
        LogD("C: ERROR - Failed to get getString method");
        (*env)->DeleteLocalRef(env, cursor);
        return JNI_FALSE;
    }

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
    jboolean canList = JNI_FALSE;

    // 1. Get ContentResolver
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class in CanListDirectory");
        return JNI_FALSE;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("C: ERROR - Failed to get getContentResolver method");
        (*env)->DeleteLocalRef(env, activityClass);
        return JNI_FALSE;
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (contentResolver == NULL) {
        LogD("C: ERROR - contentResolver is NULL");
        (*env)->DeleteLocalRef(env, activityClass);
        return JNI_FALSE;
    }

    // 2. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("C: ERROR - Failed to find Uri class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("C: ERROR - Failed to get parse method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return JNI_FALSE;
    }

    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) {
        LogD("C: ERROR - Failed to parse URI");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return JNI_FALSE;
    }

    // 3. Try to list children using DocumentsContract
    jclass documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("C: ERROR - Failed to find DocumentsContract class");
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return JNI_FALSE;
    }

    // 4. Try to build child documents URI
    jmethodID buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    if (buildChildDocumentsUriMethod == NULL) {
        LogD("C: ERROR - Failed to get buildChildDocumentsUriUsingTree method");
    } else {
        jobject childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
            buildChildDocumentsUriMethod, uri, NULL);

        if (childUri != NULL) {
            canList = JNI_TRUE;
            (*env)->DeleteLocalRef(env, childUri);
        }
    }

    // 5. Alternative approach: try to query
    if (!canList) {
        jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
        jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass,
            "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
        if (queryMethod == NULL) {
            LogD("C: ERROR - Failed to get query method for alternative approach");
        } else {
            jclass stringClass = (*env)->FindClass(env, "java/lang/String");
            if (stringClass == NULL) {
                LogD("C: ERROR - Failed to find String class for alternative approach");
            } else {
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
    }

    // Cleanup
    (*env)->DeleteLocalRef(env, documentsContractClass);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, uriClass);

    return canList;
}

static char* getMimeTypeFromUri(JNIEnv* env, jobject activity, const char* uriStr) {
    // 1. Get ContentResolver
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class in getMimeTypeFromUri");
        return NULL;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("C: ERROR - Failed to get getContentResolver method");
        (*env)->DeleteLocalRef(env, activityClass);
        return NULL;
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (contentResolver == NULL) {
        LogD("C: ERROR - contentResolver is NULL");
        (*env)->DeleteLocalRef(env, activityClass);
        return NULL;
    }

    // 2. Parse URI
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("C: ERROR - Failed to find Uri class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("C: ERROR - Failed to get parse method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return NULL;
    }

    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (uri == NULL) {
        LogD("C: ERROR - Failed to parse URI");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        return NULL;
    }

    // 3. Query for MIME type
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("C: ERROR - Failed to get query method");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return NULL;
    }

    // Projection for MIME type
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("C: ERROR - Failed to find String class");
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, uri);
        return NULL;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);
    (*env)->DeleteLocalRef(env, mimeTypeCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Cleanup
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, uriClass);

    char* result = NULL;

    if (cursor == NULL) {
        LogD("C: ERROR - cursor is NULL");
        return NULL;
    }

    // 4. Get MIME type
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    if (moveToFirst == NULL) {
        LogD("C: ERROR - Failed to get moveToFirst method");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    if (getString == NULL) {
        LogD("C: ERROR - Failed to get getString method");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

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
