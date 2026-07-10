package guides

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// syncGit clones or pulls a git repository.
func (m *Manager) syncGit(src Source) error {
	dir := m.sourceDir(src.Name)
	branch := src.Branch
	if branch == "" {
		branch = "main"
	}

	// If directory exists and has .git, pull. Otherwise clone.
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		return gitPull(dir, branch)
	}

	// Fresh clone
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clean dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return gitClone(src.URL, branch, dir)
}

// syncURL downloads a single markdown file from a raw URL.
func (m *Manager) syncURL(src Source) error {
	dir := m.sourceDir(src.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	resp, err := http.Get(src.URL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Derive filename from URL or use source name
	filename := src.Name + ".md"
	if parts := strings.Split(src.URL, "/"); len(parts) > 0 {
		last := parts[len(parts)-1]
		if strings.Contains(last, ".") {
			filename = last
		}
	}

	outPath := filepath.Join(dir, filename)
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// gitClone runs git clone --depth 1 --single-branch.
func gitClone(url, branch, dir string) error {
	cmd := exec.Command("git", "clone",
		"--depth", "1",
		"--single-branch",
		"--branch", branch,
		url, dir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w (请确认 git 已安装且 URL 有效: %s)", err, url)
	}
	return nil
}

// gitPull runs git pull in an existing repo.
func gitPull(dir, branch string) error {
	// git checkout branch then pull
	cmd := exec.Command("git", "-C", dir, "checkout", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // ignore errors (e.g., already on branch)

	cmd = exec.Command("git", "-C", dir, "pull", "--ff-only")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}
