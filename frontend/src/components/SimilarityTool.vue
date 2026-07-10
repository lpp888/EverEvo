<template>
  <div class="sim">
    <div class="sim-head">
      <button type="button" class="btn btn-sm" @click="emit('back')">← 返回</button>
      <span class="sim-model">{{ model.name }}</span>
      <div class="mode-tabs">
        <button type="button" :class="{ active: mode === 'sim' }" @click="mode = 'sim'">相似度</button>
        <button type="button" :class="{ active: mode === 'search' }" @click="mode = 'search'">语义搜索</button>
      </div>
    </div>
    <div v-if="mode === 'sim'" class="sim-body">
      <div class="pair">
        <textarea v-model="a" class="sim-area" placeholder="文本 A：猫在睡觉"></textarea>
        <textarea v-model="b" class="sim-area" placeholder="文本 B：小猫睡着了"></textarea>
      </div>
      <button type="button" class="btn btn-primary" @click="runSim" :disabled="busy">{{ busy ? '计算中…' : '计算相似度' }}</button>
      <div v-if="simResult !== null" class="sim-score">余弦相似度 <b>{{ simResult.toFixed(4) }}</b><span class="sim-judge">{{ judge(simResult) }}</span></div>
    </div>
    <div v-else class="sim-body">
      <textarea v-model="corpus" class="sim-area sim-corpus" placeholder="句库（每行一句）&#10;例如：&#10;今天的天气真好&#10;这部电影真好看&#10;我想去旅行"></textarea>
      <div class="search-row">
        <input v-model="query" type="text" class="search-input" placeholder="查询：天气怎么样" @keyup.enter="runSearch" />
        <button type="button" class="btn btn-primary" @click="runSearch" :disabled="busy">{{ busy ? '搜索中…' : '搜索' }}</button>
      </div>
      <div v-if="hits.length" class="hits">
        <div v-for="(h, i) in hits" :key="h.index" class="hit"><span class="hit-rank">{{ i + 1 }}</span><span class="hit-score">{{ (h.score * 100).toFixed(1) }}%</span><span class="hit-text">{{ corpusLines[h.index] }}</span></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { knowledgeApi } from '../api/knowledge'

const props = defineProps<{ model: { name: string; dir: string } }>()
const emit = defineEmits<{ back: [] }>()

const mode = ref('sim')
const a = ref(''); const b = ref('')
const corpus = ref(''); const query = ref('')
const busy = ref(false)
const simResult = ref<number | null>(null)
const hits = ref<{ index: number; score: number }[]>([])
const corpusLines = ref<string[]>([])

function cosine(a: number[], b: number[]): number {
  if (!a || !b || a.length !== b.length) return 0
  let dot = 0, na = 0, nb = 0
  for (let i = 0; i < a.length; i++) { dot += a[i] * b[i]; na += a[i] * a[i]; nb += b[i] * b[i] }
  return (na === 0 || nb === 0) ? 0 : dot / (Math.sqrt(na) * Math.sqrt(nb))
}
function judge(s: number): string {
  if (s >= 0.75) return '高度相似'; if (s >= 0.5) return '相关'; if (s >= 0.25) return '弱相关'; return '基本无关'
}
async function runSim() {
  if (!a.value.trim() || !b.value.trim()) return
  busy.value = true; simResult.value = null
  try {
    const embs = await knowledgeApi.embedTexts(props.model.dir, [a.value, b.value])
    simResult.value = cosine(embs[0], embs[1])
  } catch (e: any) { /* toast handled by parent */ }
  busy.value = false
}
async function runSearch() {
  corpusLines.value = corpus.value.split('\n').map(s => s.trim()).filter(Boolean)
  if (!query.value.trim() || !corpusLines.value.length) return
  busy.value = true; hits.value = []
  try {
    const all = [query.value, ...corpusLines.value]
    const embs = await knowledgeApi.embedTexts('', all)
    const qEmb = embs[0]; const scored: { index: number; score: number }[] = []
    for (let i = 0; i < corpusLines.value.length; i++) scored.push({ index: i, score: cosine(qEmb, embs[i + 1]) })
    scored.sort((a, b) => b.score - a.score)
    hits.value = scored.slice(0, 10)
  } catch (e: any) { /* */ }
  busy.value = false
}
</script>

<style scoped>
.sim { display: flex; flex-direction: column; gap: 16px; width: 100%; }
.sim-head { display: flex; align-items: center; gap: 12px; }
.sim-model { font-size: 14px; font-weight: 600; color: var(--text-primary); }
.mode-tabs { display: flex; gap: 4px; margin-left: auto; }
.mode-tabs button { padding: 4px 12px; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); background: var(--bg-elevated); color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.mode-tabs button.active { background: var(--accent); border-color: var(--accent); color: #fff; }
.sim-body { display: flex; flex-direction: column; gap: 12px; }
.pair { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.sim-area { padding: 12px; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); background: var(--bg-inset); color: var(--text-primary); font-size: 13px; resize: vertical; min-height: 80px; font-family: var(--font); }
.sim-corpus { min-height: 140px; }
.sim-score { padding: 12px; border-radius: var(--radius-sm); background: var(--bg-elevated); font-size: 14px; color: var(--text-primary); }
.sim-judge { margin-left: 10px; font-size: 12px; color: var(--accent); font-weight: 500; }
.search-row { display: flex; gap: 10px; }
.search-row .search-input { flex: 1; }
.search-input { padding: 8px 12px; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); background: var(--bg-elevated); color: var(--text-primary); font-size: 13px; }
.hits { display: flex; flex-direction: column; gap: 6px; }
.hit { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-radius: 6px; background: var(--bg-elevated); }
.hit-rank { font-size: 11px; font-weight: 600; color: var(--text-tertiary); width: 20px; }
.hit-score { font-size: 11px; font-weight: 600; color: var(--accent); min-width: 44px; }
.hit-text { font-size: 13px; color: var(--text-primary); }
</style>
