/**
 * Model Profile Configuration — declarative per-model settings.
 *
 * Replaces heuristics ("if DeepSeek V4 → 1M") with explicit profiles.
 * Edit the MODEL_PRESETS map below to add or tune models.
 *
 * Design:
 *   - Each model is keyed by a slug (case-insensitive substring match against
 *     provider name + model name).
 *   - The first matching preset wins; fallback_profile is used when nothing matches.
 *   - All values are user-editable — no magic numbers buried in code.
 *   - To override: edit MODEL_PRESETS, or wire a settings UI later.
 */

// ── Types ──

export interface ModelProfile {
  /** Human-readable label (shown in settings). */
  label: string
  /** Model context window in tokens. */
  contextWindow: number
  /** Maximum output tokens the API accepts.
   *  Set to 0 to omit max_tokens entirely (let the model/server decide —
   *  this is what Codex does). Set to a positive value only for APIs that
   *  REQUIRE it (Anthropic). */
  maxOutputTokens: number
  /** Percentage of context window usable for input (system + history + recall).
   *  Codex uses 95%; the remaining 5% is reserved for overhead + output. */
  effectivePct: number
  /** Percentage of context window at which auto-compaction triggers.
   *  Codex uses 90% (auto_compact_token_limit = context_window * 0.9). */
  compactPct: number
  /** Whether this provider supports a separate thinking/reasoning token budget
   *  (Anthropic: budget_tokens). When false, thinking counts against max_tokens. */
  supportsThinkingBudget: boolean
}

// ── Built-in Presets ──

/** Source: official docs, API responses, and Codex models.json reference. */
export const MODEL_PRESETS: Record<string, ModelProfile> = {
  // ── DeepSeek ──
  // Current models (2026-07): deepseek-v4-flash, deepseek-v4-pro.
  // Both: 1M context. max_tokens is optional for DeepSeek — omit it (0).
  'deepseek-v4-pro': {
    label: 'DeepSeek V4 Pro',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,          // optional: let API default, model stops at finish_reason="stop"
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'deepseek-v4-flash': {
    label: 'DeepSeek V4 Flash',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'deepseek-chat': {
    label: 'DeepSeek V4 (legacy: deepseek-chat)',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'deepseek-reasoner': {
    label: 'DeepSeek V4 (legacy: deepseek-reasoner)',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'deepseek': {
    label: 'DeepSeek (latest / unknown model)',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },

  // ── Anthropic Claude ──
  'claude opus': {
    label: 'Claude Opus 4+',
    contextWindow: 200_000,
    maxOutputTokens: 128_000,    // Opus max output
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true, // budget_tokens separate
  },
  'claude sonnet': {
    label: 'Claude Sonnet 4+',
    contextWindow: 200_000,
    maxOutputTokens: 64_000,     // Sonnet max output
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },
  'claude haiku': {
    label: 'Claude Haiku 4+',
    contextWindow: 200_000,
    maxOutputTokens: 64_000,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },
  'claude': {
    label: 'Claude (generic)',
    contextWindow: 200_000,
    maxOutputTokens: 64_000,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },

  // ── OpenAI ──
  'gpt-5': {
    label: 'GPT-5 / GPT-5.x',
    contextWindow: 272_000,
    maxOutputTokens: 0,          // optional for Chat Completions
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'gpt-4': {
    label: 'GPT-4o / GPT-4 Turbo',
    contextWindow: 128_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },
  'o4': {
    label: 'OpenAI o4 / o3',
    contextWindow: 200_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: false,
  },

  // ── Google Gemini ──
  'gemini 2.5': {
    label: 'Gemini 2.5+',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,          // optional
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },
  'gemini 3': {
    label: 'Gemini 3+',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },
  'gemini': {
    label: 'Gemini (generic)',
    contextWindow: 1_000_000,
    maxOutputTokens: 0,
    effectivePct: 95,
    compactPct: 90,
    supportsThinkingBudget: true,
  },
}

// ── Fallback for unknown models ──

export const FALLBACK_PROFILE: ModelProfile = {
  label: 'Unknown Model (conservative fallback)',
  contextWindow: 128_000,
  maxOutputTokens: 128_000,
  effectivePct: 80,    // conservative for unknown models
  compactPct: 80,      // compact earlier to be safe
  supportsThinkingBudget: false,
}

// ── Lookup ──

/**
 * Find the best-matching ModelProfile for a provider+model string.
 * Matches case-insensitively against keys of MODEL_PRESETS.
 * Returns FALLBACK_PROFILE when no key matches.
 */
export function getModelProfile(providerName?: string, modelName?: string): ModelProfile {
  const haystack = `${providerName || ''} ${modelName || ''}`.toLowerCase()
  for (const [key, profile] of Object.entries(MODEL_PRESETS)) {
    if (haystack.includes(key)) return profile
  }
  return FALLBACK_PROFILE
}

/**
 * Return all registered presets (for settings UI).
 */
export function listProfiles(): Array<{ key: string } & ModelProfile> {
  return Object.entries(MODEL_PRESETS).map(([key, profile]) => ({ key, ...profile }))
}
