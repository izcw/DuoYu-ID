<template>
  <div class="dy-page art-page-view" v-loading="loading">
    <div class="dy-heading"
      ><div><h1>项目邮件</h1><p>统一发送通知、业务验证码和告警</p></div
      ><el-button :loading="loading" @click="load">刷新</el-button></div
    >
    <div v-if="error" class="dy-error" role="alert">{{ error }}</div>
    <section class="dy-panel"
      ><h2>发送记录</h2
      ><p class="dy-muted"
        >支持按项目、收件人和状态查询全部记录。“SMTP
        已接受”不代表已进入收件箱；结果不确定时不要直接重复发送。正文加密暂存于队列，完成或过期后清除；此页面不展示正文。</p
      >
      <div class="dy-toolbar"
        ><el-input
          v-model="search"
          placeholder="搜索项目、收件人或消息 ID"
          clearable
          style="max-width: 360px" /><el-select
          placeholder="全部状态"
          v-model="status"
          style="width: 180px"
          ><el-option label="全部状态" value="" /><el-option
            v-for="(label, key) in statuses"
            :key="key"
            :label="label"
            :value="key" /></el-select
      ></div>
      <el-table :data="rows" empty-text="暂无项目邮件记录"
        ><el-table-column
          type="index"
          label="序号"
          width="76"
          :index="(index: number) => (page - 1) * 20 + index + 1"
        />
        <el-table-column prop="app_name" label="项目" min-width="130" /><el-table-column
          prop="recipient"
          label="收件人"
          min-width="220"
          show-overflow-tooltip
        />
        <el-table-column label="类别" width="110"
          ><template #default="{ row }">{{ categories[row.category] }}</template></el-table-column
        >
        <el-table-column label="内容格式" width="120"
          ><template #default="{ row }">{{
            row.format === 'html' ? '项目 HTML' : '默认模板'
          }}</template></el-table-column
        >
        <el-table-column label="状态" width="145"
          ><template #default="{ row }"
            ><el-tag
              :type="
                row.status === 'accepted'
                  ? 'success'
                  : row.status === 'sending'
                    ? 'info'
                    : 'warning'
              "
              >{{ statuses[row.status] || row.status }}</el-tag
            ></template
          ></el-table-column
        >
        <el-table-column label="时间" min-width="170"
          ><template #default="{ row }">{{ dateText(row.created_at) }}</template></el-table-column
        >
        <el-table-column
          prop="id"
          label="消息 ID"
          min-width="240"
          show-overflow-tooltip
        /><el-table-column prop="error_code" label="结果说明" min-width="200" /> </el-table
      ><el-pagination
        v-if="total > 20"
        v-model:current-page="page"
        :page-size="20"
        :total="total"
        layout="total, prev, pager, next"
      />
    </section>
    <section class="dy-panel"
      ><h2>项目如何发送</h2
      ><p class="dy-muted"
        >在 API 凭证中授予 mail:send 和 mail:read，通过项目后端调用。只传 text 使用多鱼默认模板；传
        html 使用项目生成的完整 HTML，无需在后台配置模板。</p
      ><pre class="dy-code">
POST /api/service/mail
Authorization: Bearer YOUR_API_KEY
Idempotency-Key: unique-business-message-id
Content-Type: application/json

{"to":"user@example.com","subject":"处理完成","text":"你的任务已完成。","category":"notification"}</pre
      ><p class="dy-muted"
        >业务验证码由项目生成、校验并限制有效期；此接口不生成账号中心验证码。当前使用统一 SMTP
        发件人，不提供附件、批量群发或自定义发件地址。</p
      ><el-button @click="$router.push('/manage/keys')">管理 API 凭证</el-button></section
    >
  </div>
</template>
<script setup lang="ts">
  import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
  import { api, dateText } from '@/api/duoyu'
  const rows = ref<any[]>([]),
    loading = ref(false),
    error = ref(''),
    search = ref(''),
    status = ref(''),
    page = ref(1),
    total = ref(0)
  const statuses: Record<string, string> = {
    queued: '等待发送',
    retry: '等待重试',
    failed: '发送失败',
    expired: '已过期',
    cancelled: '已取消',
    sending: '发送中',
    accepted: 'SMTP 已接受',
    unknown: '结果不确定',
    limited: '超过限额'
  }
  const categories: Record<string, string> = {
    notification: '通知',
    verification: '业务验证码',
    alert: '告警'
  }
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | undefined
  let generation = 0
  const reset = () => {
    if (page.value !== 1) page.value = 1
    else load()
  }
  watch(search, () => {
    clearTimeout(timer)
    controller?.abort()
    generation++
    timer = setTimeout(reset, 300)
  })
  watch(status, reset)
  watch(page, load)
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
        '/api/admin/mail?page=' +
          page.value +
          '&page_size=20&q=' +
          encodeURIComponent(search.value) +
          '&status=' +
          encodeURIComponent(status.value),
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
  onMounted(load)
</script>
<style scoped>
  .dy-muted {
    margin: 14px 0;
    line-height: 1.8;
  }
  pre.dy-code {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    padding: 16px;
    background: #fafafa;
    border: 1px solid #eee;
    border-radius: 5px;
  }
</style>
