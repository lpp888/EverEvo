//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"everevo/internal/agents"
	"everevo/internal/config"
	"everevo/internal/skills"
	"everevo/internal/tools"
)

// ─── Local Agent execution core ────────────────────────────────
//
// Resolves an Agent persona into concrete LLM call inputs (provider, system
// prompt, tool set) and runs a bounded tool loop. Used by the agent_run
// delegation tool. ChatPanel uses GetAgentChatContext to drive an agent
// persona through the streaming path.

// AgentChatContext is the pre-resolved chat configuration for an agent, used by
// the frontend chatLoop so it does not need to re-implement skill/tool resolution.
type AgentChatContext struct {
	AgentID      string           `json:"agentId"`
	Name         string           `json:"name"`
	SystemPrompt string           `json:"systemPrompt"`
	Tools        []*tools.ToolDef `json:"tools"`
	ProviderID   string           `json:"providerId"`
	Model        string           `json:"model"`
}

// resolveAgentProvider returns the provider an agent will use, honoring its
// explicit ProviderID / Model override; falls back to the active provider.
// Returns a copy so the caller may freely read Model without mutating config.
func (a *App) resolveAgentProvider(agent *agents.Agent) (*config.LLMProvider, error) {
	if agent.ProviderID != "" {
		for i := range a.cfg.LLM.Providers {
			p := &a.cfg.LLM.Providers[i]
			if p.ID == agent.ProviderID {
				if !p.Enabled {
					return nil, fmt.Errorf("agent 指定的供应商 %q 未启用", p.Name)
				}
				clone := *p
				if agent.Model != "" {
					clone.Model = agent.Model
				}
				return &clone, nil
			}
		}
		return nil, fmt.Errorf("agent 指定的供应商 %q 不存在", agent.ProviderID)
	}
	ap, err := a.resolveActiveProvider()
	if err != nil {
		return nil, err
	}
	if agent.Model != "" {
		clone := *ap
		clone.Model = agent.Model
		return &clone, nil
	}
	return ap, nil
}

// agentSelectedSkills returns the enabled skills an agent may use: all enabled
// skills when InheritSkills is set, otherwise the intersection of agent.Skills
// with the enabled set.
func (a *App) agentSelectedSkills(agent *agents.Agent) []skills.Skill {
	if a.skillManager == nil {
		return nil
	}
	enabled := a.skillManager.ListEnabled()
	if agent.InheritSkills {
		return enabled
	}
	wanted := map[string]bool{}
	for _, s := range agent.Skills {
		wanted[s] = true
	}
	var out []skills.Skill
	for _, s := range enabled {
		if wanted[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

// buildAgentSystemPrompt composes the agent persona prompt with the usage hints
// of its selected skills. Mirrors the composition in chatStore.ts so the
// default (inherit-all) agent reproduces the original chat prompt exactly.
func (a *App) buildAgentSystemPrompt(agent *agents.Agent) string {
	base := agent.SystemPrompt
	if strings.TrimSpace(base) == "" {
		base = agents.BaseSystemPrompt
	}
	var parts []string
	for _, s := range a.agentSelectedSkills(agent) {
		if strings.TrimSpace(s.SystemPrompt) != "" {
			parts = append(parts, fmt.Sprintf("【%s】%s", s.Title, s.SystemPrompt))
		}
	}
	if len(parts) == 0 {
		return base
	}
	return base + "\n\n当前启用的能力角色：\n" + strings.Join(parts, "\n")
}

// buildAgentToolNames returns the union of the agent's granted tool names:
// selected skills' tools + the agent's explicit Tools + MCPTools.
func (a *App) buildAgentToolNames(agent *agents.Agent) []string {
	seen := map[string]bool{}
	var out []string
	add := func(names ...string) {
		for _, n := range names {
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, s := range a.agentSelectedSkills(agent) {
		add(s.Tools...)
		add(s.MCPTools...)
	}
	add(agent.Tools...)
	add(agent.MCPTools...)
	return out
}

// isOrchestrationTool reports whether a tool re-enters the agent/workflow
// orchestration layer. These are stripped from sub-agent tool sets (runAgentLoop)
// to prevent unbounded recursion: an agent_run chain, or a workflow_execute call
// that spawns a fresh engine (which can itself run agent nodes), would recurse
// without a depth guard. Read-only workflow tools (list/get/status/validate) are
// left in — only the spawning/mutating ones are blocked.
func isOrchestrationTool(name string) bool {
	switch name {
	case "agent_list", "agent_create", "agent_run",
		"library_list", "agent_delegate_to_domain", "agent_delegate_multi_domain",
		"workflow_execute", "workflow_create", "workflow_update", "workflow_delete":
		return true
	}
	return false
}

// resolveAgentToolDefs builds the callable ToolDef list for an agent. External
// MCP tools are always included (matching main-chat behavior). When
// excludeOrchestration is true, agent_* tools are removed (used for sub-agents
// running via runAgentLoop).
func (a *App) resolveAgentToolDefs(agent *agents.Agent, excludeOrchestration bool) []*tools.ToolDef {
	allowed := map[string]bool{}
	for _, n := range a.buildAgentToolNames(agent) {
		allowed[n] = true
	}
	var out []*tools.ToolDef
	for _, t := range tools.List() {
		if excludeOrchestration && isOrchestrationTool(t.Name) {
			continue
		}
		if allowed[t.Name] || tools.IsExternal(t.Name) {
			out = append(out, t)
		}
	}
	return out
}

// marshalAgentToolDefs serializes ToolDefs into the OpenAI function-tool array.
func marshalAgentToolDefs(defs []*tools.ToolDef) json.RawMessage {
	type fnTool struct {
		Type     string `json:"type"`
		Function struct {
			Name        string           `json:"name"`
			Description string           `json:"description"`
			Parameters  *tools.ToolParams `json:"parameters"`
		} `json:"function"`
	}
	out := make([]fnTool, 0, len(defs))
	for _, d := range defs {
		t := fnTool{Type: "function"}
		t.Function.Name = d.Name
		t.Function.Description = d.Description
		t.Function.Parameters = d.Parameters
		out = append(out, t)
	}
	b, _ := json.Marshal(out)
	return b
}

// runAgentLoop runs an agent on a single user task with a bounded tool loop
// (max 5 rounds). It does NOT stream — used by the agent_run delegation tool.
// Orchestration tools are excluded so a sub-agent cannot recurse.
func (a *App) runAgentLoop(ctx context.Context, agent *agents.Agent, userText string) (string, error) {
	provider, err := a.resolveAgentProvider(agent)
	if err != nil {
		return "", err
	}
	systemPrompt := a.buildAgentSystemPrompt(agent)
	toolsJSON := marshalAgentToolDefs(a.resolveAgentToolDefs(agent, true))

	msgs := []map[string]any{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userText},
	}
	opts := chatOpts{Temperature: agent.Temperature, MaxTokens: agent.MaxTokens}

	const maxRounds = 5
	for round := 0; round < maxRounds; round++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		msgsJSON, _ := json.Marshal(msgs)
		data, err := a.chatCompletion(provider, msgsJSON, toolsJSON, opts)
		if err != nil {
			return "", err
		}
		choices, ok := data["choices"].([]any)
		if !ok || len(choices) == 0 {
			return "", fmt.Errorf("agent %q 无响应", agent.Name)
		}
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if message == nil {
			return "", fmt.Errorf("agent %q 响应格式无效", agent.Name)
		}

		toolCalls, _ := message["tool_calls"].([]any)
		if len(toolCalls) == 0 {
			content, _ := message["content"].(string)
			if strings.TrimSpace(content) == "" {
				content = "（" + agent.Name + " 无输出）"
			}
			return content, nil
		}

		// Feed the assistant turn (with tool_calls) back, then execute each call.
		msgs = append(msgs, message)
		for _, tc := range toolCalls {
			tcm, _ := tc.(map[string]any)
			id, _ := tcm["id"].(string)
			fn, _ := tcm["function"].(map[string]any)
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)
			var args map[string]any
			if argsStr != "" {
				_ = json.Unmarshal([]byte(argsStr), &args)
			}
			result := a.CallTool(name, args)
			resultJSON, _ := json.Marshal(result)
			msgs = append(msgs, map[string]any{
				"role":         "tool",
				"tool_call_id": id,
				"content":      string(resultJSON),
			})
		}
	}
	return "", fmt.Errorf("agent %q 达到最大工具调用轮次 (%d)", agent.Name, maxRounds)
}

// GetAgentChatContext resolves an agent into the inputs the frontend chatLoop
// needs (system prompt, tool defs, provider/model). Used when the user selects
// an agent persona in the chat panel. Orchestration tools are kept so the main
// chat can still create/delegate.
func (a *App) GetAgentChatContext(id string) (*AgentChatContext, error) {
	if a.agentManager == nil {
		return nil, fmt.Errorf("agent 管理器未初始化")
	}
	agent, err := a.agentManager.Get(id)
	if err != nil {
		return nil, err
	}
	provider, err := a.resolveAgentProvider(agent)
	if err != nil {
		return nil, err
	}
	return &AgentChatContext{
		AgentID:      agent.ID,
		Name:         agent.Name,
		SystemPrompt: a.buildAgentSystemPrompt(agent),
		Tools:        a.resolveAgentToolDefs(agent, false),
		ProviderID:   provider.ID,
		Model:        provider.Model,
	}, nil
}
