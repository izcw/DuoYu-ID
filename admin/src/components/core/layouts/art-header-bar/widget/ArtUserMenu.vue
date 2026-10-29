<template>
  <ElDropdown @command="command"
    ><div class="dy-header-user"
      ><ElAvatar :src="store.info.avatar || '/avatar-default.svg'" :size="34" /><span
        class="dy-header-username"
        :title="store.info.userName"
        >{{ store.info.userName }}</span
      ><span v-if="store.info.roles?.includes('R_SUPER')" class="dy-header-admin-badge">超级管理员</span></div
    ><template #dropdown
      ><ElDropdownMenu
        ><ElDropdownItem command="profile">账号与安全</ElDropdownItem
        ><ElDropdownItem command="logout" divided>退出多鱼账号</ElDropdownItem></ElDropdownMenu
      ></template
    ></ElDropdown
  >
</template>
<script setup lang="ts">
  import { useUserStore } from '@/store/modules/user'
  import { useRouter } from 'vue-router'
  import { api } from '@/api/duoyu'
  import { ElMessage, ElMessageBox } from 'element-plus'
  const store = useUserStore(),
    router = useRouter()
  async function command(v: string) {
    if (v === 'profile') {
      router.push('/account/profile')
      return
    }
    try {
      await ElMessageBox.confirm('退出账号中心；接入项目的已有会话按各项目规则失效。', '退出登录')
      await api('/api/auth/logout', 'POST', {})
      localStorage.clear()
      window.location.assign('/login')
    } catch (e: any) {
      if (e instanceof Error) ElMessage.error(e.message)
    }
  }
</script>
<style scoped>
  .dy-header-user {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 8px;
    cursor: pointer;
    min-width: 0;
  }
  .dy-header-username {
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    line-height: 1.5;
  }
  .dy-header-admin-badge {
    flex-shrink: 0;
    padding: 3px 6px;
    border-radius: 4px;
    background: #171717;
    color: #fff;
    font-size: 11px;
    line-height: 1.4;
    white-space: nowrap;
  }
  @media (max-width: 640px) {
    .dy-header-user { gap: 6px; }
    .dy-header-admin-badge { font-size: 10px; padding: 3px 4px; }
    .dy-header-username {
      max-width: 88px;
    }
  }
</style>
