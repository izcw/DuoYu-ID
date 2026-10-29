<template>
  <div class="dy-page art-page-view"
    ><div class="dy-heading"
      ><div
        ><h1>{{ meta.title }}</h1
        ><p>{{ meta.description }}</p></div
      ><el-button
        v-if="superAdmin && (kind === 'apps' || kind === 'keys')"
        type="primary"
        @click="create"
        >＋ {{ kind === 'apps' ? '创建项目' : '创建 API 凭证' }}</el-button
      ></div
    ><div class="dy-error" v-if="error">{{ error }}</div
    ><section class="dy-panel"
      ><div class="dy-toolbar"
        ><el-input
          v-model="search"
          placeholder="搜索名称、邮箱或标识"
          clearable
          @keyup.enter="load"
        /><el-button @click="load" :loading="loading">刷新</el-button
        ><span class="dy-muted">共 {{ total }} 条</span></div
      >
      <el-table
        v-if="kind === 'users'"
        :data="paged"
        v-loading="loading"
        empty-text="没有匹配的账号"
        ><el-table-column
          type="index"
          label="序号"
          width="76"
          :index="(index: number) => (page - 1) * 20 + index + 1"
        /><el-table-column label="账号" min-width="220"
          ><template #default="{ row }"
            ><div style="display: flex; gap: 12px; align-items: center"
              ><el-avatar :src="row.avatar || '/avatar-default.svg'" :size="36"><img src="/avatar-default.svg" alt="默认头像" /></el-avatar><div
                ><b>{{ row.name }}</b
                ><span v-if="row.admin" class="dy-super-badge">超级管理员</span
                ><div class="dy-code">ID {{ row.account_id }}</div
                ><div class="dy-muted">{{ row.email }}</div></div
              ></div
            ></template
          ></el-table-column
        ><el-table-column label="关联项目" min-width="230"><template #default="{ row }"><div v-if="row.projects?.length" class="dy-account-projects"><el-tooltip v-for="project in row.projects" :key="project.id" :content="project.name + ' · ' + project.role_name" placement="top"><router-link class="dy-account-project-link" :to="{ path: '/manage/apps/edit', query: { id: project.id } }" :aria-label="'管理项目：' + project.name"><el-image :src="project.logo?.startsWith('/project-logos/') ? project.logo : '/project-default.svg'" fit="contain" :alt="project.name"><template #error><img src="/project-default.svg" alt="默认项目图标" /></template></el-image></router-link></el-tooltip></div><span v-else class="dy-muted">暂未关联项目</span></template></el-table-column
        ><el-table-column label="角色" width="140"
          ><template #default="{ row }"
            ><el-tag :type="row.admin ? 'warning' : 'info'">{{
              row.admin ? '超级管理员' : '用户'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column label="状态" width="120"
          ><template #default="{ row }"
            ><el-tag
              :type="
                !row.enabled
                  ? 'danger'
                  : row.password_locked_until &&
                      new Date(row.password_locked_until).getTime() > Date.now()
                    ? 'warning'
                    : 'success'
              "
              >{{
                !row.enabled
                  ? '已停用'
                  : row.password_locked_until &&
                      new Date(row.password_locked_until).getTime() > Date.now()
                    ? '密码锁定'
                    : '正常'
              }}</el-tag
            ></template
          ></el-table-column
        ><el-table-column label="注册时间" min-width="180"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        ><el-table-column
          label="操作"
          width="300"
          :fixed="width >= 768 ? 'right' : false"
          class-name="dy-actions"
          ><template #default="{ row }"
            ><el-button text @click="showDetail(row)">详情</el-button
            ><el-button text :disabled="row.admin" @click="toggleUser(row)">{{
              row.enabled ? '停用' : '恢复'
            }}</el-button
            ><el-button text type="danger" @click="revokeUser(row)">撤销会话</el-button></template
          ></el-table-column
        ></el-table
      >
      <el-table
        v-else-if="kind === 'apps'"
        :data="paged"
        v-loading="loading"
        empty-text="还没有项目，点击右上角创建"
        ><el-table-column
          type="index"
          label="序号"
          width="76"
          :index="(index: number) => (page - 1) * 20 + index + 1"
        /><el-table-column label="接入项目" min-width="230"
          ><template #default="{ row }"
            ><div class="dy-project-identity"
              ><el-image
                :src="row.logo?.startsWith('/project-logos/') ? row.logo : '/project-default.svg'"
                fit="contain"
                class="dy-project-logo"
                referrerpolicy="no-referrer"
                ><template #error
                  ><img src="/project-default.svg" alt="默认项目图标" /></template></el-image
              ><div
                ><b>{{ row.name }}</b
                ><div class="dy-code">{{ row.id }}</div></div
              ></div
            ></template
          ></el-table-column
        ><el-table-column label="客户端类型" width="150"
          ><template #default="{ row }">{{
            row.public ? '公共客户端 / PKCE' : '服务端网站'
          }}</template></el-table-column
        ><el-table-column label="访问策略" width="120"
          ><template #default="{ row }">{{
            row.restricted ? '指定账号' : '开放准入'
          }}</template></el-table-column
        ><el-table-column label="状态" width="100"
          ><template #default="{ row }"
            ><el-tag :type="row.enabled ? 'success' : 'info'">{{
              row.enabled ? '启用' : '停用'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column
          label="操作"
          width="560"
          :fixed="width >= 768 ? 'right' : false"
          class-name="dy-actions"
          ><template #default="{ row }"
            ><el-button text @click="edit(row)">配置</el-button
            ><el-button text @click="router.push({ path: '/manage/apps/roles', query: { id: row.id } })">角色配置</el-button
            ><el-button text @click="grants(row)">准入名单</el-button
            ><el-button
              text
              @click="router.push({ path: '/manage/apps/blacklist', query: { id: row.id } })"
              >黑名单</el-button
            ><el-button text @click="showAppUsers(row)">登录用户</el-button
            ><el-button text :disabled="row.public" @click="rotate(row)"
              >轮换密钥</el-button
            ></template
          ></el-table-column
        ></el-table
      >
      <el-table
        v-else-if="kind === 'keys'"
        :data="paged"
        v-loading="loading"
        empty-text="还没有 API 凭证"
        ><el-table-column
          type="index"
          label="序号"
          width="76"
          :index="(index: number) => (page - 1) * 20 + index + 1"
        /><el-table-column prop="name" label="凭证名称" min-width="160" /><el-table-column
          prop="app_name"
          label="所属项目"
          min-width="150"
        /><el-table-column label="权限" width="130"
          ><template #default="{ row }"
            ><el-tag type="info">{{ row.scopes.join(', ') }}</el-tag></template
          ></el-table-column
        ><el-table-column label="状态" width="110"
          ><template #default="{ row }"
            ><el-tag
              :type="
                row.revoked
                  ? 'danger'
                  : row.expires_at && Date.now() > new Date(row.expires_at).getTime()
                    ? 'warning'
                    : 'success'
              "
              >{{
                row.revoked
                  ? '已撤销'
                  : row.expires_at && Date.now() > new Date(row.expires_at).getTime()
                    ? '已过期'
                    : '有效'
              }}</el-tag
            ></template
          ></el-table-column
        ><el-table-column label="到期时间" min-width="180"
          ><template #default="{ row }">{{
            row.expires_at ? dateText(row.expires_at) : '永久有效'
          }}</template></el-table-column
        ><el-table-column label="最近调用" min-width="180"
          ><template #default="{ row }">{{ dateText(row.last_used_at) }}</template></el-table-column
        ><el-table-column label="操作" width="180" class-name="dy-actions"
          ><template #default="{ row }"
            ><el-button text :disabled="row.revoked" @click="edit(row)">编辑</el-button
            ><el-button text type="danger" :disabled="row.revoked" @click="revokeKey(row)"
              >撤销</el-button
            ></template
          ></el-table-column
        ></el-table
      >
      <el-table v-else :data="paged" v-loading="loading" empty-text="暂无日志"
        ><el-table-column
          type="index"
          label="序号"
          width="76"
          :index="(index: number) => (page - 1) * 20 + index + 1"
        /><el-table-column
          prop="request_id"
          label="请求标识"
          min-width="190"
          show-overflow-tooltip
        /><el-table-column prop="actor" label="操作账号" min-width="190" /><el-table-column
          prop="action"
          label="事件"
          min-width="190"
        /><el-table-column
          prop="target"
          label="目标"
          min-width="200"
          show-overflow-tooltip
        /><el-table-column label="结果" width="100"
          ><template #default="{ row }"
            ><el-tag :type="row.success ? 'success' : 'danger'">{{
              row.success ? '成功' : '失败'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column label="时间" min-width="180"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        ></el-table
      >
      <el-pagination
        v-if="total > 20"
        v-model:current-page="page"
        :page-size="20"
        :total="total"
        layout="total, prev, pager, next"
        style="justify-content: flex-end"
    /></section>
  </div>
</template>
<script setup lang="ts">
  import { ref, computed, watch, onBeforeUnmount } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { useWindowSize } from '@vueuse/core'
  const { width } = useWindowSize()
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useUserStore } from '@/store/modules/user'
  import { api, dateText } from '@/api/duoyu'
  const superAdmin = useUserStore().info.roles?.includes('R_SUPER')
  const route = useRoute(),
    router = useRouter(),
    kind = computed(() => route.path.split('/').pop() || 'users')
  const definitions: Record<string, { title: string; description: string }> = {
    users: { title: '账号管理', description: '统一账号、状态与登录会话' },
    apps: { title: '项目管理', description: '接入配置、访问权限与项目用户' },
    keys: { title: 'API 凭证', description: '按项目管理服务端访问，支持永久有效与随时撤销' },
    audit: { title: '安全日志', description: '查看账号、项目和凭证的操作记录' }
  }
  const meta = computed(() => definitions[kind.value] || definitions.users),
    rows = ref<any[]>([]),
    search = ref(''),
    loading = ref(false),
    error = ref(''),
    page = ref(1),
    total = ref(0)
  const paged = computed(() => rows.value)
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | undefined
  let generation = 0
  watch(search, () => {
    clearTimeout(timer)
    controller?.abort()
    generation++
    timer = setTimeout(() => {
      if (page.value !== 1) page.value = 1
      else load()
    }, 300)
  })
  watch(page, () => load())
  onBeforeUnmount(() => {
    clearTimeout(timer)
    generation++
    controller?.abort()
  })
  async function load() {
    clearTimeout(timer)
    controller?.abort()
    controller = new AbortController()
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const result = await api(
        '/api/admin/' +
          kind.value +
          ('?page=' + page.value + '&page_size=20&q=' + encodeURIComponent(search.value)),
        'GET',
        undefined,
        controller.signal
      )
      if (current === generation) {
        rows.value = result.items
        total.value = result.total
        if (page.value > Math.max(1, Math.ceil(total.value / 20)))
          page.value = Math.max(1, Math.ceil(total.value / 20))
      }
    } catch (e: any) {
      if (current === generation && e.name !== 'AbortError') error.value = e.message
    } finally {
      if (current === generation) loading.value = false
    }
  }
  watch(
    kind,
    () => {
      search.value = ''
      page.value = 1
      rows.value = []
      load()
    },
    { immediate: true }
  )
  const create = () => router.push('/manage/' + kind.value + '/edit')
  const edit = (r: any) =>
    router.push({ path: '/manage/' + kind.value + '/edit', query: { id: r.id } })
  const showDetail = (r: any) => router.push({ path: '/manage/users/detail', query: { id: r.id } })
  const grants = (r: any) => router.push({ path: '/manage/apps/grants', query: { id: r.id } })
  const showAppUsers = (r: any) =>
    router.push({ path: '/manage/apps/members', query: { id: r.id } })
  const rotate = (r: any) => router.push({ path: '/manage/apps/edit', query: { id: r.id } })
  async function confirmed(text: string, action: () => Promise<any>) {
    try {
      await ElMessageBox.confirm(text, '确认操作', { type: 'warning' })
      await action()
      ElMessage.success('操作成功')
      await load()
    } catch (e: any) {
      if (e instanceof Error) ElMessage.error(e.message)
    }
  }
  const toggleUser = (r: any) =>
    confirmed((r.enabled ? '停用' : '恢复') + '账号 ' + r.email + '？', () =>
      api('/api/admin/users/' + r.id, 'PATCH', { enabled: !r.enabled })
    )
  const revokeUser = (r: any) =>
    confirmed('撤销此账号的中心会话与访问令牌？', () =>
      api('/api/admin/users/' + r.id + '/revoke', 'POST', {})
    )
  const revokeKey = (r: any) =>
    confirmed('撤销后不可恢复，确定撤销此凭证？', () => api('/api/admin/keys/' + r.id, 'DELETE'))
</script>

<style scoped>
  :deep(.dy-actions .cell) {
    white-space: nowrap;
  }
  :deep(.dy-actions .el-button + .el-button) {
    margin-left: 8px;
  }
</style>
