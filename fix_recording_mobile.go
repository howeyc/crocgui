//go:build android || ios

package main

// fixRecordingFile — no-op на мобильных платформах (ffmpeg недоступен).
func fixRecordingFile(root, fileName string) {}
