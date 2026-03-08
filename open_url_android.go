//go:build android

// open_url_android.go
// func OpenURL(intentStr string) error{return nil}
package main

/*
#include <jni.h>
#include <android/log.h>
#include <stdlib.h>

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

static jboolean openIntent(JNIEnv *env, jobject activity, const char *intentUriChars) {
    jclass intentClass = NULL;
    jclass contextClass = NULL;
    jobject intent = NULL;
    jboolean success = JNI_FALSE;
    jstring intentUriString = NULL;

    intentUriString = (*env)->NewStringUTF(env, intentUriChars);
    if (caseException(env, "NewStringUTF")) goto cleanup;

    intentClass = (*env)->FindClass(env, "android/content/Intent");
    if (caseException(env, "FindClass Intent")) goto cleanup;

    jmethodID parseUriMethod = (*env)->GetStaticMethodID(env, intentClass, "parseUri", "(Ljava/lang/String;I)Landroid/content/Intent;");
    intent = (*env)->CallStaticObjectMethod(env, intentClass, parseUriMethod, intentUriString, (jint)1);
    if (caseException(env, "parseUri") || !intent) goto cleanup;

    // jmethodID addFlagsMethod = (*env)->GetMethodID(env, intentClass, "addFlags", "(I)Landroid/content/Intent;");
    // if (addFlagsMethod) {
    //     (*env)->CallObjectMethod(env, intent, addFlagsMethod, (jint)0x10000000);
    // }

    contextClass = (*env)->GetObjectClass(env, activity);
    jmethodID startActivityMethod = (*env)->GetMethodID(env, contextClass, "startActivity", "(Landroid/content/Intent;)V");
    (*env)->CallVoidMethod(env, activity, startActivityMethod, intent);

    if (!caseException(env, "startActivity")) {
        success = JNI_TRUE;
    }

cleanup:
    if (intentUriString) (*env)->DeleteLocalRef(env, intentUriString);
    if (intent) (*env)->DeleteLocalRef(env, intent);
    if (intentClass) (*env)->DeleteLocalRef(env, intentClass);
    if (contextClass) (*env)->DeleteLocalRef(env, contextClass);
    return success;
}
*/
import "C"
import (
	"fmt"
	"net/url"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// OpenURL запускает строку интента на Android.
//intent://storage/emulated/0/Download/qr_code.png#Intent;
// action=android.intent.action.SEND;
// category=android.intent.category.DEFAULT;
// category=android.intent.category.BROWSABLE;
// scheme=file;type=image/png;
// package=com.example.advanced_scanner;
// component=com.example.advanced_scanner/.EditorActivity;
// launchFlags=0x10000020;S.user_name=Admin%20User;
// i.retry_count=3;
// l.timestamp=1737984000000;
// f.zoom_level=1.5;
// d.precision=0.0000001;
// b.is_debug=1;
// B.force_update=false;
// s.buffer_size=1024;
// c.marker=Q;
// SEL;
// scheme=file;
// action=android.intent.action.VIEW;
// end

func OpenURL(intentStr string) error {
	return driver.RunNative(func(ctx interface{}) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return fmt.Errorf("failed to get AndroidContext")
		}

		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))

		cStr := C.CString(intentStr)
		defer C.free(unsafe.Pointer(cStr))

		res := C.openIntent(
			env,
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			cStr,
		)

		if res == C.JNI_FALSE {
			return fmt.Errorf("intent failed: %s", intentStr)
		}
		return nil
	})
}

// Если зарегистрированы схемы то через них
// иначе через браузер
func OpenDAV(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	if schemes, _, _, ok := isDAV(s); ok {
		// Если зарегистрированы схемы то через них
		// но  я их зарегистрировал на себя
		// for _, scheme := range schemes[1:] {
		// 	u.Scheme = scheme
		// 	if err := OpenURL(u.String()); err == nil {
		// 		return nil
		// 	}
		// }
		u.Scheme = schemes[0]
	}

	return OpenURL(u.String())
}
