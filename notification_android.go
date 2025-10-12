//go:build android

package main

/*
#include <android/log.h>
#include <jni.h>
#include <stdlib.h>
#include <string.h>

void logToAndroid(const char* tag, const char* message) {
    __android_log_write(ANDROID_LOG_DEBUG, tag, message);
}

// Check if device is Android Oreo or later (renamed to avoid conflict)
jboolean checkIsOreoOrLater(JNIEnv* env) {
    jclass version_class = (*env)->FindClass(env, "android/os/Build$VERSION");
    if (version_class == NULL) {
        logToAndroid("croc", "C: ERROR - Build.VERSION class not found");
        return JNI_FALSE;
    }

    jfieldID sdk_int_field = (*env)->GetStaticFieldID(env, version_class, "SDK_INT", "I");
    if (sdk_int_field == NULL) {
        logToAndroid("croc", "C: ERROR - SDK_INT field not found");
        (*env)->DeleteLocalRef(env, version_class);
        return JNI_FALSE;
    }

    jint sdk_version = (*env)->GetStaticIntField(env, version_class, sdk_int_field);
    (*env)->DeleteLocalRef(env, version_class);

    return sdk_version >= 26 ? JNI_TRUE : JNI_FALSE; // 26 = Android 8.0 Oreo
}

// Create notification channel for Android 8+
void createCrocNotificationChannel(JNIEnv* env, jobject context) {
    logToAndroid("croc", "C: Creating notification channel");

    jclass notification_manager_class = (*env)->FindClass(env, "android/app/NotificationManager");
    if (notification_manager_class == NULL) {
        logToAndroid("croc", "C: ERROR - NotificationManager class not found");
        return;
    }

    // Get NotificationManager service
    jmethodID get_system_service = (*env)->GetMethodID(env,
        (*env)->GetObjectClass(env, context),
        "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");

    jstring service_name = (*env)->NewStringUTF(env, "notification");
    jobject notification_manager = (*env)->CallObjectMethod(env, context, get_system_service, service_name);

    // Create notification channel
    jclass channel_class = (*env)->FindClass(env, "android/app/NotificationChannel");
    if (channel_class == NULL) {
        logToAndroid("croc", "C: ERROR - NotificationChannel class not found");
        (*env)->DeleteLocalRef(env, service_name);
        (*env)->DeleteLocalRef(env, notification_manager_class);
        return;
    }

    jstring channel_id = (*env)->NewStringUTF(env, "croc_channel");
    jstring channel_name = (*env)->NewStringUTF(env, "Croc Notifications");
    jint importance = 3; // IMPORTANCE_DEFAULT

    jmethodID channel_constructor = (*env)->GetMethodID(env, channel_class, "<init>",
        "(Ljava/lang/String;Ljava/lang/CharSequence;I)V");
    jobject channel = (*env)->NewObject(env, channel_class, channel_constructor,
        channel_id, channel_name, importance);

    // Set channel description
    jmethodID set_description = (*env)->GetMethodID(env, channel_class, "setDescription",
        "(Ljava/lang/String;)V");
    jstring description = (*env)->NewStringUTF(env, "Application notifications");
    (*env)->CallVoidMethod(env, channel, set_description, description);

    // Create the channel
    jmethodID create_channel = (*env)->GetMethodID(env, notification_manager_class,
        "createNotificationChannel", "(Landroid/app/NotificationChannel;)V");
    (*env)->CallVoidMethod(env, notification_manager, create_channel, channel);

    logToAndroid("croc", "C: Notification channel created");

    // Cleanup
    (*env)->DeleteLocalRef(env, channel_id);
    (*env)->DeleteLocalRef(env, channel_name);
    (*env)->DeleteLocalRef(env, description);
    (*env)->DeleteLocalRef(env, channel);
    (*env)->DeleteLocalRef(env, service_name);
    (*env)->DeleteLocalRef(env, notification_manager_class);
    (*env)->DeleteLocalRef(env, channel_class);
}

void showCrocNotification(JNIEnv* env, jobject context, char* title, char* content) {
    logToAndroid("croc", "C: Start showCrocNotification");

    // Create notification channel for Android 8+
    if (checkIsOreoOrLater(env)) {
        createCrocNotificationChannel(env, context);
    }

    // === СОЗДАЕМ INTENT ДЛЯ ЗАПУСКА ПРИЛОЖЕНИЯ ===
    logToAndroid("croc", "C: Creating launch intent");

    // Создаем Intent для запуска главной активности
    jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
    jmethodID intent_constructor = (*env)->GetMethodID(env, intent_class, "<init>", "()V");
    jobject launch_intent = (*env)->NewObject(env, intent_class, intent_constructor);

    // Устанавливаем действие и категорию для запуска приложения
    jmethodID set_action = (*env)->GetMethodID(env, intent_class, "setAction", "(Ljava/lang/String;)Landroid/content/Intent;");
    jstring action_string = (*env)->NewStringUTF(env, "android.intent.action.MAIN");
    (*env)->CallObjectMethod(env, launch_intent, set_action, action_string);

    jmethodID add_category = (*env)->GetMethodID(env, intent_class, "addCategory", "(Ljava/lang/String;)Landroid/content/Intent;");
    jstring category_string = (*env)->NewStringUTF(env, "android.intent.category.LAUNCHER");
    (*env)->CallObjectMethod(env, launch_intent, add_category, category_string);

    // Устанавлием флаги для правильного запуска
    jmethodID set_flags = (*env)->GetMethodID(env, intent_class, "setFlags", "(I)Landroid/content/Intent;");
    jint flags = 0x10000000 | 0x00200000; // FLAG_ACTIVITY_NEW_TASK | FLAG_ACTIVITY_RESET_TASK_IF_NEEDED
    (*env)->CallObjectMethod(env, launch_intent, set_flags, flags);

    // Устанавливаем класс активности для запуска
    jmethodID set_class = (*env)->GetMethodID(env, intent_class, "setClassName", "(Ljava/lang/String;Ljava/lang/String;)Landroid/content/Intent;");
    jstring package_name = (*env)->NewStringUTF(env, "com.github.howeyc.crocgui");
    jstring class_name = (*env)->NewStringUTF(env, "org.golang.app.GoNativeActivity");
    (*env)->CallObjectMethod(env, launch_intent, set_class, package_name, class_name);

    // Создаем PendingIntent
    jclass pending_intent_class = (*env)->FindClass(env, "android/app/PendingIntent");
    jmethodID get_activity_method = (*env)->GetStaticMethodID(env, pending_intent_class,
        "getActivity", "(Landroid/content/Context;ILandroid/content/Intent;I)Landroid/app/PendingIntent;");

    jint request_code = 0;
    jint pending_flags = 0x8000000; // FLAG_UPDATE_CURRENT

    jobject pending_intent = (*env)->CallStaticObjectMethod(env, pending_intent_class,
        get_activity_method, context, request_code, launch_intent, pending_flags);

    logToAndroid("croc", "C: PendingIntent created");

    // Get NotificationManager service
    jclass context_class = (*env)->GetObjectClass(env, context);
    jmethodID get_system_service = (*env)->GetMethodID(env, context_class,
        "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;");

    jstring service_name = (*env)->NewStringUTF(env, "notification");
    jobject notification_manager = (*env)->CallObjectMethod(env, context, get_system_service, service_name);

    // Create Notification.Builder
    jclass builder_class = (*env)->FindClass(env, "android/app/Notification$Builder");
    if (builder_class == NULL) {
        logToAndroid("croc", "C: ERROR - Notification.Builder class not found");
        (*env)->DeleteLocalRef(env, service_name);
        (*env)->DeleteLocalRef(env, context_class);
        return;
    }

    jmethodID builder_constructor;
    jobject builder;

    if (checkIsOreoOrLater(env)) {
        // For Android 8+ use constructor with channel ID
        builder_constructor = (*env)->GetMethodID(env, builder_class, "<init>",
            "(Landroid/content/Context;Ljava/lang/String;)V");
        jstring channel_id = (*env)->NewStringUTF(env, "croc_channel");
        builder = (*env)->NewObject(env, builder_class, builder_constructor, context, channel_id);
        (*env)->DeleteLocalRef(env, channel_id);
    } else {
        // For older Android versions
        builder_constructor = (*env)->GetMethodID(env, builder_class, "<init>",
            "(Landroid/content/Context;)V");
        builder = (*env)->NewObject(env, builder_class, builder_constructor, context);
    }

    // Set notification content
    jstring jtitle = (*env)->NewStringUTF(env, title);
    jstring jcontent = (*env)->NewStringUTF(env, content);

    jmethodID set_title = (*env)->GetMethodID(env, builder_class, "setContentTitle",
        "(Ljava/lang/CharSequence;)Landroid/app/Notification$Builder;");
    jmethodID set_content = (*env)->GetMethodID(env, builder_class, "setContentText",
        "(Ljava/lang/CharSequence;)Landroid/app/Notification$Builder;");
    jmethodID set_small_icon = (*env)->GetMethodID(env, builder_class, "setSmallIcon",
        "(I)Landroid/app/Notification$Builder;");
    jmethodID set_auto_cancel = (*env)->GetMethodID(env, builder_class, "setAutoCancel",
        "(Z)Landroid/app/Notification$Builder;");
    jmethodID set_content_intent = (*env)->GetMethodID(env, builder_class, "setContentIntent",
        "(Landroid/app/PendingIntent;)Landroid/app/Notification$Builder;");

    // Устанавливаем контент и интент
    (*env)->CallObjectMethod(env, builder, set_title, jtitle);
    (*env)->CallObjectMethod(env, builder, set_content, jcontent);
    (*env)->CallObjectMethod(env, builder, set_small_icon, 17301651); // android.R.drawable.ic_dialog_info
    (*env)->CallObjectMethod(env, builder, set_auto_cancel, JNI_TRUE);
    (*env)->CallObjectMethod(env, builder, set_content_intent, pending_intent); // Устанавливаем PendingIntent

    // Build the notification
    jmethodID build_method = (*env)->GetMethodID(env, builder_class, "build",
        "()Landroid/app/Notification;");
    jobject notification = (*env)->CallObjectMethod(env, builder, build_method);

    // Show the notification
    jclass notification_manager_class = (*env)->FindClass(env, "android/app/NotificationManager");
    jmethodID notify_method = (*env)->GetMethodID(env, notification_manager_class,
        "notify", "(ILandroid/app/Notification;)V");
    (*env)->CallVoidMethod(env, notification_manager, notify_method, 1, notification);

    logToAndroid("croc", "C: Notification with intent created and shown");

    // Cleanup
    (*env)->DeleteLocalRef(env, jtitle);
    (*env)->DeleteLocalRef(env, jcontent);
    (*env)->DeleteLocalRef(env, builder);
    (*env)->DeleteLocalRef(env, notification);
    (*env)->DeleteLocalRef(env, notification_manager_class);
    (*env)->DeleteLocalRef(env, pending_intent);
    (*env)->DeleteLocalRef(env, launch_intent);
    (*env)->DeleteLocalRef(env, intent_class);
    (*env)->DeleteLocalRef(env, pending_intent_class);
    (*env)->DeleteLocalRef(env, action_string);
    (*env)->DeleteLocalRef(env, category_string);
    (*env)->DeleteLocalRef(env, package_name);
    (*env)->DeleteLocalRef(env, class_name);

    // Final cleanup
    (*env)->DeleteLocalRef(env, service_name);
    (*env)->DeleteLocalRef(env, context_class);
    (*env)->DeleteLocalRef(env, builder_class);

    logToAndroid("croc", "C: End showCrocNotification");
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

func showSimpleNotification(title, content string) {
	LogD("Go: Trying to show notification: " + title)

	driver.RunNative(func(ctx interface{}) error {
		LogD("Go: Starting native code")

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

		LogD("Go: Native code completed")
		return nil
	})
}

func sendNotification(a fyne.App, title, content string) {
	LogD("Calling sendNotification: " + title)
	showSimpleNotification(title, content)
}

func LogD(message string) {
	ctag := C.CString("croc")
	cmessage := C.CString(message)
	defer C.free(unsafe.Pointer(ctag))
	defer C.free(unsafe.Pointer(cmessage))

	C.logToAndroid(ctag, cmessage)
}
