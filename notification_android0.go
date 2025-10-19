//go:build ignore

package main

import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
	"github.com/timob/jnigi"
)

func showCrocNotification(title, content string) {
	log.Trace("showCrocNotification")
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := jnigi.WrapEnv(unsafe.Pointer(ac.Env))
		context := jnigi.WrapJObject(uintptr(unsafe.Pointer(ac.Ctx)), "android/content/Context", false)

		if err := showAndroidNotification(env, context, title, content); err != nil {
			log.Trace("Error showing notification: " + err.Error())
		}
		return nil
	})
}

func showAndroidNotification(env *jnigi.Env, context *jnigi.ObjectRef, title, content string) error {
	log.Trace("=== showAndroidNotification STARTED ===")

	if isOreoOrLater(env) {
		if err := createNotificationChannel(env, context); err != nil {
			return err
		}
	}

	launchIntent, err := createLaunchIntent(env)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(launchIntent)

	pendingIntent, err := createPendingIntent(env, context, launchIntent)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(pendingIntent)

	notificationManager, err := getNotificationManager(env, context)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(notificationManager)

	builder, err := createNotificationBuilder(env, context)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(builder)

	if err := setupNotification(env, builder, title, content, pendingIntent); err != nil {
		return err
	}

	if err := buildAndShowNotification(env, builder, notificationManager); err != nil {
		return err
	}

	log.Trace("=== showAndroidNotification COMPLETED ===")
	return nil
}

func isOreoOrLater(env *jnigi.Env) bool {
	var sdkVersion int32
	err := env.GetStaticField("android/os/Build$VERSION", "SDK_INT", &sdkVersion)
	if err != nil {
		log.Trace("Error getting SDK_INT: " + err.Error())
		return false
	}
	return sdkVersion >= 26
}

func createNotificationChannel(env *jnigi.Env, context *jnigi.ObjectRef) error {
	log.Trace("Creating notification channel")

	serviceName, err := env.NewObject("java/lang/String", []byte("notification"))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(serviceName)

	notificationManagerObj := jnigi.NewObjectRef("java/lang/Object")
	err = context.CallMethod(env, "getSystemService", notificationManagerObj, serviceName)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(notificationManagerObj)

	channelID, err := env.NewObject("java/lang/String", []byte("croc_channel"))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(channelID)

	channelName, err := env.NewObject("java/lang/String", []byte("Croc Notifications"))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(channelName)

	importance := int32(3) // IMPORTANCE_DEFAULT

	channel, err := env.NewObject("android/app/NotificationChannel", channelID, channelName, importance)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(channel)

	description, err := env.NewObject("java/lang/String", []byte("Application notifications"))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(description)

	err = channel.CallMethod(env, "setDescription", nil, description)
	if err != nil {
		return err
	}

	err = notificationManagerObj.CallMethod(env, "createNotificationChannel", nil, channel)
	if err != nil {
		return err
	}

	log.Trace("Notification channel created")
	return nil
}

func createLaunchIntent(env *jnigi.Env) (*jnigi.ObjectRef, error) {
	intent, err := env.NewObject("android/content/Intent")
	if err != nil {
		return nil, err
	}

	action, err := env.NewObject("java/lang/String", []byte("android.intent.action.MAIN"))
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}
	defer env.DeleteLocalRef(action)

	err = intent.CallMethod(env, "setAction", nil, action)
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}

	category, err := env.NewObject("java/lang/String", []byte("android.intent.category.LAUNCHER"))
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}
	defer env.DeleteLocalRef(category)

	err = intent.CallMethod(env, "addCategory", nil, category)
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}

	flags := int32(0x10000000 | 0x00200000) // FLAG_ACTIVITY_NEW_TASK | FLAG_ACTIVITY_RESET_TASK_IF_NEEDED
	err = intent.CallMethod(env, "setFlags", nil, flags)
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}

	pkg, err := env.NewObject("java/lang/String", []byte("com.github.howeyc.crocgui"))
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}
	defer env.DeleteLocalRef(pkg)

	cls, err := env.NewObject("java/lang/String", []byte("org.golang.app.GoNativeActivity"))
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}
	defer env.DeleteLocalRef(cls)

	err = intent.CallMethod(env, "setClassName", nil, pkg, cls)
	if err != nil {
		env.DeleteLocalRef(intent)
		return nil, err
	}

	log.Trace("Launch intent created")
	return intent, nil
}

func createPendingIntent(env *jnigi.Env, context *jnigi.ObjectRef, intent *jnigi.ObjectRef) (*jnigi.ObjectRef, error) {
	pendingIntent := jnigi.NewObjectRef("android/app/PendingIntent")
	requestCode := int32(0)
	flags := int32(0x8000000) // FLAG_UPDATE_CURRENT

	err := env.CallStaticMethod("android/app/PendingIntent", "getActivity", pendingIntent, context, requestCode, intent, flags)
	if err != nil {
		return nil, err
	}

	log.Trace("PendingIntent created")
	return pendingIntent, nil
}

func getNotificationManager(env *jnigi.Env, context *jnigi.ObjectRef) (*jnigi.ObjectRef, error) {
	serviceName, err := env.NewObject("java/lang/String", []byte("notification"))
	if err != nil {
		return nil, err
	}
	defer env.DeleteLocalRef(serviceName)

	notificationManager := jnigi.NewObjectRef("android/app/NotificationManager")
	err = context.CallMethod(env, "getSystemService", notificationManager, serviceName)
	if err != nil {
		return nil, err
	}
	return notificationManager, nil
}

func createNotificationBuilder(env *jnigi.Env, context *jnigi.ObjectRef) (*jnigi.ObjectRef, error) {
	if isOreoOrLater(env) {
		channelID, err := env.NewObject("java/lang/String", []byte("croc_channel"))
		if err != nil {
			return nil, err
		}
		defer env.DeleteLocalRef(channelID)

		builder, err := env.NewObject("android/app/Notification$Builder", context, channelID)
		if err != nil {
			return nil, err
		}
		return builder, nil
	} else {
		builder, err := env.NewObject("android/app/Notification$Builder", context)
		if err != nil {
			return nil, err
		}
		return builder, nil
	}
}

func setupNotification(env *jnigi.Env, builder *jnigi.ObjectRef, title, content string, pendingIntent *jnigi.ObjectRef) error {
	jtitle, err := env.NewObject("java/lang/String", []byte(title))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(jtitle)

	jcontent, err := env.NewObject("java/lang/String", []byte(content))
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(jcontent)

	err = builder.CallMethod(env, "setContentTitle", nil, jtitle)
	if err != nil {
		return err
	}

	err = builder.CallMethod(env, "setContentText", nil, jcontent)
	if err != nil {
		return err
	}

	iconID := int32(17301651) // android.R.drawable.ic_dialog_info
	err = builder.CallMethod(env, "setSmallIcon", nil, iconID)
	if err != nil {
		return err
	}

	err = builder.CallMethod(env, "setAutoCancel", nil, true)
	if err != nil {
		return err
	}

	err = builder.CallMethod(env, "setContentIntent", nil, pendingIntent)
	if err != nil {
		return err
	}

	log.Trace("Notification setup completed")
	return nil
}

func buildAndShowNotification(env *jnigi.Env, builder *jnigi.ObjectRef, notificationManager *jnigi.ObjectRef) error {
	notification := jnigi.NewObjectRef("android/app/Notification")
	err := builder.CallMethod(env, "build", notification)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(notification)

	err = notificationManager.CallMethod(env, "notify", nil, int32(1), notification)
	if err != nil {
		return err
	}

	log.Trace("Notification shown successfully")
	return nil
}

func sendNotification(a fyne.App, title, content string) {
	log.Trace("sendNotification")
	showCrocNotification(title, content)
}
