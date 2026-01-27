//go:build android

// open_app_info_android.go
// func openAppInfo(){}
package main

/*
#include <jni.h>
#include <android/log.h>
#include <stdio.h>

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

static void openAppInfo(JNIEnv *env, jobject activity) {
    jclass versionClass = NULL, contextClass = NULL, uriClass = NULL, intentClass = NULL;
    jstring packageName = NULL, uriString = NULL, actionJString = NULL;
    jobject uri = NULL, intent = NULL;

    LogD("C: Starting openAppInfo");

    const char *actionStr = "android.settings.APPLICATION_DETAILS_SETTINGS";

    // 2. Получаем Package Name
    contextClass = (*env)->GetObjectClass(env, activity);
    jmethodID getPkgMethod = (*env)->GetMethodID(env, contextClass, "getPackageName", "()Ljava/lang/String;");
    packageName = (jstring)(*env)->CallObjectMethod(env, activity, getPkgMethod);
    if (caseException(env, "getPackageName")) goto cleanup;

    // 3. Формируем URI
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");

    const char *pkgChars = (*env)->GetStringUTFChars(env, packageName, NULL);
    char uriBuf[512];
    snprintf(uriBuf, sizeof(uriBuf), "package:%s", pkgChars);
    (*env)->ReleaseStringUTFChars(env, packageName, pkgChars);

    uriString = (*env)->NewStringUTF(env, uriBuf);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, uriString);
    if (caseException(env, "Uri.parse")) goto cleanup;

    // 4. Создаем Intent
    intentClass = (*env)->FindClass(env, "android/content/Intent");
    jmethodID intentConstructor = (*env)->GetMethodID(env, intentClass, "<init>", "(Ljava/lang/String;Landroid/net/Uri;)V");
    actionJString = (*env)->NewStringUTF(env, actionStr);
    intent = (*env)->NewObject(env, intentClass, intentConstructor, actionJString, uri);
    if (caseException(env, "New Intent")) goto cleanup;

    // 5. Запускаем Activity
    jmethodID startActivity = (*env)->GetMethodID(env, contextClass, "startActivity", "(Landroid/content/Intent;)V");
    if (startActivity) {
        (*env)->CallVoidMethod(env, activity, startActivity, intent);
        caseException(env, "startActivity");
        LogD("C: Settings activity started");
    }

cleanup:
    if (actionJString) (*env)->DeleteLocalRef(env, actionJString);
    if (intent) (*env)->DeleteLocalRef(env, intent);
    if (intentClass) (*env)->DeleteLocalRef(env, intentClass);
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (uriString) (*env)->DeleteLocalRef(env, uriString);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (packageName) (*env)->DeleteLocalRef(env, packageName);
    if (contextClass) (*env)->DeleteLocalRef(env, contextClass);
    if (versionClass) (*env)->DeleteLocalRef(env, versionClass);
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

func openAppInfo() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		log.Debug("Calling C.openAppInfo")

		// Вызываем JNI функцию, которая сама определит API и нужный Intent
		C.openAppInfo(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.openAppInfo completed")
		return nil
	})
}
