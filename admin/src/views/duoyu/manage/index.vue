<template>
  <div class="dy-page"
    ><div class="dy-heading"
      ><div
        ><h1>{{ meta.title }}</h1
        ><p>{{ meta.description }}</p></div
      ><el-button v-if="kind === 'apps' || kind === 'keys'" type="primary" @click="create"
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
        ><el-table-column label="账号" min-width="220"
          ><template #default="{ row }"
            ><div style="display: flex; gap: 12px; align-items: center"
              ><el-avatar :src="row.avatar || '/avatar-default.svg'" :size="36" /><div
                ><b>{{ row.name }}</b
                ><div class="dy-muted">{{ row.email }}</div></div
              ></div
            ></template
          ></el-table-column
        ><el-table-column label="角色" width="110"
          ><template #default="{ row }"
            ><el-tag :type="row.admin ? 'warning' : 'info'">{{
              row.admin ? '管理员' : '用户'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column label="状态" width="100"
          ><template #default="{ row }"
            ><el-tag :type="row.enabled ? 'success' : 'danger'">{{
              row.enabled ? '正常' : '已停用'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column label="注册时间" min-width="180"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        ><el-table-column label="操作" width="220" fixed="right"
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
        ><el-table-column label="接入项目" min-width="230"
          ><template #default="{ row }"
            ><b>{{ row.name }}</b
            ><div class="dy-code">{{ row.id }}</div></template
          ></el-table-column
        ><el-table-column label="客户端类型" width="150"
          ><template #default="{ row }">{{
            row.public ? '公共客户端 / PKCE' : '服务端网站'
          }}</template></el-table-column
        ><el-table-column label="访问策略" width="120"
          ><template #default="{ row }">{{
            row.restricted ? '指定账号' : '全部账号'
          }}</template></el-table-column
        ><el-table-column label="状态" width="100"
          ><template #default="{ row }"
            ><el-tag :type="row.enabled ? 'success' : 'info'">{{
              row.enabled ? '启用' : '停用'
            }}</el-tag></template
          ></el-table-column
        ><el-table-column label="操作" width="340" fixed="right"
          ><template #default="{ row }"
            ><el-button text @click="edit(row)">配置</el-button
            ><el-button text @click="grants(row)">准入名单</el-button
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
        ><el-table-column prop="name" label="凭证名称" min-width="160" /><el-table-column
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
        ><el-table-column label="操作" width="140"
          ><template #default="{ row }"
            ><el-button text :disabled="row.revoked" @click="edit(row)">编辑</el-button
            ><el-button text type="danger" :disabled="row.revoked" @click="revokeKey(row)"
              >撤销</el-button
            ></template
          ></el-table-column
        ></el-table
      >
      <el-table v-else :data="paged" v-loading="loading" empty-text="暂无日志"
        ><el-table-column prop="actor" label="操作账号" min-width="190" /><el-table-column
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
      ></section
    >
    <el-pagination
      v-model:current-page="page"
      :page-size="20"
      :total="total"
      layout="total, prev, pager, next"
      style="justify-content: flex-end"
    />
  </div>
</template>
<script setup lang="ts">
  import { ref, computed, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { api, dateText } from '@/api/duoyu'
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
  watch(search, () => { if(page.value !== 1) page.value=1; else load() })
  watch(page, () => load())
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const result = await api(
        '/api/admin/' +
          kind.value +
          ('?page=' + page.value + '&page_size=20&q=' + encodeURIComponent(search.value))
      )
      if (current === generation) {
        rows.value = result.items
        total.value = result.total
      }
    } catch (e: any) {
      if (current === generation) error.value = e.message
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
