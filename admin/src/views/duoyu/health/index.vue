<template>
  <div class="dy-page">
    <header class="dy-heading"
      ><div><h1>运行状态</h1><p>查看认证配置、队列积压与退出通知记录</p></div
      ><el-button :loading="loading" @click="load">刷新状态</el-button></header
    >
    <div v-if="error" class="dy-error" role="alert"
      >{{ error }} <el-button text @click="load">重试</el-button></div
    >
    <div class="dy-stats" v-loading="loading">
      <div v-for="item in metrics" :key="item.label" class="dy-stat"
        ><span>{{ item.label }}</span
        ><strong>{{ diagnostic[item.key] ?? '—' }}</strong
        ><span>{{ item.note }}</span></div
      >
    </div>
    <section class="dy-panel" v-if="diagnostic.alerts?.length" role="status">
      <h2>需要处理</h2><p v-for="alert in diagnostic.alerts" :key="alert">{{ alert }}</p>
    </section>
    <div class="dy-grid">
      <section class="dy-panel"
        ><h2>认证配置</h2
        ><dl class="dy-facts"
          ><div
            ><dt>签发者</dt><dd class="dy-code">{{ diagnostic.issuer || '—' }}</dd></div
          ><div
            ><dt>运行模式</dt
            ><dd>{{
              diagnostic.issuer ? (diagnostic.development ? '本地开发' : '生产环境') : '—'
            }}</dd></div
          ><div
            ><dt>访问令牌</dt
            ><dd>{{
              diagnostic.access_token_seconds ? diagnostic.access_token_seconds / 60 + ' 分钟' : '—'
            }}</dd></div
          ><div
            ><dt>中心会话</dt
            ><dd>{{
              diagnostic.session_seconds ? diagnostic.session_seconds / 3600 + ' 小时' : '—'
            }}</dd></div
          ><div
            ><dt>签名密钥 ID</dt><dd class="dy-code">{{ diagnostic.signing_kid || '—' }}</dd></div
          ></dl
        ><p class="dy-muted"
          >退出积压 {{ diagnostic.logout_oldest_seconds ?? '—' }} 秒 · 邮件积压
          {{ diagnostic.mail_oldest_seconds ?? '—' }} 秒</p
        >
        <p class="dy-muted"
          >数据库
          {{
            diagnostic.database_bytes ? (diagnostic.database_bytes / 1048576).toFixed(1) : '—'
          }}
          MB · 24 小时退出投递 P95 {{ diagnostic.logout_p95_ms ?? '—' }} ms</p
        >
        <p class="dy-muted">配置可读取不代表公网证书、手机扫码或邮件送达已经通过验收。</p></section
      >
      <section class="dy-panel"
        ><h2>处理建议</h2
        ><p class="dy-muted">后台监控更新：{{ dateText(diagnostic.monitor_checked_at) }}</p
        ><div class="dy-listitem"
          ><div>退出通知失败<p>检查项目回调、证书和验签配置。达到重试上限后需人工处理。</p></div
          ><el-button text @click="status = 'failed'">查看</el-button></div
        ><div class="dy-listitem"
          ><div>邮件结果不确定<p>先核对发送记录，避免重复通知或验证码。</p></div
          ><el-button v-if="superAdmin" text @click="$router.push('/manage/mail')">邮件记录</el-button></div
        ><div class="dy-listitem"
          ><div>接入异常<p>核对客户端类型、回调地址、PKCE 和准入规则。</p></div
          ><el-button v-if="superAdmin" text @click="$router.push('/manage/docs')">接入文档</el-button></div
        ><p class="dy-muted">{{ updated ? '最近刷新：' + updated : '等待首次读取' }}</p></section
      >
    </div>
    <section class="dy-panel"
      ><div class="dy-heading"
        ><div><h2>退出通知</h2><p>只展示元数据；重复投递由接收项目按事件标识处理。</p></div
        ><el-select
          v-model="status"
          aria-label="退出通知状态"
          placeholder="全部状态"
          style="width: 160px"
          ><el-option label="全部状态" value="" /><el-option
            label="等待重试"
            value="queued" /><el-option label="已接收" value="delivered" /><el-option
            label="发送失败"
            value="failed" /></el-select
      ></div>
      <el-table
        :data="rows"
        v-loading="loading"
        empty-text="暂无符合条件的退出通知"
        @expand-change="loadAttempts"
        ><el-table-column type="expand"
          ><template #default="{ row }"
            ><div style="padding: 16px"
              ><p v-if="attemptErrors[row.id]" role="alert">{{ attemptErrors[row.id] }}</p
              ><el-table :data="attemptRows[row.id] || []" empty-text="暂无尝试记录"
                ><el-table-column label="时间" min-width="180"
                  ><template #default="{ row: attempt }">{{
                    dateText(attempt.created_at)
                  }}</template></el-table-column
                ><el-table-column prop="http_status" label="HTTP" /><el-table-column
                  prop="error_class"
                  label="错误类别" /><el-table-column
                  prop="duration_ms"
                  label="耗时（ms）" /></el-table></div></template></el-table-column
        ><el-table-column prop="id" label="事件" width="85" /><el-table-column
          prop="app_id"
          label="项目标识"
          min-width="210"
          show-overflow-tooltip
        /><el-table-column label="状态" width="120"
          ><template #default="{ row }"
            ><el-tag :type="row.status === 'failed' ? 'danger' : 'info'">{{
              labels[row.status] || row.status
            }}</el-tag></template
          ></el-table-column
        ><el-table-column prop="attempts" label="尝试次数" width="100" /><el-table-column
          label="创建时间"
          min-width="180"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        ><el-table-column label="下次尝试" min-width="180"
          ><template #default="{ row }">{{
            row.status === 'queued' ? dateText(row.next_attempt) : '—'
          }}</template></el-table-column
        ></el-table
      >
      <el-pagination
        v-model:current-page="page"
        :page-size="20"
        :total="total"
        layout="total, prev, pager, next"
      />
    </section>
  </div>
</template>
<script setup lang="ts">
  import { ref, onMounted, watch } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useUserStore } from '@/store/modules/user'
  import { api, dateText } from '@/api/duoyu'
  const superAdmin = useUserStore().info.roles?.includes('R_SUPER')
  const diagnostic = ref<Record<string, any>>({}),
    rows = ref<any[]>([]),
    page = ref(1),
    total = ref(0),
    status = ref(''),
    loading = ref(false),
    error = ref(''),
    updated = ref('')
  const labels: Record<string, string> = {
    queued: '等待重试',
    delivered: '已接收',
    failed: '发送失败'
  }
  const metrics = [
    { label: '待发退出通知', key: 'logout_queued', note: '等待发送或重试' },
    { label: '失败退出通知', key: 'logout_failed', note: '达到重试上限' },
    { label: '待发邮件', key: 'mail_pending', note: '认证与项目邮件队列' },
    { label: '邮件结果不确定', key: 'mail_unknown', note: '需核对后再处理' }
  ]
  const attemptRows = ref<Record<string, any[]>>({}),
    attemptErrors = ref<Record<string, string>>({})
  async function loadAttempts(row: any, expanded: any[]) {
    if (!expanded.includes(row)) return
    attemptErrors.value[row.id] = ''
    try {
      attemptRows.value[row.id] = await api('/api/admin/logout-events/' + row.id + '/attempts')
    } catch (e: any) {
      attemptErrors.value[row.id] = e.message
    }
  }
  const retrying = ref(false)
  async function retry(row: any) {
    try {
      await ElMessageBox.confirm('重新投递同一退出事件，接收项目需按事件标识去重。', '重新投递', {
        type: 'warning'
      })
    } catch {
      return
    }
    retrying.value = true
    try {
      await api('/api/admin/logout-events/' + row.id + '/retry', 'POST', {})
      ElMessage.success('已加入重试队列')
      await load()
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      retrying.value = false
    }
  }
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const [d, list] = await Promise.all([
        api('/api/admin/diagnostics'),
        api(
          '/api/admin/logout-events?page=' +
            page.value +
            '&page_size=20&status=' +
            encodeURIComponent(status.value)
        )
      ])
      if (current !== generation) return
      diagnostic.value = d
      rows.value = list.items
      total.value = list.total
      updated.value = new Date().toLocaleString('zh-CN')
    } catch (e: any) {
      if (current === generation) error.value = e.message
    } finally {
      if (current === generation) loading.value = false
    }
  }
  watch(page, load)
  watch(status, () => {
    if (page.value !== 1) page.value = 1
    else load()
  })
  onMounted(load)
</script>
