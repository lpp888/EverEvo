//go:build windows

package toolbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Type 是探测出的模型类型。
type Type string

const (
	TypeSentenceEmbedding   Type = "sentence-embedding"
	TypeImageClassification Type = "image-classification"
	TypeUnknown             Type = "unknown"
)

// ModelMeta 是模型包的探测结果。
type ModelMeta struct {
	Dir    string
	Type   Type
	Hidden int // 嵌入维度（句向量用，来自 config.json hidden_size）
}

type hfConfig struct {
	Architectures []string `json:"architectures"`
	ModelType     string   `json:"model_type"`
	HiddenSize    int      `json:"hidden_size"`
}

// Detect 探测一个已下载模型包的类型。
// 信号优先级：1_Pooling/config_sentence_transformers.json（强·句向量）
// > preprocessor_config.json（强·图像）> config.json 的 architectures/model_type（弱·启发式）。
func Detect(pkgDir string) ModelMeta {
	m := ModelMeta{Dir: pkgDir, Type: TypeUnknown}
	cfg := readHFConfig(pkgDir)
	m.Hidden = cfg.HiddenSize

	if fileExists(filepath.Join(pkgDir, "1_Pooling", "config.json")) ||
		fileExists(filepath.Join(pkgDir, "config_sentence_transformers.json")) {
		m.Type = TypeSentenceEmbedding
		return m
	}
	if fileExists(filepath.Join(pkgDir, "preprocessor_config.json")) {
		m.Type = TypeImageClassification
		return m
	}
	if isTextModel(cfg) {
		m.Type = TypeSentenceEmbedding
		return m
	}
	if isImageModel(cfg) {
		m.Type = TypeImageClassification
		return m
	}
	return m
}

func isTextModel(c hfConfig) bool {
	for _, a := range c.Architectures {
		s := strings.ToLower(a)
		if strings.Contains(s, "bert") || strings.Contains(s, "distil") ||
			strings.Contains(s, "mpnet") || strings.Contains(s, "sentence") ||
			strings.Contains(s, "embed") {
			return true
		}
	}
	switch strings.ToLower(c.ModelType) {
	case "bert", "distilbert", "mpnet", "xlm-roberta", "roberta",
		"e5", "bge", "gte", "nomic", "jina":
		return true
	}
	return false
}

func isImageModel(c hfConfig) bool {
	for _, a := range c.Architectures {
		s := strings.ToLower(a)
		if strings.Contains(s, "forimageclassification") || strings.Contains(s, "vit") ||
			strings.Contains(s, "resnet") || strings.Contains(s, "convnext") ||
			strings.Contains(s, "swin") || strings.Contains(s, "efficientnet") {
			return true
		}
	}
	switch strings.ToLower(c.ModelType) {
	case "vit", "resnet", "convnext", "swin", "efficientnet":
		return true
	}
	return false
}

func readHFConfig(dir string) hfConfig {
	var c hfConfig
	// Try root config.json first, then onnx/config.json — slim / ONNX-only
	// exports (e.g. Xenova/* quantized packages) often keep config under onnx/.
	for _, rel := range []string{"config.json", "onnx/config.json"} {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		_ = json.Unmarshal(b, &c)
		if c.HiddenSize > 0 || len(c.Architectures) > 0 || c.ModelType != "" {
			return c
		}
	}
	return c
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
