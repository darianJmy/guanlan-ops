package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"guanlan/internal/models"
	"guanlan/pkg/archiveutil"
)

func TestValidateGitScript(t *testing.T) {
	valid := models.Script{RepositoryURL: "git@github.com:team/ops.git", GitRef: "v1.2.0", Entrypoint: "scripts/deploy.sh"}
	if err := validateGitScript(valid); err != nil {
		t.Fatalf("valid git script rejected: %v", err)
	}
	invalid := valid
	invalid.Entrypoint = "../outside.sh"
	if err := validateGitScript(invalid); err == nil {
		t.Fatal("expected traversal entrypoint to be rejected")
	}
}

func TestBuildAutomationGitCommandUsesInputAndRepositoryRoot(t *testing.T) {
	script := models.Script{Type: "shell", RepositoryURL: "git@github.com:team/ops.git", GitRef: "v1.2.0", Entrypoint: "scripts/run.sh"}
	command := buildAutomationGitCommand(script, []byte(`{"hello":"world"}`))
	for _, fragment := range []string{"git clone --depth 1", "--branch 'v1.2.0'", "AIOPS_INPUT_FILE", "cd \"$AIOPS_TASK_DIR/repository\"", "bash 'scripts/run.sh'"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command does not contain %q", fragment)
		}
	}
}

func TestBuildAutomationScriptCommandExportsJSONInputFile(t *testing.T) {
	command := buildAutomationScriptCommand("shell", "cat \"$AIOPS_INPUT_FILE\"", []byte(`{"name":"hello"}`))
	for _, fragment := range []string{"AIOPS_INPUT_FILE=", "export AIOPS_INPUT_FILE", "base64 -d", "trap 'rm -rf"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command does not contain %q", fragment)
		}
	}
}

func TestNormalizeAutomationJSONValueDecodesNestedResult(t *testing.T) {
	var value interface{}
	input := `{"result":"{\"error\":\"curl failed\",\"exit_code\":6}"}`
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		t.Fatal(err)
	}
	normalized := normalizeAutomationJSONValue(value).(map[string]interface{})
	result, ok := normalized["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested result was not decoded: %#v", normalized["result"])
	}
	if result["error"] != "curl failed" || result["exit_code"] != float64(6) {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
}

func TestRunLocalAutomationCommandReturnsTerminalOutput(t *testing.T) {
	result, err := runLocalAutomationCommand("printf 'abc\\n'; printf 'ddd\\n'; lllll", time.Second)
	if err == nil {
		t.Fatal("expected command failure")
	}
	for _, fragment := range []string{"abc\n", "ddd\n", "lllll: command not found"} {
		if !strings.Contains(result.Output, fragment) {
			t.Fatalf("terminal output does not contain %q: %q", fragment, result.Output)
		}
	}
	if result.ExitCode != 127 {
		t.Fatalf("expected exit code 127, got %d", result.ExitCode)
	}
}

func TestRepositoryArtifactRoundTrip(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "certificate"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "certificate", "deploy.sh"), []byte("echo ok\n"), 0755); err != nil {
		t.Fatal(err)
	}
	artifact, err := archiveRepository(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := archiveutil.ExtractTarGzip(artifact, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "certificate", "deploy.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "echo ok\n" {
		t.Fatalf("unexpected artifact content: %q", content)
	}
}

func TestSyncedRepositoryArtifactRoundTrip(t *testing.T) {
	t.Setenv("AIOPS_GIT_REPOSITORY_DIR", t.TempDir())
	data := []byte("synced repository artifact")
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	artifact := repositoryArtifact{Data: data, Revision: strings.Repeat("a", 40), SHA256: sha}
	if err := storeRepositoryArtifact(42, artifact); err != nil {
		t.Fatal(err)
	}
	loaded, err := readSyncedRepositoryArtifact(models.GitRepository{ID: 42, SyncedRevision: artifact.Revision, SyncedSHA256: sha})
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Data) != string(data) || loaded.Revision != artifact.Revision || loaded.SHA256 != sha {
		t.Fatalf("unexpected synced artifact: %#v", loaded)
	}
}

func TestSyncedRepositoryArtifactRejectsCorruption(t *testing.T) {
	t.Setenv("AIOPS_GIT_REPOSITORY_DIR", t.TempDir())
	data := []byte("valid artifact")
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	artifact := repositoryArtifact{Data: data, Revision: strings.Repeat("b", 40), SHA256: sha}
	if err := storeRepositoryArtifact(7, artifact); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repositoryArtifactFile(7, sha), []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSyncedRepositoryArtifact(models.GitRepository{ID: 7, SyncedRevision: artifact.Revision, SyncedSHA256: sha}); err == nil {
		t.Fatal("corrupted synced artifact accepted")
	}
}

func TestRemoteArtifactCommandDoesNotCloneGit(t *testing.T) {
	task := models.AutomationTask{WorkingDirectory: "certificate", Command: "bash deploy.sh"}
	artifact := repositoryArtifact{Revision: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}
	command := buildRemoteArtifactCommand(task, artifact, []byte(`{}`))
	if strings.Contains(command, "git clone") {
		t.Fatal("execution node must not clone Git")
	}
	for _, fragment := range []string{"cat > \"$AIOPS_TASK_DIR/artifact.tar.gz\"", "sha256sum", "AIOPS_GIT_COMMIT", "bash -c 'bash deploy.sh'"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command does not contain %q", fragment)
		}
	}
}
