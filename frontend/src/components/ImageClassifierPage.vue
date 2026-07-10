<template>
  <div class="icp">
    <ImageClassifierTool v-if="active" :model="active" @back="active = null" />
    <template v-else>
      <header class="page-head"><h2 class="title">图像分类</h2><p class="subtitle">基于图像分类模型的图片识别与标签预测</p></header>
      <div v-if="loading" class="hint">加载中…</div>
      <div v-else-if="!models.length" class="empty"><div class="empty-icon">◫</div><p class="empty-title">暂无可用的图像分类模型</p><p class="empty-hint">到「模型市场」下载一个 image-classification 模型即可</p></div>
      <div v-else class="card-grid-wider">
        <div v-for="m in models" :key="m.repoId" class="glass-panel tcard" @click="open(m)">
          <span class="tag tag-ok">图像分类</span><div class="tcard-name">{{ m.name }}</div><div class="tcard-hint">选图 → 类别预测</div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import ImageClassifierTool from './ImageClassifierTool.vue'
import { modelsApi } from '../api/models'

interface ToolModel { repoId: string; name: string; dir: string; type: string }

const models = ref<ToolModel[]>([])
const loading = ref(false)
const active = ref<(ToolModel & { id: string }) | null>(null)
let _opening = false

async function refresh() {
  loading.value = true
  try { const all = (await modelsApi.listToolModels()) || []; models.value = all.filter((m: any) => m.type === 'image-classification') } catch (_) {}
  loading.value = false
}
async function open(m: ToolModel) {
  if (active.value || _opening) return
  _opening = true
  try {
    const id = 'tool-' + m.repoId
    await modelsApi.loadModelFile(id, m.name, m.dir)
    active.value = { ...m, id }
  } catch (_) {}
  _opening = false
}
refresh()
</script>

<style scoped>
.icp { width: 100%; }
.page-head { display: flex; flex-direction: column; gap: 4px; margin-bottom: 20px; }
.title { font-size: 22px; font-weight: 600; }
.subtitle { font-size: 12px; color: var(--text-tertiary); }
</style>
