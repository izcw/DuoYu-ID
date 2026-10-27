<template>
  <div class="dy-page dy-editor" v-loading="loading">
    <div class="dy-heading"
      ><div
        ><el-button text @click="back"
          >← 返回{{
            kind === 'apps' ? '项目管理' : kind === 'keys' ? 'API 凭证' : '账号管理'
          }}</el-button
        ><h1>{{ title }}</h1
        ><p v-if="form.id" class="dy-code">{{ form.id }}</p></div
      ><span class="dy-tag">{{ secret ? '密钥待保存' : id ? '编辑与管理' : '新建' }}</span></div
    >
    <div v-if="error" class="dy-error" role="alert"
      >{{ error }} <el-button text @click="load">重试加载</el-button></div
    >
    <section v-if="secret" class="dy-panel"
      ><h2>保存你的密钥</h2><p class="dy-muted">密钥仅在这里显示一次，离开页面后无法再次读取。</p
      ><div class="dy-secret">{{ secret }}</div
      ><el-button @click="copy">复制密钥</el-button
      ><el-button type="primary" @click="finish">已保存，返回列表</el-button></section
    >
    <template v-else-if="!loading && !error">
      <section v-if="view === 'members'" class="dy-panel"
        ><h2>{{ form.name }} · 登录用户</h2
        ><p class="dy-muted">成功完成项目授权的账号；登录记录不等于准入权限。可按页查看全部记录。</p
        ><el-pagination
          v-model:current-page="memberPage"
          :page-size="20"
          :total="memberTotal"
          layout="total, prev, pager, next"
          @current-change="load" /><el-table :data="members" empty-text="暂无登录记录"
          ><el-table-column prop="email" label="邮箱" min-width="200" /><el-table-column
            prop="name"
            label="昵称" /><el-table-column label="首次登录" min-width="170"
            ><template #default="{ row }">{{
              dateText(row.first_login_at)
            }}</template></el-table-column
          ><el-table-column label="最近登录" min-width="170"
            ><template #default="{ row }">{{
              dateText(row.last_login_at)
            }}</template></el-table-column
          ><el-table-column prop="login_count" label="次数" /></el-table
      ></section>
      <section v-else-if="kind === 'users'" class="dy-panel"
        ><h2>{{ form.name }}</h2
        ><el-descriptions :column="1" border
          ><el-descriptions-item label="邮箱">{{ form.email }}</el-descriptions-item
          ><el-descriptions-item label="角色">{{
            form.admin ? '管理员' : '普通用户'
          }}</el-descriptions-item
          ><el-descriptions-item label="状态">{{
            form.enabled ? '正常' : '停用'
          }}</el-descriptions-item
          ><el-descriptions-item label="注册时间">{{
            dateText(form.created_at)
          }}</el-descriptions-item></el-descriptions
        ><el-button
          v-if="!form.admin"
          @click="router.push({ path: '/manage/users/access', query: { id } })"
          >管理权限</el-button
        ></section
      >
      <el-form v-else label-position="top" @submit.prevent="save" class="dy-form">
        <section v-if="view === 'grants'" class="dy-panel"
          ><h2>{{ form.name }} · 准入名单</h2
          ><el-alert
            :closable="false"
            :type="form.restricted ? 'info' : 'warning'"
            :title="
              form.restricted
                ? '仅名单中的账号可以登录此项目'
                : '项目当前允许全部账号登录，保存名单不会自动开启限制'
            "
          /><el-form-item label="已注册邮箱 · 每行一个" style="margin-top: 24px"
            ><el-input
              v-model="grantText"
              type="textarea"
              :rows="12"
              placeholder="user@example.com" /></el-form-item
          ><p class="dy-muted"
            >保存将替换当前名单；空名单配合限制模式会阻止全部账号登录。</p
          ></section
        >
        <template v-else>
          <section class="dy-panel"
            ><h2>基本信息</h2
            ><el-form-item label="名称" required
              ><el-input v-model="form.name" maxlength="60" placeholder="输入便于识别的名称"
            /></el-form-item>
            <template v-if="kind === 'apps'"
              ><el-form-item label="描述"
                ><el-input
                  v-model="form.description"
                  type="textarea"
                  maxlength="1000"
                  :rows="3" /></el-form-item
              ><el-form-item label="客户端类型" required
                ><el-radio-group v-model="form.public" :disabled="!!id"
                  ><el-radio :value="false">服务端网站</el-radio
                  ><el-radio :value="true">公共客户端</el-radio></el-radio-group
                ></el-form-item
              ><div class="dy-client-options"
                ><div :class="{ selected: !form.public }"
                  ><b>服务端网站</b
                  ><p>有自己的后端，密钥保存在服务器。适合 Go、Java、Node 等后端网站。</p
                  ><span>client_id + client_secret + PKCE</span></div
                ><div :class="{ selected: form.public }"
                  ><b>公共客户端</b
                  ><p>无法安全保存密钥，适合浏览器或原生应用，请同时选择对应平台。</p
                  ><span>client_id + PKCE，无 client_secret</span></div
                ></div
              ><p class="dy-muted"
                >“公共”指无法保守密钥，不代表允许所有用户访问。访问范围由准入规则控制。创建后客户端类型不可变更。</p
              ></template
            >
            <template v-else
              ><el-form-item label="所属项目" required
                ><el-input v-if="id" :model-value="form.app_name" disabled /><el-select
                  v-else
                  v-model="form.app_id"
                  filterable
                  style="width: 100%"
                  ><el-option
                    v-for="a in apps"
                    :key="a.id"
                    :label="a.name"
                    :value="a.id" /></el-select></el-form-item
              ><el-alert
                v-if="!id && !apps.length"
                :closable="false"
                title="请先创建并启用一个项目，再创建 API 凭证。"
              /><el-form-item label="凭证权限" required
                ><el-checkbox-group v-model="form.scopes"
                  ><el-checkbox value="app:read">读取项目资料</el-checkbox
                  ><el-checkbox value="mail:send">发送项目邮件</el-checkbox
                  ><el-checkbox value="mail:read">查询邮件状态</el-checkbox></el-checkbox-group
                ></el-form-item
              ><p class="dy-muted"
                >按用途分配权限。发送邮件的凭证只能保存在项目后端；旧凭证不会自动获得发信权限。</p
              ></template
            >
          </section>
          <section v-if="kind === 'apps'" class="dy-panel"
            ><h2>回调与访问</h2
            ><el-form-item label="后台退出通知地址 · 可留空"
              ><el-input
                v-model="form.backchannel_logout_uri"
                placeholder="https://app.example.com/auth/backchannel-logout" /></el-form-item
            ><p class="dy-muted"
              >项目后端接收签名退出通知，并按 sid 清理本地会话。需要接入方实现验签和幂等处理。</p
            >
            <el-form-item v-if="form.public" label="应用平台"
              ><el-radio-group v-model="form.platform"
                ><el-radio value="spa">浏览器 SPA</el-radio
                ><el-radio value="native">桌面 / 手机原生应用</el-radio></el-radio-group
              ></el-form-item
            >
            <p v-if="form.platform === 'native'" class="dy-muted"
              >使用系统浏览器和 PKCE；支持 HTTPS、127.0.0.1 / [::1]
              回环地址（允许动态端口），或反向域名形式的私有 scheme，如
              com.example.app:/callback。手机实机仍需验收。</p
            >
            <div class="dy-listitem"
              ><div
                >允许会话续期<p
                  >显式请求
                  offline_access；刷新令牌每次轮换，重放会撤销整个令牌家族。有效期不超过中心会话。</p
                ></div
              ><el-switch v-model="form.refresh_enabled" /></div
            ><el-form-item label="登录回调地址 · 每行一个" required
              ><el-input
                v-model="form.redirectText"
                type="textarea"
                :rows="4"
                placeholder="https://app.example.com/auth/callback" /></el-form-item
            ><el-form-item label="退出返回地址 · 可留空"
              ><el-input v-model="form.logoutText" type="textarea" :rows="2" /></el-form-item
            ><p class="dy-muted"
              >地址须精确匹配，不支持通配符。生产环境使用 HTTPS；开发环境可使用 localhost。</p
            ><div class="dy-listitem"
              ><div>启用项目<p>停用后拒绝新的认证请求</p></div
              ><el-switch v-model="form.enabled" /></div
            ><div class="dy-listitem"
              ><div>限制准入<p>仅允许准入名单中的账号登录</p></div
              ><el-switch v-model="form.restricted" /></div
          ></section>
          <section v-else class="dy-panel"
            ><h2>有效期</h2><el-checkbox v-if="id" v-model="changeExpiry">修改有效期</el-checkbox
            ><p v-if="id && !changeExpiry" class="dy-muted"
              >当前：{{
                form.expires_at ? dateText(form.expires_at) : '永久有效'
              }}，仅修改名称不会延长有效期。</p
            ><div v-if="!id || changeExpiry" style="margin-top: 20px"
              ><el-radio-group v-model="form.permanent"
                ><el-radio :value="true">永久有效</el-radio
                ><el-radio :value="false">指定天数</el-radio></el-radio-group
              ><el-form-item
                v-if="!form.permanent"
                label="从本次保存起计算"
                style="margin-top: 16px"
                ><el-input-number v-model="form.days" :min="1" :max="365" /> 天</el-form-item
              ><p class="dy-muted"
                >永久凭证不会自动过期，仍可随时撤销；所属项目停用后也无法调用。</p
              ></div
            ></section
          >
        </template>
        <div class="dy-form-footer"
          ><el-button @click="back">取消</el-button
          ><el-button
            type="primary"
            native-type="submit"
            :loading="saving"
            :disabled="form.revoked"
            >{{ view === 'grants' ? '保存名单' : id ? '保存修改' : '创建' }}</el-button
          ></div
        >
      </el-form>
      <section
        v-if="kind === 'apps' && id && view === 'edit'"
        class="dy-panel"
        style="margin-top: 24px"
        ><h2>项目管理</h2><el-button @click="checkApp">检查接入配置</el-button
        ><div v-if="checks.length"
          ><p v-for="item in checks" :key="item.label"
            >{{ item.passed ? '✓' : '待检查' }} · {{ item.label }}</p
          ><p class="dy-muted">仅校验配置，不代表回调网络或项目验签通过。</p></div
        ><el-button @click="router.push({ path: '/manage/apps/grants', query: { id } })"
          >准入名单</el-button
        ><el-button @click="router.push({ path: '/manage/apps/members', query: { id } })"
          >登录用户</el-button
        ><el-button v-if="!form.public" :loading="saving" @click="rotate">轮换客户端密钥</el-button
        ><p v-if="!form.public" class="dy-muted"
          >轮换后旧密钥立即失效，需要同步更新项目后端。</p
        ></section
      >
      <section v-if="kind === 'keys' && id && !form.revoked" class="dy-panel">
        <h2>轮换 API 凭证</h2
        ><p class="dy-muted"
          >新旧凭证在过渡期内均可用，权限和有效期不变。请保存新凭证并更新接入项目。</p
        >
        <p v-if="form.previous_until" class="dy-muted"
          >旧凭证截止：{{ dateText(form.previous_until) }}</p
        >
        <p class="dy-muted"
          >最近调用：{{ dateText(form.last_used_at) }} · {{ form.last_ip || '暂无来源 IP' }}</p
        >
        <el-form-item label="过渡小时数"
          ><el-input-number v-model="overlapHours" :min="0" :max="168"
        /></el-form-item>
        <el-button :loading="saving" @click="rotateKey">生成新凭证</el-button>
      </section>
    </template>
  </div>
</template>
<script setup lang="ts">
  import { ref, computed, watch, onBeforeUnmount } from 'vue'
  import { useRoute, useRouter, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { api, dateText } from '@/api/duoyu'
  const checks = ref<any[]>([])
  async function checkApp() {
    try {
      const result = await api('/api/admin/apps/' + id.value + '/checks')
      checks.value = result.checks
    } catch (e: any) {
      ElMessage.error(e.message)
    }
  }
  const overlapHours = ref(24)
  async function rotateKey() {
    try {
      await ElMessageBox.confirm('确认轮换此凭证？新凭证只显示一次。', '轮换 API 凭证')
    } catch {
      return
    }
    saving.value = true
    try {
      const result = await api('/api/admin/keys/' + id.value + '/rotate', 'POST', {
        overlap_hours: overlapHours.value
      })
      secret.value = result.secret
      baseline.value = snapshot()
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
  const memberPage = ref(1),
    memberTotal = ref(0)
  const route = useRoute(),
    router = useRouter(),
    kind = computed(() => route.path.split('/')[2]),
    view = computed(() => route.path.split('/')[3]),
    id = computed(() => String(route.query.id || ''))
  const form = ref<any>({}),
    apps = ref<any[]>([]),
    members = ref<any[]>([]),
    grantText = ref(''),
    loading = ref(true),
    saving = ref(false),
    error = ref(''),
    secret = ref(''),
    baseline = ref(''),
    changeExpiry = ref(false)
  const title = computed(() =>
    view.value === 'grants'
      ? '项目准入名单'
      : view.value === 'members'
        ? '项目登录用户'
        : kind.value === 'users'
          ? '账号资料'
          : (id.value ? '编辑' : '创建') + (kind.value === 'apps' ? '项目' : ' API 凭证')
  )
  const snapshot = () =>
    JSON.stringify({ form: form.value, grant: grantText.value, change: changeExpiry.value })
  const dirty = computed(
    () =>
      !loading.value &&
      !error.value &&
      baseline.value !== snapshot() &&
      !['members', 'detail'].includes(view.value)
  )
  async function leave() {
    if (!dirty.value && !secret.value) return true
    try {
      await ElMessageBox.confirm(
        secret.value ? '密钥仅显示一次，确认已保存并离开？' : '修改尚未保存，确定离开？',
        '离开页面',
        { type: 'warning' }
      )
      secret.value = ''
      return true
    } catch {
      return false
    }
  }
  onBeforeRouteLeave(leave)
  onBeforeRouteUpdate(leave)
  const unload = (e: BeforeUnloadEvent) => {
    if (dirty.value || secret.value) {
      e.preventDefault()
      e.returnValue = ''
    }
  }
  window.addEventListener('beforeunload', unload)
  onBeforeUnmount(() => window.removeEventListener('beforeunload', unload))
  const back = () => router.push('/manage/' + kind.value)
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    secret.value = ''
    changeExpiry.value = false
    grantText.value = ''
    try {
      let f: any = {
        name: '',
        description: '',
        public: false,
        enabled: true,
        restricted: false,
        refresh_enabled: false,
        platform: 'web',
        backchannel_logout_uri: '',
        redirectText: '',
        logoutText: '',
        permanent: true,
        scopes: ['app:read'],
        days: 30,
        app_id: ''
      }
      if (id.value) {
        const rows = await api('/api/admin/' + kind.value + '/' + encodeURIComponent(id.value))
        if (!rows.length) throw Error('记录不存在或已删除')
        f = {
          ...f,
          ...rows[0],
          redirectText: (rows[0].redirects || []).join('\n'),
          logoutText: (rows[0].logout_redirects || []).join('\n'),
          permanent: !rows[0].expires_at
        }
      }
      if (current !== generation) return
      form.value = f
      if (view.value === 'grants') {
        const data = await api<any[]>('/api/admin/apps/' + id.value + '/grants')
        if (current !== generation) return
        grantText.value = data.map((x) => x.email).join('\n')
      }
      if (view.value === 'members') {
        const data = await api<any>(
          '/api/admin/apps/' + id.value + '/users?page=' + memberPage.value + '&page_size=20'
        )
        if (current !== generation) return
        members.value = data.items
        memberTotal.value = data.total
      }
      if (kind.value === 'keys' && !id.value) {
        const data = await api<any[]>('/api/admin/apps')
        if (current !== generation) return
        apps.value = data.filter((x) => x.enabled)
      }
      baseline.value = snapshot()
    } catch (e: any) {
      if (current === generation) error.value = e.message
    } finally {
      if (current === generation) loading.value = false
    }
  }
  watch(() => route.fullPath, load, { immediate: true })
  const lines = (v: string) => [
    ...new Set(
      v
        .split('\n')
        .map((x) => x.trim())
        .filter(Boolean)
    )
  ]
  async function save() {
    if (saving.value) return
    if (view.value !== 'grants' && !form.value.name.trim()) {
      ElMessage.error('请输入名称')
      return
    }
    if (kind.value === 'apps' && view.value === 'edit' && !lines(form.value.redirectText).length) {
      ElMessage.error('请填写登录回调地址')
      return
    }
    saving.value = true
    try {
      let out: any
      if (view.value === 'grants') {
        out = await api('/api/admin/apps/' + id.value + '/grants', 'PUT', {
          emails: lines(grantText.value)
        })
      } else if (kind.value === 'apps') {
        const f = form.value
        out = await api(
          '/api/admin/apps' + (id.value ? '/' + id.value : ''),
          id.value ? 'PUT' : 'POST',
          {
            name: f.name.trim(),
            description: f.description,
            public: f.public,
            enabled: f.enabled,
            restricted: f.restricted,
            refresh_enabled: f.refresh_enabled,
            backchannel_logout_uri: f.backchannel_logout_uri,
            platform: f.public ? (f.platform === 'web' ? 'spa' : f.platform) : 'web',
            redirects: lines(f.redirectText),
            logout_redirects: lines(f.logoutText)
          }
        )
      } else {
        const f = form.value
        out = await api(
          '/api/admin/keys' + (id.value ? '/' + id.value : ''),
          id.value ? 'PUT' : 'POST',
          id.value
            ? {
                name: f.name.trim(),
                permanent: f.permanent,
                days: f.days,
                change_expiry: changeExpiry.value,
                scopes: f.scopes
              }
            : {
                name: f.name.trim(),
                app_id: f.app_id,
                permanent: f.permanent,
                days: f.days,
                scopes: f.scopes
              }
        )
      }
      baseline.value = snapshot()
      ElMessage.success('已保存')
      if (out?.secret) {
        secret.value = out.secret
        window.scrollTo(0, 0)
      } else back()
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
  async function rotate() {
    try {
      await ElMessageBox.confirm('旧密钥立即失效，确定轮换？', '轮换密钥', { type: 'warning' })
      saving.value = true
      const out = await api('/api/admin/apps/' + id.value + '/rotate', 'POST', {})
      secret.value = out.secret
    } catch (e: any) {
      if (e instanceof Error) ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(secret.value)
      ElMessage.success('已复制')
    } catch {
      ElMessage.warning('请手动复制并保存')
    }
  }
  function finish() {
    secret.value = ''
    back()
  }
</script>
