package kind_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// labDir resolves hack/kind/ relative to this test file (not CWD).
func labDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func TestKindLabScriptsExist(t *testing.T) {
	dir := labDir(t)
	for _, name := range []string{"common.sh", "setup.sh", "teardown.sh", "status.sh", "kind-config.yaml", "README.md"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing kind lab file %s: %v", name, err)
		}
	}
}

func TestKindConfigNamesClusterAndExposesGitLabPorts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(labDir(t), "kind-config.yaml"))
	if err != nil {
		t.Fatalf("read kind-config.yaml: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "name: assent") {
		t.Error("kind-config.yaml must name cluster 'assent'")
	}
	// Host ports fixed by Spike B / e2e docs — clients dial localhost:8929.
	if !strings.Contains(text, "hostPort: 8929") {
		t.Error("kind-config.yaml must map GitLab HTTP to hostPort 8929")
	}
	if !strings.Contains(text, "hostPort: 2224") {
		t.Error("kind-config.yaml must map GitLab SSH to hostPort 2224")
	}
	if !strings.Contains(text, "containerPort: 30080") {
		t.Error("kind-config.yaml must expose NodePort 30080 for GitLab HTTP")
	}
}
