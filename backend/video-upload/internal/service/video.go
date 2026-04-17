package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"os/exec"
	"strconv"
	"strings"
)

var execCommand = exec.Command

func CheckVideoResolution(video multipart.File) (string, error) {
	cmd := execCommand(
		"ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "stream=height",
		"-of", "csv=p=0",
		"-i", "pipe:0",
	)

	cmd.Stdin = video

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe error: %w", err)
	}

	height := strings.TrimSpace(string(out))

	// need to seek so file is available again due to cursor adv
	_, err = video.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to rewind video file: %w", err)
	}

	return height + "p", nil
}

// convert resolution strings like "720p" to 720 for proper comparison
func ConvertResStringToInt(resString string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.ToLower(resString), "p"))

	return n
}
