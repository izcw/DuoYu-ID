<template>
  <div class="dy-page art-page-view" v-loading="loading">
    <div class="dy-heading"
      ><div
        ><el-button text class="dy-back-button" @click="back"
          >← 返回{{
            kind === 'apps' ? '项目管理' : kind === 'keys' ? 'API 凭证' : '账号管理'
          }}</el-button
        ><h1>{{ title }}</h1
        ><p v-if="form.id" class="dy-code">{{
          kind === 'users' ? form.account_id : form.id
        }}</p></div
      ></div
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
        ><el-table :data="members" empty-text="暂无登录记录"
          ><el-table-column
            type="index"
            label="序号"
            width="76"
            :index="(index: number) => (memberPage - 1) * 20 + index + 1" /><el-table-column
            label="头像"
            width="76"
            ><template #default="{ row }"
              ><el-avatar :src="row.avatar || '/avatar-default.svg'" :size="36" alt="用户头像"
                ><img
                  src="/avatar-default.svg"
                  alt="默认头像" /></el-avatar></template></el-table-column
          ><el-table-column prop="email" label="邮箱" min-width="200" /><el-table-column
            prop="name"
            label="昵称" /><el-table-column prop="role_name" label="项目角色" min-width="120" /><el-table-column label="角色操作" width="110"><template #default="{ row }"><el-button text @click="router.push({path:'/manage/apps/member-role',query:{id,user:row.id}})">修改角色</el-button></template></el-table-column><el-table-column label="首次登录" min-width="170"
            ><template #default="{ row }">{{
              dateText(row.first_login_at)
            }}</template></el-table-column
          ><el-table-column label="最近登录" min-width="170"
            ><template #default="{ row }">{{
              dateText(row.last_login_at)
            }}</template></el-table-column
          ><el-table-column prop="login_count" label="次数" /></el-table
        ><el-pagination
          v-if="memberTotal > 20"
          v-model:current-page="memberPage"
          :page-size="20"
          :total="memberTotal"
          layout="total, prev, pager, next"
          @current-change="load"
      /></section>
      <section v-else-if="kind === 'users'" class="dy-panel"
        ><div class="dy-account-detail-title"><el-avatar :src="form.avatar || '/avatar-default.svg'" :size="48"><img src="/avatar-default.svg" alt="默认头像" /></el-avatar><h2>{{ form.name }}</h2></div><el-descriptions :column="1" border
          ><el-descriptions-item label="账号 ID">{{ form.account_id }}</el-descriptions-item
          ><el-descriptions-item label="密码登录">{{
            form.password_locked_until &&
            new Date(form.password_locked_until).getTime() > Date.now()
              ? '锁定至 ' + dateText(form.password_locked_until)
              : '可用'
          }}</el-descriptions-item
          ><el-descriptions-item label="邮箱">{{ form.email }}</el-descriptions-item
          ><el-descriptions-item label="角色">{{
            form.admin ? '超级管理员' : '普通用户'
          }}</el-descriptions-item
          ><el-descriptions-item label="状态">{{
            form.enabled ? '正常' : '停用'
          }}</el-descriptions-item
          ><el-descriptions-item label="注册时间">{{
            dateText(form.created_at)
          }}</el-descriptions-item></el-descriptions
        ><el-descriptions :column="1" border style="margin-top:16px"><el-descriptions-item label="昵称">{{ form.name }}</el-descriptions-item><el-descriptions-item label="关联项目"><div v-if="form.projects?.length" class="dy-account-projects"><el-tag v-for="project in form.projects" :key="project.id" type="info">{{ project.name }} · {{ project.role_name }}</el-tag></div><span v-else class="dy-muted">暂未关联项目</span></el-descriptions-item></el-descriptions><p class="dy-muted">昵称由用户本人在账号与安全中修改。</p></section
      >
      <el-form v-else label-position="top" @submit.prevent="save" class="dy-form">
        <section v-if="view === 'grants' || view === 'blacklist'" class="dy-panel"
          ><h2>{{ form.name }} · {{ view === 'blacklist' ? '黑名单' : '准入名单' }}</h2
          ><el-alert
            :closable="false"
            :type="view === 'blacklist' ? 'info' : form.restricted ? 'info' : 'warning'"
            :title="
              view === 'blacklist'
                ? '黑名单优先于准入名单，仅限制当前项目。可添加未注册邮箱。'
                : form.restricted
                  ? '仅名单中的账号可以登录此项目'
                  : '项目当前开放准入（黑名单除外），保存准入名单不会自动开启限制'
            "
          /><el-form-item
            :label="view === 'blacklist' ? '账号 ID 或邮箱 · 每行一个' : '已注册邮箱 · 每行一个'"
            style="margin-top: 24px"
            ><el-input
              v-model="grantText"
              type="textarea"
              :rows="12"
              placeholder="user@example.com" /></el-form-item
          ><p class="dy-muted">{{
            view === 'blacklist'
              ? '默认空名单。保存后名单内账号不能注册、登录或续期本项目；已有令牌被撤销。删除条目后需重新登录。'
              : '保存将替换当前名单；空名单配合限制模式会阻止全部账号登录。'
          }}</p></section
        >
        <template v-else>
          <section class="dy-panel"
            ><h2>基本信息</h2
            ><el-form-item label="名称" required
              ><el-input v-model="form.name" maxlength="60" placeholder="输入便于识别的名称"
            /></el-form-item>
            <template v-if="kind === 'apps'"
              ><el-form-item label="项目 Logo" required>
                <div class="dy-logo-editor">
                  <el-image
                    :key="form.logo"
                    :src="
                      form.logo?.startsWith('/project-logos/') ? form.logo : '/project-default.svg'
                    "
                    fit="contain"
                    class="dy-project-logo dy-project-logo-preview"
                    referrerpolicy="no-referrer"
                    ><template #error
                      ><img src="/project-default.svg" alt="默认项目图标" /></template
                  ></el-image>
                  <div class="dy-logo-input">
                    <el-upload
                      accept="image/png,image/jpeg,image/webp"
                      :show-file-list="false"
                      :auto-upload="false"
                      :on-change="uploadLogo"
                      :disabled="logoUploading"
                    >
                      <el-button :loading="logoUploading">{{
                        form.logo?.startsWith('/project-logos/') ? '替换 Logo' : '上传 Logo'
                      }}</el-button> </el-upload
                    ><p class="dy-muted"
                      >必填，支持 PNG、JPEG、WebP，最大 2
                      MB；图片保存在账号中心，保持原比例与透明背景。</p
                    ></div
                  >
                </div> </el-form-item
              ><el-form-item label="描述"
                ><el-input v-model="form.description" type="textarea" maxlength="1000" :rows="3"
              /></el-form-item>
              <el-form-item label="客户端类型" required>
                <div
                  class="dy-client-options"
                  role="radiogroup"
                  aria-label="客户端类型"
                  @keydown="clientTypeKey"
                >
                  <button
                    type="button"
                    role="radio"
                    aria-label="服务端网站"
                    :aria-checked="!form.public"
                    :tabindex="!form.public ? 0 : -1"
                    :disabled="!!id"
                    :class="{ selected: !form.public }"
                    @click="form.public = false"
                  >
                    <b>服务端网站</b
                    ><p>有自己的后端，密钥保存在服务器。适合 Go、Java、Node 等后端网站。</p
                    ><span>client_id + client_secret + PKCE</span>
                  </button>
                  <button
                    type="button"
                    role="radio"
                    aria-label="公共客户端"
                    :aria-checked="!!form.public"
                    :tabindex="form.public ? 0 : -1"
                    :disabled="!!id"
                    :class="{ selected: form.public }"
                    @click="form.public = true"
                  >
                    <b>公共客户端</b
                    ><p>无法安全保存密钥，适合浏览器或原生应用，请同时选择对应平台。</p
                    ><span>client_id + PKCE，无 client_secret</span>
                  </button>
                </div> </el-form-item
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
            <el-form-item label="允许的登录方式"
              ><el-checkbox-group v-model="form.login_methods" class="dy-choice-options"
                ><el-checkbox value="code"><span class="dy-choice-icon" aria-hidden="true"><ArtSvgIcon icon="ri:mail-line" /></span><span>邮箱验证码</span></el-checkbox
                ><el-checkbox value="password"><span class="dy-choice-icon" aria-hidden="true"><ArtSvgIcon icon="ri:lock-password-line" /></span><span>账号密码</span></el-checkbox
                ><el-checkbox value="passkey"><span class="dy-choice-icon" aria-hidden="true"><img src="/passkey.png" alt="" /></span><span>通行密钥</span></el-checkbox
                ><el-checkbox value="qr"><span class="dy-choice-icon" aria-hidden="true"><img src="/iphone.png" alt="" /></span><span>iPhone 扫码</span></el-checkbox><el-checkbox value="totp"><span class="dy-choice-icon" aria-hidden="true"><img src="/authenticator.png" alt="" /></span><span>身份验证器</span></el-checkbox></el-checkbox-group
              ></el-form-item
            ><p class="dy-muted"
              >默认全部启用；关闭后项目登录页隐藏对应入口，已有账号也须使用允许的方式重新验证。全部取消将暂停该项目登录。个人账号页不能修改此策略，仍可管理自己的密码和通行密钥。</p
            >
            <el-form-item v-if="form.public" label="应用平台"
              ><el-radio-group v-model="form.platform" class="dy-choice-options"
                ><el-radio value="spa"><span class="dy-choice-icon" aria-hidden="true"><ArtSvgIcon icon="ri:window-line" /></span><span>浏览器 SPA</span></el-radio
                ><el-radio value="native"><span class="dy-choice-icon" aria-hidden="true"><ArtSvgIcon icon="ri:device-line" /></span><span>桌面 / 手机原生应用</span></el-radio
                ><el-radio value="extension"><span class="dy-choice-icon" aria-hidden="true"><ArtSvgIcon icon="ri:puzzle-line" /></span><span>浏览器扩展</span></el-radio></el-radio-group
              ></el-form-item
            >
            <div v-if="form.platform === 'extension'">
              <p class="dy-muted"
                >公共客户端，无客户端密钥；使用 S256 PKCE。Chrome / Edge 回调取自
                chrome.identity.getRedirectURL('oidc')，不能填写 chrome-extension:// 页面地址。</p
              >
              <el-form-item label="Chrome / Edge 扩展 ID · 可选生成配置"
                ><el-input v-model="extensionID" placeholder="32 位扩展 ID" /><el-button
                  @click="applyExtension"
                  >填入扩展回调与请求来源</el-button
                ></el-form-item
              >
              <p class="dy-muted"
                >Firefox 请填写 identity.getRedirectURL() 的实际 HTTPS 回调，并单独登记
                moz-extension:// 来源。安装目录或发布身份改变后需重新核对。</p
              >
            </div>
            <p v-if="form.platform === 'native'" class="dy-muted"
              >使用系统浏览器和 PKCE；支持 HTTPS、127.0.0.1 / [::1]
              回环地址（允许动态端口），或反向域名形式的私有 scheme，如
              com.example.app:/callback。手机实机仍需验收。</p
            >
            <div class="dy-listitem"
              ><div
                >允许会话续期<p
                  >显式请求 offline_access；刷新令牌每次轮换，重放会撤销整个令牌家族。中心会话最长
                  12 小时，刷新不会延长，不等于永久记住登录。</p
                ></div
              ><el-switch v-model="form.refresh_enabled" /></div
            ><el-form-item label="登录回调地址 · 每行一个" required
              ><el-input
                v-model="form.redirectText"
                type="textarea"
                :rows="4"
                placeholder="https://app.example.com/auth/callback"
            /></el-form-item>
            <el-form-item label="允许的浏览器请求来源 · 每行一个"
              ><el-input
                v-model="form.originText"
                type="textarea"
                :rows="3"
                placeholder="https://app.example.com" /></el-form-item
            ><p class="dy-muted"
              >独立于登录和退出回调。只填写 origin，不含路径、末尾斜杠或通配符；扩展可填
              chrome-extension://扩展ID。留空不允许跨来源浏览器请求，不影响不带 Origin
              的后端请求。</p
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
            >{{
              ['grants', 'blacklist'].includes(view) ? '保存名单' : id ? '保存修改' : '创建'
            }}</el-button
          ></div
        >
      </el-form>
      <section
        v-if="kind === 'apps' && id && view === 'edit'"
        class="dy-panel"
        style="margin-top: 24px"
        ><h2>项目管理</h2><el-button @click="copyProjectAI">复制 AI 接入资料</el-button><el-button @click="checkApp">检查接入配置</el-button
        ><div v-if="checks.length"
          ><p v-for="item in checks" :key="item.label"
            >{{ item.passed ? '✓' : '待检查' }} · {{ item.label }}</p
          ><p class="dy-muted">仅校验配置，不代表回调网络或项目验签通过。</p></div
        ><el-button @click="router.push({ path: '/manage/apps/roles', query: { id } })">角色配置</el-button
        ><el-button @click="router.push({ path: '/manage/apps/grants', query: { id } })"
          >准入名单</el-button
        ><el-button @click="router.push({ path: '/manage/apps/blacklist', query: { id } })"
          >黑名单</el-button
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
 import {copyDuoyuText} from '@/utils/duoyu-clipboard'
 async function copyProjectAI(){try{const doc=await api('/api/admin/ai-integration');const a=form.value;await copyDuoyuText(doc.guide+'\n\n## 当前已登记项目（优先更新，不要重复创建）\n'+JSON.stringify({client_id:id.value,name:a.name,public:a.public,platform:a.platform,redirects:a.redirects,allowed_origins:a.allowed_origins,login_methods:a.login_methods},null,2)+'\n请从本地环境读取管理凭证；以上内容不包含任何密钥。');ElMessage.success('项目 AI 接入资料已复制')}catch(e:any){ElMessage.error(e.message)}}
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
  function clientTypeKey(event: KeyboardEvent) {
    if (
      id.value ||
      !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)
    )
      return
    event.preventDefault()
    form.value.public =
      event.key === 'Home' ? false : event.key === 'End' ? true : !form.value.public
    const group = event.currentTarget as HTMLElement
    group.querySelectorAll<HTMLButtonElement>('[role="radio"]')[form.value.public ? 1 : 0].focus()
  }
  const logoUploading = ref(false)
  async function uploadLogo(upload: { raw?: File }) {
    const file = upload.raw
    if (!file) return
    if (file.size > 2 * 1024 * 1024) {
      ElMessage.error('Logo 不能超过 2 MB')
      return
    }
    const body = new FormData()
    body.append('logo', file)
    logoUploading.value = true
    try {
      const out = await api('/api/admin/project-logos', 'POST', body)
      form.value.logo = out.logo
      ElMessage.success('Logo 已上传，请保存项目')
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      logoUploading.value = false
    }
  }
  const extensionID = ref('')
  function applyExtension() {
    const id = extensionID.value.trim()
    if (!/^[a-p]{32}$/.test(id)) {
      ElMessage.error('请填写有效的 32 位扩展 ID')
      return
    }
    form.value.public = true
    form.value.platform = 'extension'
    form.value.redirectText = `https://${id}.chromiumapp.org/oidc`
    form.value.originText = `chrome-extension://${id}`
  }
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
    view.value === 'blacklist'
      ? '项目黑名单'
      : view.value === 'grants'
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
      !loading.value && !error.value && baseline.value !== snapshot() && view.value !== 'members'
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
        logo: '',
        public: false,
        enabled: true,
        restricted: false,
        refresh_enabled: false,
        login_methods: ['password', 'code', 'passkey', 'qr', 'totp'],
        platform: 'web',
        backchannel_logout_uri: '',
        redirectText: '',
        logoutText: '',
        originText: '',
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
          originText: (rows[0].allowed_origins || []).join('\n'),
          permanent: !rows[0].expires_at
        }
      }
      if (current !== generation) return
      form.value = f
      if (view.value === 'blacklist') {
        const data = await api<any[]>('/api/admin/apps/' + id.value + '/blacklist')
        if (current !== generation) return
        grantText.value = data.map((x) => x.identifier).join('\n')
      }
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
        if (memberPage.value > Math.max(1, Math.ceil(data.total / 20))) {
          memberPage.value = Math.max(1, Math.ceil(data.total / 20))
          await load()
        }
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
    if (
      kind.value === 'apps' &&
      view.value === 'edit' &&
      (logoUploading.value || !form.value.logo?.startsWith('/project-logos/'))
    ) {
      ElMessage.error('请先上传项目 Logo')
      return
    }
    if (kind.value === 'apps' && view.value === 'edit' && !lines(form.value.redirectText).length) {
      ElMessage.error('请填写登录回调地址')
      return
    }
    saving.value = true
    try {
      let out: any
      if (view.value === 'blacklist') {
        out = await api('/api/admin/apps/' + id.value + '/blacklist', 'PUT', {
          entries: lines(grantText.value)
        })
      } else if (view.value === 'grants') {
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
            logo: f.logo.trim(),
            public: f.public,
            enabled: f.enabled,
            restricted: f.restricted,
            refresh_enabled: f.refresh_enabled,
            login_methods: f.login_methods,
            backchannel_logout_uri: f.backchannel_logout_uri,
            platform: f.public ? (f.platform === 'web' ? 'spa' : f.platform) : 'web',
            redirects: lines(f.redirectText),
            logout_redirects: lines(f.logoutText),
            allowed_origins: lines(f.originText)
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
