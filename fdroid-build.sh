#!/usr/bin/env bash

curl -Lso go.tar.gz https://go.dev/dl/go1.17.3.linux-amd64.tar.gz
echo "550f9845451c0c94be679faf116291e7807a8d78b43149f9506c1b15eb89008c go.tar.gz" | sha256sum -c -
mkdir -p gobuild/golang
tar -C gobuild/golang -xzf go.tar.gz
mkdir -p gobuild/gopath
mkdir -p gobuild/gocache
export GOPATH="$PWD/gobuild/gopath"
export GOCACHE="$PWD/gobuild/gocache"
export GO_LANG="$PWD/gobuild/golang/go/bin"
export GO_COMPILED="$GOPATH/bin"
export PATH="$GO_LANG:$GO_COMPILED:$PATH"
go version
go install fyne.io/fyne/v2/cmd/fyne\@v2.1.1
fyne version
if [[ $# -eq 0 ]]; then
	fyne package -os android -release -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
	zip -d crocgui.apk "META-INF/*"
else
	fyne package -os android -appID com.github.howeyc.crocgui -icon metadata/en-US/images/icon.png
fi
