//go:build ignore

package main

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
	"github.com/timob/jnigi"
)

// processIntent обрабатывает Android интент через jnigi
func processIntent() {
	log.Trace("processIntent called")

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := jnigi.WrapEnv(unsafe.Pointer(ac.Env))

		activity := jnigi.WrapJObject(uintptr(ac.Ctx), "android/app/Activity", false)

		if err := processAndroidIntent(env, activity); err != nil {
			log.Trace("Error processing intent: " + err.Error())
		}

		log.Trace("Intent processing completed")
		return nil
	})
}

func processAndroidIntent(env *jnigi.Env, activity *jnigi.ObjectRef) error {
	log.Trace("=== processAndroidIntent STARTED ===")

	// Получаем интент активности
	intent, err := getIntent(env, activity)
	if err != nil {
		return err
	}
	defer env.DeleteLocalRef(intent)

	// Получаем action интента
	action, isSend, isView, isSendMultiple, isMain, err := getIntentAction(env, intent)
	if err != nil {
		return err
	}

	log.Trace("Intent action: " + action)

	if isMain {
		log.Trace("MAIN intent - starting main app")
		return nil
	}

	// Флаг для отслеживания успешной обработки
	hasValidData := false

	// 1. Проверяем ClipData (приоритетный способ)
	if processed := processClipData(env, intent, &hasValidData); processed {
		log.Trace("ClipData processed successfully")
	}

	// 2. Проверяем ACTION_SEND text/plain
	if !hasValidData && isSend {
		if processed := processSendText(env, intent, &hasValidData); processed {
			log.Trace("SEND text processed successfully")
		}
	}

	// 3. Проверяем ACTION_VIEW URI
	if !hasValidData && isView {
		if processed := processViewUri(env, intent, &hasValidData); processed {
			log.Trace("VIEW URI processed successfully")
		}
	}

	// 4. Проверяем ACTION_SEND stream
	if !hasValidData && isSend {
		if processed := processSendStream(env, intent, &hasValidData); processed {
			log.Trace("SEND stream processed successfully")
		}
	}

	// 5. Проверяем ACTION_SEND_MULTIPLE
	if !hasValidData && isSendMultiple {
		if processed := processSendMultiple(env, intent, &hasValidData); processed {
			log.Trace("SEND_MULTIPLE processed successfully")
		}
	}

	// Устанавливаем результат и завершаем активность
	if hasValidData {
		setActivityResult(env, activity, -1) // RESULT_OK
		log.Trace("Setting RESULT_OK")
	} else {
		setActivityResult(env, activity, 0) // RESULT_CANCELED
		log.Trace("Setting RESULT_CANCELED")
	}

	// finishActivity(env, activity)
	log.Trace("=== processAndroidIntent COMPLETED ===")
	return nil
}

// Вспомогательные функции

func getIntent(env *jnigi.Env, activity *jnigi.ObjectRef) (*jnigi.ObjectRef, error) {
	intent := jnigi.NewObjectRef("android/content/Intent")
	err := activity.CallMethod(env, "getIntent", intent)
	if err != nil {
		return nil, err
	}

	if intent.IsNil() {
		return nil, &IntentError{"Intent is NULL"}
	}

	return intent, nil
}

func getIntentAction(env *jnigi.Env, intent *jnigi.ObjectRef) (string, bool, bool, bool, bool, error) {
	actionStr := jnigi.NewObjectRef("java/lang/String")
	err := intent.CallMethod(env, "getAction", actionStr)
	if err != nil {
		return "", false, false, false, false, err
	}
	defer env.DeleteLocalRef(actionStr)

	if actionStr.IsNil() {
		return "", false, false, false, false, nil
	}

	var actionBytes []byte
	err = actionStr.CallMethod(env, "getBytes", &actionBytes)
	if err != nil {
		return "", false, false, false, false, err
	}

	action := string(actionBytes)
	isSend := action == "android.intent.action.SEND"
	isView := action == "android.intent.action.VIEW"
	isSendMultiple := action == "android.intent.action.SEND_MULTIPLE"
	isMain := action == "android.intent.action.MAIN"

	return action, isSend, isView, isSendMultiple, isMain, nil
}

func processClipData(env *jnigi.Env, intent *jnigi.ObjectRef, hasValidData *bool) bool {
	log.Trace("Checking for ClipData...")

	clipData := jnigi.NewObjectRef("android/content/ClipData")
	err := intent.CallMethod(env, "getClipData", clipData)
	if err != nil || clipData.IsNil() {
		log.Trace("No ClipData found")
		return false
	}
	defer env.DeleteLocalRef(clipData)

	var itemCount int32
	err = clipData.CallMethod(env, "getItemCount", &itemCount)
	if err != nil {
		return false
	}

	log.Tracef("ClipData item count: %d", itemCount)

	for i := int32(0); i < itemCount; i++ {
		item := jnigi.NewObjectRef("android/content/ClipData$Item")
		err = clipData.CallMethod(env, "getItemAt", item, i)
		if err != nil || item.IsNil() {
			continue
		}

		// Обрабатываем URI из ClipData
		if processItemUri(env, item) {
			*hasValidData = true
		}

		// Обрабатываем текст из ClipData
		if processItemText(env, item) {
			*hasValidData = true
		}

		env.DeleteLocalRef(item)
	}

	return *hasValidData
}

func processItemUri(env *jnigi.Env, item *jnigi.ObjectRef) bool {
	uri := jnigi.NewObjectRef("android/net/Uri")
	err := item.CallMethod(env, "getUri", uri)
	if err != nil || uri.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uri)

	uriStr := jnigi.NewObjectRef("java/lang/String")
	err = uri.CallMethod(env, "toString", uriStr)
	if err != nil || uriStr.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uriStr)

	var uriBytes []byte
	err = uriStr.CallMethod(env, "getBytes", &uriBytes)
	if err != nil {
		return false
	}

	uriString := string(uriBytes)
	log.Tracef("Sending URI from ClipData: %s", uriString)

	select {
	case uriFromIntent <- uriString:
		log.Trace("URI sent to channel successfully")
		return true
	default:
		log.Trace("WARNING - URI channel full")
		return false
	}
}

func processItemText(env *jnigi.Env, item *jnigi.ObjectRef) bool {
	textSeq := jnigi.NewObjectRef("java/lang/CharSequence")
	err := item.CallMethod(env, "getText", textSeq)
	if err != nil || textSeq.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(textSeq)

	textStr := jnigi.NewObjectRef("java/lang/String")
	err = textSeq.CallMethod(env, "toString", textStr)
	if err != nil || textStr.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(textStr)

	var textBytes []byte
	err = textStr.CallMethod(env, "getBytes", &textBytes)
	if err != nil {
		return false
	}

	textString := string(textBytes)
	logText := textString
	if len(logText) > 100 {
		logText = logText[:100] + "..."
	}
	log.Tracef("Sending text from ClipData: %s", logText)

	select {
	case textFromIntent <- textString:
		log.Trace("Text sent to channel successfully")
		return true
	default:
		log.Trace("WARNING - Text channel full")
		return false
	}
}

func processSendText(env *jnigi.Env, intent *jnigi.ObjectRef, hasValidData *bool) bool {
	log.Trace("Checking for SEND text/plain...")

	typeStr := jnigi.NewObjectRef("java/lang/String")
	err := intent.CallMethod(env, "getType", typeStr)
	if err != nil || typeStr.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(typeStr)

	var typeBytes []byte
	err = typeStr.CallMethod(env, "getBytes", &typeBytes)
	if err != nil {
		return false
	}

	if string(typeBytes) != "text/plain" {
		return false
	}

	extraKey, _ := env.NewObject("java/lang/String", []byte("android.intent.extra.TEXT"))
	defer env.DeleteLocalRef(extraKey)

	text := jnigi.NewObjectRef("java/lang/String")
	err = intent.CallMethod(env, "getStringExtra", text, extraKey)
	if err != nil || text.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(text)

	var textBytes []byte
	err = text.CallMethod(env, "getBytes", &textBytes)
	if err != nil {
		return false
	}

	textString := string(textBytes)
	logText := textString
	if len(logText) > 100 {
		logText = logText[:100] + "..."
	}
	log.Tracef("Sending SEND text: %s", logText)

	select {
	case textFromIntent <- textString:
		log.Trace("SEND text sent to channel successfully")
		*hasValidData = true
		return true
	default:
		log.Trace("WARNING - Text channel full")
		return false
	}
}

func processViewUri(env *jnigi.Env, intent *jnigi.ObjectRef, hasValidData *bool) bool {
	log.Trace("Checking for VIEW URI...")

	uri := jnigi.NewObjectRef("android/net/Uri")
	err := intent.CallMethod(env, "getData", uri)
	if err != nil || uri.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uri)

	uriStr := jnigi.NewObjectRef("java/lang/String")
	err = uri.CallMethod(env, "toString", uriStr)
	if err != nil || uriStr.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uriStr)

	var uriBytes []byte
	err = uriStr.CallMethod(env, "getBytes", &uriBytes)
	if err != nil {
		return false
	}

	uriString := string(uriBytes)
	log.Tracef("Sending VIEW URI: %s", uriString)

	select {
	case uriFromIntent <- uriString:
		log.Trace("VIEW URI sent to channel successfully")
		*hasValidData = true
		return true
	default:
		log.Trace("WARNING - URI channel full")
		return false
	}
}

func processSendStream(env *jnigi.Env, intent *jnigi.ObjectRef, hasValidData *bool) bool {
	log.Trace("Checking for SEND stream...")

	extraKey, _ := env.NewObject("java/lang/String", []byte("android.intent.extra.STREAM"))
	defer env.DeleteLocalRef(extraKey)

	uri := jnigi.NewObjectRef("android/net/Uri")
	err := intent.CallMethod(env, "getParcelableExtra", uri, extraKey)
	if err != nil || uri.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uri)

	uriStr := jnigi.NewObjectRef("java/lang/String")
	err = uri.CallMethod(env, "toString", uriStr)
	if err != nil || uriStr.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uriStr)

	var uriBytes []byte
	err = uriStr.CallMethod(env, "getBytes", &uriBytes)
	if err != nil {
		return false
	}

	uriString := string(uriBytes)
	log.Tracef("Sending SEND stream URI: %s", uriString)

	select {
	case uriFromIntent <- uriString:
		log.Trace("SEND stream URI sent to channel successfully")
		*hasValidData = true
		return true
	default:
		log.Trace("WARNING - URI channel full")
		return false
	}
}

func processSendMultiple(env *jnigi.Env, intent *jnigi.ObjectRef, hasValidData *bool) bool {
	log.Trace("Checking for SEND_MULTIPLE...")

	arrayKey, _ := env.NewObject("java/lang/String", []byte("android.intent.extra.STREAM"))
	defer env.DeleteLocalRef(arrayKey)

	uriList := jnigi.NewObjectRef("java/util/ArrayList")
	err := intent.CallMethod(env, "getParcelableArrayListExtra", uriList, arrayKey)
	if err != nil || uriList.IsNil() {
		return false
	}
	defer env.DeleteLocalRef(uriList)

	var size int32
	err = uriList.CallMethod(env, "size", &size)
	if err != nil {
		return false
	}

	log.Tracef("SEND_MULTIPLE URI count: %d", size)

	for i := int32(0); i < size; i++ {
		uri := jnigi.NewObjectRef("android/net/Uri")
		err = uriList.CallMethod(env, "get", uri, i)
		if err != nil || uri.IsNil() {
			continue
		}
		defer env.DeleteLocalRef(uri)

		uriStr := jnigi.NewObjectRef("java/lang/String")
		err = uri.CallMethod(env, "toString", uriStr)
		if err != nil || uriStr.IsNil() {
			continue
		}
		defer env.DeleteLocalRef(uriStr)

		var uriBytes []byte
		err = uriStr.CallMethod(env, "getBytes", &uriBytes)
		if err != nil {
			continue
		}

		uriString := string(uriBytes)
		log.Tracef("Sending SEND_MULTIPLE URI: %s", uriString)

		select {
		case uriFromIntent <- uriString:
			log.Trace("SEND_MULTIPLE URI sent to channel successfully")
			*hasValidData = true
		default:
			log.Trace("WARNING - URI channel full")
		}
	}

	return *hasValidData
}

func setActivityResult(env *jnigi.Env, activity *jnigi.ObjectRef, resultCode int32) {
	err := activity.CallMethod(env, "setResult", nil, resultCode)
	if err != nil {
		log.Trace("Error setting activity result: " + err.Error())
	}
}

func finishActivity(env *jnigi.Env, activity *jnigi.ObjectRef) {
	err := activity.CallMethod(env, "finish", nil)
	if err != nil {
		log.Trace("Error finishing activity: " + err.Error())
	}
}

// IntentError представляет ошибку обработки интента
type IntentError struct {
	Message string
}

func (e *IntentError) Error() string {
	return "IntentError: " + e.Message
}
