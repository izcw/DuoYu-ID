<!-- 系统logo -->
<template>
  <div class="flex-cc">
    <img :style="logoStyle" src="/logo.png" alt="多鱼账号" class="w-full h-full" style="object-fit:contain;border-radius:4px" />
  </div>
</template>

<script setup lang="ts">
  defineOptions({ name: 'ArtLogo' })

  interface Props {
    /** logo 大小 */
    size?: number | string
  }

  const props = withDefaults(defineProps<Props>(), {
    size: 36
  })

  const logoStyle = computed(() => ({ width: `${props.size}px` }))
</script>
