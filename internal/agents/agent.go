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
	"sync"
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
const BaseSystemPrompt = "你是 EverEvo 桌面软件的 AI 助手，遵循 ReAct（推理-行动）框架工作。\n\n## 工作流程 (ReAct Framework)\n1. 分析 (Thought): 理解用户意图，判断需要什么信息、调用哪些工具。\n2. 行动 (Action): 选择合适的工具，用精确的参数调用。\n3. 观察 (Observation): 仔细阅读工具返回结果。\n4. 重复 直到掌握足够信息，然后给出最终答案。\n5. 最终回答 (Final Answer): 用简洁中文直接回复用户，不要照搬工具输出的原始 JSON。\n\n## 工具使用规则\n- 先思考再行动，失败时分析原因尝试替代方案。\n- JSON 结果提取关键字段后再回复，不要整套贴出。\n- 不需要工具就直接回答。\n\n## 其他\n- 用户说中文用中文回复。文件上传通过 read_file / read_media_file 读取。\n\n## 高级功能\n- 任务规划: 复杂任务用 plan_create 拆解，plan_step_update 标记进度。\n- 多 Agent 协同: collab_create 创建协同会话，collab_dispatch_async 派发，collab_wait 汇总。"

// Manager holds the agent list and handles persistence. All mutating ops take
// the mutex because concurrent multi-domain delegation + UI CRUD race on the
// Agents slice (and on Save).
type Manager struct {
	mu     sync.RWMutex
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
	// Migrate the default agent's system prompt to the latest BaseSystemPrompt
	// when it lacks the collaboration/planning guidance. Safe for user-customized
	// prompts: only touches agents whose prompt matches the old built-in shape
	// (no "【任务规划】" marker = pre-collab version).
	m.migrateDefaultPrompt()
	log.Printf("[agents] 已加载 %d 个本地 Agent", len(m.Agents))
	return m
}

// migrateDefaultPrompt updates the default agent's SystemPrompt to the current
// BaseSystemPrompt if it was seeded from an older version. Detected by the
// absence of the "【任务规划】" marker that the new prompt always contains.
// User-customized prompts that happen to omit the marker are also updated —
// the trade-off for keeping the built-in default fresh across upgrades.
func (m *Manager) migrateDefaultPrompt() {
	changed := false
	for i := range m.Agents {
		if !m.Agents[i].IsDefault {
			continue
		}
		if !strings.Contains(m.Agents[i].SystemPrompt, "ReAct") {
			m.Agents[i].SystemPrompt = BaseSystemPrompt
			m.Agents[i].UpdatedAt = time.Now().UnixMilli()
			changed = true
			log.Printf("[agents] 默认 Agent prompt 已升级（加入协同/规划引导）")
		}
	}
	if changed {
		_ = m.Save()
	}
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
// Save persists the agent list to disk atomically. Takes the write lock.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

// saveLocked persists without taking the lock — for callers already holding it.
func (m *Manager) saveLocked() error {
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
func (m *Manager) List() []Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Agents
}

// Get returns an agent by ID.
func (m *Manager) Get(id string) (*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q 不存在", id)
}

// FindByName returns the first agent matching the given name (case-insensitive).
func (m *Manager) FindByName(name string) (*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	a.ID = newID()
	a.IsDefault = false // only the seeded default is default
	a.CreatedAt = now
	a.UpdatedAt = now
	m.Agents = append(m.Agents, a)
	if err := m.saveLocked(); err != nil {
		m.Agents = m.Agents[:len(m.Agents)-1]
		return nil, err
	}
	return &m.Agents[len(m.Agents)-1], nil
}

// Update modifies an existing agent by ID.
func (m *Manager) Update(id string, a Agent) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			a.ID = id
			a.CreatedAt = m.Agents[i].CreatedAt
			a.UpdatedAt = time.Now().UnixMilli()
			a.IsDefault = m.Agents[i].IsDefault // default flag is immutable here
			m.Agents[i] = a
			if err := m.saveLocked(); err != nil {
				return nil, err
			}
			return &m.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q 不存在", id)
}

// Delete removes an agent by ID. The default agent cannot be deleted.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Agents {
		if m.Agents[i].ID == id {
			if m.Agents[i].IsDefault {
				return fmt.Errorf("默认主 Agent 不能删除")
			}
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return m.saveLocked()
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

// EnsureLibraryIDs backfills empty or invalid LibraryID fields with the given
// default ID and saves. Safe to call at startup after the memory store is ready.
// validIDs is the set of current domain library IDs from the memory store.
func (m *Manager) EnsureLibraryIDs(defaultLibraryID string, validIDs []string) error {
	valid := make(map[string]bool, len(validIDs))
	for _, id := range validIDs {
		valid[id] = true
	}
	changed := false
	for i := range m.Agents {
		if m.Agents[i].LibraryID == "" || !valid[m.Agents[i].LibraryID] {
			if m.Agents[i].LibraryID != "" {
				log.Printf("[agents] Agent %q 的 libraryId %q 无效，回填为默认领域", m.Agents[i].Name, m.Agents[i].LibraryID)
			}
			m.Agents[i].LibraryID = defaultLibraryID
			changed = true
		}
	}
	if changed {
		return m.Save()
	}
	return nil
}

// SetLibrary changes an agent's library/domain association and persists.
func (m *Manager) SetLibrary(agentID, libraryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Agents {
		if m.Agents[i].ID == agentID {
			m.Agents[i].LibraryID = libraryID
			return m.saveLocked()
		}
	}
	return fmt.Errorf("agent %q 不存在", agentID)
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
