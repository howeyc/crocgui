//go:build android

// child_android.go
package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

// Функция для создания файла через DocumentsContract.createDocument()
char* CreateFileViaDocumentsContract(JNIEnv* env, jobject activity,
                                    const char* tree_uri,
                                    const char* file_name,
                                    const char* mime_type) {

    // 1. Получаем ContentResolver
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        return strdup("error: activity_class == NULL");
    }

    jmethodID get_content_resolver_method = (*env)->GetMethodID(env, activity_class, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (get_content_resolver_method == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        return strdup("error: get_content_resolver_method == NULL");
    }

    jobject content_resolver = (*env)->CallObjectMethod(env, activity, get_content_resolver_method);
    if (content_resolver == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        return strdup("error: content_resolver == NULL");
    }

    // 2. Парсим treeUri в объект Uri
    jclass uri_class = (*env)->FindClass(env, "android/net/Uri");
    if (uri_class == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        return strdup("error: uri_class == NULL");
    }

    jmethodID parse_method = (*env)->GetStaticMethodID(env, uri_class, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parse_method == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: parse_method == NULL");
    }

    jstring tree_uri_jstr = (*env)->NewStringUTF(env, tree_uri);
    jobject tree_uri_obj = (*env)->CallStaticObjectMethod(env, uri_class, parse_method, tree_uri_jstr);
    (*env)->DeleteLocalRef(env, tree_uri_jstr);

    if (tree_uri_obj == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        return strdup("error: tree_uri_obj == NULL");
    }

    // 3. Получаем класс DocumentsContract
    jclass documents_contract_class = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documents_contract_class == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        return strdup("error: documents_contract_class == NULL");
    }

    // 4. Получаем documentId из tree URI (как в Rust коде)
    jmethodID get_tree_document_id_method = (*env)->GetStaticMethodID(env, documents_contract_class, "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
    if (get_tree_document_id_method == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        (*env)->DeleteLocalRef(env, documents_contract_class);
        return strdup("error: get_tree_document_id_method == NULL");
    }

    jstring document_id_obj = (jstring)(*env)->CallStaticObjectMethod(env, documents_contract_class, get_tree_document_id_method, tree_uri_obj);
    if (document_id_obj == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        (*env)->DeleteLocalRef(env, documents_contract_class);
        return strdup("error: document_id_obj == NULL");
    }

    // 5. Строим child documents URI (как в Rust коде)
    jmethodID build_child_documents_uri_method = (*env)->GetStaticMethodID(env, documents_contract_class, "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    if (build_child_documents_uri_method == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        (*env)->DeleteLocalRef(env, documents_contract_class);
        (*env)->DeleteLocalRef(env, document_id_obj);
        return strdup("error: build_child_documents_uri_method == NULL");
    }

    jobject child_documents_uri_obj = (*env)->CallStaticObjectMethod(env, documents_contract_class, build_child_documents_uri_method, tree_uri_obj, document_id_obj);
    (*env)->DeleteLocalRef(env, document_id_obj);

    if (child_documents_uri_obj == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        (*env)->DeleteLocalRef(env, documents_contract_class);
        return strdup("error: child_documents_uri_obj == NULL");
    }

    // 6. Получаем метод createDocument
    jmethodID create_document_method = (*env)->GetStaticMethodID(env, documents_contract_class, "createDocument", "(Landroid/content/ContentResolver;Landroid/net/Uri;Ljava/lang/String;Ljava/lang/String;)Landroid/net/Uri;");
    if (create_document_method == NULL) {
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, content_resolver);
        (*env)->DeleteLocalRef(env, uri_class);
        (*env)->DeleteLocalRef(env, tree_uri_obj);
        (*env)->DeleteLocalRef(env, child_documents_uri_obj);
        (*env)->DeleteLocalRef(env, documents_contract_class);
        return strdup("error: create_document_method == NULL");
    }

    // 7. Создаем файл - используем child_documents_uri_obj вместо tree_uri_obj!
    jstring mime_type_jstr = (*env)->NewStringUTF(env, mime_type);
    jstring file_name_jstr = (*env)->NewStringUTF(env, file_name);

    // Очищаем возможные предыдущие исключения
    (*env)->ExceptionClear(env);

    jobject new_file_uri_obj = (*env)->CallStaticObjectMethod(env, documents_contract_class, create_document_method,
                                                            content_resolver, child_documents_uri_obj,
                                                            mime_type_jstr, file_name_jstr);

    (*env)->DeleteLocalRef(env, mime_type_jstr);
    (*env)->DeleteLocalRef(env, file_name_jstr);
    (*env)->DeleteLocalRef(env, child_documents_uri_obj);

    // 8. Обрабатываем результат
    char *result = NULL;
    if (new_file_uri_obj != NULL) {
        jmethodID to_string_method = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
        if (to_string_method != NULL) {
            jstring new_file_uri_jstr = (jstring)(*env)->CallObjectMethod(env, new_file_uri_obj, to_string_method);
            if (new_file_uri_jstr != NULL) {
                const char *new_file_uri_cstr = (*env)->GetStringUTFChars(env, new_file_uri_jstr, NULL);
                result = strdup(new_file_uri_cstr);
                (*env)->ReleaseStringUTFChars(env, new_file_uri_jstr, new_file_uri_cstr);
                (*env)->DeleteLocalRef(env, new_file_uri_jstr);
            }
        }
        (*env)->DeleteLocalRef(env, new_file_uri_obj);
    } else {
        // Обработка ошибок...
        jthrowable exception = (*env)->ExceptionOccurred(env);
        if (exception) {
            jclass exception_class = (*env)->GetObjectClass(env, exception);
            jmethodID get_message_method = (*env)->GetMethodID(env, exception_class, "getMessage", "()Ljava/lang/String;");

            jstring message_jstr = NULL;
            if (get_message_method != NULL) {
                message_jstr = (jstring)(*env)->CallObjectMethod(env, exception, get_message_method);
            }

            if (message_jstr != NULL) {
                const char *message_cstr = (*env)->GetStringUTFChars(env, message_jstr, NULL);
                char error_msg[1024];
                snprintf(error_msg, sizeof(error_msg), "error: createDocument failed: %s", message_cstr);
                result = strdup(error_msg);
                (*env)->ReleaseStringUTFChars(env, message_jstr, message_cstr);
                (*env)->DeleteLocalRef(env, message_jstr);
            } else {
                result = strdup("error: createDocument failed with unknown exception");
            }

            (*env)->ExceptionClear(env);
            (*env)->DeleteLocalRef(env, exception_class);
            (*env)->DeleteLocalRef(env, exception);
        } else {
            result = strdup("error: createDocument returned null without exception");
        }
    }

    // 9. Освобождаем ресурсы
    (*env)->DeleteLocalRef(env, activity_class);
    (*env)->DeleteLocalRef(env, content_resolver);
    (*env)->DeleteLocalRef(env, uri_class);
    (*env)->DeleteLocalRef(env, tree_uri_obj);
    (*env)->DeleteLocalRef(env, documents_contract_class);

    return result;
}

// Вспомогательная функция для проверки и очистки исключений
static jboolean caseException(JNIEnv* env) {
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE; // Было исключение
    }
    return JNI_FALSE; // Не было исключения
}

char* GetParentDocumentUri(JNIEnv* env, jobject activity, const char* child_uri) {
    char* result = NULL;
    jobject child_uri_obj = NULL;
    jobject parent_uri_obj = NULL;
    jstring parent_uri_jstr = NULL;
    jobject content_resolver = NULL;

    jclass activity_class = NULL;
    jclass uri_class = NULL;
    jclass documents_contract_class = NULL;

    // 1. Получаем ContentResolver
    activity_class = (*env)->GetObjectClass(env, activity);
    if (caseException(env)) {
        result = strdup("error: exception in GetObjectClass");
        goto cleanup;
    }
    if (activity_class == NULL) {
        result = strdup("error: activity_class == NULL");
        goto cleanup;
    }

    jmethodID get_content_resolver_method = (*env)->GetMethodID(env, activity_class, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (caseException(env)) {
        result = strdup("error: exception in GetMethodID getContentResolver");
        goto cleanup;
    }
    if (get_content_resolver_method == NULL) {
        result = strdup("error: get_content_resolver_method == NULL");
        goto cleanup;
    }

    content_resolver = (*env)->CallObjectMethod(env, activity, get_content_resolver_method);
    if (caseException(env)) {
        result = strdup("error: exception in CallObjectMethod getContentResolver");
        goto cleanup;
    }
    if (content_resolver == NULL) {
        result = strdup("error: content_resolver == NULL");
        goto cleanup;
    }

    // 2. Парсим URI
    uri_class = (*env)->FindClass(env, "android/net/Uri");
    if (caseException(env)) {
        result = strdup("error: exception in FindClass Uri");
        goto cleanup;
    }
    if (uri_class == NULL) {
        result = strdup("error: uri_class == NULL");
        goto cleanup;
    }

    jmethodID parse_method = (*env)->GetStaticMethodID(env, uri_class, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (caseException(env)) {
        result = strdup("error: exception in GetStaticMethodID parse");
        goto cleanup;
    }
    if (parse_method == NULL) {
        result = strdup("error: parse_method == NULL");
        goto cleanup;
    }

    jstring child_uri_jstr = (*env)->NewStringUTF(env, child_uri);
    if (caseException(env)) {
        result = strdup("error: exception in NewStringUTF child_uri");
        goto cleanup;
    }

    child_uri_obj = (*env)->CallStaticObjectMethod(env, uri_class, parse_method, child_uri_jstr);
    (*env)->DeleteLocalRef(env, child_uri_jstr);
    if (caseException(env)) {
        result = strdup("error: exception in CallStaticObjectMethod parse");
        goto cleanup;
    }
    if (child_uri_obj == NULL) {
        result = strdup("error: child_uri_obj == NULL");
        goto cleanup;
    }

    // 3. Получаем DocumentsContract
    documents_contract_class = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (caseException(env)) {
        result = strdup("error: exception in FindClass DocumentsContract");
        goto cleanup;
    }
    if (documents_contract_class == NULL) {
        result = strdup("error: documents_contract_class == NULL");
        goto cleanup;
    }

    jmethodID get_parent_document_uri_method = NULL;

    // 4. Сначала пробуем новую сигнатуру (API >= 24): getParentDocumentUri(Uri)
    get_parent_document_uri_method = (*env)->GetStaticMethodID(
        env,
        documents_contract_class,
        "getParentDocumentUri",
        "(Landroid/net/Uri;)Landroid/net/Uri;"
    );

    // Если при загрузке метода было исключение - очищаем и пробуем дальше
    if (caseException(env)) {
        // Очистили исключение, продолжаем пробовать старую сигнатуру
        get_parent_document_uri_method = NULL;
    }

    if (get_parent_document_uri_method != NULL) {
        // API >= 24: вызываем с одним аргументом
        parent_uri_obj = (*env)->CallStaticObjectMethod(env, documents_contract_class, get_parent_document_uri_method, child_uri_obj);
        if (caseException(env)) {
            // Если вызов с новой сигнатурой вызвал исключение, пробуем старую
            parent_uri_obj = NULL;
        }
    }

    // 5. Если новая сигнатура не сработала, пробуем старую (API 21-23)
    if (parent_uri_obj == NULL) {
        get_parent_document_uri_method = (*env)->GetStaticMethodID(
            env,
            documents_contract_class,
            "getParentDocumentUri",
            "(Landroid/content/ContentResolver;Landroid/net/Uri;)Landroid/net/Uri;"
        );

        // Если при загрузке старого метода было исключение - очищаем
        if (caseException(env)) {
            get_parent_document_uri_method = NULL;
        }

        if (get_parent_document_uri_method != NULL) {
            parent_uri_obj = (*env)->CallStaticObjectMethod(env, documents_contract_class, get_parent_document_uri_method, content_resolver, child_uri_obj);
            if (caseException(env)) {
                result = strdup("error: exception in CallStaticObjectMethod getParentDocumentUri (old)");
                goto cleanup;
            }
        } else {
            result = strdup("error: getParentDocumentUri method not available on this API level");
            goto cleanup;
        }
    }

    // Проверяем результат после обработки исключений
    if (parent_uri_obj == NULL) {
        result = strdup("error: getParentDocumentUri returned null - may be root directory");
        goto cleanup;
    }

    // 6. Конвертируем результат в строку
    jmethodID to_string_method = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
    if (caseException(env)) {
        result = strdup("error: exception in GetMethodID toString");
        goto cleanup;
    }
    if (to_string_method == NULL) {
        result = strdup("error: to_string_method == NULL");
        goto cleanup;
    }

    parent_uri_jstr = (jstring)(*env)->CallObjectMethod(env, parent_uri_obj, to_string_method);
    if (caseException(env)) {
        result = strdup("error: exception in CallObjectMethod toString");
        goto cleanup;
    }
    if (parent_uri_jstr == NULL) {
        result = strdup("error: parent_uri_jstr == NULL");
        goto cleanup;
    }

    const char *uri_str = (*env)->GetStringUTFChars(env, parent_uri_jstr, NULL);
    if (uri_str == NULL) {
        result = strdup("error: failed to get URI string");
        goto cleanup;
    }

    result = strdup(uri_str);
    (*env)->ReleaseStringUTFChars(env, parent_uri_jstr, uri_str);

cleanup:
    // Освобождение JNI ресурсов
    if (parent_uri_jstr) (*env)->DeleteLocalRef(env, parent_uri_jstr);
    if (parent_uri_obj) (*env)->DeleteLocalRef(env, parent_uri_obj);
    if (documents_contract_class) (*env)->DeleteLocalRef(env, documents_contract_class);
    if (child_uri_obj) (*env)->DeleteLocalRef(env, child_uri_obj);
    if (uri_class) (*env)->DeleteLocalRef(env, uri_class);
    if (content_resolver) (*env)->DeleteLocalRef(env, content_resolver);
    if (activity_class) (*env)->DeleteLocalRef(env, activity_class);

    return result;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/storage"
)

// CreateFileInTree создает файл в указанном treeUri каталоге
func CreateFileInTree(treeUri, fileName, mimeType string) (string, error) {
	var result string
	var err error

	if mimeType == "" {
		mimeType = detectMimeType(fileName)
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		cTreeUri := C.CString(treeUri)
		defer C.free(unsafe.Pointer(cTreeUri))
		cFileName := C.CString(fileName)
		defer C.free(unsafe.Pointer(cFileName))
		cMimeType := C.CString(mimeType)
		defer C.free(unsafe.Pointer(cMimeType))

		cResult := C.CreateFileViaDocumentsContract(env, activity, cTreeUri, cFileName, cMimeType)
		if cResult == nil {
			err = errors.New("неизвестная ошибка в JNI-функции")
			return nil
		}

		defer C.free(unsafe.Pointer(cResult))
		resultStr := C.GoString(cResult)

		if strings.HasPrefix(resultStr, "error:") {
			err = errors.New(resultStr)
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if result == "" {
		return "", errors.New("пустой результат от DocumentsContract")
	}

	return result, nil
}

func Child(parent fyne.URI, component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// 1. Пробуем стандартный способ
	child, err = storage.Child(parent, component)
	if err == nil {
		return
	}

	// 2. Создаём component в parent
	newFileURL, err := CreateFileInTree(parent.String(), component, "")
	if err != nil {
		err = fmt.Errorf("CreateFileInTree failed: %v", err)
		return
	}

	// 3. Конвертируем в fyne.URI
	child, err = storage.ParseURI(newFileURL)
	if err != nil {
		err = fmt.Errorf("parse URI failed: %v", err)
		return
	}

	return
}

func Parent(child fyne.URI) (parent fyne.ListableURI, err error) {
	// Сначала пробуем стандартный способ Fyne
	if parentUri, parentErr := storage.Parent(child); parentErr == nil {
		if listable, listerErr := storage.ListerForURI(parentUri); listerErr == nil {
			return listable, nil
		}
		// Если не получилось сделать listable, продолжаем к Android-специфичному способу
	}

	// Android-специфичная реализация через DocumentsContract
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		cChildUri := C.CString(child.String())
		defer C.free(unsafe.Pointer(cChildUri))

		// Получаем родительский URI через JNI
		cParentUri := C.GetParentDocumentUri(env, activity, cChildUri)
		if cParentUri == nil {
			err = errors.New("getParentDocumentUri returned null")
			return nil
		}

		defer C.free(unsafe.Pointer(cParentUri))
		parentUriStr := C.GoString(cParentUri)

		if strings.HasPrefix(parentUriStr, "error:") {
			err = errors.New(parentUriStr)
			return nil
		}

		parentUri, parseErr := storage.ParseURI(parentUriStr)
		if parseErr != nil {
			err = fmt.Errorf("parse parent URI: %v", parseErr)
			return nil
		}

		// Конвертируем в ListableURI
		parent, err = storage.ListerForURI(parentUri)
		if err != nil {
			err = fmt.Errorf("make listable: %v", err)
		}

		return nil
	})

	return
}
