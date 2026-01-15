//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

void LogD(const char* message);

// Функция для проверки jobject на NULL
static jboolean isJObjectNull(JNIEnv* env, jobject obj) {
    jboolean result = (obj == NULL) ? JNI_TRUE : JNI_FALSE;
    if (result == JNI_TRUE) {
        LogD("C: JObject is NULL");
    }
    return result;
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
    if (context_class == NULL) {
        LogD("C: ERROR - Failed to get context class");
        return JNI_FALSE;
    }

    jmethodID get_package_manager = (*env)->GetMethodID(env, context_class, "getPackageManager", "()Landroid/content/pm/PackageManager;");
    if (get_package_manager == NULL) {
        LogD("C: ERROR - Failed to get getPackageManager method");
        (*env)->DeleteLocalRef(env, context_class);
        return JNI_FALSE;
    }

    jobject package_manager = (*env)->CallObjectMethod(env, context, get_package_manager);
    if (package_manager == NULL) {
        LogD("C: ERROR - PackageManager is NULL");
        (*env)->DeleteLocalRef(env, context_class);
        return JNI_FALSE;
    }

    // 2. Получаем класс PackageManager
    jclass pm_class = (*env)->GetObjectClass(env, package_manager);
    if (pm_class == NULL) {
        LogD("C: ERROR - Failed to get PackageManager class");
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, package_manager);
        return JNI_FALSE;
    }

    jmethodID query_intent_activities = (*env)->GetMethodID(env, pm_class, "queryIntentActivities", "(Landroid/content/Intent;I)Ljava/util/List;");
    if (query_intent_activities == NULL) {
        LogD("C: ERROR - Failed to get queryIntentActivities method");
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        return JNI_FALSE;
    }

    // 3. Вызываем queryIntentActivities
    jint flags = 0;
    jobject activities_list = (*env)->CallObjectMethod(env, package_manager, query_intent_activities, intent, flags);

    if (activities_list == NULL) {
        LogD("C: ERROR - Activities list is NULL");
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        return JNI_FALSE;
    }

    // 4. Получаем класс List и проверяем размер
    jclass list_class = (*env)->GetObjectClass(env, activities_list);
    if (list_class == NULL) {
        LogD("C: ERROR - Failed to get List class");
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        (*env)->DeleteLocalRef(env, activities_list);
        return JNI_FALSE;
    }

    jmethodID list_size = (*env)->GetMethodID(env, list_class, "size", "()I");
    if (list_size == NULL) {
        LogD("C: ERROR - Failed to get list size method");
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pm_class);
        (*env)->DeleteLocalRef(env, package_manager);
        (*env)->DeleteLocalRef(env, activities_list);
        (*env)->DeleteLocalRef(env, list_class);
        return JNI_FALSE;
    }

    jint size = (*env)->CallIntMethod(env, activities_list, list_size);

    char sizeLog[64];
    snprintf(sizeLog, sizeof(sizeLog), "C: Found %d activities supporting intent", size);
    LogD(sizeLog);

    // 5. Очищаем ресурсы
    (*env)->DeleteLocalRef(env, context_class);
    (*env)->DeleteLocalRef(env, pm_class);
    (*env)->DeleteLocalRef(env, package_manager);
    (*env)->DeleteLocalRef(env, activities_list);
    (*env)->DeleteLocalRef(env, list_class);

    jboolean result = (size > 0) ? JNI_TRUE : JNI_FALSE;
    if (result == JNI_FALSE) {
        LogD("C: Intent is not supported");
    }

    return result;
}

static jobject createIntent(JNIEnv* env, const char* action, const char* mime_type) {
    // 1. Find the Intent class
    jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
    if (intent_class == NULL) {
        LogD("C: ERROR - Failed to find Intent class");
        return NULL;
    }

    // 2. Get the constructor Intent(String action)
    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "(Ljava/lang/String;)V");
    if (intent_constructor == NULL) {
        LogD("C: ERROR - Failed to get Intent constructor");
        (*env)->DeleteLocalRef(env, intent_class);
        return NULL;
    }

    // 3. Create the Intent object
    jstring action_str = (*env)->NewStringUTF(env, action);
    jobject intent = (*env)->NewObject(env, intent_class, intent_constructor, action_str);
    (*env)->DeleteLocalRef(env, action_str);

    if (intent == NULL) {
        LogD("C: ERROR - Failed to create Intent object");
        (*env)->DeleteLocalRef(env, intent_class);
        return NULL;
    }

    // 4. Add Categories (CRITICAL for API 30-36 Package Visibility)
    // Most file managers and system pickers require CATEGORY_DEFAULT to be explicitly set
    // to be discovered by queryIntentActivities.
    jmethodID add_category = (*env)->GetMethodID(env, intent_class, "addCategory", "(Ljava/lang/String;)Landroid/content/Intent;");
    if (add_category != NULL) {
        // CATEGORY_DEFAULT: Necessary for the Intent to be resolved by PackageManager
        jstring cat_default = (*env)->NewStringUTF(env, "android.intent.category.DEFAULT");
        (*env)->CallObjectMethod(env, intent, add_category, cat_default);
        (*env)->DeleteLocalRef(env, cat_default);

        // CATEGORY_OPENABLE: Required for GET_CONTENT/OPEN_DOCUMENT to ensure
        // the returned URI can be opened as a stream (essential for croc file transfer)
        jstring cat_openable = (*env)->NewStringUTF(env, "android.intent.category.OPENABLE");
        (*env)->CallObjectMethod(env, intent, add_category, cat_openable);
        (*env)->DeleteLocalRef(env, cat_openable);
    } else {
        LogD("C: WARNING - Failed to get addCategory method");
    }

    // 5. Set MIME type if provided
    if (mime_type != NULL) {
        jmethodID set_type = (*env)->GetMethodID(env, intent_class, "setType", "(Ljava/lang/String;)Landroid/content/Intent;");
        if (set_type != NULL) {
            jstring mime_type_str = (*env)->NewStringUTF(env, mime_type);
            (*env)->CallObjectMethod(env, intent, set_type, mime_type_str);
            (*env)->DeleteLocalRef(env, mime_type_str);
        } else {
            LogD("C: WARNING - Failed to get setType method");
        }
    }

    // Clean up local reference to the class
    (*env)->DeleteLocalRef(env, intent_class);
    return intent;
}
*/
import "C"
import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

func IsIntentSupported(action, mimeType string) (bool, error) {
	if noDialogs {
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
			log.Error("Failed to create Intent object")
			err = errors.New("failed to create Intent")
			return nil
		}

		defer C.deleteLocalRef(env, intent) // Используем безопасное удаление

		// Проверяем поддержку Intent
		result = C.isIntentSupported(env, context, intent)

		return nil
	})

	if err != nil {
		log.Error("Error checking intent support: ", err.Error())
		return false, err
	}

	return result == C.JNI_TRUE, nil
}

// IsFilePickerSupported проверяет поддержку диалога выбора файлов
func IsFilePickerSupported() (bool, error) {
	supported, err := IsIntentSupported("android.intent.action.GET_CONTENT", "*/*")
	if err != nil {
		log.Error("File picker support check failed: ", err.Error())
	}
	return supported, err
}

// IsSaveDialogSupported проверяет поддержку диалога сохранения файлов
func IsSaveDialogSupported() (bool, error) {
	supported, err := IsIntentSupported("android.intent.action.CREATE_DOCUMENT", "*/*")
	if err != nil {
		log.Error("Save dialog support check failed: ", err.Error())
	}
	return supported, err
}

// IsFolderPickerSupported проверяет поддержку диалога выбора папки
func IsFolderPickerSupported() (bool, error) {
	supported, err := IsIntentSupported("android.intent.action.OPEN_DOCUMENT_TREE", "")
	if err != nil {
		log.Error("Folder picker support check failed: ", err.Error())
	}
	return supported, err
}

// Example использования:
func checkIntents() {
	// Проверка диалога выбора файлов
	if supported, err := IsFilePickerSupported(); err != nil {
		log.Error("Error checking file picker: ", err.Error())
	} else if !supported {
		log.Warn("File picker not supported - need to install file manager")
	}

	// Проверка диалога сохранения
	if supported, err := IsSaveDialogSupported(); err != nil {
		log.Error("Error checking save dialog: ", err.Error())
	} else if !supported {
		log.Warn("Save dialog not supported - need to install file manager")
	}

	// Проверка диалога выбора папки
	if supported, err := IsFolderPickerSupported(); err != nil {
		log.Error("Error checking folder picker: ", err.Error())
	} else if !supported {
		log.Warn("Folder picker not supported - need to install file manager")
	}
}
