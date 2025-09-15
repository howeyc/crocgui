//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>

// Функция для проверки jobject на NULL
static jboolean isJObjectNull(JNIEnv* env, jobject obj) {
    return (obj == NULL) ? JNI_TRUE : JNI_FALSE;
}

// Функция для безопасного удаления local reference
static void deleteLocalRef(JNIEnv* env, jobject ref) {
    if (ref != NULL) {
        (*env)->DeleteLocalRef(env, ref);
    }
}

// Объявляем функцию для проверки поддержки Intent
static jboolean isIntentSupported(JNIEnv* env, jobject context, jobject intent) {
    // 1. Получаем класс PackageManager
    jclass context_class = (*env)->GetObjectClass(env, context);
    jmethodID get_package_manager = (*env)->GetMethodID(env, context_class, "getPackageManager", "()Landroid/content/pm/PackageManager;");
    jobject package_manager = (*env)->CallObjectMethod(env, context, get_package_manager);

    if (package_manager == NULL) {
        (*env)->DeleteLocalRef(env, context_class);
        return JNI_FALSE;
    }

    // 2. Получаем класс PackageManager
    jclass pm_class = (*env)->GetObjectClass(env, package_manager);
    jmethodID query_intent_activities = (*env)->GetMethodID(env, pm_class, "queryIntentActivities", "(Landroid/content/Intent;I)Ljava/util/List;");

    if (query_intent_activities == NULL) {
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        return JNI_FALSE;
    }

    // 3. Вызываем queryIntentActivities
    jint flags = 0;
    jobject activities_list = (*env)->CallObjectMethod(env, package_manager, query_intent_activities, intent, flags);

    if (activities_list == NULL) {
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        return JNI_FALSE;
    }

    // 4. Получаем класс List и проверяем размер
    jclass list_class = (*env)->GetObjectClass(env, activities_list);
    jmethodID list_size = (*env)->GetMethodID(env, list_class, "size", "()I");
    jint size = (*env)->CallIntMethod(env, activities_list, list_size);

    // 5. Очищаем ресурсы
    (*env)->DeleteLocalRef(env, context_class);
    (*env)->DeleteLocalRef(env, pm_class);
    (*env)->DeleteLocalRef(env, package_manager);
    (*env)->DeleteLocalRef(env, activities_list);
    (*env)->DeleteLocalRef(env, list_class);

    return (size > 0) ? JNI_TRUE : JNI_FALSE;
}

// Вспомогательная функция для создания Intent
static jobject createIntent(JNIEnv* env, const char* action, const char* mime_type) {
    jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
    if (intent_class == NULL) {
        return NULL;
    }

    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "(Ljava/lang/String;)V");
    if (intent_constructor == NULL) {
        (*env)->DeleteLocalRef(env, intent_class);
        return NULL;
    }

    jstring action_str = (*env)->NewStringUTF(env, action);
    jobject intent = (*env)->NewObject(env, intent_class, intent_constructor, action_str);
    (*env)->DeleteLocalRef(env, action_str);

    if (mime_type != NULL) {
        jmethodID set_type = (*env)->GetMethodID(env, intent_class, "setType", "(Ljava/lang/String;)Landroid/content/Intent;");
        if (set_type != NULL) {
            jstring mime_type_str = (*env)->NewStringUTF(env, mime_type);
            (*env)->CallObjectMethod(env, intent, set_type, mime_type_str);
            (*env)->DeleteLocalRef(env, mime_type_str);
        }
    }

    (*env)->DeleteLocalRef(env, intent_class);
    return intent;
}
*/
import "C"
import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

func IsIntentSupported(action, mimeType string) (bool, error) {
	if noDialogDebug {
		return false, nil
	}
	var result C.jboolean
	var err error

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		context := C.jobject(unsafe.Pointer(ac.Ctx))

		// Создаем Intent
		cAction := C.CString(action)
		defer C.free(unsafe.Pointer(cAction))

		var cMimeType *C.char
		if mimeType != "" {
			cMimeType = C.CString(mimeType)
			defer C.free(unsafe.Pointer(cMimeType))
		}

		intent := C.createIntent(env, cAction, cMimeType)

		// Используем JNI-функцию для проверки вместо прямого сравнения с nil
		if C.isJObjectNull(env, intent) == C.JNI_TRUE {
			err = errors.New("failed to create Intent")
			return nil
		}
		defer C.deleteLocalRef(env, intent) // Используем безопасное удаление

		// Проверяем поддержку Intent
		result = C.isIntentSupported(env, context, intent)
		return nil
	})

	if err != nil {
		return false, err
	}

	return result == C.JNI_TRUE, nil
}

// IsFilePickerSupported проверяет поддержку диалога выбора файлов
func IsFilePickerSupported() (bool, error) {
	return IsIntentSupported("android.intent.action.GET_CONTENT", "*/*")
}

// IsSaveDialogSupported проверяет поддержку диалога сохранения файлов
func IsSaveDialogSupported() (bool, error) {
	return IsIntentSupported("android.intent.action.CREATE_DOCUMENT", "*/*")
}

// IsFolderPickerSupported проверяет поддержку диалога выбора папки
func IsFolderPickerSupported() (bool, error) {
	return IsIntentSupported("android.intent.action.OPEN_DOCUMENT_TREE", "")
}

// Example использования:
func checkIntents() {
	// Проверка диалога выбора файлов
	if supported, err := IsFilePickerSupported(); err != nil {
		println("Error checking file picker:", err.Error())
	} else if !supported {
		println("File picker not supported - need to install file manager")
	} else {
		println("File picker is supported")
	}

	// Проверка диалога сохранения
	if supported, err := IsSaveDialogSupported(); err != nil {
		println("Error checking save dialog:", err.Error())
	} else if !supported {
		println("Save dialog not supported - need to install file manager")
	} else {
		println("Save dialog is supported")
	}

	// Проверка диалога выбора папки
	if supported, err := IsFolderPickerSupported(); err != nil {
		println("Error checking folder picker:", err.Error())
	} else if !supported {
		println("Folder picker not supported - need to install file manager")
	} else {
		println("Folder picker is supported")
	}
}
