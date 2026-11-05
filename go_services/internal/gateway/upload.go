package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"
)

const (
	uploadTimeout     = 10 * time.Minute // this needs to stay under the http server WriteTimeout
	uploadConcurrency = 6                // max uploads of chunked videos to storage at once
)

type uploadResponse struct {
	JobID string `json:"job_id"`
}

type uploadParams struct {
	filename, targetRes, targetFormat, processType, denoiseQuality string
}

var probeVideoFn = probeVideo // used for tests

// handler for video upload POST requests, Accepts a multipart video upload, saves it to disk,
// and publishes a scene split message to NATS for downstream processing
func (v *videoHandler) uploadVideoRoute(w http.ResponseWriter, r *http.Request) {
	release, ok := v.acquireUploadSlot(r.Context())
	if !ok {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "server busy, try again shortly", http.StatusServiceUnavailable)
		return
	}
	defer release()

	uploadParams, src, err := v.parseUploadRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		v.logger.Warn(err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	tempFile, err := os.CreateTemp("", "upload-*"+filepath.Ext(uploadParams.filename))
	if err != nil {
		http.Error(w, "failed to process upload", http.StatusInternalServerError)
		v.logger.Error("failed to create temp file for validation", "err", err)
		return
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	size, err := io.Copy(tempFile, src)
	if err != nil {
		http.Error(w, "failed to process upload", http.StatusInternalServerError)
		v.logger.Error("failed to copy upload to temp file", "err", err)
		return
	}

	handleVideoErr := func(err error, failMsg string) {
		if rejected, ok := errors.AsType[*rejectError](err); ok {
			http.Error(w, rejected.reason, http.StatusUnprocessableEntity)
			v.logger.Warn("rejected uploaded video", "reason", rejected.reason, "file", uploadParams.filename)
			return
		}
		http.Error(w, failMsg, http.StatusInternalServerError)
		v.logger.Error(failMsg, "err", err)
	}

	info, err := probeVideoFn(r.Context(), tempFile.Name())
	if err != nil {
		handleVideoErr(err, "failed to extract video info")
		return
	}

	if err := validateVideo(info, size); err != nil {
		handleVideoErr(err, "failed to validate video")
		return
	}

	jobID := uuid.New().String()

	kh := KVHandler{logger: v.logger, kv: v.kv}
	if err := kh.updateJobStatusKV(r.Context(), jobID, pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "upload"}); err != nil {
		http.Error(w, "failed to record job status", http.StatusInternalServerError)
		v.logger.Error("failed to update job status kv to processing", "err", err)
		return
	}

	var submitErr error

	splitProcessing := []string{"transcode", "denoise"}
	if slices.Contains(splitProcessing, uploadParams.processType) {
		submitErr = v.submitChunked(r.Context(), jobID, tempFile.Name(), uploadParams, info)
	} else {
		submitErr = v.submitWhole(r.Context(), jobID, tempFile.Name(), uploadParams, info)
	}
	if submitErr != nil {
		v.logger.Error("failed to submit job", "job_id", jobID, "err", submitErr)
		if kvErr := kh.updateJobStatusKV(r.Context(), jobID, pipeline.JobStatus{State: pipeline.StateFailed, Stage: "upload"}); kvErr != nil {
			v.logger.Error("failed to mark job failed", "job_id", jobID, "err", kvErr)
		}
		http.Error(w, "failed to process upload", http.StatusInternalServerError)
		return
	}

	v.logger.Info("video upload job submitted", "job_id", jobID, "file", uploadParams.filename)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(uploadResponse{JobID: jobID})
	if err != nil {
		v.logger.Error("error encoding success http response", "err", err)
		return
	}
}

var uploadAcquireTimeout = 10 * time.Second

// for the upload route concurrency control semaphore
func (v *videoHandler) acquireUploadSlot(ctx context.Context) (release func(), ok bool) {
	if v.uploadSem == nil { // unlimited for tests
		return func() {}, true
	}
	select {
	case v.uploadSem <- struct{}{}: // acquire the upload slot
		return func() { <-v.uploadSem }, true
	case <-time.After(uploadAcquireTimeout): // upload limit reached
		return nil, false
	case <-ctx.Done(): // client gave up while waiting
		return nil, false
	}
}

// sets the limit for the upload size, validates the upload request and returns the upload values
func (v *videoHandler) parseUploadRequest(w http.ResponseWriter, r *http.Request) (uploadParams, multipart.File, error) {
	limit := v.maxUploadBytes
	if limit == 0 {
		limit = 10 << 30 // 10 GB
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		return uploadParams{}, nil, fmt.Errorf("invalid multipart form: %w", err)
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		return uploadParams{}, nil, errors.New("missing video file")
	}

	targetRes, targetFormat, denoiseQuality, processType := r.FormValue("target_resolution"), r.FormValue("target_format"), r.FormValue("denoise_quality"), r.FormValue("process_type")
	if err := validateUploadFields(header.Filename, targetRes, targetFormat, processType, denoiseQuality); err != nil {
		_ = file.Close()
		return uploadParams{}, nil, err
	}

	return uploadParams{header.Filename, targetRes, targetFormat, processType, denoiseQuality}, file, nil
}

// splits the video into video and audio chunks, uploads the chunks to storage, and publishes the chunk messages to nats, used for transcode/denoise
func (v *videoHandler) submitChunked(ctx context.Context, jobID, videoPath string, params uploadParams, info videoInfo) error {
	tempDir, err := os.MkdirTemp("", "split-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory to put split videos in: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	splitRes, err := splitVideo(ctx, info, videoPath, tempDir)
	if err != nil {
		return fmt.Errorf("failed to split video: %w", err)
	}

	videoChunkURLs, audioURL, err := v.uploadSplit(ctx, splitRes, jobID, params.filename)
	if err != nil {
		return fmt.Errorf("failed to upload split video chunks to storage: %w", err)
	}

	for i, chunkURL := range videoChunkURLs {
		if err := msgbus.Publish(v.js, pipeline.VideoChunkMessage{
			ChunkRef:    pipeline.ChunkRef{JobID: jobID, ChunkIndex: i},
			TotalChunks: len(videoChunkURLs), StorageURL: chunkURL, AudioURL: audioURL,
			TargetResolution: params.targetRes, DenoiseQuality: params.denoiseQuality, Operation: params.processType,
		}, processSubjects[params.processType],
		); err != nil {
			return fmt.Errorf("error publishing chunk %d: %w", i, err)
		}
	}

	return nil
}

// uploads split chunks and audio to storage in parallel (bounded by uploadConcurrency), returns chunk urls indexed by chunk index
// and audio url ("" if no audio). Used for transcode/denoise processing type
func (v *videoHandler) uploadSplit(ctx context.Context, splitRes splitResult, jobID, filename string) ([]string, string, error) {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	base := fmt.Sprintf("%s/%s/%s", v.storageURL, jobID, url.PathEscape(stem))

	uploadGroup, ctx := errgroup.WithContext(ctx)
	uploadGroup.SetLimit(uploadConcurrency)

	chunkURLs := make([]string, len(splitRes.chunkPaths)) // no mutex due to each 1:1 index goroutine
	for i, path := range splitRes.chunkPaths {
		chunkURLs[i] = fmt.Sprintf("%s_chunk_%04d%s", base, i, filepath.Ext(path))
		uploadGroup.Go(func() error {
			return storage.UploadFile(ctx, chunkURLs[i], path, storage.TempStorageTTL, uploadTimeout)
		})
	}

	var audioURL string
	if splitRes.audioPath != "" {
		audioURL = base + "_audio.mka"
		uploadGroup.Go(func() error {
			return storage.UploadFile(ctx, audioURL, splitRes.audioPath, storage.TempStorageTTL, uploadTimeout)
		})
	}

	if err := uploadGroup.Wait(); err != nil {
		return nil, "", fmt.Errorf("error uploading split video: %w", err)
	}
	return chunkURLs, audioURL, nil
}

// uploads the whole video file to storage and submits the nats message for video job. used for upscale/convert services
func (v *videoHandler) submitWhole(ctx context.Context, jobID, videoPath string, params uploadParams, info videoInfo) error {
	url := fmt.Sprintf("%s/%s/%s", v.storageURL, jobID, url.PathEscape(params.filename))
	if err := storage.UploadFile(ctx, url, videoPath, storage.TempStorageTTL, uploadTimeout); err != nil {
		return fmt.Errorf("failed to save uploaded video: %w", err)
	}

	if err := msgbus.Publish(
		v.js, pipeline.VideoJobMessage{
			JobID:            jobID,
			TargetResolution: params.targetRes, TargetFormat: params.targetFormat, SourceResolution: fmt.Sprintf("%dp", info.height),
			StorageURL: url, Operation: params.processType,
		}, processSubjects[params.processType],
	); err != nil {
		return fmt.Errorf("error publishing whole video job: %w", err)
	}

	return nil
}
