<template>
  <el-form-item label="检查登录回调">
    <el-input v-model="input" placeholder="粘贴实际 redirect_uri" clearable aria-label="实际登录回调地址" />
    <p v-if="result" class="dy-muted" role="status">{{ result }}</p>
  </el-form-item>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
const props = defineProps<{ registered: string[] }>()
const input = ref('')
const result = computed(() => {
  const raw = input.value.trim()
  if (!raw) return ''
  let actual: URL
  try { actual = new URL(raw) } catch { return '请输入完整的回调地址，包含协议、主机和路径。' }
  if (props.registered.includes(raw)) return '与当前表单中的登录回调完全匹配。'
  const parts = { protocol: '协议', hostname: '主机', port: '端口', pathname: '路径', search: '查询参数', hash: '片段' } as const
  const candidates = props.registered.flatMap(address => {
    try {
      const url = new URL(address)
      const differences = (Object.keys(parts) as Array<keyof typeof parts>).filter(key => actual[key] !== url[key])
      return [{ address, differences }]
    } catch { return [] }
  }).sort((a, b) => a.differences.length - b.differences.length)
  const closest = candidates[0]
  if (!closest) return '请先填写登录回调地址。'
  const diff = closest.differences.map(key => parts[key]).join('、') || '地址原文'
  return `未匹配。与 ${closest.address} 的差异为${diff}。`
})
</script>
