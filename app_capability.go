//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"everevo/internal/config"
)

// ─── 本地模型能力检测 ─────────────────────────────────────────────

// FetchOllamaModels 从 Ollama 服务拉取已安装的模型列表。
func (a *App) FetchOllamaModels(endpoint string) ([]map[string]any, error) {
	base := strings.TrimRight(endpoint, "/")
	// endpoint is like http://127.0.0.1:11434/v1 — strip /v1 to get base
	base = strings.TrimSuffix(base, "/v1")
	url := base + "/api/tags"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Ollama (%s): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama 返回 %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			ModifiedAt string `json:"modified_at"`
			Size       int64  `json:"size"`
			Digest     string `json:"digest"`
			Details    struct {
				Format            string `json:"format"`
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Ollama 响应失败: %w", err)
	}

	var models []map[string]any
	for _, m := range result.Models {
		info := map[string]any{
			"name":       m.Name,
			"size":       m.Size,
			"modifiedAt": m.ModifiedAt,
			"family":     m.Details.Family,
			"params":     m.Details.ParameterSize,
			"quant":      m.Details.QuantizationLevel,
		}
		models = append(models, info)
	}
	return models, nil
}

// FetchOpenAIModels fetches available models from an OpenAI-compatible /v1/models endpoint.
// Used for llama.cpp and other local servers. Also probes real capabilities for each model.
func (a *App) FetchOpenAIModels(endpoint string, apiKey string) ([]map[string]any, error) {
	base := strings.TrimRight(endpoint, "/")
	url := base + "/models"

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接 (%s): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("返回 %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	var models []map[string]any
	for _, m := range result.Data {
		info := map[string]any{
			"name":    m.ID,
			"ownedBy": m.OwnedBy,
		}
		models = append(models, info)
	}
	return models, nil
}

// DetectModelCapability probes the model API to detect real capabilities.
func (a *App) DetectModelCapability(providerID string, modelName string) config.ModelCapability {
	if providerID != "" {
		for i := range a.cfg.LLM.Providers {
			if a.cfg.LLM.Providers[i].ID == providerID {
				prov := &a.cfg.LLM.Providers[i]
				endpoint := strings.TrimRight(prov.Endpoint, "/")
				cap := config.ProbeModelCapability(endpoint, prov.APIKey, modelName, prov.APIFormat)
				if cap != nil {
					log.Printf("[cap] %s via API: vision=%v tools=%v stream=%v reason=%v ctx=%d",
						modelName, cap.SupportsVision, cap.SupportsTools, cap.SupportsStreaming, cap.SupportsReasoning, cap.MaxContextTokens)
					return *cap
				}
				break
			}
		}
	}
	return config.UnknownCapability()
}

// ProbeModelCap always performs a real API probe regardless of cached data.
// Takes raw endpoint/apiKey/model/apiFormat — works without a saved provider.
func (a *App) ProbeModelCap(endpoint, apiKey, model, apiFormat string) config.ModelCapability {
	endpoint = strings.TrimRight(endpoint, "/")
	cap := config.ProbeModelCapability(endpoint, apiKey, model, apiFormat)
	if cap != nil {
		log.Printf("[cap] %s probed: vision=%v tools=%v stream=%v reason=%v ctx=%d",
			model, cap.SupportsVision, cap.SupportsTools, cap.SupportsStreaming, cap.SupportsReasoning, cap.MaxContextTokens)
		return *cap
	}
	return config.UnknownCapability()
}

// ProbeAllModels probes every model in the provider's model list via real API calls.
func (a *App) ProbeAllModels(providerID string) map[string]config.ModelCapability {
	result := make(map[string]config.ModelCapability)
	if providerID == "" {
		return result
	}
	var prov *config.LLMProvider
	for i := range a.cfg.LLM.Providers {
		if a.cfg.LLM.Providers[i].ID == providerID {
			prov = &a.cfg.LLM.Providers[i]
			break
		}
	}
	if prov == nil {
		return result
	}
	endpoint := strings.TrimRight(prov.Endpoint, "/")
	for _, m := range prov.Models {
		cap := config.ProbeModelCapability(endpoint, prov.APIKey, m, prov.APIFormat)
		if cap != nil {
			result[m] = *cap
		} else {
			result[m] = config.UnknownCapability()
		}
	}
	return result
}
