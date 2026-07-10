<template>
  <div class="toolbox">
    <SimilarityTool v-if="active && active.type === 'sentence-embedding'" :model="active" @back="active = null" />
    <ImageClassifierTool v-else-if="active && active.type === 'image-classification'" :model="active" @back="active = null" />
    <template v-else>
      <div class="toolbar"><h2 class="title">工具箱</h2><div class="toolbar-actions"><button type="button" class="btn" @click="refresh" :disabled="loading">{{ loading ? '刷新中…' : '刷新' }}</button></div></div>
      <div v-if="loading" class="hint">加载中…</div>
      <div v-else-if="!models.length" class="empty"><div class="empty-icon">🧰</div><p class="empty-title">暂无可用的工具模型</p><p class="empty-hint">先到「模型市场」下载一个句向量或图像分类模型，再回到这里使用。</p></div>
      <div v-else class="card-grid-wider">
        <div v-for="m in models" :key="m.repoId" class="glass-panel tcard" @click="open(m)">
          <span class="tag" :class="tagClass(m.type)">{{ typeLabel(m.type) }}</span>
          <div class="tcard-name">{{ m.name }}</div><div class="tcard-hint">{{ typeHint(m.type) }}</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import SimilarityTool from './SimilarityTool.vue'
import ImageClassifierTool from './ImageClassifierTool.vue'
import { modelsApi } from '../api/models'

interface ToolModel { repoId: string; name: string; dir: string; type: string }

const models = ref<ToolModel[]>([])
const loading = ref(false)
const active = ref<(ToolModel & { id: string }) | null>(null)
let _opening = false

async function refresh() {
  loading.value = true
  try { models.value = (await modelsApi.listToolModels()) || [] }
  catch (e: any) { /* toast via parent */ }
  loading.value = false
}
async function open(m: ToolModel) {
  if (active.value || _opening) return
  _opening = true
  // id is deterministic; valid whether freshly loaded or already loaded (the
  // catch path), so the tool can always open with it. Required by
  // ImageClassifierTool's `model: { id; name }` contract.
  const id = 'tool-' + m.repoId
  try {
    await modelsApi.loadModelFile(id, m.name, m.dir)
  } catch (_) { /* already loaded — id remains valid */ }
  _opening = false
  if (m.type === 'sentence-embedding' || m.type === 'image-classification') {
    active.value = { ...m, id }
  }
}
function typeLabel(t: string) {
  const map: Record<string, string> = { 'sentence-embedding': '句向量', 'image-classification': '图像分类' }
  return map[t] || t
}
function tagClass(t: string) {
  const map: Record<string, string> = { 'sentence-embedding': 'tag-accent', 'image-classification': 'tag-ok' }
  return map[t] || 'tag-muted'
}
function typeHint(t: string) {
  const map: Record<string, string> = { 'sentence-embedding': '相似度 / 语义搜索', 'image-classification': '选图 → 类别' }
  return map[t] || ''
}

// self-mount refresh
refresh()
</script>

<style scoped>
.toolbox { width: 100%; }
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.title { font-size: 22px; font-weight: 600; }
.toolbar-actions { display: flex; gap: 8px; }
</style>
