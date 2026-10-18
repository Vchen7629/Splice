package gateway

import (
	"errors"
	"slices"
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
	maxTotalPixels = 1e12        // w*h*frames, approximate workload limit against decompression-bomb
	probeTimeout   = time.Minute
)

var allowedContainers = []string{"mov", "mp4", "matroska", "webm", "avi"} // allowed video formats
var allowedTargetRes = map[string][]string{
	"Transcode": {"480p", "720p", "1080p"},
	"Upscale":   {"720p", "1080p", "1440p", "4K"},
}

type rejectError struct{ reason string }

func (e *rejectError) Error() string { return e.reason }

// validates that the video uploaded by user is a proper video type,
// not extremely large, non-empty, etc
func validateVideo(info videoInfo, size int64) error {
	if size == 0 {
		return &rejectError{"empty file"}
	}

	// checking that the video format is on allowed list
	// format_name is comma-joined (e.g. "mov,mp4,m4a,3gp,3g2,mj2"), so accept if any part is allowed
	if !slices.ContainsFunc(strings.Split(info.format, ","), func(f string) bool {
		return slices.Contains(allowedContainers, f)
	}) {
		return &rejectError{"unsupported video format"}
	}

	pixels := int64(info.width) * int64(info.height)

	switch {
	case pixels <= 0 || info.packets <= 0:
		return &rejectError{"video is empty"}
	case pixels > maxFramePixels:
		return &rejectError{"video resolution too large"}
	case float64(pixels)*float64(info.packets) > maxTotalPixels:
		return &rejectError{"video too large to process"}
	}

	return nil
}

var processSubjects = map[string]string{
	"Transcode": "jobs.video.scene-split",
	"Upscale":   "jobs.video.upscale",
	"Denoise":   "jobs.video.scene-split",
	"Convert":   "jobs.video.convert",
}

// validates that the request FormValues are non empty, filename is valid, and processType is valid
func validateUploadFields(filename, targetRes, processType string) error {
	allowedRes, takesResolution := allowedTargetRes[processType]
	_, processTypeKnown := processSubjects[processType]

	// NOTE: Right now it ignores target and source res for services that are not transcode or upscale so
	// attackers can just pass in any resolution but its harmless since the services itself will not use them
	checks := []struct {
		isValid bool
		msg     string
	}{
		{processType != "", "missing process_type field"},
		{processTypeKnown, "invalid process_type field"},
		{!takesResolution || targetRes != "", "missing target_resolution field"},
		{!takesResolution || slices.Contains(allowedRes, targetRes), "uploaded file target res not allowed"},
		{storage.ValidatePathSegment(filename) == nil, "invalid video filename"},
	}
	for _, c := range checks {
		if !c.isValid {
			return errors.New(c.msg)
		}
	}

	return nil
}
