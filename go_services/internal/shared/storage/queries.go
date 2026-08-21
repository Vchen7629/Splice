package storage

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// save the video chunk to seaweedfs storage
func UploadVideoChunk(url, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening video file: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("error closing the file, %v", err)
		}
	}()

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return "", fmt.Errorf("error creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error connecting to seaweedfs: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body, %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seaweedfs upload failed with status: %d", resp.StatusCode)
	}

	return url, nil
}

// validatePathSegment rejects path segments that could escape the intended
// base directory (empty, ".", "..", or containing a path separator), while
// allowing ordinary file/job names such as "my.video.mp4" or "clip (1).mov".
func validatePathSegment(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid path segment: %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("path segment must not contain path seperators: %q", name)
	}
	return nil
}

var removeAll = os.RemoveAll

// fetch the video chunk seaweedfs storage
func GetVideoChunk(storageURL, fileName string) (string, error) {
	err := validatePathSegment(fileName)
	if err != nil {
		return "", err
	}

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
			return "", fmt.Errorf("video not found")
		case http.StatusForbidden:
			return "", fmt.Errorf("access denied")
		case http.StatusInternalServerError:
			return "", fmt.Errorf("error accessing seedweedfs")
		}
		return "", fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	filename := storageURL[strings.LastIndex(storageURL, "/")+1:]
	// validate filename so external malicious filenames doesnt get through
	err = validatePathSegment(fileName)
	if err != nil {
		return "", err
	}
	jobDir := filepath.Join("/tmp/" + fileName)

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
			log.Printf("error closing the file, %v", err)
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
