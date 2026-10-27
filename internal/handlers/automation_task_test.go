package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"guanlan/internal/models"
	"guanlan/pkg/archiveutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestAutomationCertificateDomainIDFromFaultContext(t *testing.T) {
	input := map[string]interface{}{"alert": map[string]interface{}{"source": "certificate-domain", "source_id": float64(35)}}
	if got := automationCertificateDomainID(input); got != 35 {
		t.Fatalf("expected domain id 35, got %d", got)
	}
}

func TestPrimaryCertificateDomainFallsBackToCertificateName(t *testing.T) {
	if got := primaryCertificateDomain(models.Certificate{Name: "fallback", Domains: `["example.com","www.example.com"]`}); got != "example.com" {
		t.Fatalf("expected first certificate domain, got %q", got)
	}
	if got := primaryCertificateDomain(models.Certificate{Name: "fallback", Domains: `[]`}); got != "fallback" {
		t.Fatalf("expected certificate name fallback, got %q", got)
	}
}

func TestBuildCertificateRepositoryCommandInjectsTemporaryFiles(t *testing.T) {
	command := buildCertificateRepositoryCommand("bash deploy.sh", "certificate-data", "private-key-data", map[string]string{"DOMAIN": "example.com", "CERT_NAME": "example"})
	for _, fragment := range []string{"CERT_DEPLOY_DIR=", "CERT_FILE=", "KEY_FILE=", "export CERT_DEPLOY_DIR CERT_FILE KEY_FILE", "export DOMAIN='example.com'", "bash -c 'bash deploy.sh'", "trap 'rm -rf"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command does not contain %q", fragment)
		}
	}
}

func TestBuildDeployScriptCommandNormalizesWindowsLineEndings(t *testing.T) {
	script := "#!/bin/bash\r\n\r\necho \"$CERT_FILE\"\r\n\r\necho \"$KEY_FILE\"\r\n"
	command := buildDeployScriptCommand("shell", script, "certificate-data", "private-key-data", map[string]string{"DOMAIN": "example.com"})
	output, err := exec.Command("bash", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("CRLF deployment script failed: %v\n%s", err, output)
	}
	text := string(output)
	if strings.Contains(text, "command not found") || strings.Contains(text, "$'\\r'") {
		t.Fatalf("deployment output still contains a carriage-return failure:\n%s", text)
	}
	for _, fragment := range []string{"certificate.pem", "private_key.pem"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("deployment output does not contain %q:\n%s", fragment, text)
		}
	}
	if strings.Contains(command, "bash -x") {
		t.Fatal("deployment command must not enable shell xtrace")
	}
	if strings.Contains(text, "[cert-deploy]") {
		t.Fatalf("deployment output contains platform logs:\n%s", text)
	}
	if lines := strings.Split(strings.TrimSpace(text), "\n"); len(lines) != 2 {
		t.Fatalf("expected only the script's two stdout lines, got %d:\n%s", len(lines), text)
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

func TestRetiredCertificateAutomationCannotExecute(t *testing.T) {
	for _, source := range []string{"script", "repository"} {
		_, err := executeAutomationTaskOnce(models.AutomationTask{RuntimeProfile: "certificate_deploy", SourceType: source}, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "模块已移除") {
			t.Fatalf("retired %s task must be rejected before execution: %v", source, err)
		}
	}
}
