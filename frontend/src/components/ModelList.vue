<template>
  <div class="model-list">
    <div class="toolbar">
      <h2 class="title">模型</h2>
      <button class="btn btn-primary" @click="showAdd = true">＋ 加载模型</button>
    </div>

    <!-- 空状态 -->
    <div v-if="models.length === 0" class="empty">
      <div class="empty-icon">⊡</div>
      <p class="empty-title">暂无模型</p>
      <p class="empty-hint">点击「＋ 加载模型」添加一个</p>
    </div>

    <!-- 卡片列表 -->
    <div v-else class="cards">
      <ModelCard
        v-for="m in models"
        :key="m.id"
        :model="m"
        @unload="id => emit('unload', id)"
      />
    </div>

    <!-- 加载对话框 -->
    <DialogModal v-model:visible="showAdd" title="加载模型" okLabel="加载" :okDisabled="!newId || !newName" @ok="addModel" @cancel="showAdd = false">
      <div class="field">
        <label>模型 ID</label>
        <input v-model="newId" type="text" placeholder="例如 model-1" />
      </div>
      <div class="field">
        <label>模型名称</label>
        <input v-model="newName" type="text" placeholder="例如 测试模型" />
      </div>
    </DialogModal>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import ModelCard from './ModelCard.vue'
import DialogModal from './DialogModal.vue'

defineProps<{ models?: any[] }>()
const emit = defineEmits<{ load: [id: string, name: string]; unload: [id: string]; run: [id: string, input: any] }>()

const showAdd = ref(false); const newId = ref(''); const newName = ref('')
function addModel() { if (!newId.value || !newName.value) return; emit('load', newId.value, newName.value); newId.value = ''; newName.value = ''; showAdd.value = false }
</script>

<style scoped>
.model-list { width: 100%; }
.toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 24px; }
.title { font-size: 22px; font-weight: 600; letter-spacing: -0.01em; }

/* empty */
.empty { text-align: center; padding: 80px 0; }
.empty-icon { font-size: 40px; opacity: 0.2; margin-bottom: 12px; }
.empty-title { font-size: 15px; font-weight: 500; margin-bottom: 4px; }
.empty-hint { font-size: 13px; color: var(--text-tertiary); }

.cards { display: flex; flex-direction: column; gap: 10px; }

.field { margin-bottom: 14px; }
.field label { display: block; font-size: 12px; font-weight: 500; color: var(--text-secondary); margin-bottom: 6px; }
.field input { width: 100%; }
</style>
