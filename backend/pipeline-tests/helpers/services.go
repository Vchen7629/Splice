//go:build integration

package helpers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)


func BuildBinaries(t *testing.T, binDir string) (videoUpload, transcoderWorker, videoRecombiner string) {
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

func StartGoService(t *testing.T, binary, cwd string, env map[string]string) {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Dir = cwd
	cmd.Env = MergeEnv(env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

func StartSceneDetector(t *testing.T, cwd, natsURL, filerURL string) {
	t.Helper()
	sceneDetectorRoot, err := filepath.Abs("../scene-detector")
	require.NoError(t, err)

	cmd := exec.Command(
		filepath.Join(sceneDetectorRoot, ".venv", "bin", "python"),
		"-m", "src.service",
	)
	cmd.Dir = cwd
	cmd.Env = MergeEnv(map[string]string{
		"NATS_URL":         natsURL,
		"BASE_STORAGE_URL": filerURL,
		"PYTHONPATH":       sceneDetectorRoot,
	})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
}

// SetupPipeline starts all services and returns the video-upload base URL, nats URL, and filer URL.
// numTranscoderWorkers controls how many competing worker instances are started.
//
// Directory layout under t.TempDir():
//
//	bins/                       compiled Go binaries
//	services/
//	  .env                      empty — satisfies godotenv.Load("../.env")
//	  video-upload/
//	  scene-detector/
//	  transcoder-worker-{n}/    one CWD per worker instance
//	  video-recombiner/
func SetupPipeline(t *testing.T, numTranscoderWorkers int, filerURL string) (string, string) {
	t.Helper()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bins")
	servicesDir := filepath.Join(tmp, "services")

	cwds := []string{
		binDir, servicesDir,
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

	natsURL, _ := StartNats(t)
	uploadBin, transcoderBin, recombinerBin := BuildBinaries(t, binDir)

	sharedEnv := map[string]string{
		"NATS_URL":         natsURL,
		"BASE_STORAGE_URL": filerURL,
	}

	StartGoService(t, recombinerBin, filepath.Join(servicesDir, "video-recombiner"), sharedEnv)
	StartSceneDetector(t, filepath.Join(servicesDir, "scene-detector"), natsURL, filerURL)

	for i := range numTranscoderWorkers {
		cwd := filepath.Join(servicesDir, fmt.Sprintf("transcoder-worker-%d", i))
		StartGoService(t, transcoderBin, cwd, sharedEnv)
	}

	port := FreePort(t)
	StartGoService(t, uploadBin, filepath.Join(servicesDir, "video-upload"), map[string]string{
		"NATS_URL":    natsURL,
		"STORAGE_URL": filerURL,
		"HTTP_PORT":   fmt.Sprintf("%d", port),
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	WaitForHTTP(t, baseURL+"/jobs/probe/status", 10*time.Second)
	return baseURL, natsURL
}

func PollJobStatus(t *testing.T, baseURL, jobID string) string {
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

func WaitForJobComplete(t *testing.T, baseURL, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if PollJobStatus(t, baseURL, jobID) == "COMPLETE" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("job %s did not reach COMPLETE within %s", jobID, timeout)
}