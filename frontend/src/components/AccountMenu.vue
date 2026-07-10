<template>
  <div class="account-menu">
    <div ref="avatarEl" class="avatar-wrap"
      @mouseenter="openPanel" @mouseleave="startClose">
      <div v-if="!loggedInPlatforms.length" class="avatar avatar-none" title="未登录">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <circle cx="12" cy="8" r="4" />
          <path d="M12 14c-4 0-8 2-8 6v2h16v-2c0-4-4-6-8-6z" />
        </svg>
      </div>
      <div v-else class="avatar-stack">
        <div v-for="(p, i) in loggedInPlatforms" :key="p.source"
          class="avatar chip" :class="'chip-' + p.source"
          :style="{ zIndex: 10 - i, marginLeft: i === 0 ? '0' : '-10px' }"
          :title="p.name">
          {{ sourceIcon(p.source) }}
        </div>
      </div>
    </div>

    <Teleport to="body">
      <transition name="acc-fade">
        <div v-if="show && panelStyle" class="acc-panel glass-panel"
          :style="panelStyle"
          @mouseenter="cancelClose" @mouseleave="startClose">
        <div class="acc-head">账号</div>
        <div v-for="acc in accounts" :key="acc.source" class="acc-row">
          <div class="acc-info">
            <span class="acc-dot" :class="dotClass(acc)"></span>
            <span class="acc-name">{{ acc.name }}</span>
            <span v-if="acc.verifying" class="acc-reason acc-reason-loading">验证中…</span>
            <span v-else-if="acc.valid && acc.username" class="acc-user">@{{ acc.username }}</span>
            <span v-else-if="acc.hasToken && !acc.valid && isNetError(acc)" class="acc-reason acc-reason-net" title="无法连接服务器，可能是网络或代理问题">⚠ {{ acc.reason }}</span>
            <span v-else-if="acc.hasToken && !acc.valid && acc.reason" class="acc-reason">{{ acc.reason }}</span>
            <span v-else-if="acc.hasToken && acc.valid" class="acc-reason">已登录</span>
          </div>
          <div class="acc-btns">
            <button v-if="acc.hasToken" class="acc-refresh" @click="verifyOne(acc)" :disabled="verifying === acc.source" :title="'刷新验证'">↻</button>
            <button v-if="!acc.hasToken" class="acc-login-btn" @click="loginInApp(acc)" :disabled="loggingIn === acc.source">{{ loggingIn === acc.source ? '…' : '登录' }}</button>
            <button v-else-if="!acc.valid && isNetError(acc)" class="acc-login-btn" @click="verifyOne(acc)" :disabled="verifying === acc.source">{{ verifying === acc.source ? '…' : '重试' }}</button>
            <button v-else-if="!acc.valid" class="acc-login-btn" @click="loginInApp(acc)" :disabled="loggingIn === acc.source">重新登录</button>
            <button v-else class="acc-logout-btn" @click="logout(acc)">退出</button>
          </div>
        </div>
      </div>
    </transition>
  </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { accountsApi } from '../api/accounts'

interface Account {
  source: string; name: string; hasToken: boolean; valid: boolean
  verifying?: boolean; username?: string; reason?: string
}

const show = ref(false)
const accounts = ref<Account[]>([])
const tokens = ref<Record<string, string>>({})
const panelStyle = ref<Record<string, string> | null>(null)
let closeTimer: ReturnType<typeof setTimeout> | null = null
const loggingIn = ref<string | null>(null)
const verifying = ref<string | null>(null)
const avatarEl = ref<HTMLElement | null>(null)

const anyLoggedIn = computed(() => accounts.value.some(a => a.hasToken))
const loggedInCount = computed(() => accounts.value.filter(a => a.hasToken).length)
const loggedInPlatforms = computed(() => accounts.value.filter(a => a.hasToken))

function sourceIcon(s: string) { return s === 'huggingface' ? '🤗' : s === 'modelscope' ? '🅼' : '●' }
function isNetError(acc: Account) { return !acc.valid && !!acc.reason && acc.reason.indexOf('网络错误') === 0 }
function dotClass(acc: Account) {
  if (acc.verifying) return 'dot-verifying'
  if (acc.valid) return 'dot-on'
  if (!acc.hasToken) return 'dot-off'
  if (isNetError(acc)) return 'dot-net'
  return 'dot-warn'
}
function onVerified(data: { source: string; valid: boolean; username: string; reason: string }) {
  const idx = accounts.value.findIndex(a => a.source === data.source)
  if (idx < 0) return
  const acc = accounts.value[idx]
  acc.verifying = false; acc.valid = !!data.valid
  if (data.valid) acc.hasToken = true // login/verify success → mark as logged in
  acc.username = data.username || ''; acc.reason = data.reason || ''
}

// ── exact original hover logic ──
function openPanel() {
  cancelClose()
  nextTick(() => {
    const r = avatarEl.value?.getBoundingClientRect()
    if (!r) return
    panelStyle.value = { left: r.left + 'px', bottom: (window.innerHeight - r.top + 8) + 'px' }
    show.value = true
  })
}
function startClose() { closeTimer = setTimeout(() => { show.value = false }, 150) }
function cancelClose() { if (closeTimer) { clearTimeout(closeTimer); closeTimer = null } }

async function refresh() { try { accounts.value = await accountsApi.list() } catch (_) {} }
async function verifyOne(acc: Account) {
  verifying.value = acc.source
  try {
    const info = await accountsApi.verify(acc.source)
    const idx = accounts.value.findIndex(a => a.source === acc.source)
    if (idx >= 0) { accounts.value[idx].valid = info.valid; accounts.value[idx].username = info.username; accounts.value[idx].reason = info.reason }
  } catch (e: any) { /* */ }
  verifying.value = null
}
function openLogin(acc: Account) { accountsApi.openLogin(acc.source) }
async function loginInApp(acc: Account) {
  loggingIn.value = acc.source
  try {
    await accountsApi.login(acc.source)
    acc.hasToken = true; acc.valid = true; acc.verifying = false
    await refresh()
  } catch (e: any) { /* */ }
  loggingIn.value = null
}
async function saveToken(acc: Account) {
  const tok = tokens.value[acc.source]; if (!tok) return
  try { await accountsApi.setToken(acc.source, tok); tokens.value[acc.source] = ''; await refresh() } catch (e: any) { /* */ }
}
async function logout(acc: Account) {
  try { await accountsApi.setToken(acc.source, ''); await refresh() } catch (_) {}
}

let _cancelAcc: (() => void) | null = null
onMounted(async () => {
  await refresh()
  _cancelAcc = window.runtime.EventsOn('account-verified', (data: any) => onVerified(data)) as any
})
onBeforeUnmount(() => {
  if (closeTimer) clearTimeout(closeTimer)
  if (_cancelAcc) { try { _cancelAcc() } catch (_) {} }
})
</script>

<style scoped>
.account-menu { position: relative; }
.avatar-wrap { cursor: pointer; display: flex; align-items: center; }
.avatar { width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; border: 1px solid var(--border-subtle); }
.avatar-none { background: var(--bg-hover); color: var(--text-tertiary); }
.avatar-wrap:hover .avatar-none { color: var(--text-secondary); border-color: var(--border-soft); }
.avatar-stack { display: flex; align-items: center; }
.avatar-stack .chip { width: 24px; height: 24px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 12px; border: 2px solid var(--bg-sidebar); transition: all var(--transition); }
.avatar-wrap:hover .chip { transform: translateY(-1px); }
.chip-huggingface { background: #ff9f0a; color: #fff; }
.chip-modelscope { background: var(--accent); color: #fff; }
.acc-panel { position: fixed; min-width: 300px; padding: 14px; z-index: 3000; box-shadow: 0 8px 32px rgba(0,0,0,0.6); }
.acc-head { font-size: 10px; font-weight: 600; color: var(--text-tertiary); text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 10px; }
.acc-row { display: flex; align-items: center; justify-content: space-between; padding: 8px 0; border-top: 1px solid var(--border-subtle); gap: 8px; }
.acc-row:first-of-type { border-top: none; }
.acc-info { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; }
.acc-btns { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.acc-refresh { width: 26px; height: 26px; border: 1px solid var(--border-subtle); border-radius: 50%; background: var(--bg-hover); color: var(--text-tertiary); cursor: pointer; font-size: 12px; display: inline-flex; align-items: center; justify-content: center; transition: all var(--transition); flex-shrink: 0; line-height: 1; }
.acc-refresh:hover { background: var(--bg-active); color: var(--accent); }
.acc-refresh:disabled { opacity: 0.4; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.acc-login-btn { height: 26px; padding: 0 12px; border: none; border-radius: 6px; background: var(--accent); color: #fff; font-size: 11px; font-weight: 600; cursor: pointer; transition: all var(--transition); flex-shrink: 0; display: inline-flex; align-items: center; line-height: 1; }
.acc-login-btn:hover { background: var(--accent-hover, #3395ff); }
.acc-login-btn:disabled { opacity: 0.5; cursor: default; }
.acc-logout-btn { height: 26px; padding: 0 12px; border: 1px solid rgba(255,69,58,0.3); border-radius: 6px; background: transparent; color: var(--danger); font-size: 11px; cursor: pointer; flex-shrink: 0; display: inline-flex; align-items: center; line-height: 1; }
.acc-logout-btn:hover { background: var(--danger-dim); }
.acc-dot { width: 7px; height: 7px; border-radius: 50%; }
.dot-on { background: var(--success); box-shadow: 0 0 4px rgba(48,209,88,0.5); }
.dot-off { background: var(--text-tertiary); opacity: 0.5; }
.dot-warn { background: var(--warning); box-shadow: 0 0 4px rgba(255,159,10,0.5); }
.dot-net { background: #ff9f0a; box-shadow: 0 0 4px rgba(255,159,10,0.5); animation: pulse-net 1.6s ease-in-out infinite; }
.dot-verifying { background: var(--text-tertiary); animation: pulse-net 1.2s ease-in-out infinite; }
@keyframes pulse-net { 0%,100% { opacity: 1; } 50% { opacity: 0.45; } }
.acc-user { font-size: 11px; color: var(--success); font-weight: 500; }
.acc-reason { font-size: 10px; color: var(--warning); }
.acc-reason-net { color: #ff9f0a; }
.acc-reason-loading { color: var(--text-tertiary); }
.acc-name { font-size: 12px; font-weight: 500; }
.acc-actions { margin-left: auto; }
.acc-token-row { display: flex; align-items: center; gap: 6px; }
.acc-token-set { font-size: 10px; color: var(--success); flex: 1; }
.acc-input { flex: 1; padding: 4px 8px; font-size: 11px; }
.acc-btn { width: 24px; height: 24px; border: 1px solid var(--border-subtle); border-radius: 5px; background: var(--bg-hover); color: var(--text-secondary); cursor: pointer; font-size: 11px; display: flex; align-items: center; justify-content: center; }
.acc-btn:hover { background: var(--bg-active); color: var(--accent); }
.acc-btn:disabled { opacity: 0.3; cursor: default; }
.acc-btn-ok { background: var(--accent-dim); color: var(--accent); border-color: var(--accent); }
.acc-btn-del { background: transparent; color: var(--danger); border-color: rgba(255,69,58,0.3); }
.acc-fade-enter-active, .acc-fade-leave-active { transition: opacity 0.15s ease; }
.acc-fade-enter-from, .acc-fade-leave-to { opacity: 0; }
</style>
