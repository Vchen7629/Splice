//go:build unit || integration

package converter

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeTestClip(t *testing.T, withAudio bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc=d=1:s=320x240"}
	if withAudio {
		args = append(args, "-f", "lavfi", "-i", "sine=d=1")
	}
	out, err := exec.Command("ffmpeg", append(args, "-pix_fmt", "yuv420p", "-y", path)...).CombinedOutput()
	require.NoError(t, err, "failed to generate test clip: %s", out)
	return path
}
