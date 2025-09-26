.PHONY: all clean arm arm64 386 amd64 linux window wsl darwin ios install darm emulator adb

all: android

android: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml fdroid-build.sh
	ANDROID_HOME=~/android bash fdroid-build.sh test

clean:
	go clean
	rm crocgui.apk

arm: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml
	fyne package -os android/arm --release

arm64: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml
	fyne package -os android/arm64 --release

386: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml
	fyne package -os android/386 --release

amd64: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml
	fyne package -os android/amd64 --release

emulator:
	emulator -avd Medium_Phone_API_36.1

adb:
	adb install crocgui.apk

linux: main.go send.go recv.go settings.go theme.go about.go
	fyne package -os linux --release

windows: main.go send.go recv.go settings.go theme.go about.go
	#sudo apt-get install gcc-mingw-w64-x86-64
	CC=x86_64-w64-mingw32-gcc fyne package -os windows --release -tags=opengl

wsl: main.go send.go recv.go settings.go theme.go about.go
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOFLAGS=-ldflags=-s go build -tags=opengl

darwin: main.go send.go recv.go settings.go theme.go about.go
	fyne package -os darwin --release
	cp -r crocgui.app /Applications/
	cp crocgui.app/Contents/Info.plist darm/crocgui.app/Contents/
	cp crocgui.app/Contents/Resources/* darm/crocgui.app/Contents/Resources/
	mkdir -p darm/crocgui.app/Contents/MacOS

ios: main.go send.go recv.go settings.go theme.go about.go AndroidManifest.xml
	fyne package -os ios --release

install:
	GOFLAGS=-ldflags=-s go install

darm: 
	#brew install glfw
	GOFLAGS=-ldflags=-s CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o darm/crocgui.app/Contents/MacOS/crocgui .&&cp -r darm/crocgui.app /Applications/

damd: 
	#brew install glfw
	GOFLAGS=-ldflags=-s CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o crocgui.app/Contents/MacOS/crocgui .&&cp -r crocgui.app /Applications/
