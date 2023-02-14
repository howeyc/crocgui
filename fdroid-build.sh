#!/usr/bin/env bash

curl -Lso go.tar.gz https://go.dev/dl/go1.20.1.linux-amd64.tar.gz
echo "000a5b1fca4f75895f78befeb2eecf10bfff3c428597f3f1e69133b63b911b02 go.tar.gz" | sha256sum -c -
mkdir -p gobuild/go{lang,path,cache}
tar -C gobuild/golang -xzf go.tar.gz
rm go.tar.gz
export GOPATH="$PWD/gobuild/gopath"
export GOCACHE="$PWD/gobuild/gocache"
export GO_LANG="$PWD/gobuild/golang/go/bin"
export GO_COMPILED="$GOPATH/bin"
export PATH="$GO_LANG:$GO_COMPILED:$PATH"
go version
go install fyne.io/fyne/v2/cmd/fyne\@v2.3.0
fyne version
if [[ $# -eq 0 ]]; then
	fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
	zip -d crocgui.apk "META-INF/*"
else
	fyne package -os android -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
fi
chmod -R u+w gobuild
rm -rf gobuild
