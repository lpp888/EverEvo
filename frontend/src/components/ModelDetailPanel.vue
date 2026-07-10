<template>
  <transition name="slide">
    <aside v-if="detail" class="detail-panel">
      <div class="detail-inner">
        <div class="dp-head">
          <div class="dp-title-row">
            <span class="src-badge" :class="'src-' + detail.source">{{ sourceLabel(detail.source) }}</span>
            <span class="dp-name">{{ detail.name }}</span>
          </div>
          <button type="button" class="dp-close" @click="emit('close')">✕</button>
        </div>
        <div class="dp-tabs">
          <button type="button" class="dp-tab" :class="{ active: dpTab === 'info' }" @click="emit('tab', 'info')">模型信息</button>
          <button type="button" class="dp-tab" :class="{ active: dpTab === 'files' }" @click="emit('tab', 'files')">文件列表</button>
        </div>

        <!-- Tab: 模型信息 -->
        <div v-if="dpTab === 'info'" class="dp-tab-content">
          <div class="dp-meta">
            <div v-if="detail.author" class="meta-item"><span class="meta-k">作者</span><span class="meta-v">{{ detail.author }}</span></div>
            <div v-if="detail.task" class="meta-item"><span class="meta-k">任务</span><span class="meta-v">{{ detail.task }}</span></div>
            <div class="meta-item"><span class="meta-k">下载量</span><span class="meta-v">⬇ {{ fmtCount(detail.downloads) }}</span></div>
            <div class="meta-item"><span class="meta-k">来源</span><span class="meta-v">{{ detail.source }} / {{ detail.repoId }}</span></div>
            <div class="meta-item"><span class="meta-k">文件数</span><span class="meta-v">{{ totalFileCount }} 个</span></div>
            <div v-if="totalSize" class="meta-item"><span class="meta-k">总大小</span><span class="meta-v">{{ fmtSize(totalSize) }}</span></div>
          </div>
          <a v-if="detail.url" :href="detail.url" target="_blank" class="dp-link">↗ 在 {{ sourceLabel(detail.source) }} 查看原文</a>
          <div class="dp-desc">
            <h4 class="dp-section-title">简介</h4>
            <p v-if="detail.description" class="dp-desc-text">{{ detail.description }}</p>
            <p v-else class="dp-desc-text dp-desc-empty">暂无描述</p>
          </div>
        </div>

        <!-- Tab: 文件列表（递归树） -->
        <div v-if="dpTab === 'files'" class="dp-tab-content">
          <div v-if="detailLoading" class="dp-loading">加载文件列表中…</div>
          <div v-else-if="detail.filesError" class="dp-error">
            <div class="net-error-icon">⚠</div>
            <div class="net-error-title">{{ detail.filesError }}</div>
            <div class="net-error-hint">无法获取文件列表，请检查网络后重试</div>
            <button type="button" class="btn btn-sm" @click="emit('reload')">重试</button>
          </div>
          <div v-else-if="!hasTree" class="dp-empty">仓库无文件</div>
          <div v-else class="dp-files">
            <div class="files-head">
              <button type="button" class="btn btn-primary btn-dl-all"
                @click="emit('downloadChecked')"
                :disabled="pkgDownloading || checkedFileCount === 0">
                {{ pkgBtnText }}
              </button>
              <button type="button" class="btn btn-sm" @click="emit('toggleAll', true)">全选</button>
              <button type="button" class="btn btn-sm" @click="emit('toggleAll', false)">取消全选</button>
              <span v-if="checkedSize" class="files-total-size">{{ fmtSize(checkedSize) }}</span>
            </div>
            <div class="dp-file-tree">
              <FileTreeNode
                v-if="rootNode"
                :node="rootNode"
                :level="0"
                :is-root="true"
                :checked-files="checkedFiles"
                :expanded-dirs="expandedDirs"
                :dl-state="dlState"
                :downloaded-set="downloadedSet"
                @toggle-dir="p => emit('toggleDir', p)"
                @toggle-check="emit('toggleCheck', $event)"
                @download="f => emit('downloadFile', f)"
              />
            </div>
          </div>
        </div>
      </div>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import FileTreeNode from './FileTreeNode.vue'
import { fmtSize, fmtCount } from '../utils/formatters'

interface FileTree { name: string; path: string; type: 'file' | 'directory'; size?: number; children?: FileTree[] }

const props = defineProps<{
  detail: Record<string, any> | null
  detailLoading: boolean
  dpTab: string
  dlState: Record<string, any>
  checkedFiles: Record<string, boolean>
  expandedDirs: Record<string, boolean>
  downloadedSet: Record<string, boolean>
  pkgDownloading: boolean
  checkedFileCount: number
  checkedSize: number
  pkgBtnText: string
}>()

const emit = defineEmits<{
  close: []
  tab: [tab: string]
  reload: []
  downloadChecked: []
  downloadFile: [file: string]
  toggleAll: [val: boolean]
  toggleDir: [path: string]
  toggleCheck: [node: FileTree]
}>()

function sourceLabel(s: string) {
  return s === 'huggingface' ? 'HF' : s === 'modelscope' ? 'MS' : (s || '')
}

const allFilePaths = computed<string[]>(() => {
  const paths: string[] = []
  const walk = (nodes: FileTree[]) => {
    for (const node of nodes) {
      if (node.type === 'file') paths.push(node.path)
      if (node.children) walk(node.children)
    }
  }
  if (props.detail?.fileTree) walk(props.detail.fileTree)
  return paths
})

const totalFileCount = computed(() => {
  const entries = props.detail?.fileEntries
  if (!entries) return 0
  let count = 0
  for (const e of entries) { if (e.type === 'file' || e.type === '') count++ }
  return count || entries.length
})

const totalSize = computed(() => {
  const entries = props.detail?.fileEntries
  if (!entries) return 0
  return entries.reduce((s: number, e: any) => s + (e.size > 0 ? e.size : 0), 0)
})

const hasTree = computed(() => !!(props.detail?.fileTree?.length))

const rootNode = computed<FileTree | null>(() => {
  if (!hasTree.value) return null
  return {
    name: props.detail.name || props.detail.repoId,
    path: '__root__',
    type: 'directory',
    children: props.detail.fileTree,
  }
})
</script>

<style scoped>
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
