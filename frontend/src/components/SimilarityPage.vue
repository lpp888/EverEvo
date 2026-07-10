<template>
  <div class="sp">
    <SimilarityTool v-if="active" :model="active" @back="active = null" />
    <template v-else>
      <header class="page-head"><h2 class="title">语义相似度</h2><p class="subtitle">基于句向量模型的语义相似度计算与语义搜索</p></header>
      <div v-if="loading" class="hint">加载中…</div>
      <div v-else-if="!models.length" class="empty"><div class="empty-icon">≈</div><p class="empty-title">暂无可用的句向量模型</p><p class="empty-hint">到「模型市场」下载一个 sentence-embedding 模型即可</p></div>
      <div v-else class="card-grid-wider">
        <div v-for="m in models" :key="m.repoId" class="glass-panel tcard" @click="open(m)">
          <span class="tag tag-accent">句向量</span><div class="tcard-name">{{ m.name }}</div><div class="tcard-hint">相似度 / 语义搜索</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import SimilarityTool from './SimilarityTool.vue'
import { modelsApi } from '../api/models'

interface ToolModel { repoId: string; name: string; dir: string; type: string }

const models = ref<ToolModel[]>([])
const loading = ref(false)
const active = ref<(ToolModel & { id?: string }) | null>(null)
let _opening = false

async function refresh() {
  loading.value = true
  try { const all = (await modelsApi.listToolModels()) || []; models.value = all.filter((m: any) => m.type === 'sentence-embedding') } catch (_) {}
  loading.value = false
}
async function open(m: ToolModel) {
  if (active.value || _opening) return
  _opening = true
  try { const id = 'tool-' + m.repoId; await modelsApi.loadModelFile(id, m.name, m.dir); (m as any).id = id } catch (_) {}
  _opening = false
  active.value = m as any
}
refresh()
</script>

<style scoped>
.sp { width: 100%; }
.page-head { display: flex; flex-direction: column; gap: 4px; margin-bottom: 20px; }
.title { font-size: 22px; font-weight: 600; }
.subtitle { font-size: 12px; color: var(--text-tertiary); }
</style>
