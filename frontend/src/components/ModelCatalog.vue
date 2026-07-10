<template>
  <div class="market">
    <div class="market-layout">
      <!-- 左侧：搜索 + 卡片网格 -->
      <div class="market-main" :class="{ dimmed: !!detail }" @click="onMainClick">
        <header class="market-head">
          <div class="head-left"><h2 class="title">模型市场</h2></div>
          <div class="search-box">
            <span class="search-icon">🔍</span>
            <input v-model="query" type="text" placeholder="搜索…" @keyup.enter="search" class="search-input" />
          </div>
        </header>
        <div class="filter-bar">
          <select v-model="filterSort" class="filter-select" @change="onFilterChange">
            <option value="downloads">下载量</option>
            <option value="likes">点赞数</option>
            <option value="lastModified">最近更新</option>
          </select>
          <select v-model="filterTask" class="filter-select" @change="onFilterChange" v-if="activeSource === 'huggingface'">
            <option value="">全部类型</option>
            <option value="text-generation">文本生成</option>
            <option value="text-classification">文本分类</option>
            <option value="image-classification">图像分类</option>
            <option value="text-to-image">图像生成</option>
            <option value="object-detection">目标检测</option>
            <option value="translation">翻译</option>
            <option value="summarization">摘要</option>
            <option value="question-answering">问答</option>
            <option value="fill-mask">填空</option>
            <option value="token-classification">词分类</option>
            <option value="automatic-speech-recognition">语音识别</option>
            <option value="sentence-similarity">句向量</option>
            <option value="feature-extraction">特征提取</option>
          </select>
          <select v-if="activeSource === 'huggingface'" v-model="filterLibrary" class="filter-select" @change="onFilterChange">
            <option value="">全部框架</option>
            <option value="transformers">Transformers</option>
            <option value="diffusers">Diffusers</option>
            <option value="ggml">GGML</option>
            <option value="peft">PEFT</option>
            <option value="pytorch">PyTorch</option>
            <option value="safetensors">SafeTensors</option>
            <option value="tensorflow">TensorFlow</option>
            <option value="onnx">ONNX</option>
          </select>
          <select v-if="activeSource === 'modelscope'" v-model="filterLanguage" class="filter-select" @change="onFilterChange">
            <option value="">全部语言</option>
            <option value="zh">中文</option>
            <option value="en">英文</option>
            <option value="multilingual">多语言</option>
          </select>
        </div>
        <div class="source-tabs">
          <button type="button" class="source-tab" :class="{ active: activeSource === 'huggingface' }" @click="switchSource('huggingface')">🤗 Hugging Face</button>
          <button type="button" class="source-tab" :class="{ active: activeSource === 'modelscope' }" @click="switchSource('modelscope')">🅼 ModelScope</button>
        </div>
        <section class="grid-section">
          <div class="grid-head">
            <h3 class="grid-title">{{ loading ? '加载中…' : (searched ? '搜索结果' : '热门模型') }}</h3>
            <button v-if="searched" type="button" class="btn btn-sm" @click="clearSearch">清除</button>
            <button v-else type="button" class="btn btn-sm" @click="loadFeatured" :disabled="loading">刷新</button>
          </div>
          <div class="card-grid-wrap">
            <div v-if="loading" class="grid-loading-mask">
              <span class="grid-spinner">⟳</span>
            </div>
            <div v-if="gridVisible" class="card-grid">
            <div v-for="m in (searched ? results : featured)" :key="m.id"
              class="glass-panel mcard"
              :class="{ active: detail && detail.repoId === parseId(m).repo }"
              @click="openDetail(m)">
              <div class="mcard-head"><span class="src-badge" :class="'src-' + m.source">{{ sourceLabel(m.source) }}</span><span class="mcard-dl">⬇ {{ fmtCount(m.downloads) }}</span></div>
              <div class="mcard-name">{{ parseId(m).repo }}</div>
              <div v-if="m.task" class="mcard-task-zh">{{ m.task }}</div>
              <div v-if="m.author" class="mcard-author">{{ m.author }}</div>
            </div>
          </div>
          </div>
          <div v-if="!loading && netError" class="net-error">
            <div class="net-error-icon">⚠</div>
            <div class="net-error-title">{{ netError }}</div>
            <div class="net-error-hint">请检查网络连接或代理设置后重试</div>
            <button type="button" class="btn btn-sm" @click="searched ? search() : loadFeatured()">重试</button>
          </div>
          <div v-else-if="!loading && (searched ? results : featured).length === 0" class="empty">未找到模型</div>
          <div v-if="!loading && !netError && (searched ? results : featured).length > 0 && (searched ? results : featured).length >= pageSize" class="load-more-wrap">
            <button class="btn btn-sm load-more-btn" @click="loadMore" :disabled="loadingMore">
              {{ loadingMore ? '加载中…' : '加载更多' }}
            </button>
          </div>
        </section>
      </div>

      <!-- 右侧：详情面板 -->
      <ModelDetailPanel
        :detail="detail"
        :detailLoading="detailLoading"
        :dpTab="dpTab"
        :dlState="dlStore.fileProgress"
        :checkedFiles="checkedFiles"
        :expandedDirs="expandedDirs"
        :downloadedSet="downloadedSet"
        :pkgDownloading="pkgDownloading"
        :checkedFileCount="checkedFileCount"
        :checkedSize="checkedSize"
        :pkgBtnText="pkgBtnText"
        @close="detail = null"
        @tab="t => dpTab = t"
        @reload="reloadDetail"
        @downloadChecked="downloadChecked"
        @downloadFile="doDownload"
        @toggleAll="toggleAll"
        @toggleDir="onToggleDir"
        @toggleCheck="onToggleCheck"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import ModelDetailPanel from './ModelDetailPanel.vue'
import { useDownloadStore } from '../stores/downloadStore'
import { useToast } from '../composables/useToast'
import { modelsApi } from '../api/models'
import { fmtSize, fmtCount } from '../utils/formatters'

// ── Toast ──
const toast = useToast()

// ── Download store (file-level progress for tree) ──
const dlStore = useDownloadStore()

// ── Cache & request-id ──
const _cacheTTL = 30 * 60 * 1000
const _cacheMax = 50
let _reqId = 0
const _detailCache = new Map<string, { d: any; t: number }>()

// ── Grid visibility + pagination ──
const gridVisible = ref(true)
const pageSize = 20
const moreOffset = ref(0)
const loadingMore = ref(false)

// ── Filter state ──
const filterTask = ref('')
const filterLibrary = ref('')
const filterLanguage = ref('')
const filterSort = ref('downloads')

// ── Data (refs) ──
const query = ref('')
const loading = ref(false)
const searched = ref(false)
const activeSource = ref('huggingface')
const featured = ref<any[]>([])
const results = ref<any[]>([])
const netError = ref('')
const detail = ref<any>(null)
const detailLoading = ref(false)
const dpTab = ref('info')
const dlID = ref<Record<string, any>>({})
const pkgDownloading = ref(false)
const pkgFiles = ref<Record<string, boolean>>({})
const pkgTotal = ref(0)
const pkgDone = ref(0)
const pkgFailed = ref(0)
const expandedDirs = ref<Record<string, boolean>>({})
const checkedFiles = ref<Record<string, boolean>>({})
const downloadedSet = ref<Record<string, boolean>>({})

// ── Utility functions (used in template) ──
function parseId(m: any): { source: string; repo: string } {
  const p = (m.id || '').split('|')
  return { source: m.source || activeSource.value, repo: p.length > 1 ? p[1] : p[0] }
}

function sourceLabel(s: string): string {
  return s === 'huggingface' ? 'HF' : s === 'modelscope' ? 'MS' : (s || '')
}

// ── Computed ──
const allFilePaths = computed<string[]>(() => {
  const paths: string[] = []
  const walk = (nodes: any[]) => {
    for (const node of nodes) {
      if (node.type === 'file') paths.push(node.path)
      if (node.children) walk(node.children)
    }
  }
  if (detail.value && detail.value.fileTree) walk(detail.value.fileTree)
  return paths
})

const checkedFileCount = computed(() => {
  let count = 0
  for (const p of allFilePaths.value) {
    if (checkedFiles.value[p] !== false) count++
  }
  return count
})

const checkedSize = computed(() => {
  let size = 0
  const walk = (nodes: any[]) => {
    for (const node of nodes) {
      if (node.type === 'file' && checkedFiles.value[node.path] !== false) {
        size += node.size > 0 ? node.size : 0
      }
      if (node.children) walk(node.children)
    }
  }
  if (detail.value && detail.value.fileTree) walk(detail.value.fileTree)
  return size
})

const totalFileCount = computed(() => {
  const entries = detail.value && detail.value.fileEntries
  if (!entries) return 0
  let count = 0
  for (const e of entries) { if (e.type === 'file' || e.type === '') count++ }
  return count || entries.length
})

const totalSize = computed(() => {
  const entries = detail.value && detail.value.fileEntries
  if (!entries) return 0
  return entries.reduce((s: number, e: any) => s + (e.size > 0 ? e.size : 0), 0)
})

const hasTree = computed(() => {
  return !!(detail.value && detail.value.fileTree && detail.value.fileTree.length)
})

const rootNode = computed(() => {
  if (!hasTree.value) return null
  return {
    name: detail.value.name || detail.value.repoId,
    path: '__root__',
    type: 'directory',
    children: detail.value.fileTree,
  }
})

const pkgBtnText = computed(() => {
  if (pkgDownloading.value) {
    const finished = pkgDone.value + pkgFailed.value
    return '下载中 ' + finished + '/' + pkgTotal.value + '…'
  }
  return '下载选中 (' + checkedFileCount.value + ' 个文件)'
})

// ── Internal helpers ──
function _checkPkgDone() {
  const finished = pkgDone.value + pkgFailed.value
  if (finished < pkgTotal.value) return
  pkgDownloading.value = false
  refreshDownloadedSet()
  if (pkgFailed.value === 0) {
    toast.show('success', '全部下载完成', pkgTotal.value + ' 个文件')
  } else {
    toast.show('error', '下载完成（含失败）',
      '成功 ' + pkgDone.value + ' / 失败 ' + pkgFailed.value + ' / 共 ' + pkgTotal.value)
  }
  pkgFiles.value = {}
}

function _collectFiles(dirNode: any): string[] {
  const files: string[] = []
  const walk = (nodes: any[]) => {
    for (const n of (nodes || [])) {
      if (n.type === 'file') files.push(n.path)
      else if (n.children) walk(n.children)
    }
  }
  walk(dirNode.children || [])
  return files
}

function _dirChecked(dirNode: any): boolean {
  const files = _collectFiles(dirNode)
  if (files.length === 0) return false
  return files.every(f => checkedFiles.value[f] !== false)
}

function _sanitizeDir(repoId: string): string {
  return String(repoId || '').replace(/[\\/]/g, '_')
}

function initState() {
  // 全部文件默认勾选
  const checked: Record<string, boolean> = {}
  for (const p of allFilePaths.value) checked[p] = true
  checkedFiles.value = checked
  // 所有目录（含根）默认展开
  const expanded: Record<string, boolean> = { '__root__': true }
  const walk = (nodes: any[]) => {
    for (const n of (nodes || [])) {
      if (n.type === 'directory') {
        expanded[n.path] = true
        if (n.children) walk(n.children)
      }
    }
  }
  if (detail.value && detail.value.fileTree) walk(detail.value.fileTree)
  expandedDirs.value = expanded
}

async function refreshDownloadedSet() {
  if (!detail.value) { downloadedSet.value = {}; return }
  const prefix = _sanitizeDir(detail.value.repoId) + '/'
  try {
    const list = await modelsApi.listDownloaded() || []
    const set: Record<string, boolean> = {}
    for (const f of list) {
      const p = String(f.name || '').replace(/\\/g, '/')
      if (p.startsWith(prefix)) {
        set[p.slice(prefix.length)] = true
      }
    }
    downloadedSet.value = set
  } catch (_) {
    downloadedSet.value = {}
  }
}

// ── Cache ──
function cacheGet(key: string): any {
  const e = _detailCache.get(key)
  if (!e) return null
  if (Date.now() - e.t > _cacheTTL) { _detailCache.delete(key); return null }
  _detailCache.delete(key)
  _detailCache.set(key, e)
  return e.d
}

function cacheSet(key: string, data: any) {
  if (_detailCache.size >= _cacheMax) {
    _detailCache.delete(_detailCache.keys().next().value)
  }
  _detailCache.set(key, { d: data, t: Date.now() })
}

// ── Tree operations ──
function onToggleDir(path: string) {
  const cur = expandedDirs.value[path] !== false
  expandedDirs.value = { ...expandedDirs.value, [path]: !cur }
}

function onToggleCheck(node: any) {
  const next = { ...checkedFiles.value }
  if (node.type === 'directory') {
    const files = _collectFiles(node)
    const newVal = !_dirChecked(node)
    for (const f of files) next[f] = newVal
  } else {
    next[node.path] = checkedFiles.value[node.path] === false
  }
  checkedFiles.value = next
}

function toggleAll(val: boolean) {
  const next: Record<string, boolean> = {}
  for (const p of allFilePaths.value) next[p] = val
  checkedFiles.value = next
}

// ── Detail panel ──
async function openDetail(m: any) {
  if (detail.value) { detail.value = null; return }
  dpTab.value = 'info'
  const { source, repo } = parseId(m)
  const cacheKey = source + '|' + repo
  const cached = cacheGet(cacheKey)
  if (cached) {
    detail.value = cached
    await nextTick()
    initState()
    refreshDownloadedSet()
    return
  }
  detailLoading.value = true
  detail.value = { source, repoId: repo, name: m.name || repo, files: [], fileEntries: [], fileTree: [], author: m.author, task: m.task, downloads: m.downloads, url: m.url }
  try {
    const d = await modelsApi.getDetail(source, repo)
    if (d) {
      const merged = Object.assign({}, d, {
        downloads: (d.downloads || 0) || (m.downloads || 0),
        author: d.author || m.author || '',
        task: d.task || m.task || '',
        name: (d.name && d.name !== repo) ? d.name : (m.name || repo),
        url: d.url || m.url || '',
      })
      cacheSet(cacheKey, merged)
      if (detail.value && detail.value.repoId === repo) {
        detail.value = merged
        await nextTick()
        initState()
        refreshDownloadedSet()
      }
    }
  } catch (_) {}
  detailLoading.value = false
}

async function reloadDetail() {
  if (!detail.value) return
  const { source, repoId } = detail.value
  _detailCache.delete(source + '|' + repoId)
  try { await modelsApi.invalidateCache('files:' + source + ':' + repoId) } catch (_) {}
  detailLoading.value = true
  try {
    const d = await modelsApi.getDetail(source, repoId)
    if (d) {
      detail.value = d
      await nextTick()
      initState()
      refreshDownloadedSet()
    }
  } catch (_) {}
  detailLoading.value = false
}

// ── Download ──
async function doDownload(file: string) {
  const { source, repoId } = detail.value

  // Check on-disk (source of truth) before starting
  try {
    const already = await modelsApi.isFileDownloaded(source, repoId, file)
    if (already) {
      downloadedSet.value = { ...downloadedSet.value, [file]: true }
      toast.show('info', '文件已下载', file)
      return
    }
  } catch (_) { /* proceed with download on check failure */ }

  dlStore.setFileDownloading(file)
  try {
    const id = await modelsApi.downloadFile(source, repoId, file)
    dlID.value[file] = id
  } catch (e: any) {
    const msg = e?.message || String(e)
    if (msg.includes('文件已下载')) {
      downloadedSet.value = { ...downloadedSet.value, [file]: true }
      toast.show('info', '文件已下载', file)
      return
    }
    dlStore.fileProgress = { ...dlStore.fileProgress, [file]: { active: false, pct: 0, status: 'error', written: 0, total: 0, speed: 0, filename: file, reason: msg } }
    toast.show('error', '下载失败', msg)
  }
}

async function downloadChecked() {
  const { source, repoId } = detail.value
  const selected = allFilePaths.value.filter(p => checkedFiles.value[p] !== false)
  if (selected.length === 0) return
  const pending = selected.filter(p => !downloadedSet.value[p])
  if (pending.length === 0) {
    toast.show('info', '所选文件已全部下载', '没有需要下载的资源')
    return
  }
  const skipped = selected.length - pending.length
  pkgFiles.value = {}
  const dlNext = { ...dlStore.fileProgress }
  for (const p of pending) { pkgFiles.value[p] = true; dlNext[p] = { active: true, pct: 0, status: 'downloading', written: 0, total: 0, speed: 0, filename: p, reason: '' } }
  dlStore.fileProgress = dlNext
  pkgTotal.value = pending.length
  pkgDone.value = 0
  pkgFailed.value = 0
  pkgDownloading.value = true
  try {
    await modelsApi.downloadSelected(source, repoId, pending)
    const note = skipped > 0 ? ('（已跳过 ' + skipped + ' 个已下载）') : ''
    toast.show('success', '下载已启动', repoId + ' (' + pending.length + ' 个文件)' + note)
  } catch (e: any) {
    pkgDownloading.value = false
    toast.show('error', '下载启动失败', e.message || String(e))
  }
}

function pauseDownload(file: string) {
  const id = dlID.value[file]
  if (id) {
    modelsApi.pauseDownload(id)
    if (dlStore.fileProgress[file]) dlStore.fileProgress[file].status = 'paused'
  }
}

function resumeDownload(file: string) {
  const id = dlID.value[file]
  if (id) {
    if (dlStore.fileProgress[file]) dlStore.fileProgress[file].status = 'downloading'
    ;modelsApi.resumeDownload(id)
  }
}

// ── Search ──
function switchSource(src: string) {
  if (activeSource.value === src) return
  activeSource.value = src
  searched.value = false
  query.value = ''
  detail.value = null
  featured.value = []
  results.value = []
  netError.value = ''
  loadFeatured()
}

function buildFilter(offset?: number) {
  return {
    task: filterTask.value || undefined,
    library: filterLibrary.value || undefined,
    language: filterLanguage.value || undefined,
    sort: filterSort.value || undefined,
    offset: offset ?? 0,
  }
}

function onFilterChange() {
  moreOffset.value = 0
  if (searched.value) search()
  else loadFeatured()
}

async function loadFeatured() {
  const myId = ++_reqId
  loading.value = true
  netError.value = ''
  gridVisible.value = false
  await nextTick()
  try {
    await modelsApi.invalidateCache('search:' + activeSource.value + ':')
    const r = await modelsApi.search(activeSource.value, '', buildFilter())
    if (myId !== _reqId) return
    if (r && r.error) { netError.value = r.error; featured.value = [] }
    else { featured.value = (r.models || []) }
    gridVisible.value = true
  } catch (e: any) {
    if (myId !== _reqId) return
    netError.value = '网络错误：' + (e.message || String(e))
    featured.value = []
    gridVisible.value = true
  }
  if (myId === _reqId) loading.value = false
}

async function search() {
  if (!query.value.trim()) { searched.value = false; return }
  const myId = ++_reqId
  loading.value = true
  searched.value = true
  detail.value = null
  netError.value = ''
  gridVisible.value = false
  await nextTick()
  try {
    const r = await modelsApi.search(activeSource.value, query.value.trim(), buildFilter())
    if (myId !== _reqId) return
    if (r && r.error) { netError.value = r.error; results.value = [] }
    else { results.value = (r.models || []) }
    gridVisible.value = true
  } catch (e: any) {
    if (myId !== _reqId) return
    netError.value = '网络错误：' + (e.message || String(e))
    results.value = []
    gridVisible.value = true
  }
  if (myId === _reqId) loading.value = false
}

async function loadMore() {
  loadingMore.value = true
  moreOffset.value += pageSize
  try {
    const r = await modelsApi.search(activeSource.value, '', buildFilter(moreOffset.value))
    if (r && !r.error) {
      featured.value = [...featured.value, ...(r.models || [])]
    }
  } catch (_) { moreOffset.value -= pageSize }
  loadingMore.value = false
}

function clearSearch() {
  query.value = ''
  searched.value = false
  results.value = []
  detail.value = null
}

function onMainClick(e: MouseEvent) {
  if (detail.value && !(e.target as HTMLElement).closest('.mcard')) { detail.value = null }
}

// ── Lifecycle ──

// Watch dlStore.fileProgress in real-time (replaces 500ms polling).
// Immediately picks up download completion/error for both individual
// files and package downloads — no delay, no missed events.
watch(
  () => dlStore.fileProgress,
  (fp) => {
    let changed = false
    for (const f of Object.keys(fp)) {
      const entry = fp[f]
      if (!entry) continue
      if (entry.status === 'completed' && !downloadedSet.value[f]) {
        downloadedSet.value = { ...downloadedSet.value, [f]: true }
        changed = true
        if (pkgFiles.value[f]) { pkgDone.value++; delete pkgFiles.value[f] }
      } else if (entry.status === 'error') {
        changed = true
        if (pkgFiles.value[f]) { pkgFailed.value++; delete pkgFiles.value[f] }
      }
    }
    if (changed) _checkPkgDone()
  },
  { deep: true }
)

onMounted(() => {
  loadFeatured()
})
</script>

<style scoped>
.market { width: 100%; }
.market-layout { display: flex; gap: 0; height: 100%; position: relative; }
.market-main { flex: 1; min-width: 0; transition: filter 0.3s ease; }
.market-main.dimmed { filter: blur(4px) brightness(0.55); }
.market-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 20px; gap: 20px; }
.title { font-size: 22px; font-weight: 600; letter-spacing: -0.01em; }
.search-box { position: relative; min-width: 240px; }
.search-icon { position: absolute; left: 10px; top: 50%; transform: translateY(-50%); font-size: 12px; opacity: 0.5; }
.search-input { width: 100%; padding-left: 28px; }
.filter-bar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.filter-select {
  padding: 6px 10px; border: 1px solid var(--border-soft); border-radius: 6px;
  background: var(--bg-elevated); color: var(--text-primary); font-size: 12px;
  cursor: pointer; min-width: 100px;
}
.filter-select:focus { border-color: var(--accent); outline: none; }
.source-tabs { display: flex; gap: 6px; margin-bottom: 16px; }
.source-tab {
  padding: 6px 16px; border: 1px solid var(--border-subtle); border-radius: 8px;
  background: var(--bg-elevated); color: var(--text-secondary);
  font-size: 12px; font-weight: 500; cursor: pointer; transition: all var(--transition);
}
.source-tab:hover { background: var(--bg-hover); color: var(--text-primary); }
.source-tab.active { background: var(--accent); border-color: var(--accent); color: #fff; }
.grid-section { display: flex; flex-direction: column; gap: 12px; }
.grid-head { display: flex; align-items: center; justify-content: space-between; }
.grid-title { font-size: 14px; font-weight: 600; }
.card-grid-wrap { position: relative; }
.grid-loading-mask {
  position: absolute; inset: 0; z-index: 10;
  background: rgba(0,0,0,0.3); backdrop-filter: blur(2px);
  border-radius: 8px;
  display: flex; align-items: flex-start; justify-content: center;
  padding-top: 40px;
}
.grid-spinner {
  font-size: 32px; color: var(--accent);
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
.mcard { padding: 14px; cursor: pointer; transition: all var(--transition); display: flex; flex-direction: column; gap: 8px; }
.mcard:hover { border-color: var(--accent); transform: translateY(-1px); }
.mcard.active { border-color: var(--accent); box-shadow: 0 0 0 2px var(--accent-dim); }
.mcard-head { display: flex; align-items: center; justify-content: space-between; }
.mcard-dl { font-size: 11px; color: var(--text-tertiary); }
.mcard-name { font-size: 13px; font-weight: 600; line-height: 1.3; word-break: break-all; }
.mcard-task-zh { font-size: 10px; padding: 2px 8px; border-radius: 4px; background: rgba(0,122,255,0.12); color: var(--accent); width: fit-content; }
.mcard-author { font-size: 10px; color: var(--text-tertiary); }

.detail-panel {
  position: fixed; right: 0; top: 0; bottom: 0;
  width: min(420px, 45vw); z-index: 100;
  background: var(--bg-sidebar);
  border-left: 1px solid var(--border-subtle);
  box-shadow: -8px 0 32px rgba(0,0,0,0.3);
  backdrop-filter: blur(20px);
  display: flex; flex-direction: column;
}
.detail-inner { padding: 24px; overflow-y: auto; scrollbar-gutter: stable; flex: 1; }
.dp-head { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 20px; }
.dp-title-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.dp-name { font-size: 16px; font-weight: 600; word-break: break-all; }
.dp-close {
  width: 28px; height: 28px; flex-shrink: 0;
  border: none; border-radius: 7px; background: var(--bg-hover); color: var(--text-tertiary);
  font-size: 14px; cursor: pointer; transition: all var(--transition);
}
.dp-close:hover { background: var(--danger-dim); color: var(--danger); }
.dp-tabs { display: flex; gap: 0; margin-bottom: 16px; border-bottom: 1px solid var(--border-subtle); }
.dp-tab {
  padding: 8px 16px; border: none; border-bottom: 2px solid transparent;
  background: transparent; color: var(--text-tertiary);
  font-size: 12px; font-weight: 500; cursor: pointer; transition: all var(--transition);
}
.dp-tab:hover { color: var(--text-primary); }
.dp-tab.active { color: var(--accent); border-bottom-color: var(--accent); }
.dp-tab-content { flex: 1; overflow-y: auto; scrollbar-gutter: stable; }
.dp-meta { display: flex; flex-direction: column; gap: 8px; margin-bottom: 20px; padding-bottom: 16px; border-bottom: 1px solid var(--border-subtle); }
.meta-item { display: flex; gap: 10px; font-size: 12px; }
.meta-k { color: var(--text-tertiary); min-width: 40px; }
.meta-v { color: var(--text-primary); font-weight: 450; }
.dp-link { font-size: 11px; color: var(--accent); text-decoration: none; margin-top: 4px; }
.dp-link:hover { text-decoration: underline; }
.dp-desc { margin-bottom: 20px; }
.dp-section-title { font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.04em; }
.dp-desc-text { font-size: 12px; color: var(--text-tertiary); line-height: 1.6; word-break: break-word; }
.dp-desc-empty { font-style: italic; }
.dp-files { display: flex; flex-direction: column; gap: 8px; }
.dp-loading, .dp-empty { font-size: 12px; color: var(--text-tertiary); padding: 8px 0; }

.files-head {
  display: flex; align-items: center; gap: 8px;
  padding: 4px 0 8px; flex-wrap: wrap;
}
.files-total-size { font-size: 11px; color: var(--text-tertiary); font-family: var(--font-mono); margin-left: auto; }
.btn-dl-all { padding: 7px 14px; font-size: 12px; white-space: nowrap; }

.dp-file-tree {
  display: flex; flex-direction: column;
  border: 1px solid var(--border-subtle); border-radius: 8px;
  overflow: hidden;
}

.btn:hover { border-color: var(--accent-dim); }
.btn-primary:hover { opacity: 0.85; }
.empty { font-size: 13px; color: var(--text-tertiary); padding: 24px; text-align: center; }
.load-more-wrap { display: flex; justify-content: center; padding: 16px 0; }
.load-more-btn { padding: 6px 24px !important; }

/* ── Network error ── */
.net-error, .dp-error {
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  padding: 40px 24px; text-align: center;
}
.net-error-icon { font-size: 28px; color: var(--warning); }
.net-error-title { font-size: 13px; color: var(--text-primary); font-weight: 550; word-break: break-word; }
.net-error-hint { font-size: 11px; color: var(--text-tertiary); }

.slide-enter-active, .slide-leave-active { transition: transform 0.3s cubic-bezier(0.22,0.61,0.36,1), opacity 0.3s; }
.slide-enter-from, .slide-leave-to { transform: translateX(100%); opacity: 0; }
</style>
