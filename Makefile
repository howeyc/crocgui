.PHONY: clean all

all: crocgui.apk crocgui

crocgui.apk: main.go send.go recv.go settings.go about.go platforms_android.go AndroidManifest.xml
	ANDROID_HOME=~/android fyne package -os android -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png

crocgui: main.go send.go recv.go settings.go about.go platforms-all.go
	go build

clean:
	go clean
	rm crocgui.apk
