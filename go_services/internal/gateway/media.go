package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

type videoInfo struct {
	format  string
	width   int
	height  int
	packets int64
}

// runs ffprobe on video file on disk and returns the video info like format, width, height, packets
func probeVideo(ctx context.Context, videoPath string) (videoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	ffprobeOut, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file",
		"-select_streams", "v:0", "-count_packets", "-of", "json",
		"-show_entries", "format=format_name:stream=width,height,nb_read_packets", videoPath).Output()

	if err != nil {
		var exitErr *exec.ExitError
		if ctx.Err() != nil || !errors.As(err, &exitErr) {
			return videoInfo{}, fmt.Errorf("ffprobe failed to run: %w", err) // timeout, cancel, or ffprobe missing
		}
		return videoInfo{}, &rejectError{"unreadable video"}
	}

	var probe struct {
		Format struct {
			Name string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Width, Height int
			Packets       string `json:"nb_read_packets"`
		} `json:"streams"`
	}
	if json.Unmarshal(ffprobeOut, &probe) != nil || len(probe.Streams) == 0 {
		return videoInfo{}, &rejectError{"no video stream"}
	}

	stream := probe.Streams[0]
	packets, _ := strconv.ParseInt(stream.Packets, 10, 64)

	return videoInfo{
		format:  probe.Format.Name,
		width:   stream.Width,
		height:  stream.Height, // stored height, ignores rotation
		packets: packets,
	}, nil
}
