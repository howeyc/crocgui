//go:build android

// notification_android.go
package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

// Get Android API level - static
static jint get_api_level(JNIEnv* env) {
    jclass version_class = (*env)->FindClass(env, "android/os/Build$VERSION");
    if (version_class == NULL) {
        LogD("C: ERROR - Build.VERSION class not found");
        return -1;
    }

    jfieldID sdk_int_field = (*env)->GetStaticFieldID(env, version_class, "SDK_INT", "I");
    if (sdk_int_field == NULL) {
        LogD("C: ERROR - SDK_INT field not found");
        (*env)->DeleteLocalRef(env, version_class);
        return -1;
    }

    jint sdk_version = (*env)->GetStaticIntField(env, version_class, sdk_int_field);
    (*env)->DeleteLocalRef(env, version_class);

    return sdk_version;
}

// Create notification channel for Android 8+
static void createCrocNotificationChannel(JNIEnv* env, jobject context) {
    jclass notification_manager_class = (*env)->FindClass(env, "android/app/NotificationManager");
    if (notification_manager_class == NULL) {
        LogD("C: ERROR - NotificationManager class not found");
        return;
    }

    // Get NotificationManager service
    jmethodID get_system_service = (*env)->GetMethodID(env,
        (*env)->GetObjectClass(env, context),
        "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");
    if (get_system_service == NULL) {
        LogD("C: ERROR - getSystemService method not found");
        (*env)->DeleteLocalRef(env, notification_manager_class);
        return;
    }

    jstring service_name = (*env)->NewStringUTF(env, "notification");
    jobject notification_manager = (*env)->CallObjectMethod(env, context, get_system_service, service_name);

    // Create notification channel
    jclass channel_class = (*env)->FindClass(env, "android/app/NotificationChannel");
    if (channel_class == NULL) {
        LogD("C: ERROR - NotificationChannel class not found");
        (*env)->DeleteLocalRef(env, service_name);
        (*env)->DeleteLocalRef(env, notification_manager_class);
        return;
    }

    jstring channel_id = (*env)->NewStringUTF(env, "croc_channel");
    jstring channel_name = (*env)->NewStringUTF(env, "Croc Notifications");
    jint importance = 3; // IMPORTANCE_DEFAULT

    jmethodID channel_constructor = (*env)->GetMethodID(env, channel_class, "<init>",
        "(Ljava/lang/String;Ljava/lang/CharSequence;I)V");
    if (channel_constructor == NULL) {
        LogD("C: ERROR - NotificationChannel constructor not found");
        (*env)->DeleteLocalRef(env, channel_id);
        (*env)->DeleteLocalRef(env, channel_name);
        (*env)->DeleteLocalRef(env, service_name);
        (*env)->DeleteLocalRef(env, notification_manager_class);
        (*env)->DeleteLocalRef(env, channel_class);
        return;
    }

    jobject channel = (*env)->NewObject(env, channel_class, channel_constructor,
        channel_id, channel_name, importance);

    // Set channel description
    jmethodID set_description = (*env)->GetMethodID(env, channel_class, "setDescription",
        "(Ljava/lang/String;)V");
    if (set_description != NULL) {
        jstring description = (*env)->NewStringUTF(env, "Application notifications");
        (*env)->CallVoidMethod(env, channel, set_description, description);
        (*env)->DeleteLocalRef(env, description);
    }

    // Create the channel
    jmethodID create_channel = (*env)->GetMethodID(env, notification_manager_class,
        "createNotificationChannel", "(Landroid/app/NotificationChannel;)V");
    if (create_channel == NULL) {
        LogD("C: ERROR - createNotificationChannel method not found");
    } else {
        (*env)->CallVoidMethod(env, notification_manager, create_channel, channel);
    }

    // Cleanup
    (*env)->DeleteLocalRef(env, channel_id);
    (*env)->DeleteLocalRef(env, channel_name);
    (*env)->DeleteLocalRef(env, channel);
    (*env)->DeleteLocalRef(env, service_name);
    (*env)->DeleteLocalRef(env, notification_manager_class);
    (*env)->DeleteLocalRef(env, channel_class);
}

static void showCrocNotification(JNIEnv* env, jobject context, char* title, char* content) {
    jint api_level = get_api_level(env);
    LogD("showCrocNotification: API level = %d", api_level);

    // Create notification channel for Android 8+ (API level 26+)
    if (api_level >= 26) {
        createCrocNotificationChannel(env, context);
    }

    // Создаем Intent для запуска главной активности
    jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
    if (intent_class == NULL) {
        LogD("C: ERROR - Intent class not found");
        return;
    }

    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "()V");
    if (intent_constructor == NULL) {
        LogD("C: ERROR - Intent constructor not found");
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jobject launch_intent = (*env)->NewObject(env, intent_class, intent_constructor);

    // Устанавливаем действие MAIN и категорию LAUNCHER
    jmethodID set_action = (*env)->GetMethodID(env, intent_class, "setAction", "(Ljava/lang/String;)Landroid/content/Intent;");
    if (set_action == NULL) {
        LogD("C: ERROR - setAction method not found");
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jstring action_string = (*env)->NewStringUTF(env, "android.intent.action.MAIN");
    (*env)->CallObjectMethod(env, launch_intent, set_action, action_string);

    jmethodID add_category = (*env)->GetMethodID(env, intent_class, "addCategory", "(Ljava/lang/String;)Landroid/content/Intent;");
    if (add_category == NULL) {
        LogD("C: ERROR - addCategory method not found");
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jstring category_string = (*env)->NewStringUTF(env, "android.intent.category.LAUNCHER");
    (*env)->CallObjectMethod(env, launch_intent, add_category, category_string);

    // Устанавливаем пакет и класс активности
    jmethodID set_class = (*env)->GetMethodID(env, intent_class, "setClassName", "(Ljava/lang/String;Ljava/lang/String;)Landroid/content/Intent;");
    if (set_class == NULL) {
        LogD("C: ERROR - setClassName method not found");
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, category_string);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jstring package_name = (*env)->NewStringUTF(env, "com.github.howeyc.crocgui");
    jstring class_name = (*env)->NewStringUTF(env, "org.golang.app.GoNativeActivity");
    (*env)->CallObjectMethod(env, launch_intent, set_class, package_name, class_name);

    // Устанавливаем флаги
    jmethodID set_flags = (*env)->GetMethodID(env, intent_class, "setFlags", "(I)Landroid/content/Intent;");
    if (set_flags != NULL) {
        jint flags = 0x10000000 | 0x00200000; // FLAG_ACTIVITY_NEW_TASK | FLAG_ACTIVITY_CLEAR_TOP
        (*env)->CallObjectMethod(env, launch_intent, set_flags, flags);
    }

    // Создаем PendingIntent с правильными флагами для Android 12+
    jclass pending_intent_class = (*env)->FindClass(env, "android/app/PendingIntent");
    if (pending_intent_class == NULL) {
        LogD("C: ERROR - PendingIntent class not found");
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, category_string);
        (*env)->DeleteLocalRef(env, package_name);
        (*env)->DeleteLocalRef(env, class_name);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jmethodID get_activity_method = (*env)->GetStaticMethodID(env, pending_intent_class,
        "getActivity", "(Landroid/content/Context;ILandroid/content/Intent;I)Landroid/app/PendingIntent;");
    if (get_activity_method == NULL) {
        LogD("C: ERROR - getActivity method not found");
        (*env)->DeleteLocalRef(env, pending_intent_class);
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, category_string);
        (*env)->DeleteLocalRef(env, package_name);
        (*env)->DeleteLocalRef(env, class_name);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jint request_code = 0;

    // Правильные флаги для Android 12+ (API 31+)
    jint pending_flags = 0x04000000; // FLAG_IMMUTABLE (добавлен в API 23, обязателен с API 31)

    // Для Android 12+ используем FLAG_IMMUTABLE, для старых версий можем использовать 0
    if (api_level < 31) {
        pending_flags = 0;
    }

    jobject pending_intent = (*env)->CallStaticObjectMethod(env, pending_intent_class,
        get_activity_method, context, request_code, launch_intent, pending_flags);

    // Get NotificationManager service
    jclass context_class = (*env)->GetObjectClass(env, context);
    jmethodID get_system_service = (*env)->GetMethodID(env, context_class,
        "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");
    if (get_system_service == NULL) {
        LogD("C: ERROR - getSystemService method not found");
        (*env)->DeleteLocalRef(env, pending_intent_class);
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, category_string);
        (*env)->DeleteLocalRef(env, package_name);
        (*env)->DeleteLocalRef(env, class_name);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, context_class);
        return;
    }

    jstring service_name = (*env)->NewStringUTF(env, "notification");
    jobject notification_manager = (*env)->CallObjectMethod(env, context, get_system_service, service_name);

    // Create Notification.Builder
    jclass builder_class = (*env)->FindClass(env, "android/app/Notification$Builder");
    if (builder_class == NULL) {
        LogD("C: ERROR - Notification.Builder class not found");
        (*env)->DeleteLocalRef(env, service_name);
        (*env)->DeleteLocalRef(env, context_class);
        (*env)->DeleteLocalRef(env, pending_intent_class);
        (*env)->DeleteLocalRef(env, action_string);
        (*env)->DeleteLocalRef(env, category_string);
        (*env)->DeleteLocalRef(env, package_name);
        (*env)->DeleteLocalRef(env, class_name);
        (*env)->DeleteLocalRef(env, launch_intent);
        (*env)->DeleteLocalRef(env, intent_class);
        return;
    }

    jmethodID builder_constructor;
    jobject builder;

    // Use channel for Android 8+ (API level 26+)
    if (api_level >= 26) {
        builder_constructor = (*env)->GetMethodID(env, builder_class, "<init>",
            "(Landroid/content/Context;Ljava/lang/String;)V");
        if (builder_constructor == NULL) {
            LogD("C: ERROR - Notification.Builder constructor (with channel) not found");
            (*env)->DeleteLocalRef(env, service_name);
            (*env)->DeleteLocalRef(env, context_class);
            (*env)->DeleteLocalRef(env, builder_class);
            (*env)->DeleteLocalRef(env, pending_intent_class);
            (*env)->DeleteLocalRef(env, action_string);
            (*env)->DeleteLocalRef(env, category_string);
            (*env)->DeleteLocalRef(env, package_name);
            (*env)->DeleteLocalRef(env, class_name);
            (*env)->DeleteLocalRef(env, launch_intent);
            (*env)->DeleteLocalRef(env, intent_class);
            return;
        }
        jstring channel_id = (*env)->NewStringUTF(env, "croc_channel");
        builder = (*env)->NewObject(env, builder_class, builder_constructor, context, channel_id);
        (*env)->DeleteLocalRef(env, channel_id);
    } else {
        builder_constructor = (*env)->GetMethodID(env, builder_class, "<init>",
            "(Landroid/content/Context;)V");
        if (builder_constructor == NULL) {
            LogD("C: ERROR - Notification.Builder constructor not found");
            (*env)->DeleteLocalRef(env, service_name);
            (*env)->DeleteLocalRef(env, context_class);
            (*env)->DeleteLocalRef(env, builder_class);
            (*env)->DeleteLocalRef(env, pending_intent_class);
            (*env)->DeleteLocalRef(env, action_string);
            (*env)->DeleteLocalRef(env, category_string);
            (*env)->DeleteLocalRef(env, package_name);
            (*env)->DeleteLocalRef(env, class_name);
            (*env)->DeleteLocalRef(env, launch_intent);
            (*env)->DeleteLocalRef(env, intent_class);
            return;
        }
        builder = (*env)->NewObject(env, builder_class, builder_constructor, context);
    }

    // Set notification content
    jstring jtitle = (*env)->NewStringUTF(env, title);
    jstring jcontent = (*env)->NewStringUTF(env, content);

    jmethodID set_title = (*env)->GetMethodID(env, builder_class, "setContentTitle",
        "(Ljava/lang/CharSequence;)Landroid/app/Notification$Builder;");
    if (set_title != NULL) {
        (*env)->CallObjectMethod(env, builder, set_title, jtitle);
    }

    jmethodID set_content = (*env)->GetMethodID(env, builder_class, "setContentText",
        "(Ljava/lang/CharSequence;)Landroid/app/Notification$Builder;");
    if (set_content != NULL) {
        (*env)->CallObjectMethod(env, builder, set_content, jcontent);
    }

    jmethodID set_small_icon = (*env)->GetMethodID(env, builder_class, "setSmallIcon",
        "(I)Landroid/app/Notification$Builder;");
    if (set_small_icon != NULL) {
        (*env)->CallObjectMethod(env, builder, set_small_icon, 17301651); // android.R.drawable.ic_dialog_info
    }

    jmethodID set_auto_cancel = (*env)->GetMethodID(env, builder_class, "setAutoCancel",
        "(Z)Landroid/app/Notification$Builder;");
    if (set_auto_cancel != NULL) {
        (*env)->CallObjectMethod(env, builder, set_auto_cancel, JNI_TRUE);
    }

    jmethodID set_content_intent = (*env)->GetMethodID(env, builder_class, "setContentIntent",
        "(Landroid/app/PendingIntent;)Landroid/app/Notification$Builder;");
    if (set_content_intent != NULL && pending_intent != NULL) {
        (*env)->CallObjectMethod(env, builder, set_content_intent, pending_intent);
    }

    // Build the notification
    jmethodID build_method = (*env)->GetMethodID(env, builder_class, "build",
        "()Landroid/app/Notification;");
    if (build_method != NULL) {
        jobject notification = (*env)->CallObjectMethod(env, builder, build_method);

        // Show the notification
        jclass notification_manager_class = (*env)->FindClass(env, "android/app/NotificationManager");
        if (notification_manager_class == NULL) {
            LogD("C: ERROR - NotificationManager class not found for notify");
        } else {
            jmethodID notify_method = (*env)->GetMethodID(env, notification_manager_class,
                "notify", "(ILandroid/app/Notification;)V");
            if (notify_method == NULL) {
                LogD("C: ERROR - notify method not found");
            } else {
                (*env)->CallVoidMethod(env, notification_manager, notify_method, 1, notification);
            }
            (*env)->DeleteLocalRef(env, notification_manager_class);
        }
        (*env)->DeleteLocalRef(env, notification);
    }

    // Cleanup
    (*env)->DeleteLocalRef(env, jtitle);
    (*env)->DeleteLocalRef(env, jcontent);
    (*env)->DeleteLocalRef(env, builder);
    (*env)->DeleteLocalRef(env, pending_intent);
    (*env)->DeleteLocalRef(env, launch_intent);
    (*env)->DeleteLocalRef(env, intent_class);
    (*env)->DeleteLocalRef(env, pending_intent_class);
    (*env)->DeleteLocalRef(env, action_string);
    (*env)->DeleteLocalRef(env, category_string);
    (*env)->DeleteLocalRef(env, package_name);
    (*env)->DeleteLocalRef(env, class_name);
    (*env)->DeleteLocalRef(env, service_name);
    (*env)->DeleteLocalRef(env, context_class);
    (*env)->DeleteLocalRef(env, builder_class);
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// apiLevel возвращает уровень API Android устройства
func apiLevel() int {
	level := -1
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		level = int(C.get_api_level((*C.JNIEnv)(unsafe.Pointer(ac.Env))))
		return nil
	})
	return level
}

func showCrocNotification(title, content string) {
	log.Debug("Showing notification: ", title)

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		ctitle := C.CString(title)
		ccontent := C.CString(content)
		defer C.free(unsafe.Pointer(ctitle))
		defer C.free(unsafe.Pointer(ccontent))

		C.showCrocNotification(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			ctitle,
			ccontent,
		)

		return nil
	})
}

func sendNotification(_ fyne.App, title, content string) {
	showCrocNotification(title, content)
}
