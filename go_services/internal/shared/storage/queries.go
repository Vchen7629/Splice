package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const TempStorageTTL = "1d"      // for things like raw chunk uploads, unprocessed videos
const ProcessedStorageTTL = "3d" // for final processed output storage

var ErrNotFound = errors.New("not found")

// uploads (PUT) body to the storage url
func Upload(ctx context.Context, url, storageTTL string, body io.Reader, httpUploadTimeout time.Duration) error {
	client := &http.Client{Timeout: httpUploadTimeout} // 0 = no httpUploadTimeout
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("error creating upload request: %w", err)
	}
	if _, ok := body.(io.ReadCloser); ok {
		req.Body = io.NopCloser(body)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if storageTTL != "" {
		q := req.URL.Query()
		q.Set("ttl", storageTTL)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error connecting to seaweedfs: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body, %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("seaweedfs upload failed with status: %d", resp.StatusCode)
	}

	return nil
}

// save (upload) the file on local disk to seaweedfs storage
func UploadFile(ctx context.Context, destURL, filePath, storageTTL string, httpUploadTimeout time.Duration) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening video file: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("error closing the file, %v", err)
		}
	}()

	if err := Upload(ctx, destURL, storageTTL, file, httpUploadTimeout); err != nil {
		return err
	}

	return nil
}

// ValidatePathSegment rejects path segments that could escape the intended
// base directory (empty, ".", "..", or containing a path separator), while
// allowing ordinary file/job names such as "my.video.mp4" or "clip (1).mov".
func ValidatePathSegment(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid path segment: %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("path segment must not contain path seperators: %q", name)
	}
	return nil
}

var removeAll = os.RemoveAll

// sends HEAD request to check that object at url exists in storage (seaweedfs)
func CheckExists(url string, timeout time.Duration) error {
	httpClient := &http.Client{Timeout: timeout}
	resp, err := httpClient.Head(url)
	if err != nil {
		return fmt.Errorf("error connecting to seaweedfs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("object not found: %w", ErrNotFound)
	default:
		return fmt.Errorf("seaweedfs returned status %d", resp.StatusCode)
	}
}

// returns local directory DownloadVideo downloads into
func TempUnprocessedDir(videoID string) string {
	return filepath.Join("/tmp/temp-unprocessed-" + videoID)
}

// down the video from seaweedfs storage and save it to a temp directory with dirName
func DownloadVideo(storageURL, dirName string, httpUploadTimeout time.Duration) (string, error) {
	err := ValidatePathSegment(dirName)
	if err != nil {
		return "", err
	}

	httpClient := &http.Client{Timeout: httpUploadTimeout}
	resp, err := httpClient.Get(storageURL)
	if err != nil {
		return "", fmt.Errorf("error connecting to seedweedfs, %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body, %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return "", fmt.Errorf("video not found: %w", ErrNotFound)
		case http.StatusForbidden:
			return "", fmt.Errorf("access denied")
		case http.StatusInternalServerError:
			return "", fmt.Errorf("error accessing seedweedfs")
		}
		return "", fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	filename := storageURL[strings.LastIndex(storageURL, "/")+1:]
	// validate filename so external malicious filenames doesnt get through
	err = ValidatePathSegment(filename)
	if err != nil {
		return "", err
	}
	jobDir := TempUnprocessedDir(dirName)

	err = os.MkdirAll(jobDir, 0755)
	if err != nil {
		return "", fmt.Errorf("error created job temp dir: %w", err)
	}

	filePath := filepath.Join(jobDir, filename)
	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating video file: %w", err)
	}
	defer func() {
		err := outFile.Close()
		if err != nil {
			log.Printf("error closing the file 2, %v", err)
		}
	}()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		rmErr := removeAll(jobDir)
		if rmErr != nil {
			return "", fmt.Errorf("error writing video to file: %w (error removing all files: %w)", err, rmErr)
		}
		return "", fmt.Errorf("error writing video to file: %w", err)
	}

	return filePath, nil
}
