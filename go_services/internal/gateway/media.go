package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	probeTimeout     = time.Minute
	splitTimeout     = 5 * time.Minute
	chunkSegmentTime = "30" // seconds, ffmpeg cuts at first keyframe at or after each multiple of this
)

type videoInfo struct {
	format, videoCodec string
	width, height      int
	packets            int64
	hasAudio           bool // gateway only adds audio.mka output when true
	avOffset           float64
}

// runs ffprobe on video file on disk and returns the video info like format, width, height, packets
func probeVideo(ctx context.Context, videoPath string) (videoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	ffprobeOut, err := exec.CommandContext(ctx,
		"ffprobe", "-v", "error", "-protocol_whitelist", "file",
		"-count_packets", "-of", "json",
		"-show_entries", "format=format_name:"+
			"stream=codec_type,codec_name,width,height,start_time,nb_read_packets:"+
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
			StartTime   string `json:"start_time"`
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
	var videoStart float64
	var audioStart float64

	for _, stream := range probe.Streams {
		switch {
		// first real vikdeo stream, skipping cover art which ffprobe also reports as video
		case stream.CodecType == "video" && stream.Disposition.AttachedPic == 0 && !foundVideo:
			foundVideo = true
			videoStart = parseSeconds(stream.StartTime)
			info.videoCodec = stream.CodecName
			info.width, info.height = stream.Width, stream.Height
			info.packets, _ = strconv.ParseInt(stream.Packets, 10, 64)

			for _, sideData := range stream.SideData {
				// fprobe only reports portrait video as +- 90
				if r := sideData.Rotation % 180; r == 90 || r == -90 {
					info.width, info.height = info.height, info.width
					break
				}
			}
		case stream.CodecType == "audio" && !info.hasAudio:
			info.hasAudio = true
			audioStart = parseSeconds(stream.StartTime)
		}
	}

	if !foundVideo {
		return videoInfo{}, &rejectError{"no video stream"}
	}

	if info.hasAudio {
		info.avOffset = audioStart - videoStart // > 0: audio starts late, < 0: video starts late
	}

	return info, nil
}

// parses ffprobe seconds string for video stream start_time, "N/A" or empty becomes 0
func parseSeconds(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// local files video was split into. chunkPaths are in playback order
// audioPath is "" when source has no audio
type splitResult struct {
	chunkPaths []string
	audioPath  string
}

// splits video into keyframe aligned video-only chunks and a seperate audio file in one stream-copy pass
// and writes them to outDir. Videos that can't be keyframe split are remuxed into single mkv chunk
func splitVideo(ctx context.Context, info videoInfo, videoPath, outDir string) (splitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, splitTimeout)
	defer cancel()

	var res splitResult
	args := []string{"-v", "error", "-fflags", "+genpts", "-protocol_whitelist", "file", "-i", videoPath}

	if info.shouldSplit() {
		args = append(args,
			"-map", "0:V:0", "-c", "copy",
			"-f", "segment", "-segment_time", chunkSegmentTime, "-reset_timestamps", "1", "-segment_format", "matroska",
			filepath.Join(outDir, "chunk_%04d.mkv"))
	} else {
		// can't keyframe split, remux the whole video stream (stream copy) into one mkv chunk so every chunk is matroska
		args = append(args, "-map", "0:V:0", "-c", "copy", filepath.Join(outDir, "chunk_0000.mkv"))
	}

	if info.hasAudio {
		res.audioPath = filepath.Join(outDir, "audio.mka")
		args = append(args, "-map", "0:a:0", "-vn", "-c", "copy", res.audioPath)
	}

	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return splitResult{}, fmt.Errorf("ffmpeg split timed out after %s: %w", splitTimeout, ctxErr)
		}
		return splitResult{}, fmt.Errorf("ffmpeg split error: %w\n%s", err, out)
	}

	res.chunkPaths, err = filepath.Glob(filepath.Join(outDir, "chunk_*.mkv"))
	if err != nil {
		return splitResult{}, fmt.Errorf("error listing split chunks: %w", err)
	}
	if len(res.chunkPaths) == 0 {
		return splitResult{}, errors.New("ffmpeg split produced no chunks")
	}

	return res, nil
}

// video format and codecs that are able to be split on keyframes into chunks
func (v videoInfo) shouldSplit() bool {
	return v.format != "avi" && v.videoCodec != "hevc"
}
