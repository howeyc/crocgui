//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>

#define LogD(fmt, ...) __android_log_print(ANDROID_LOG_DEBUG, "croc", fmt, ##__VA_ARGS__)

// Функция для проверки и очистки исключений
static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE; // Было исключение
    }
    return JNI_FALSE; // Не было исключения
}

// Функция для проверки поддержки Intent
static jboolean isIntentSupported(JNIEnv* env, jobject context, jobject intent) {
    jboolean result = JNI_FALSE;
    jclass context_class = NULL;
    jclass pm_class = NULL;
    jclass list_class = NULL;
    jobject package_manager = NULL;
    jobject activities_list = NULL;

    // 1. Получаем класс PackageManager
    context_class = (*env)->GetObjectClass(env, context);
    if (context_class == NULL) {
        LogD("C: ERROR - Failed to get context class");
        goto cleanup;
    }

    jmethodID get_package_manager = (*env)->GetMethodID(env, context_class,
        "getPackageManager", "()Landroid/content/pm/PackageManager;");
    if (get_package_manager == NULL) {
        LogD("C: ERROR - Failed to get getPackageManager method");
        goto cleanup;
    }

    package_manager = (*env)->CallObjectMethod(env, context, get_package_manager);
    if (caseException(env, "getPackageManager") || package_manager == NULL) {
        LogD("C: ERROR - PackageManager is NULL");
        goto cleanup;
    }

    // 2. Получаем класс PackageManager
    pm_class = (*env)->GetObjectClass(env, package_manager);
    if (pm_class == NULL) {
        LogD("C: ERROR - Failed to get PackageManager class");
        goto cleanup;
    }

    jmethodID query_intent_activities = (*env)->GetMethodID(env, pm_class,
        "queryIntentActivities", "(Landroid/content/Intent;I)Ljava/util/List;");
    if (query_intent_activities == NULL) {
        LogD("C: ERROR - Failed to get queryIntentActivities method");
        goto cleanup;
    }

    // 3. Вызываем queryIntentActivities
    jint flags = 0x00010000; // MATCH_DEFAULT_ONLY
    activities_list = (*env)->CallObjectMethod(env, package_manager,
        query_intent_activities, intent, flags);

    if (caseException(env, "queryIntentActivities") || activities_list == NULL) {
        LogD("C: ERROR - Activities list is NULL");
        goto cleanup;
    }

    // 4. Получаем класс List и проверяем размер
    list_class = (*env)->GetObjectClass(env, activities_list);
    if (list_class == NULL) {
        LogD("C: ERROR - Failed to get List class");
        goto cleanup;
    }

    jmethodID list_size = (*env)->GetMethodID(env, list_class, "size", "()I");
    if (list_size == NULL) {
        LogD("C: ERROR - Failed to get list size method");
        goto cleanup;
    }

    jint size = (*env)->CallIntMethod(env, activities_list, list_size);
    if (caseException(env, "get list size")) {
        size = 0;
    }

    LogD("C: Found %d activities supporting intent", size);

    result = (size > 0) ? JNI_TRUE : JNI_FALSE;
    if (result == JNI_FALSE) {
        LogD("C: Intent is not supported");
    }

cleanup:
    // Очищаем ресурсы в обратном порядке создания
    if (list_class) (*env)->DeleteLocalRef(env, list_class);
    if (activities_list) (*env)->DeleteLocalRef(env, activities_list);
    if (pm_class) (*env)->DeleteLocalRef(env, pm_class);
    if (package_manager) (*env)->DeleteLocalRef(env, package_manager);
    if (context_class) (*env)->DeleteLocalRef(env, context_class);

    return result;
}

static jobject createIntent(JNIEnv* env, const char* action, const char* mime_type) {
    jobject intent = NULL;
    jclass intent_class = NULL;
    jmethodID intent_constructor = NULL;
    jstring action_str = NULL;
    jmethodID add_category = NULL;
    jmethodID set_type = NULL;

    // 1. Find the Intent class
    intent_class = (*env)->FindClass(env, "android/content/Intent");
    if (intent_class == NULL) {
        LogD("C: ERROR - Failed to find Intent class");
        goto cleanup;
    }

    // 2. Get the constructor Intent(String action)
    intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>",
        "(Ljava/lang/String;)V");
    if (intent_constructor == NULL) {
        LogD("C: ERROR - Failed to get Intent constructor");
        goto cleanup;
    }

    // 3. Create the Intent object
    action_str = (*env)->NewStringUTF(env, action);
    if (action_str == NULL) {
        LogD("C: ERROR - Failed to create action string");
        goto cleanup;
    }

    intent = (*env)->NewObject(env, intent_class, intent_constructor, action_str);
    if (caseException(env, "create Intent") || intent == NULL) {
        LogD("C: ERROR - Failed to create Intent object");
        goto cleanup;
    }

    // 4. Add Categories
    add_category = (*env)->GetMethodID(env, intent_class, "addCategory",
        "(Ljava/lang/String;)Landroid/content/Intent;");
    if (add_category == NULL) {
        LogD("C: WARNING - Failed to get addCategory method");
    } else {
        // CATEGORY_DEFAULT: Necessary for the Intent to be resolved by PackageManager
        jstring cat_default = (*env)->NewStringUTF(env, "android.intent.category.DEFAULT");
        if (cat_default != NULL) {
            jobject temp = (*env)->CallObjectMethod(env, intent, add_category, cat_default);
            if (temp != NULL) (*env)->DeleteLocalRef(env, temp);
            (*env)->DeleteLocalRef(env, cat_default);
        }

        // CATEGORY_OPENABLE: только для не-деревьев документов
        if (strstr(action, "OPEN_DOCUMENT_TREE") == NULL) {
            jstring cat_openable = (*env)->NewStringUTF(env, "android.intent.category.OPENABLE");
            if (cat_openable != NULL) {
                jobject temp = (*env)->CallObjectMethod(env, intent, add_category, cat_openable);
                if (temp != NULL) (*env)->DeleteLocalRef(env, temp);
                (*env)->DeleteLocalRef(env, cat_openable);
            }
        }
    }

    // 5. Set MIME type if provided
    if (mime_type != NULL && *mime_type != '\0') {
        set_type = (*env)->GetMethodID(env, intent_class, "setType",
            "(Ljava/lang/String;)Landroid/content/Intent;");
        if (set_type == NULL) {
            LogD("C: WARNING - Failed to get setType method");
        } else {
            jstring mime_type_str = (*env)->NewStringUTF(env, mime_type);
            if (mime_type_str != NULL) {
                jobject temp = (*env)->CallObjectMethod(env, intent, set_type, mime_type_str);
                if (temp != NULL) (*env)->DeleteLocalRef(env, temp);
                (*env)->DeleteLocalRef(env, mime_type_str);
            }
        }
    }

cleanup:
    // Очищаем локальные ссылки
    if (action_str) (*env)->DeleteLocalRef(env, action_str);
    if (intent_class) (*env)->DeleteLocalRef(env, intent_class);

    // Возвращаем intent (может быть NULL в случае ошибки)
    return intent;
}

// Функция для безопасного удаления local reference
static void deleteLocalRef(JNIEnv* env, jobject ref) {
    if (ref != NULL) {
        (*env)->DeleteLocalRef(env, ref);
    }
}

// Функция для проверки jobject на NULL
static jboolean isJObjectNull(JNIEnv* env, jobject obj) {
    jboolean result = (obj == NULL) ? JNI_TRUE : JNI_FALSE;
    if (result == JNI_TRUE) {
        LogD("C: JObject is NULL");
    }
    return result;
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

	err = driver.RunNative(func(ctx interface{}) error {
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

		// Исправление: используем JNI функцию для проверки вместо прямого сравнения с nil
		if C.isJObjectNull(env, intent) == C.JNI_TRUE {
			log.Error("Failed to create Intent object")
			return errors.New("failed to create Intent")
		}
		defer C.deleteLocalRef(env, intent)

		// Проверяем поддержку Intent
		result = C.isIntentSupported(env, context, intent)
		return nil
	})

	if err != nil {
		log.Error("Error in RunNative: ", err.Error())
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
