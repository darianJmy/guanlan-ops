package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/textutil"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxRepositoryFiles    = 10000
	maxRepositoryBytes    = 256 << 20
	maxRepositoryArtifact = 64 << 20
)

type repositoryArtifact struct {
	Data     []byte
	Revision string
	SHA256   string
}

func repositoryArtifactDirectory(repositoryID uint) string {
	root := strings.TrimSpace(os.Getenv("AIOPS_GIT_REPOSITORY_DIR"))
	if root == "" {
		root = filepath.Join("data", "git-repositories")
	}
	return filepath.Join(root, fmt.Sprintf("%d", repositoryID))
}

func repositoryArtifactFile(repositoryID uint, sha string) string {
	return filepath.Join(repositoryArtifactDirectory(repositoryID), sha+".tar.gz")
}

func storeRepositoryArtifact(repositoryID uint, artifact repositoryArtifact) error {
	if repositoryID == 0 || len(artifact.Data) == 0 || artifact.Revision == "" || artifact.SHA256 == "" {
		return errors.New("Git 制品信息不完整")
	}
	directory := repositoryArtifactDirectory(repositoryID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errors.New("创建 Git 制品目录失败")
	}
	temporary, err := os.CreateTemp(directory, "artifact-*.tmp")
	if err != nil {
		return errors.New("创建 Git 临时制品失败")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return errors.New("设置 Git 制品权限失败")
	}
	if _, err := temporary.Write(artifact.Data); err != nil {
		temporary.Close()
		return errors.New("写入 Git 制品失败")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return errors.New("持久化 Git 制品失败")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("关闭 Git 制品失败")
	}
	if err := os.Rename(temporaryName, repositoryArtifactFile(repositoryID, artifact.SHA256)); err != nil {
		return errors.New("更新 Git 制品失败")
	}
	return nil
}

func readSyncedRepositoryArtifact(repository models.GitRepository) (repositoryArtifact, error) {
	if repository.SyncedRevision == "" || repository.SyncedSHA256 == "" {
		return repositoryArtifact{}, errors.New("Git 仓库尚未同步，请先在脚本库中同步仓库")
	}
	data, err := os.ReadFile(repositoryArtifactFile(repository.ID, repository.SyncedSHA256))
	if err != nil {
		return repositoryArtifact{}, errors.New("Git 同步制品不存在，请重新同步仓库")
	}
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	if sha != repository.SyncedSHA256 {
		return repositoryArtifact{}, errors.New("Git 同步制品校验失败，请重新同步仓库")
	}
	return repositoryArtifact{Data: data, Revision: repository.SyncedRevision, SHA256: sha}, nil
}

func removeRepositoryArtifact(repositoryID uint) error {
	return os.RemoveAll(repositoryArtifactDirectory(repositoryID))
}

func buildRepositoryArtifact(repository models.GitRepository, ref string) (repositoryArtifact, error) {
	if repository.Status != 1 {
		return repositoryArtifact{}, errors.New("Git 仓库已停用")
	}
	if strings.TrimSpace(ref) == "" {
		ref = repository.DefaultRef
	}
	root, err := os.MkdirTemp("", "aiops-git-source-")
	if err != nil {
		return repositoryArtifact{}, errors.New("创建仓库临时目录失败")
	}
	defer os.RemoveAll(root)
	repoDir := filepath.Join(root, "repository")
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref, "--single-branch")
	}
	args = append(args, "--", repository.RepositoryURL, repoDir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := configureGitCredential(root, repository, command); err != nil {
		return repositoryArtifact{}, err
	}
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return repositoryArtifact{}, errors.New("拉取 Git 仓库超时")
	}
	if err != nil {
		return repositoryArtifact{}, fmt.Errorf("拉取 Git 仓库失败：%s", textutil.Ellipsis(strings.TrimSpace(string(output)), 1000))
	}
	revisionOutput, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return repositoryArtifact{}, errors.New("读取 Git commit 失败")
	}
	data, err := archiveRepository(repoDir)
	if err != nil {
		return repositoryArtifact{}, err
	}
	digest := sha256.Sum256(data)
	return repositoryArtifact{Data: data, Revision: strings.TrimSpace(string(revisionOutput)), SHA256: hex.EncodeToString(digest[:])}, nil
}

func configureGitCredential(root string, repository models.GitRepository, command *exec.Cmd) error {
	if repository.CredentialID == 0 {
		return nil
	}
	var credential models.GitCredential
	if database.DB.First(&credential, repository.CredentialID).Error != nil || credential.Status != 1 {
		return errors.New("Git 认证不存在或已停用")
	}
	credentialType, credentialUser, credentialCipher := credential.AuthType, credential.Username, credential.SecretCipher
	if credentialType == "none" || credentialType == "" {
		return nil
	}
	secret, err := decryptSecret(credentialCipher)
	if err != nil {
		return errors.New("Git 凭据解密失败")
	}
	switch credentialType {
	case "token":
		if !strings.HasPrefix(repository.RepositoryURL, "https://") {
			return errors.New("Token 凭据只支持 HTTPS 仓库")
		}
		taskpass := filepath.Join(root, "git-askpass.sh")
		content := "#!/bin/sh\ncase \"$1\" in *Username*) printf '%s\\n' \"$AIOPS_GIT_USER\" ;; *) printf '%s\\n' \"$AIOPS_GIT_TOKEN\" ;; esac\n"
		if err := os.WriteFile(taskpass, []byte(content), 0700); err != nil {
			return errors.New("创建 Git 凭据代理失败")
		}
		command.Env = append(command.Env, "GIT_ASKPASS="+taskpass, "AIOPS_GIT_USER="+credentialUser, "AIOPS_GIT_TOKEN="+secret)
	case "ssh_key":
		keyFile := filepath.Join(root, "git_deploy_key")
		if err := os.WriteFile(keyFile, []byte(secret), 0600); err != nil {
			return errors.New("创建 Git 部署密钥失败")
		}
		knownHosts := filepath.Join(root, "git_known_hosts")
		command.Env = append(command.Env, "GIT_SSH_COMMAND=ssh -i "+keyFile+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="+knownHosts)
	default:
		return errors.New("不支持的 Git 凭据类型")
	}
	return nil
}

func archiveRepository(root string) ([]byte, error) {
	var result bytes.Buffer
	gzipWriter := gzip.NewWriter(&result)
	tarWriter := tar.NewWriter(gzipWriter)
	files, totalBytes := 0, int64(0)
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		if files > maxRepositoryFiles {
			return errors.New("Git 仓库文件数量超过限制")
		}
		totalBytes += info.Size()
		if totalBytes > maxRepositoryBytes {
			return errors.New("Git 仓库解压后大小超过 256 MB")
		}
		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(filePath)
			if err != nil {
				return err
			}
			cleaned := filepath.Clean(linkTarget)
			if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
				return errors.New("Git 仓库包含指向仓库外部的符号链接")
			}
		}
		header, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.ModTime = time.Unix(0, 0)
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tarWriter, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if result.Len() > maxRepositoryArtifact {
			return errors.New("Git 仓库压缩包超过 64 MB")
		}
		return nil
	})
	if err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		return nil, err
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	if result.Len() > maxRepositoryArtifact {
		return nil, errors.New("Git 仓库压缩包超过 64 MB")
	}
	return result.Bytes(), nil
}
