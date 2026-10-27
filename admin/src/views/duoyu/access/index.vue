<template>
  <div class="dy-page dy-editor"
    ><header class="dy-heading"
      ><div
        ><el-button text @click="$router.push('/manage/users')">← 返回账号管理</el-button
        ><h1>管理权限</h1></div
      ></header
    ><div v-if="error" class="dy-error" role="alert">{{ error }}</div
    ><el-form class="dy-panel" label-position="top" v-loading="loading" @submit.prevent="save"
      ><el-form-item label="管理角色"
        ><el-select v-model="role"
          ><el-option label="普通账号" value="none" /><el-option
            label="审计员 · 只读日志与运行状态"
            value="auditor" /><el-option
            label="项目维护员 · 指定项目配置与准入"
            value="maintainer" /></el-select></el-form-item
      ><el-form-item v-if="role === 'maintainer'" label="负责项目"
        ><el-select v-model="assigned" multiple filterable
          ><el-option
            v-for="app in apps"
            :key="app.id"
            :label="app.name"
            :value="app.id" /></el-select></el-form-item
      ><p class="dy-muted"
        >项目维护员可修改负责项目的配置、准入名单和客户端密钥，不能管理全站账号、API
        凭证或系统设置。</p
      ><el-button
        type="primary"
        native-type="submit"
        :loading="saving"
        :disabled="loading || !!error"
        >保存权限</el-button
      ></el-form
    ></div
  >
</template>
<script setup lang="ts">
  import { ref, onMounted } from 'vue'
  import { useRoute, onBeforeRouteLeave } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { api } from '@/api/duoyu'
  const route = useRoute(),
    role = ref('none'),
    assigned = ref<string[]>([]),
    apps = ref<any[]>([]),
    loading = ref(true),
    saving = ref(false),
    error = ref(''),
    baseline = ref('')
  const snapshot = () => JSON.stringify([role.value, assigned.value])
  onBeforeRouteLeave(async () => {
    if (loading.value || baseline.value === snapshot()) return true
    try {
      await ElMessageBox.confirm('修改尚未保存，确定离开？', '离开页面')
      return true
    } catch {
      return false
    }
  })
  onMounted(async () => {
    try {
      const [access, list] = await Promise.all([
        api('/api/admin/users/' + encodeURIComponent(String(route.query.id)) + '/access'),
        api('/api/admin/apps?page=1&page_size=100')
      ])
      if (!access.length) throw new Error('账号不存在')
      role.value = access[0].management_role
      assigned.value = access[0].app_ids
      apps.value = list.items
      for (let page = 2; apps.value.length < list.total; page++) {
        const next = await api('/api/admin/apps?page=' + page + '&page_size=100')
        apps.value.push(...next.items)
      }
      baseline.value = snapshot()
    } catch (e: any) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  })
  async function save() {
    saving.value = true
    try {
      await api(
        '/api/admin/users/' + encodeURIComponent(String(route.query.id)) + '/access',
        'PUT',
        { role: role.value, app_ids: assigned.value }
      )
      baseline.value = snapshot()
      ElMessage.success('管理权限已保存')
    } catch (e: any) {
      ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
</script>
