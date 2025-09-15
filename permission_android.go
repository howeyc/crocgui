//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>

// Функция для проверки разрешения
static jboolean hasPermission(JNIEnv* env, jobject context, const char* permission) {
    jclass context_class = (*env)->GetObjectClass(env, context);
    jmethodID check_permission = (*env)->GetMethodID(env, context_class, "checkSelfPermission", "(Ljava/lang/String;)I");

    jstring permission_str = (*env)->NewStringUTF(env, permission);
    jint result = (*env)->CallIntMethod(env, context, check_permission, permission_str);

    (*env)->DeleteLocalRef(env, permission_str);
    (*env)->DeleteLocalRef(env, context_class);

    return (result == 0) ? JNI_TRUE : JNI_FALSE; // 0 = PERMISSION_GRANTED
}

// Функция для запроса разрешений
static void requestPermissions(JNIEnv* env, jobject activity, const char** permissions, jint size) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    jmethodID request_permissions = (*env)->GetMethodID(env, activity_class, "requestPermissions", "([Ljava/lang/String;I)V");

    // Создаем массив строк Java
    jclass string_class = (*env)->FindClass(env, "java/lang/String");
    jobjectArray permissions_array = (*env)->NewObjectArray(env, size, string_class, NULL);

    for (int i = 0; i < size; i++) {
        jstring permission = (*env)->NewStringUTF(env, permissions[i]);
        (*env)->SetObjectArrayElement(env, permissions_array, i, permission);
        (*env)->DeleteLocalRef(env, permission);
    }

    // Вызываем requestPermissions
    (*env)->CallVoidMethod(env, activity, request_permissions, permissions_array, 123); // 123 - request code

    // Очищаем ресурсы
    (*env)->DeleteLocalRef(env, permissions_array);
    (*env)->DeleteLocalRef(env, string_class);
    (*env)->DeleteLocalRef(env, activity_class);
}

// Функция для открытия настроек приложения
static void openAppSettings(JNIEnv* env, jobject context) {
    // Получаем класс Context
    jclass context_class = (*env)->GetObjectClass(env, context);

    jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
    jclass uri_class = (*env)->FindClass(env, "android/net/Uri");

    // Создаем Intent с action ACTION_APPLICATION_DETAILS_SETTINGS
    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "(Ljava/lang/String;)V");
    jstring action_str = (*env)->NewStringUTF(env, "android.settings.APPLICATION_DETAILS_SETTINGS");
    jobject intent = (*env)->NewObject(env, intent_class, intent_constructor, action_str);
    (*env)->DeleteLocalRef(env, action_str);

    // Создаем URI: package:<package_name>
    jmethodID parse_method = (*env)->GetStaticMethodID(env, uri_class, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    jstring uri_str = (*env)->NewStringUTF(env, "package:com.github.howeyc.crocgui");
    jobject uri = (*env)->CallStaticObjectMethod(env, uri_class, parse_method, uri_str);
    (*env)->DeleteLocalRef(env, uri_str);

    // Устанавливаем данные Intent
    jmethodID set_data_method = (*env)->GetMethodID(env, intent_class, "setData", "(Landroid/net/Uri;)Landroid/content/Intent;");
    (*env)->CallObjectMethod(env, intent, set_data_method, uri);

    // Добавляем флаги
    jmethodID add_flags_method = (*env)->GetMethodID(env, intent_class, "addFlags", "(I)Landroid/content/Intent;");
    jint flags = 0x10000000; // FLAG_ACTIVITY_NEW_TASK
    (*env)->CallObjectMethod(env, intent, add_flags_method, flags);

    // Запускаем Activity
    jmethodID start_activity_method = (*env)->GetMethodID(env, context_class, "startActivity", "(Landroid/content/Intent;)V");
    (*env)->CallVoidMethod(env, context, start_activity_method, intent);

    // Очищаем ресурсы
    (*env)->DeleteLocalRef(env, intent);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, intent_class);
    (*env)->DeleteLocalRef(env, uri_class);
    (*env)->DeleteLocalRef(env, context_class);
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// HasStoragePermission проверяет наличие разрешений на чтение и запись хранилища
func HasStoragePermission() bool {
	var result C.jboolean

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		context := C.jobject(unsafe.Pointer(ac.Ctx))

		// Проверяем оба разрешения
		readPermission := C.CString("android.permission.READ_EXTERNAL_STORAGE")
		defer C.free(unsafe.Pointer(readPermission))

		writePermission := C.CString("android.permission.WRITE_EXTERNAL_STORAGE")
		defer C.free(unsafe.Pointer(writePermission))

		hasRead := C.hasPermission(env, context, readPermission)
		hasWrite := C.hasPermission(env, context, writePermission)

		result = hasRead & hasWrite // Оба должны быть true
		return nil
	})

	return result == C.JNI_TRUE
}

// RequestStoragePermission запрашивает разрешения на чтение и запись хранилища
func RequestStoragePermission() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		context := C.jobject(unsafe.Pointer(ac.Ctx))

		// Создаем массив разрешений для запроса
		permissions := []*C.char{
			C.CString("android.permission.READ_EXTERNAL_STORAGE"),
			C.CString("android.permission.WRITE_EXTERNAL_STORAGE"),
		}
		defer func() {
			for _, perm := range permissions {
				C.free(unsafe.Pointer(perm))
			}
		}()

		// Запрашиваем разрешения
		C.requestPermissions(env, context, &permissions[0], C.jint(len(permissions)))
		return nil
	})
}

// OpenAppSettings открывает настройки приложения
func OpenAppSettings() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		context := C.jobject(unsafe.Pointer(ac.Ctx))

		C.openAppSettings(env, context)
		return nil
	})
}
