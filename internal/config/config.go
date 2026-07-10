package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"everevo/internal/atomic"
)

// ModelCapability describes what a specific model can do.
type ModelCapability struct {
	SupportsVision    bool `json:"supportsVision"`
	SupportsTools     bool `json:"supportsTools"`
	SupportsStreaming bool `json:"supportsStreaming"`
	SupportsReasoning bool `json:"supportsReasoning"`
	SupportsJSON      bool `json:"supportsJSON"`  // native JSON mode (response_format json_object)
	SupportsFIM       bool `json:"supportsFIM"`   // Fill-in-the-Middle completion (DeepSeek /beta/completions)
	MaxContextTokens  int  `json:"maxContextTokens"`
}

// LLMProvider represents a single LLM API provider configuration.
type LLMProvider struct {
	ID                string                     `json:"id"`
	Name              string                     `json:"name"`
	Icon              string                     `json:"icon,omitempty"`
	Notes             string                     `json:"notes,omitempty"`
	Type              string                     `json:"type"`      // preset key or "custom"
	APIFormat         string                     `json:"apiFormat"` // "openai" | "anthropic" | "openai-compat"
	Endpoint          string                     `json:"endpoint"`
	APIKey            string                     `json:"apiKey"`
	Model             string                     `json:"model"`
	Models            []string                   `json:"models,omitempty"`            // available model names
	ModelCapabilities map[string]ModelCapability `json:"modelCapabilities,omitempty"` // per-model capability info
	EnableJSONOutput  bool                       `json:"enableJSONOutput"`            // user toggle: force JSON structured output
	Website           string                     `json:"website,omitempty"`
	Enabled           bool                       `json:"enabled"`
	CreatedAt         int64                      `json:"createdAt"`
}

// A2AAgentConfig holds the local agent identity and server settings.
type A2AAgentConfig struct {
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Port        int    `json:"port"`
	Secret      string `json:"secret,omitempty"` // Feishu-style HMAC secret; when set, inbound task requests must be signed
}

// LLMConfig holds all LLM providers, the active one, and global MCP port.
type LLMConfig struct {
	Providers          []LLMProvider `json:"providers"`
	ActiveProvider     string        `json:"activeProvider"`
	ExtractionProvider string        `json:"extractionProvider"` // provider ID for memory fact/graph extraction; "" → active
	MCPPort            int           `json:"mcpPort"`             // global MCP server port
	HTTPProxy          string        `json:"httpProxy"`           // user-configured proxy URL (e.g. "http://127.0.0.1:7890")
	ProxyEnabled       *bool         `json:"proxyEnabled"`        // proxy kill-switch; nil → true (default on)
	A2AConfig          A2AAgentConfig `json:"a2aConfig"`
	FeishuConfig       FeishuConfig   `json:"feishuConfig"`
}

// FeishuConfig holds credentials for the Feishu self-built-app bot (WebSocket
// long-connection mode). Persisted as part of LLMConfig.
type FeishuConfig struct {
	Enabled           bool   `json:"enabled"`
	AppID             string `json:"appId"`
	AppSecret         string `json:"appSecret"`
	VerificationToken string `json:"verificationToken"`
}

// Config 用户配置。
type Config struct {
	Version        string            `json:"version"`
	ModelDirs      []string          `json:"model_dirs"`
	DefaultBackend string            `json:"default_backend"`
	Theme          string            `json:"theme"`
	Language       string            `json:"language"`
	Credentials    map[string]string `json:"credentials"`
	LLM            LLMConfig         `json:"llm"`
}

// Preset defines a built-in LLM provider preset.
type Preset struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	APIFormat string  `json:"apiFormat"`
	Endpoint string   `json:"endpoint"`
	Models   []string `json:"models"`
	Icon     string   `json:"icon,omitempty"`
}

// Presets returns all built-in provider presets.
func Presets() []Preset {
	return []Preset{
		{Name: "OpenAI", Type: "openai", APIFormat: "openai", Endpoint: "https://api.openai.com/v1", Icon: "◎"},
		{Name: "DeepSeek", Type: "deepseek", APIFormat: "openai", Endpoint: "https://api.deepseek.com", Icon: "◈"},
		{Name: "通义千问", Type: "qwen", APIFormat: "openai", Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1", Icon: "◇"},
		{Name: "智谱 GLM", Type: "glm", APIFormat: "openai", Endpoint: "https://open.bigmodel.cn/api/paas/v4", Icon: "□"},
		{Name: "Moonshot", Type: "moonshot", APIFormat: "openai", Endpoint: "https://api.moonshot.cn/v1", Icon: "◉"},
			{Name: "Ollama 本地", Type: "ollama", APIFormat: "openai-compat", Endpoint: "http://127.0.0.1:11434/v1", Icon: "●"},
			{Name: "llama.cpp 本地", Type: "llamacpp", APIFormat: "openai-compat", Endpoint: "http://127.0.0.1:8080/v1", Icon: "◆"},
		}
	}

// UnknownCapability returns the default capability for an unprobed model.
// All capabilities are false; clients must call ProbeModelCapability to get real data.
func UnknownCapability() ModelCapability {
	return ModelCapability{MaxContextTokens: 0}
}

// Defaults 返回默认配置。
func Defaults() *Config {
	return &Config{
		Version:        "0.1.0",
		ModelDirs:      []string{},
		DefaultBackend: "auto",
		Theme:          "system",
		Language:       "zh-CN",
		Credentials:    map[string]string{},
		LLM: LLMConfig{
			Providers:      []LLMProvider{},
			ActiveProvider: "",
			A2AConfig: A2AAgentConfig{
				Name:    "EverEvo Agent",
				Version: "0.1.0",
				Port:    19801,
			},
			FeishuConfig: FeishuConfig{},
		},
	}
}

// Path 返回用户配置文件路径。
func Path() string {
	return filepath.Join(UserConfigDir(), "user_config.json")
}

// UserConfigDir returns the user-level config directory (%APPDATA%\EverEvo).
func UserConfigDir() string {
	dir := os.Getenv("APPDATA")
	if dir == "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	return filepath.Join(dir, "EverEvo")
}

// Load 读取用户配置，文件不存在则返回默认值。
// Automatically migrates old single-provider format to new multi-provider format.
func Load() (*Config, error) {
	cfgPath := Path()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// Try new format first
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// Migrate from old single-provider format
	migrateOldLLM(cfg, data)

	// If migrated or new, ensure valid state
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = []LLMProvider{}
	}

	return cfg, nil
}

// migrateOldLLM detects and migrates old LLM config format.
func migrateOldLLM(cfg *Config, raw []byte) {
	// Old format had "endpoint"/"apiKey"/"model"/"mcpPort" directly under "llm"
	var legacy struct {
		LLM struct {
			Endpoint string `json:"endpoint"`
			APIKey   string `json:"apiKey"`
			Model    string `json:"model"`
			MCPPort  int    `json:"mcpPort"`
		} `json:"llm"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return
	}
	// If old fields exist and no providers, migrate
	if legacy.LLM.Endpoint != "" && len(cfg.LLM.Providers) == 0 {
		id := fmt.Sprintf("migrated-%d", time.Now().Unix())
		cfg.LLM.Providers = []LLMProvider{{
			ID:        id,
			Name:      "默认供应商",
			Type:      "custom",
			APIFormat: "openai",
			Endpoint:  legacy.LLM.Endpoint,
			APIKey:    legacy.LLM.APIKey,
			Model:     legacy.LLM.Model,
			Models:    []string{legacy.LLM.Model},
			Enabled:   true,
			CreatedAt: time.Now().UnixMilli(),
		}}
		cfg.LLM.MCPPort = legacy.LLM.MCPPort
		cfg.LLM.ActiveProvider = id
	}
}

// Save 保存用户配置，自动创建目录。
func Save(cfg *Config) error {
	cfgPath := Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return atomic.WriteFile(cfgPath, data, 0644)
}
