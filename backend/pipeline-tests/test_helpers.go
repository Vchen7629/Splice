//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func startNats(t *testing.T) (string, jetstream.JetStream) {
	t.Helper()
	ctx := context.Background()

	container, err := natstc.Run(ctx, "nats:2.10-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	url, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "jobs",
		Subjects: []string{"jobs.>"},
	})
	require.NoError(t, err)

	return url, js
}

func buildBinaries(t *testing.T, binDir string) (videoUpload, transcoderWorker, videoRecombiner string) {
	t.Helper()

	services := []struct{ src, name string }{
		{"../video-upload", "video-upload"},
		{"../transcoder-worker", "transcoder-worker"},
		{"../video-recombiner", "video-recombiner"},
	}

	bins := make([]string, len(services))
	for i, svc := range services {
		dest := filepath.Join(binDir, svc.name)
		cmd := exec.Command("go", "build", "-o", dest, "./cmd/main.go")
		cmd.Dir = svc.src
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "build failed for %s:\n%s", svc.src, out)
		bins[i] = dest
	}

	return bins[0], bins[1], bins[2]
}

func startGoService(t *testing.T, binary, cwd string, env map[string]string) {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

func startSceneDetector(t *testing.T, cwd, natsURL string) {
	t.Helper()
	sceneDetectorRoot, err := filepath.Abs("../scene-detector")
	require.NoError(t, err)

	cmd := exec.Command(
		filepath.Join(sceneDetectorRoot, ".venv", "bin", "python"),
		"-m", "src.service",
	)
	cmd.Dir = cwd
	cmd.Env = mergeEnv(map[string]string{
		"NATS_URL":   natsURL,
		"PYTHONPATH": sceneDetectorRoot,
	})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

// setupPipeline starts all services and returns the video-upload base URL.
// numTranscoderWorkers controls how many competing worker instances are started.
//
// Directory layout under t.TempDir():
//
//	bins/                       compiled Go binaries
//	splice/                     OUTPUT_DIR for all Go services
//	services/
//	  .env                      empty — satisfies godotenv.Load("../.env")
//	  temp/                     raw chunks from scene-detector ("../temp" from each CWD)
//	  video-upload/
//	  scene-detector/
//	  transcoder-worker-{n}/    one CWD per worker instance
//	  video-recombiner/
func setupPipeline(t *testing.T, numTranscoderWorkers int) (string, string, string) {
	t.Helper()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bins")
	spliceDir := filepath.Join(tmp, "splice")
	servicesDir := filepath.Join(tmp, "services")

	cwds := []string{
		binDir, spliceDir, servicesDir,
		filepath.Join(servicesDir, "temp"),
		filepath.Join(servicesDir, "video-upload"),
		filepath.Join(servicesDir, "scene-detector"),
		filepath.Join(servicesDir, "video-recombiner"),
	}
	for i := range numTranscoderWorkers {
		cwds = append(cwds, filepath.Join(servicesDir, fmt.Sprintf("transcoder-worker-%d", i)))
	}
	for _, d := range cwds {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(servicesDir, ".env"), nil, 0o644))

	natsURL, _ := startNats(t)
	uploadBin, transcoderBin, recombinerBin := buildBinaries(t, binDir)

	sharedEnv := map[string]string{
		"NATS_URL":   natsURL,
		"OUTPUT_DIR": spliceDir,
	}

	startGoService(t, recombinerBin, filepath.Join(servicesDir, "video-recombiner"), sharedEnv)
	startSceneDetector(t, filepath.Join(servicesDir, "scene-detector"), natsURL)

	for i := range numTranscoderWorkers {
		cwd := filepath.Join(servicesDir, fmt.Sprintf("transcoder-worker-%d", i))
		startGoService(t, transcoderBin, cwd, sharedEnv)
	}

	port := freePort(t)
	startGoService(t, uploadBin, filepath.Join(servicesDir, "video-upload"), map[string]string{
		"NATS_URL":   natsURL,
		"OUTPUT_DIR": spliceDir,
		"HTTP_PORT":  fmt.Sprintf("%d", port),
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTP(t, baseURL+"/jobs/probe/status", 10*time.Second)
	return baseURL, natsURL, spliceDir
}

// generateTestVideo creates a 6s MP4 with a hard red→blue cut so scene-detector produces multiple chunks.
func generateTestVideo(t *testing.T, destPath string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=red:duration=3:size=320x240:rate=24",
		"-f", "lavfi", "-i", "color=blue:duration=3:size=320x240:rate=24",
		"-filter_complex", "[0][1]concat=n=2:v=1:a=0",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		destPath,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg failed:\n%s", out)
}

// generateSingleSceneVideo creates a solid-colour video with no scene boundary (1 chunk).
func generateSingleSceneVideo(t *testing.T, destPath string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=green:duration=4:size=320x240:rate=24",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		destPath,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg failed:\n%s", out)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("service at %s did not become ready within %s", url, timeout)
}

func uploadVideo(t *testing.T, baseURL, videoPath, targetResolution string) string {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fw, err := w.CreateFormFile("video", filepath.Base(videoPath))
	require.NoError(t, err)
	data, err := os.ReadFile(videoPath)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.WriteField("target_resolution", targetResolution))
	require.NoError(t, w.Close())

	resp, err := http.Post(baseURL+"/jobs", w.FormDataContentType(), &body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.JobID)
	return result.JobID
}

func pollJobStatus(t *testing.T, baseURL, jobID string) string {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", baseURL, jobID))
	require.NoError(t, err)
	defer resp.Body.Close()

	var body struct {
		State string `json:"state"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.State
}

func waitForJobComplete(t *testing.T, baseURL, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pollJobStatus(t, baseURL, jobID) == "COMPLETE" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("job %s did not reach COMPLETE within %s", jobID, timeout)
}

func mergeEnv(overrides map[string]string) []string {
	keys := make(map[string]bool, len(overrides))
	for k := range overrides {
		keys[k] = true
	}
	filtered := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !keys[strings.SplitN(e, "=", 2)[0]] {
			filtered = append(filtered, e)
		}
	}
	for k, v := range overrides {
		filtered = append(filtered, k+"="+v)
	}
	return filtered
}
