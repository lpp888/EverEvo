<template>
  <div class="ftn-wrap">
    <div class="ftn-row"
      :style="rowStyle"
      :class="{ 'ftn-dir': node.type === 'directory', 'ftn-root-row': isRoot, 'ftn-got': node.type === 'file' && isDownloaded }"
      @click="onRowClick">
      <button type="button" class="ftn-arrow" v-if="node.type === 'directory'"
        :class="{ expanded: expanded }" @click.stop="onToggleDir"
        :title="expanded ? '收起' : '展开'">▸</button>
      <span class="ftn-arrow-spacer" v-else></span>
      <button type="button" class="ftn-check" :class="'is-' + checkState.state"
        :title="checkTitle" @click.stop.prevent="onToggleCheck">
        <svg viewBox="0 0 16 16" class="ftn-check-glyph" aria-hidden="true">
          <path v-if="checkState.state === 'all'" class="ftn-check-mark" d="M3.6 8.3 L6.7 11.4 L12.5 4.8" />
          <rect v-else-if="checkState.state === 'some'" class="ftn-check-mark" x="3.6" y="7" width="8.8" height="2" rx="1" />
        </svg>
      </button>
      <span class="ftn-icon" @click.stop="onRowClick">{{ icon }}</span>
      <span class="ftn-name" :title="node.path" @click.stop="onRowClick">{{ node.name }}</span>
      <span class="ftn-size" v-if="node.type === 'file'">{{ fmtSize(node.size) }}</span>
      <span class="ftn-size ftn-size-sub" v-else>{{ childFileCount }} 项</span>
      <span class="ftn-actions">
        <template v-if="dl && dl.active && dl.status !== 'paused'">
          <span class="ftn-dl-info">
            <span class="ftn-bar">
              <span class="ftn-fill"
                :class="{ 'ftn-indeterminate': (dl.pct || 0) === 0 }"
                :style="{ width: ((dl.pct || 0) > 0 ? dl.pct : 12) + '%' }"></span>
            </span>
            <span class="ftn-pct" v-if="(dl.pct || 0) > 0">{{ dl.pct }}%</span>
            <span class="ftn-pct ftn-pct-live" v-else>↓</span>
            <span class="ftn-speed" v-if="dl.speed > 0" :title="fmtSize(dl.written) + ' / ' + fmtSize(dl.total)">{{ fmtSpeed(dl.speed) }}</span>
          </span>
        </template>
        <template v-else-if="dl && dl.status === 'paused'">
          <span class="ftn-paused">已暂停 {{ dl.pct || 0 }}%</span>
        </template>
        <template v-else-if="dl && dl.status === 'error'">
          <span class="ftn-error" :title="dl.reason">下载失败</span>
          <button type="button" class="btn btn-sm btn-retry" @click.stop="emit('download', node.path)" title="重新下载">↻ 重试</button>
        </template>
        <template v-else-if="node.type === 'file'">
          <span v-if="isDownloaded" class="ftn-done">✓ 已下载</span>
          <button v-else type="button" class="btn btn-sm btn-primary" @click.stop="emit('download', node.path)">下载</button>
        </template>
      </span>
    </div>
    <template v-if="node.type === 'directory' && expanded && node.children && node.children.length">
      <FileTreeNode
        v-for="child in node.children" :key="child.path"
        :node="child" :level="level + 1"
        :checked-files="checkedFiles" :expanded-dirs="expandedDirs"
        :dl-state="dlState" :downloaded-set="downloadedSet"
        @toggle-dir="p => emit('toggle-dir', p)"
        @toggle-check="n => emit('toggle-check', n)"
        @download="p => emit('download', p)"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { fmtSize } from '../utils/formatters'
import type { FileProgress } from '../stores/downloadStore'

interface FileNode {
  name: string; path: string; type: 'file' | 'directory'; size?: number; children?: FileNode[]
}

const props = defineProps<{
  node: FileNode
  level?: number
  checkedFiles?: Record<string, boolean>
  expandedDirs?: Record<string, boolean>
  dlState?: Record<string, FileProgress>
  downloadedSet?: Record<string, boolean>
  isRoot?: boolean
}>()

const emit = defineEmits<{
  'toggle-dir': [path: string]
  'toggle-check': [node: FileNode]
  download: [path: string]
}>()

const rowStyle = computed(() => ({ paddingLeft: ((props.level ?? 0) * 22 + 12) + 'px' }))
const expanded = computed(() => (props.expandedDirs ?? {})[props.node.path] !== false)

function collectFiles(dirNode: FileNode): string[] {
  const files: string[] = []
  const walk = (nodes: FileNode[]) => {
    for (const n of nodes || []) {
      if (n.type === 'file') files.push(n.path)
      else if (n.children) walk(n.children)
    }
  }
  walk(dirNode.children || [])
  return files
}

const childFileCount = computed(() => collectFiles(props.node).length)

const checkState = computed(() => {
  const cf = props.checkedFiles ?? {}
  if (props.node.type !== 'directory') {
    return { state: cf[props.node.path] !== false ? 'all' as const : 'none' as const }
  }
  const files = collectFiles(props.node)
  if (files.length === 0) return { state: 'none' as const }
  let c = 0
  for (const f of files) { if (cf[f] !== false) c++ }
  if (c === files.length) return { state: 'all' as const }
  if (c === 0) return { state: 'none' as const }
  return { state: 'some' as const }
})

const checkTitle = computed(() => {
  const dir = props.node.type === 'directory'
  return checkState.value.state === 'all' ? (dir ? '取消全选' : '取消选择') : (dir ? '全选此项' : '选择')
})

const icon = computed(() => {
  if (props.node.type === 'directory') return expanded.value ? '📂' : '📁'
  const n = props.node.name
  const ext = n.includes('.') ? n.slice(n.lastIndexOf('.')).toLowerCase() : ''
  if (['.bin','.safetensors','.h5','.pt','.pth','.onnx','.gguf','.pb','.mlmodel','.msgpack'].includes(ext)) return '🧩'
  if (['.json','.yaml','.yml'].includes(ext)) return '⚙'
  if (['.txt','.model','.tokenizer'].includes(ext)) return '📝'
  if (ext === '.md') return '📋'
  if (['.jpg','.png','.jpeg','.gif'].includes(ext)) return '🖼'
  return '📄'
})

// downloadedSet is updated by ModelCatalog polling — may lag behind the
// real-time download-progress event. Use dl.status as the primary source
// so the UI reacts immediately when a download completes.
const dl = computed(() => (props.dlState ?? {})[props.node.path] || null)
const isDownloaded = computed(() =>
  !!(props.downloadedSet ?? {})[props.node.path] ||
  dl.value?.status === 'completed'
)

function fmtSpeed(n: number): string {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB/s'
  if (n >= 1e3) return (n / 1e3).toFixed(0) + ' KB/s'
  return n + ' B/s'
}

function onRowClick() { if (props.node.type === 'directory') emit('toggle-dir', props.node.path) }
function onToggleDir() { emit('toggle-dir', props.node.path) }
function onToggleCheck() { emit('toggle-check', props.node) }
</script>

<style scoped>
.ftn-wrap { display: block; }
.ftn-row {
  position: relative; display: flex; align-items: center; gap: 8px;
  padding: 6px 12px; min-height: 34px;
  border-bottom: 1px solid rgba(255,255,255,0.025);
  transition: background 0.12s ease; cursor: default;
}
.ftn-row:last-child { border-bottom: none; }
.ftn-row:hover { background: rgba(255,255,255,0.045); }
.ftn-dir { cursor: pointer; }
.ftn-got .ftn-name, .ftn-got .ftn-icon { opacity: 0.5; }
.ftn-got:hover .ftn-name, .ftn-got:hover .ftn-icon { opacity: 1; }
.ftn-root-row {
  padding-top: 10px; padding-bottom: 10px;
  background: rgba(255,255,255,0.028);
  border-bottom: 1px solid rgba(255,255,255,0.07);
}
.ftn-root-row .ftn-name { color: var(--text-primary); font-weight: 600; font-size: 13px; letter-spacing: -0.005em; }
.ftn-root-row .ftn-icon { font-size: 15px; }
.ftn-root-row .ftn-check { width: 16px; height: 16px; }
.ftn-arrow {
  width: 18px; height: 18px; flex-shrink: 0; display: inline-flex; align-items: center; justify-content: center;
  border: none; border-radius: 5px; background: transparent; color: var(--text-tertiary);
  font-size: 10px; cursor: pointer; padding: 0;
  transition: background 0.12s, color 0.12s, transform 0.18s cubic-bezier(0.22,0.61,0.36,1);
}
.ftn-arrow:hover { background: rgba(255,255,255,0.08); color: var(--text-primary); }
.ftn-arrow.expanded { transform: rotate(90deg); }
.ftn-arrow-spacer { width: 18px; flex-shrink: 0; }
.ftn-check {
  position: relative; flex-shrink: 0; width: 15px; height: 15px; margin: 0; padding: 0;
  display: inline-flex; align-items: center; justify-content: center;
  border: 1.5px solid rgba(255,255,255,0.22); border-radius: 4px;
  background: rgba(0,0,0,0.22); cursor: pointer; line-height: 0;
  transition: background 0.12s ease, border-color 0.12s ease, transform 0.08s ease;
}
.ftn-check:hover { border-color: rgba(255,255,255,0.5); background: rgba(255,255,255,0.07); }
.ftn-check:active { transform: scale(0.86); }
.ftn-check.is-all, .ftn-check.is-some { background: var(--accent); border-color: var(--accent); }
.ftn-check.is-all:hover, .ftn-check.is-some:hover { background: var(--accent-hover); border-color: var(--accent-hover); }
.ftn-check-glyph { width: 100%; height: 100%; display: block; overflow: visible; }
.ftn-check-mark { fill: none; stroke: #fff; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
.ftn-check.is-some .ftn-check-mark { fill: #fff; stroke: none; }
.ftn-icon { flex-shrink: 0; font-size: 13px; width: 18px; text-align: center; user-select: none; }
.ftn-name { flex: 1; font-size: 12px; font-family: var(--font-mono); color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; user-select: none; }
.ftn-size { font-size: 10px; color: var(--text-tertiary); font-family: var(--font-mono); flex-shrink: 0; min-width: 64px; text-align: right; }
.ftn-size-sub { opacity: 0.7; }
.ftn-actions { flex-shrink: 0; min-width: 110px; display: inline-flex; align-items: center; gap: 6px; justify-content: flex-end; }
.ftn-dl-info { display: inline-flex; align-items: center; gap: 4px; }
.ftn-done { font-size: 10px; color: var(--success); white-space: nowrap; font-weight: 500; }
.ftn-bar { width: 54px; height: 5px; background: rgba(255,255,255,0.12); border-radius: 3px; overflow: hidden; flex-shrink: 0; }
.ftn-fill { height: 100%; background: linear-gradient(90deg, var(--accent), #5ac8fa); border-radius: 3px; transition: width 0.3s; }
.ftn-indeterminate {
  animation: ftn-slide 1.4s ease-in-out infinite;
  background: linear-gradient(90deg, transparent, var(--accent), #5ac8fa);
  background-size: 200% 100%;
}
@keyframes ftn-slide {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
.ftn-pct { font-size: 10px; color: var(--text-tertiary); font-family: var(--font-mono); min-width: 28px; text-align: right; }
.ftn-pct-live { color: var(--accent); animation: ftn-blink 0.8s ease-in-out infinite; }
.ftn-speed { font-size: 9px; color: var(--text-tertiary); font-family: var(--font-mono); min-width: 48px; text-align: right; white-space: nowrap; }
@keyframes ftn-blink { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
.ftn-paused { font-size: 10px; color: var(--warning); }
.ftn-error { font-size: 10px; color: var(--danger); white-space: nowrap; max-width: 80px; overflow: hidden; text-overflow: ellipsis; }
.btn { padding: 3px 10px; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); background: var(--bg-elevated); color: var(--text-primary); font-size: 11px; cursor: pointer; transition: all var(--transition); }
.btn:hover { background: var(--bg-hover); }
.btn-primary { background: var(--accent); border-color: var(--accent); color: #fff; }
.btn-primary:hover { opacity: 0.85; }
.btn-retry { color: var(--danger); border-color: rgba(255,69,58,0.35); font-size: 10px; padding: 2px 8px; white-space: nowrap; }
.btn-retry:hover { background: rgba(255,69,58,0.12); }
</style>
