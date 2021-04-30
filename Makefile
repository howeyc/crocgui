.PHONY: clean all

all: crocgui.apk crocgui

crocgui.apk: main.go send.go recv.go settings.go about.go AndroidManifest.xml fdroid-build.sh
	ANDROID_HOME=~/android bash fdroid-build.sh test

crocgui: main.go send.go recv.go settings.go about.go
	go build

clean:
	go clean
	rm crocgui.apk
