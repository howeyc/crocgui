#!/usr/bin/env bash

curl -Lso go.tar.gz https://golang.org/dl/go1.16.linux-amd64.tar.gz
echo "013a489ebb3e24ef3d915abe5b94c3286c070dfe0818d5bca8108f1d6e8440d2 go.tar.gz" | sha256sum -c -
mkdir -p golang
tar -C golang -xzf go.tar.gz
mkdir -p gopath
export GOPATH="$PWD/gopath"
export GO_LANG="$PWD/golang/go/bin"
export GO_COMPILED="$PWD/bin"
export PATH="$GO_LANG:$GO_COMPILED:$PATH"
export PATH=$(pwd)/go/bin:$PATH
go version
curl -Lso fyne-uri-name.zip https://github.com/howeyc/fyne/archive/uri-name.zip
unzip fyne-uri-name
pushd fyne-uri-name
go build fyne.io/fyne/v2/cmd/fyne
popd
./fyne-uri-name/fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
zip -d crocgui.apk "META-INF/*"
 
