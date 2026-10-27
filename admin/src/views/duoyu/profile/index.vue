<template>
  <div class="dy-page" v-loading="loading"
    ><div class="dy-heading"
      ><div
        ><h1>{{ dialog ? titles[dialog] : '账号与安全' }}</h1
        ><p>个人资料、登录方式与设备会话</p></div
      ><span class="dy-tag">我的账号</span></div
    ><div v-if="error" class="dy-error">{{ error }}</div
    ><div v-if="!dialog" class="dy-shortcuts"
      ><div class="dy-shortcut"
        ><b>登录邮箱</b><span>{{ me.email || '—' }}</span></div
      ><div class="dy-shortcut"
        ><b>{{ passkeys.length }} 个通行密钥</b><span>使用设备验证完成登录</span></div
      ><div class="dy-shortcut"
        ><b>{{ sessions.length }} 个中心会话</b><span>检查设备并撤销不再使用的会话</span></div
      ></div
    ><div v-if="!dialog" class="dy-grid"
      ><section class="dy-panel"
        ><h2>个人资料</h2
        ><div style="display: flex; align-items: center; margin: 24px 0"
          ><img class="dy-avatar" :src="me.avatar || '/avatar-default.svg'" alt="头像" /><div
            ><el-button @click="uploadInput?.click()">更换头像</el-button
            ><p class="dy-muted">自动居中裁剪 · JPEG / PNG / WebP · 最大 2 MB</p
            ><input
              ref="uploadInput"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              style="display: none"
              @change="upload" /></div></div
        ><el-form label-position="top"
          ><el-form-item label="昵称"><el-input v-model="me.name" maxlength="40" /></el-form-item
          ><el-form-item label="邮箱"><el-input :model-value="me.email" disabled /></el-form-item
          ><el-form-item label="用户标识"
            ><span class="dy-code">{{ me.id }}</span></el-form-item
          ></el-form
        ><el-button type="primary" :loading="saving" @click="saveProfile"
          >保存资料</el-button
        ></section
      ><section class="dy-panel"
        ><h2>安全设置</h2
        ><div class="dy-listitem"
          ><div>账号密码<p>设置或修改密码，需验证当前邮箱</p></div
          ><el-button @click="dialog = 'password'">修改</el-button></div
        ><div class="dy-listitem"
          ><div>登录邮箱<p>同时验证原邮箱与新邮箱</p></div
          ><el-button @click="dialog = 'email'">更换</el-button></div
        ><div class="dy-listitem"
          ><div>通行密钥 / 手机通行证<p>支持 iPhone Face ID、Touch ID 和电脑扫码登录</p></div
          ><el-button @click="dialog = 'passkey'">添加</el-button></div
        ><div class="dy-muted" style="margin-top: 24px"
          >修改密码或邮箱后，中心会话与访问令牌将被撤销。已接入退出通知的项目会同步处理；其他项目需在线复核登录状态。</div
        ></section
      ></div
    ><section v-if="!dialog" class="dy-panel"
      ><h2>我的通行密钥</h2
      ><div class="dy-empty" v-if="!passkeys.length"
        >还没有通行密钥。添加后可使用设备验证快速登录。</div
      ><div class="dy-listitem" v-for="p in passkeys" :key="p.id"
        ><div
          >{{ p.name }}<p>创建于 {{ dateText(p.created_at) }}</p></div
        ><el-button text type="danger" @click="selectPasskey(p.id)">移除</el-button></div
      ></section
    ><section v-if="!dialog" class="dy-panel"
      ><h2>中心登录会话</h2
      ><p class="dy-muted">仅展示多鱼账号中心的会话，不代表各项目的在线设备。</p
      ><el-table :data="sessions" empty-text="暂无有效会话"
        ><el-table-column
          label="设备 / 浏览器"
          min-width="220"
          show-overflow-tooltip
          prop="agent"
        /><el-table-column label="登录时间" min-width="170"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        ><el-table-column label="过期时间" min-width="170"
          ><template #default="{ row }">{{ dateText(row.expires_at) }}</template></el-table-column
        ><el-table-column label="操作" width="150"
          ><template #default="{ row }"
            ><el-tag v-if="row.current" size="small">当前</el-tag
            ><el-button text type="danger" @click="revoke(row)">撤销</el-button></template
          ></el-table-column
        ></el-table
      ></section
    >
    <section v-if="dialog" class="dy-panel dy-editor"
      ><el-button text @click="closeDialog">← 返回账号与安全</el-button
      ><el-form label-position="top"
        ><template v-if="dialog === 'email'"
          ><el-form-item label="新的邮箱"><el-input v-model="newEmail" type="email" /></el-form-item
          ><el-form-item label="新邮箱验证码"
            ><div style="display: flex; gap: 10px; width: 100%"
              ><el-input v-model="newCode" maxlength="6" /><el-button
                :disabled="sending"
                @click="send(newEmail, 'email')"
                >发送</el-button
              ></div
            ></el-form-item
          ></template
        ><el-form-item :label="'当前邮箱验证码 · ' + me.email"
          ><div style="display: flex; gap: 10px; width: 100%"
            ><el-input v-model="code" maxlength="6" placeholder="6 位验证码" /><el-button
              :disabled="sending"
              @click="
                send(
                  me.email,
                  dialog === 'password' ? 'reset' : dialog === 'email' ? 'email' : 'login'
                )
              "
              >发送</el-button
            ></div
          ></el-form-item
        ><el-form-item v-if="dialog === 'password'" label="新密码"
          ><el-input
            v-model="password"
            type="password"
            show-password
            placeholder="至少 12 位"
            maxlength="128" /></el-form-item
        ><div v-if="dialog === 'passkey'">
          <el-radio-group v-model="passkeyTarget"
            ><el-radio value="device">当前设备</el-radio
            ><el-radio value="phone">iPhone / 其他手机</el-radio></el-radio-group
          >
          <p class="dy-muted"
            >可在 iPhone 的 Safari
            中登录账号后添加，保存到“密码”App。电脑添加时选择手机，按系统提示扫码；浏览器不支持时请直接在手机添加。</p
          >
        </div>
        <p v-if="dialog === 'deletePasskey'" class="dy-muted"
          >移除后仍可使用邮箱登录。设备上的密钥副本需在设备密码管理器中删除。</p
        ></el-form
      ><div class="dy-form-footer"
        ><el-button @click="dialog = ''">取消</el-button
        ><el-button type="primary" :loading="saving" @click="securityAction">继续</el-button></div
      ></section
    ></div
  >
</template>
<script setup lang="ts">
  import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
  import { useRoute, useRouter, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { api, dateText, toBuffer, toBase64 } from '@/api/duoyu'
  import { useUserStore } from '@/store/modules/user'
  const route = useRoute(),
    router = useRouter()
  const store = useUserStore(),
    passkeyTarget = ref('device'),
    me = ref<any>({}),
    sessions = ref<any[]>([]),
    passkeys = ref<any[]>([]),
    loading = ref(true),
    saving = ref(false),
    error = ref(''),
    uploadInput = ref<HTMLInputElement>(),
    dialog = computed({
      get: () =>
        route.path === '/account/security' &&
        ['password', 'email', 'passkey', 'deletePasskey'].includes(String(route.query.action))
          ? String(route.query.action)
          : '',
      set: (value: string) => {
        router.push(
          value ? { path: '/account/security', query: { action: value } } : '/account/profile'
        )
      }
    }),
    code = ref(''),
    newCode = ref(''),
    newEmail = ref(''),
    password = ref(''),
    deleteID = ref(''),
    sending = ref(false)
  const titles: Record<string, string> = {
    password: '修改密码',
    email: '更换邮箱',
    passkey: '添加通行密钥',
    deletePasskey: '移除通行密钥'
  }
  const securityDirty = computed(
    () => !!dialog.value && !!(code.value || newCode.value || newEmail.value || password.value)
  )
  async function leaveSecurity() {
    if (!securityDirty.value) return true
    try {
      await ElMessageBox.confirm('修改尚未提交，确定离开？', '离开页面', { type: 'warning' })
      return true
    } catch {
      return false
    }
  }
  onBeforeRouteLeave(leaveSecurity)
  onBeforeRouteUpdate(leaveSecurity)
  const beforeUnload = (event: BeforeUnloadEvent) => {
    if (securityDirty.value) {
      event.preventDefault()
      event.returnValue = ''
    }
  }
  window.addEventListener('beforeunload', beforeUnload)
  onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
  function clearSecurity() {
    code.value = ''
    newCode.value = ''
    password.value = ''
    newEmail.value = ''
  }
  watch(() => route.fullPath, clearSecurity)
  function selectPasskey(id: string) {
    deleteID.value = id
    router.push({ path: '/account/security', query: { action: 'deletePasskey', key: id } })
  }
  function closeDialog() {
    dialog.value = ''
  }
  async function load() {
    try {
      ;[me.value, sessions.value, passkeys.value] = await Promise.all([
        api('/api/me'),
        api('/api/me/sessions'),
        api('/api/me/passkeys')
      ])
    } catch (e: any) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }
  onMounted(load)
  async function saveProfile() {
    saving.value = true
    try {
      await api('/api/me', 'PATCH', { name: me.value.name })
      store.setUserInfo(await api('/api/user/info'))
      ElMessage.success('资料已更新')
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
  async function upload(event: Event) {
    const file = (event.target as HTMLInputElement).files?.[0]
    if (!file) return
    const data = new FormData()
    data.append('avatar', file)
    saving.value = true
    try {
      const out = await api('/api/me/avatar', 'POST', data)
      me.value.avatar = out.avatar
      store.setUserInfo(await api('/api/user/info'))
      ElMessage.success('头像已更新')
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      saving.value = false
      if (uploadInput.value) uploadInput.value.value = ''
    }
  }
  async function send(email: string, purpose: string) {
    sending.value = true
    try {
      const receipt = await api('/api/auth/code', 'POST', { email, purpose })
      ElMessage.success(
        receipt.local_mail
          ? '验证码已进入本机测试收件箱 localhost:18025'
          : '邮件请求已提交，请查看收件箱或垃圾邮件'
      )
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      sending.value = false
    }
  }
  async function securityAction() {
    saving.value = true
    try {
      if (dialog.value === 'password') {
        await api('/api/me/password', 'POST', { code: code.value, password: password.value })
        clearSecurity()
        window.location.assign('/login')
      } else if (dialog.value === 'email') {
        await api('/api/me/email', 'POST', {
          email: newEmail.value,
          old_code: code.value,
          new_code: newCode.value
        })
        clearSecurity()
        window.location.assign('/login')
      } else if (dialog.value === 'deletePasskey') {
        await api(
          '/api/me/passkeys/' + (deleteID.value || String(route.query.key || '')),
          'DELETE',
          { code: code.value }
        )
        clearSecurity()
        dialog.value = ''
        await load()
      } else {
        if (!window.PublicKeyCredential) throw Error('浏览器不支持通行密钥')
        const options = await api('/api/me/passkeys/begin', 'POST', { code: code.value })
        const p = options.publicKey
        if (passkeyTarget.value === 'phone') p.hints = ['hybrid']
        p.challenge = toBuffer(p.challenge)
        p.user.id = toBuffer(p.user.id)
        p.excludeCredentials?.forEach((c: any) => (c.id = toBuffer(c.id)))
        const c = (await navigator.credentials.create({
          publicKey: p
        })) as PublicKeyCredential | null
        if (!c) throw Error('操作已取消')
        const response = c.response as AuthenticatorAttestationResponse
        await api('/api/me/passkeys/finish', 'POST', {
          id: c.id,
          rawId: toBase64(c.rawId),
          type: c.type,
          authenticatorAttachment: c.authenticatorAttachment,
          clientExtensionResults: c.getClientExtensionResults(),
          response: {
            clientDataJSON: toBase64(response.clientDataJSON),
            attestationObject: toBase64(response.attestationObject),
            transports: response.getTransports()
          }
        })
        clearSecurity()
        dialog.value = ''
        await load()
      }
      ElMessage.success('操作成功')
    } catch (e: any) {
      ElMessage.error(e.name === 'NotAllowedError' ? '设备验证已取消或超时' : e.message)
    } finally {
      saving.value = false
    }
  }
  async function revoke(row: any) {
    try {
      await ElMessageBox.confirm('撤销此中心会话？', '确认操作')
      await api('/api/me/sessions/' + row.id, 'DELETE')
      if (row.current) window.location.assign('/login')
      else await load()
    } catch (e: any) {
      if (e instanceof Error) ElMessage.error(e.message)
    }
  }
</script>
