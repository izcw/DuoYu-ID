<template>
  <div class="box-border w-full h-full" v-loading="isLoading">
    <iframe
      v-if="iframeUrl"
      :key="iframeUrl"
      ref="iframeRef"
      :src="iframeUrl"
      frameborder="0"
      class="w-full h-full min-h-[calc(100vh-120px)] border-none"
      @load="handleIframeLoad"
    ></iframe>
    <p v-else>页面链接无效</p>
  </div>
</template>

<script setup lang="ts">
  import { IframeRouteManager } from '@/router/core'
  import { webNavigationUrl } from '@/utils/navigation/route'

  defineOptions({ name: 'IframeView' })

  const route = useRoute()
  const isLoading = ref(true)
  const iframeUrl = ref('')
  const iframeRef = ref<HTMLIFrameElement | null>(null)

  /**
   * 初始化 iframe URL
   * 从路由配置中获取对应的外部链接地址
   */
  watch(
    () => route.path,
    () => {
      const iframeRoute = IframeRouteManager.getInstance().findByPath(route.path)

      iframeUrl.value = webNavigationUrl(iframeRoute?.meta?.link) || ''
      isLoading.value = !!iframeUrl.value
    },
    { immediate: true }
  )

  /**
   * 处理 iframe 加载完成事件
   * 隐藏加载状态
   */
  const handleIframeLoad = (): void => {
    isLoading.value = false
  }
</script>
