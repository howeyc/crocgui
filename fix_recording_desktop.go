//go:build !android && !ios

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/schollz/logger"
)

// fixRecordingFile запускает ffmpeg для ремукса записанного файла:
// WebM — дописывает Cues + Duration, MP4 — перемещает moov в начало.
// Вызывается в горутине, ошибки только логируются.
func fixRecordingFile(root, fileName string) {
	srcPath := filepath.Join(root, fileName)
	ext := strings.ToLower(filepath.Ext(fileName))

	// Проверяем наличие ffmpeg
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		log.Debugf("fixRecordingFile: ffmpeg not found, skip fix for %s", fileName)
		return
	}

	fixedPath := srcPath + ".fixed" + ext
	defer os.Remove(fixedPath) // очистка временного файла

	args := []string{ffmpegPath, "-y", "-i", srcPath, "-c", "copy"}
	if ext == ".mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, fixedPath)

	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Debugf("fixRecordingFile: ffmpeg failed for %s: %v (%s)", fileName, err, string(out))
		return
	}

	if err := os.Rename(fixedPath, srcPath); err != nil {
		log.Debugf("fixRecordingFile: rename failed for %s: %v", fileName, err)
		return
	}

	log.Debugf("fixRecordingFile: fixed %s", fileName)
}
