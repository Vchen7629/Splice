package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

const probeTimeout = time.Minute

type videoInfo struct {
	format, VideoCodec string
	width, height      int
	packets            int64
	hasAudio           bool // gateway only adds audio.mka output when true
}

// video format and codecs that are not able to be split on keyframes into chunks
func (v videoInfo) splitTable() bool {
	return v.format != "avi" && v.VideoCodec != "hevc"
}

// runs ffprobe on video file on disk and returns the video info like format, width, height, packets
func probeVideo(ctx context.Context, videoPath string) (videoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	ffprobeOut, err := exec.CommandContext(ctx,
		"ffprobe", "-v", "error", "-protocol_whitelist", "file",
		"-count_packets", "-of", "json",
		"-show_entries", "format=format_name:"+
			"stream=codec_type,codec_name,width,height,nb_read_packets:"+
			"stream_disposition=attached_pic:stream_side_data=rotation",
		videoPath).Output()

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
			CodecType   string `json:"codec_type"`
			CodecName   string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Packets     string `json:"nb_read_packets"` // used for determining size of video
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
			SideData []struct {
				Rotation int `json:"rotation"` //  handling phone video edge case
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if json.Unmarshal(ffprobeOut, &probe) != nil {
		return videoInfo{}, &rejectError{"unreadable video"}
	}

	info := videoInfo{format: probe.Format.Name}
	foundVideo := false

	for _, stream := range probe.Streams {
		switch {
		// first real vikdeo stream, skipping cover art which ffprobe also reports as video
		case stream.CodecType == "video" && stream.Disposition.AttachedPic == 0 && !foundVideo:
			foundVideo = true
			info.VideoCodec = stream.CodecName
			info.width, info.height = stream.Width, stream.Height
			info.packets, _ = strconv.ParseInt(stream.Packets, 10, 64)

			for _, sideData := range stream.SideData {
				// fprobe only reports portrait video as +- 90
				if r := sideData.Rotation % 180; r == 90 || r == -90 {
					info.width, info.height = info.height, info.width
					break
				}
			}
		case stream.CodecType == "audio":
			info.hasAudio = true
		}
	}

	if !foundVideo {
		return videoInfo{}, &rejectError{"no video stream"}
	}

	return info, nil
}
