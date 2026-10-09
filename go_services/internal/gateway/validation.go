package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"splice.com/go_services/internal/shared/storage"
)

// validates that jobID is non-empty and is a uuid
func validateJobID(jobID string) error {
	parsed, err := uuid.Parse(jobID)
	if err != nil || parsed.String() != jobID {
		return errors.New("job_id is not a valid uuid")
	}

	return nil
}

const (
	maxFramePixels = 8192 * 4608 // allows videos up to 8K
	maxTotalPixels = 1e12        // w*h*frames, guard against decompression-bomb
	probeTimeout   = time.Minute
)

var allowedContainers = []string{"mov", "mp4", "matroska", "webm", "avi"} // allowed video formats

type rejectError struct{ reason string }

func (e *rejectError) Error() string { return e.reason }

// validates that the video uploaded by user is a proper video type,
// not extremely large, non-empty, etc just ffprobe on file on disk
func validateVideo(ctx context.Context, path string, size int64) error {
	if size == 0 {
		return &rejectError{"empty file"}
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	ffprobeOut, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file",
		"-select_streams", "v:0", "-count_packets", "-of", "json",
		"-show_entries", "format=format_name:stream=width,height,nb_read_packets", path).Output()

	if err != nil {
		return &rejectError{"unreadable video"} // bad input or timeout
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
		return &rejectError{"no video stream"}
	}

	// checking that the video format is on allowed list
	// format_name is comma-joined (e.g. "mov,mp4,m4a,3gp,3g2,mj2"), so accept if any part is allowed
	if !slices.ContainsFunc(strings.Split(probe.Format.Name, ","), func(f string) bool {
		return slices.Contains(allowedContainers, f)
	}) {
		return &rejectError{"unsupported video format"}
	}

	stream := probe.Streams[0]
	frames, _ := strconv.ParseInt(stream.Packets, 10, 64)
	pixels := int64(stream.Width) * int64(stream.Height)

	switch {
	case pixels <= 0 || frames <= 0:
		return &rejectError{"video is empty"}
	case pixels > maxFramePixels:
		return &rejectError{"video resolution too large"}
	case float64(pixels)*float64(frames) > maxTotalPixels:
		return &rejectError{"video too large to process"}
	}

	return nil
}

var processSubjects = map[string]string{
	"Transcode": "jobs.video.scene-split",
	"Upscale":   "jobs.video.upscale",
	"Denoise":   "jobs.video.denoise",
	"Convert":   "jobs.video.convert",
}

// validates that the request FormValues are non empty, filename is valid, and processType is valid
func validateUploadFields(filename, targetRes, sourceRes, processType string) error {
	if targetRes == "" {
		return errors.New("missing target_resolution field")
	}
	if sourceRes == "" {
		return errors.New("missing source_resolution field")
	}
	if processType == "" {
		return errors.New("missing process_type field")
	}
	if _, ok := processSubjects[processType]; !ok {
		return errors.New("invalid process_type field")
	}

	if err := storage.ValidatePathSegment(filename); err != nil {
		return errors.New("invalid video filename")
	}

	return nil
}
