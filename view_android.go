//go:build ignore

// view_android.go

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

void LogD(const char* message);

static char* OpenFileInDefaultApp(JNIEnv* env, jobject context, const char* uriString, const char* mimeType) {
    jclass context_class = NULL;
    jclass intent_class = NULL;
    jclass uri_class = NULL;
    jstring action_str = NULL;
    jobject intent = NULL;
    jstring uri_str = NULL;
    jobject uri_obj = NULL;
    jstring mime_type_str = NULL;

    // Проверяем исключения в начале
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionClear(env);
        return strdup("error: Java exception already pending");
    }

    context_class = (*env)->GetObjectClass(env, context);
    if (context_class == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        return strdup("error: failed to get context class");
    }

    intent_class = (*env)->FindClass(env, "android/content/Intent");
    if (intent_class == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, context_class);
        return strdup("error: failed to find Intent class");
    }

    uri_class = (*env)->FindClass(env, "android/net/Uri");
    if (uri_class == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        return strdup("error: failed to find Uri class");
    }

    // Создаем Intent с action ACTION_VIEW
    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "(Ljava/lang/String;)V");
    if (intent_constructor == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to get Intent constructor");
    }

    action_str = (*env)->NewStringUTF(env, "android.intent.action.VIEW");
    intent = (*env)->NewObject(env, intent_class, intent_constructor, action_str);
    (*env)->DeleteLocalRef(env, action_str);

    if (intent == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to create Intent object");
    }

    // Парсим строку URI в объект Uri
    jmethodID parse_method = (*env)->GetStaticMethodID(env, uri_class, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parse_method == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to get Uri parse method");
    }

    uri_str = (*env)->NewStringUTF(env, uriString);
    uri_obj = (*env)->CallStaticObjectMethod(env, uri_class, parse_method, uri_str);
    (*env)->DeleteLocalRef(env, uri_str);

    if (uri_obj == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to parse URI string");
    }

    // Проверяем исключения после парсинга URI
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: exception during URI parsing");
    }

    // Устанавливаем данные и тип для Intent
    jmethodID set_data_and_type_method = (*env)->GetMethodID(env, intent_class, "setDataAndType", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/content/Intent;");
    if (set_data_and_type_method == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to get setDataAndType method");
    }

    mime_type_str = (*env)->NewStringUTF(env, mimeType);
    jobject result_intent = (*env)->CallObjectMethod(env, intent, set_data_and_type_method, uri_obj, mime_type_str);
    (*env)->DeleteLocalRef(env, mime_type_str);

    if (result_intent != NULL) {
        (*env)->DeleteLocalRef(env, result_intent);
    }

    // Проверяем исключения после setDataAndType
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: exception during setDataAndType");
    }

    // Добавляем флаг FLAG_ACTIVITY_NEW_TASK
    jmethodID add_flags_method = (*env)->GetMethodID(env, intent_class, "addFlags", "(I)Landroid/content/Intent;");
    if (add_flags_method == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to get addFlags method");
    }

    jint flags = 0x10000000; // FLAG_ACTIVITY_NEW_TASK
    jobject flag_result = (*env)->CallObjectMethod(env, intent, add_flags_method, flags);
    if (flag_result != NULL) {
        (*env)->DeleteLocalRef(env, flag_result);
    }

    // Проверяем исключения после addFlags
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: exception during addFlags");
    }

    // Запускаем Activity
    jmethodID start_activity_method = (*env)->GetMethodID(env, context_class, "startActivity", "(Landroid/content/Intent;)V");
    if (start_activity_method == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        (*env)->DeleteLocalRef(env, uri_obj);
        (*env)->DeleteLocalRef(env, intent);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: failed to get startActivity method");
    }

    (*env)->CallVoidMethod(env, context, start_activity_method, intent);

    // Проверяем исключения после startActivity (самая важная проверка!)
    if ((*env)->ExceptionCheck(env)) {
        jthrowable exception = (*env)->ExceptionOccurred(env);
        if (exception != NULL) {
            jclass exception_class = (*env)->GetObjectClass(env, exception);
            jclass class_class = (*env)->FindClass(env, "java/lang/Class");
            jmethodID get_name_method = (*env)->GetMethodID(env, class_class, "getName", "()Ljava/lang/String;");

            jstring class_name = (*env)->CallObjectMethod(env, exception_class, get_name_method);
            const char* class_name_str = (*env)->GetStringUTFChars(env, class_name, NULL);

            char error_msg[512];
            snprintf(error_msg, sizeof(error_msg), "error: startActivity failed: %s", class_name_str);

            (*env)->ReleaseStringUTFChars(env, class_name, class_name_str);
            (*env)->DeleteLocalRef(env, class_name);
            (*env)->DeleteLocalRef(env, exception_class);
            (*env)->DeleteLocalRef(env, class_class);
            (*env)->ExceptionClear(env);

            (*env)->DeleteLocalRef(env, uri_obj);
            (*env)->DeleteLocalRef(env, intent);
            (*env)->DeleteLocalRef(env, context_class);
            (*env)->DeleteLocalRef(env, intent_class);
            (*env)->DeleteLocalRef(env, uri_class);

            return strdup(error_msg);
        }
    }

    // Очищаем ресурсы
    (*env)->DeleteLocalRef(env, uri_obj);
    (*env)->DeleteLocalRef(env, intent);
    (*env)->DeleteLocalRef(env, intent_class);
    (*env)->DeleteLocalRef(env, uri_class);
    (*env)->DeleteLocalRef(env, context_class);

    return NULL; // Успех
}
*/
import "C"
import (
	"fmt"
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// OpenFileInDefaultApp запускает приложение по умолчанию для открытия файла по URI.
func OpenFileInDefaultApp(uriString, mimeType string) error {
	log.Debugf("Opening file in default app: URI=%s, MIME=%s", uriString, mimeType)

	var resultErr error

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		context := C.jobject(unsafe.Pointer(ac.Ctx))

		cURIString := C.CString(uriString)
		defer C.free(unsafe.Pointer(cURIString))

		cMimeType := C.CString(mimeType)
		defer C.free(unsafe.Pointer(cMimeType))

		// Вызываем C функцию, которая возвращает ошибку как строку
		cError := C.OpenFileInDefaultApp(env, context, cURIString, cMimeType)
		if cError != nil {
			defer C.free(unsafe.Pointer(cError))
			errorMsg := C.GoString(cError)
			resultErr = fmt.Errorf("native error: %s", errorMsg)
		}

		return nil
	})

	return resultErr
}
