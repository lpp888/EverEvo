package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KnownModelExtensions 已知模型文件扩展名。
var KnownModelExtensions = []string{".gguf", ".onnx", ".safetensors", ".bin", ".pt", ".pth"}

// AppDataDir returns the user-scoped application data directory under %APPDATA%.
// This is where critical, small data lives (config, memory, agents, wiki).
// Survives EXE reinstall / directory cleanup.
func AppDataDir() (string, error) {
	dir := os.Getenv("APPDATA")
	if dir == "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	if dir == "" {
		return "", fmt.Errorf("无法确定 APPDATA 目录")
	}
	return filepath.Join(dir, "EverEvo"), nil
}

// ExeDir returns the directory containing the running executable.
func ExeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// DataDir returns the application data root directory — always beside the EXE.
// Use for large, re-downloadable data (models, cache).
func DataDir() (string, error) {
	return ExeDir()
}

// ModelsDir 返回模型存放目录。多级搜索策略以适应不同运行模式。
func ModelsDir() (string, error) {
	candidates := []string{}

	// 1. 工作目录下的 data/models（wails dev 模式项目根目录）
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "data", "models"))
	}
	// 2. EXE 所在目录的 data/models
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "data", "models"))
		// 3. EXE 上级目录的 data/models（wails build 后 build/bin/exe → 项目根）
		candidates = append(candidates, filepath.Join(filepath.Dir(exeDir), "data", "models"))
	}

	for _, d := range candidates {
		if st, e := os.Stat(d); e == nil && st.IsDir() {
			return d, nil
		}
	}
	// Fallback: EXE dir
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "data", "models"), nil
	}
	return "", fmt.Errorf("无法确定模型目录")
}

// CacheDir 返回缓存目录。
func CacheDir() (string, error) {
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "data", "cache"), nil
}

// EnsureDataDir 创建数据目录及其子目录。
func EnsureDataDir() error {
	base, err := DataDir()
	if err != nil {
		return err
	}
	for _, sub := range []string{"data", "data/models", "data/cache", "data/plugins", "data/plugin-tmp"} {
		p := filepath.Join(base, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", p, err)
		}
	}
	// AppData: critical user data — survive reinstalls.
	appDir, err := AppDataDir()
	if err != nil {
		return err
	}
	for _, sub := range []string{"", "knowledge", "knowledge/chromem", "memory", "wiki", "wiki/chromem", "workflows"} {
		p := filepath.Join(appDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", p, err)
		}
	}
	return nil
}

// DiscoverModels 遍历给定目录，返回找到的模型文件路径。
func DiscoverModels(dirs []string) []string {
	var found []string
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			for _, known := range KnownModelExtensions {
				if ext == known {
					found = append(found, path)
					break
				}
			}
			return nil
		})
	}
	return found
}
