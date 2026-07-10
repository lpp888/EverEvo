<template>
  <div class="ict">
    <div class="ict-head">
      <button class="btn btn-sm" @click="emit('back')">← 返回</button>
      <span class="ict-name">{{ model.name }}</span>
      <span class="tag tag-ok">图像分类</span>
    </div>
    <div class="ict-body">
      <div class="ict-drop" @click="pickImage" @dragover.prevent @drop.prevent="onDrop">
        <input ref="fileInput" type="file" accept="image/*" class="ict-file-input" @change="onFile" />
        <div v-if="!imageSrc" class="ict-placeholder"><span class="ict-placeholder-icon">🖼</span><span class="ict-placeholder-text">点击选择图片或拖拽到此处</span></div>
        <img v-else :src="imageSrc" class="ict-preview" alt="preview" />
      </div>
      <div class="ict-actions">
        <button v-if="imageSrc" class="btn btn-sm" @click="clearImage">清除</button>
        <button class="btn btn-primary" @click="runClassify" :disabled="busy || !imageBase64">{{ busy ? '分析中…' : '运行分类' }}</button>
      </div>
      <div v-if="error" class="ict-error">{{ error }}</div>
      <div v-if="results && results.length" class="ict-results">
        <div class="ict-results-title">分类结果</div>
        <div v-for="(r, i) in results" :key="i" class="ict-result-row">
          <span class="ict-rank">#{{ i + 1 }}</span><span class="ict-label">{{ r.label }}</span>
          <div class="ict-bar-wrap"><div class="ict-bar" :style="{ width: (r.score * 100).toFixed(1) + '%' }"></div></div>
          <span class="ict-score">{{ (r.score * 100).toFixed(2) }}%</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { modelsApi } from '../api/models'

const props = defineProps<{ model: { id: string; name: string } }>()
const emit = defineEmits<{ back: [] }>()

const fileInput = ref<HTMLInputElement | null>(null)
const imageSrc = ref<string | null>(null)
const imageBase64 = ref<string | null>(null)
const busy = ref(false)
const results = ref<any[] | null>(null)
const error = ref<string | null>(null)

function pickImage() { fileInput.value?.click() }
function onFile(e: Event) { const f = (e.target as HTMLInputElement).files?.[0]; if (f) loadImage(f) }
function onDrop(e: DragEvent) { const f = e.dataTransfer?.files[0]; if (f) loadImage(f) }
function loadImage(file: File) {
  results.value = null; error.value = null
  const reader = new FileReader()
  reader.onload = () => {
    imageSrc.value = reader.result as string
    const comma = (reader.result as string).indexOf(',')
    imageBase64.value = comma >= 0 ? (reader.result as string).slice(comma + 1) : (reader.result as string)
  }
  reader.readAsDataURL(file)
}
function clearImage() { imageSrc.value = null; imageBase64.value = null; results.value = null; error.value = null }
async function runClassify() {
  if (!imageBase64.value) return
  busy.value = true; results.value = null; error.value = null
  try {
    const output = await modelsApi.runModel(props.model.id, imageBase64.value)
    try { results.value = JSON.parse(output) } catch (_) { results.value = [{ label: '模型输出', score: 1, raw: output }] }
  } catch (e: any) { error.value = typeof e === 'string' ? e : (e?.message || String(e)) }
  busy.value = false
}
</script>

<style scoped>
.ict { display: flex; flex-direction: column; gap: 18px; }
.ict-head { display: flex; align-items: center; gap: 10px; }
.ict-name { font-size: 16px; font-weight: 600; }
.ict-body { display: flex; flex-direction: column; gap: 14px; }
.ict-drop { position: relative; border: 2px dashed var(--border-soft); border-radius: var(--radius); min-height: clamp(120px, 25vh, 220px); display: flex; align-items: center; justify-content: center; cursor: pointer; transition: border-color var(--transition); overflow: hidden; }
.ict-drop:hover { border-color: var(--accent); }
.ict-file-input { display: none; }
.ict-placeholder { text-align: center; display: flex; flex-direction: column; align-items: center; gap: 6px; }
.ict-placeholder-icon { font-size: 36px; opacity: 0.3; }
.ict-placeholder-text { font-size: 13px; color: var(--text-tertiary); }
.ict-preview { max-width: 100%; max-height: clamp(180px, 40vh, 400px); object-fit: contain; }
.ict-actions { display: flex; gap: 8px; }
.ict-error { padding: 8px 12px; font-size: 12px; color: var(--danger); background: var(--danger-dim); border-radius: var(--radius-sm); }
.ict-results { display: flex; flex-direction: column; gap: 8px; }
.ict-results-title { font-size: 13px; font-weight: 600; margin-bottom: 2px; }
.ict-result-row { display: flex; align-items: center; gap: 10px; padding: 8px 12px; background: var(--bg-inset); border-radius: var(--radius-sm); }
.ict-rank { font-size: 12px; font-weight: 600; color: var(--accent); font-family: var(--font-mono); min-width: 22px; }
.ict-label { font-size: 13px; flex: 1; min-width: 0; font-weight: 500; }
.ict-bar-wrap { flex: 2; height: 6px; background: var(--bg-elevated); border-radius: 3px; overflow: hidden; }
.ict-bar { height: 100%; background: var(--accent); border-radius: 3px; transition: width 0.4s; }
.ict-score { font-size: 11px; color: var(--text-secondary); font-family: var(--font-mono); min-width: 54px; text-align: right; }
.tag { display: inline-block; padding: 1px 7px; border-radius: 4px; font-size: 11px; font-weight: 500; }
.tag-ok { background: var(--ok-dim, #e6f7ed); color: var(--ok, #1a7f4b); }
.btn { padding: 6px 14px; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); background: var(--bg-elevated); color: var(--text-primary); font-size: 12px; cursor: pointer; }
.btn:hover { background: var(--bg-hover); }
.btn-sm { padding: 3px 10px !important; font-size: 11px; }
.btn-primary { background: var(--accent); border-color: var(--accent); color: #fff; }
.btn-primary:hover { background: var(--accent-hover); }
.btn:disabled { opacity: 0.4; cursor: default; }
</style>
