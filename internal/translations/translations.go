package translations

//go install golang.org/x/text/cmd/gotext@latest

//go:generate gotext -srclang=en-US update -out=catalog.go -lang=en-US,tr-TR,ja-JP,zh-CN,zh-HK,zh-TW,ru-RU crocgui
