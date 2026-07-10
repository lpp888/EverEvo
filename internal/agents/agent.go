// Package agents defines EverEvo local Agent personas — named, reusable LLM
// profiles that bundle a system prompt, an optional provider/model override,
// and a selected set of skills/tools. Agents can be managed in the UI, created
// at runtime by the main agent, used as delegation targets, and selected as the
// active persona in chat.
//
// This is distinct from the A2A stack (internal/a2a), which handles
// inter-process / remote agent networking. An Agent here is a *local* persona.
package agents

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"everevo/internal/atomic"
	"everevo/internal/storage"
)

// Agent is one local agent persona.
type Agent struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Icon          string   `json:"icon,omitempty"`
	SystemPrompt  string   `json:"systemPrompt"`
	ProviderID    string   `json:"providerId,omitempty"`   // override; "" = use active provider
	Model         string   `json:"model,omitempty"`        // override model name (within the provider)
	InheritSkills bool     `json:"inheritSkills"`          // true = use all currently-enabled skills
	Skills        []string `json:"skills,omitempty"`       // skill names (used when InheritSkills is false)
	Tools         []string `json:"tools,omitempty"`        // extra built-in tool names to grant
	MCPTools      []string `json:"mcpTools,omitempty"`     // external MCP tool names (mcp__*) to grant
	Temperature   *float64 `json:"temperature,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	IsDefault     bool     `json:"isDefault,omitempty"` // the main agent; cannot be deleted
	LibraryID     string   `json:"libraryId,omitempty"` // domain library this agent belongs to
	CreatedAt     int64    `json:"createdAt"`
	UpdatedAt     int64    `json:"updatedAt"`
}

// BaseSystemPrompt is the default persona prompt for the main agent. It mirrors
// the chat base prompt (chatStore.ts) so the seeded default agent reproduces the
// pre-existing chat behavior exactly.
const BaseSystemPrompt = "你是 EverEvo 桌面软件的 AI 助手。用户说中文，用中文回复。当需要执行操作时使用工具调用。每次回复尽量简洁。\n\n用户可能通过拖拽或粘贴上传文件到对话中。对于文本文件（TXT、MD、CSV、JSON 等），内容会自动注入。对于 PDF 和图片文件，请使用 read_file 或 read_media_file 工具读取。对于扫描件 PDF（isScanned=true），请使用 read_media_file 工具以图片形式查看页面。"

// Manager holds the agent list and handles persistence.
type Manager struct {
	Agents []Agent `json:"agents"`
}

// agentsPath returns the path to the persisted agents file.
func agentsPath() string {
	dataDir, err := storage.AppDataDir()
	if err != nil {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "agents.json")
}

// NewManager creates an agent manager, loading from disk if available and
// ensuring a default main agent always exists.
func NewManager() *Manager {
	m := &Manager{}
	loaded := loadFromDisk()
	if loaded != nil {
		m.Agents = loaded
	}
	if !m.hasDefault() {
		m.Agents = append(m.Agents, defaultAgent())
		_ = m.Save()
	}
	log.Printf("[agents] 已加载 %d 个本地 Agent", len(m.Agents))
	return m
}

func (m *Manager) hasDefault() bool {
	for _, a := range m.Agents {
		if a.IsDefault {
			return true
		}
	}
	return false
}

// defaultAgent builds the seeded main agent that reproduces current chat behavior.
func defaultAgent() Agent {
	now := time.Now().UnixMilli()
	return Agent{
		ID:            newID(),
		Name:          "Evo",
		Description:   "EverEvo 核心调度智能体，统领所有领域 Agent，可委派跨域任务。",
		Icon:          "◉",
		SystemPrompt:  BaseSystemPrompt,
		InheritSkills: true,
		IsDefault:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// loadFromDisk reads persisted agents from data/agents.json.
func loadFromDisk() []Agent {
	path := agentsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		log.Printf("[agents] 解析 %s 失败: %v", path, err)
		return nil
	}
	return agents
}

// Save persists the agent list to disk atomically.
func (m *Manager) Save() error {
	path := agentsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建 agents 目录失败: %w", err)
	}
	data, err := json.MarshalIndent(m.Agents, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 agents 失败: %w", err)
	}
	if err := atomic.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入 agents.json 失败: %w", err)
	}
	return nil
}

// List returns all agents.
func (m *Manager) List() []Agent { return m.Agents }

// Get returns an agent by ID.
func (m *Manager) Get(id string) (*Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q 不存在", id)
}

// FindByName returns the first agent matching the given name (case-insensitive).
func (m *Manager) FindByName(name string) (*Agent, error) {
	for i := range m.Agents {
		if equalFoldName(m.Agents[i].Name, name) {
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("名为 %q 的 agent 不存在", name)
}

// Create adds a new agent. ID/timestamps are assigned here.
func (m *Manager) Create(a Agent) (*Agent, error) {
	if a.Name == "" {
		return nil, fmt.Errorf("agent 名称不能为空")
	}
	now := time.Now().UnixMilli()
	a.ID = newID()
	a.IsDefault = false // only the seeded default is default
	a.CreatedAt = now
	a.UpdatedAt = now
	m.Agents = append(m.Agents, a)
	if err := m.Save(); err != nil {
		m.Agents = m.Agents[:len(m.Agents)-1]
		return nil, err
	}
	return &m.Agents[len(m.Agents)-1], nil
}

// Update modifies an existing agent by ID.
func (m *Manager) Update(id string, a Agent) (*Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			a.ID = id
			a.CreatedAt = m.Agents[i].CreatedAt
			a.UpdatedAt = time.Now().UnixMilli()
			a.IsDefault = m.Agents[i].IsDefault // default flag is immutable here
			m.Agents[i] = a
			if err := m.Save(); err != nil {
				return nil, err
			}
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q 不存在", id)
}

// Delete removes an agent by ID. The default agent cannot be deleted.
func (m *Manager) Delete(id string) error {
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			if m.Agents[i].IsDefault {
				return fmt.Errorf("默认主 Agent 不能删除")
			}
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return m.Save()
		}
	}
	return fmt.Errorf("agent %q 不存在", id)
}

// ListByLibrary returns agents that belong to the given domain library.
func (m *Manager) ListByLibrary(libraryID string) []Agent {
	var out []Agent
	for _, a := range m.Agents {
		if a.LibraryID == libraryID {
			out = append(out, a)
		}
	}
	return out
}

// GetCoreAgent returns the default agent in the core (first) library, or
// the global default agent as a fallback.
func (m *Manager) GetCoreAgent(defaultLibraryID string) (*Agent, error) {
	for i := range m.Agents {
		if m.Agents[i].IsDefault && m.Agents[i].LibraryID == defaultLibraryID {
			return &m.Agents[i], nil
		}
	}
	// Fallback: any default agent
	for i := range m.Agents {
		if m.Agents[i].IsDefault {
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("no core agent found")
}

// EnsureLibraryIDs backfills empty LibraryID fields with the given default ID
// and saves. Safe to call at startup after the memory store is ready.
func (m *Manager) EnsureLibraryIDs(defaultLibraryID string) error {
	changed := false
	for i := range m.Agents {
		if m.Agents[i].LibraryID == "" {
			m.Agents[i].LibraryID = defaultLibraryID
			changed = true
		}
	}
	if changed {
		return m.Save()
	}
	return nil
}

// newID returns a short unique ID (8 random hex chars + unix seconds base36-ish).
func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "ag-" + hex.EncodeToString(b)
}

// equalFoldName compares two display names case-insensitively, ignoring spaces.
func equalFoldName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
