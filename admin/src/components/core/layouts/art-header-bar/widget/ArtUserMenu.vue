<template><ElDropdown @command="command"><div style="display:flex;align-items:center;gap:10px;cursor:pointer"><ElAvatar :src="store.info.avatar || '/avatar-default.svg'" :size="34"/><span>{{store.info.userName}}</span></div><template #dropdown><ElDropdownMenu><ElDropdownItem command="profile">账号与安全</ElDropdownItem><ElDropdownItem command="logout" divided>退出多鱼账号</ElDropdownItem></ElDropdownMenu></template></ElDropdown></template>
<script setup lang="ts">
import {useUserStore} from '@/store/modules/user'
import {useRouter} from 'vue-router'
import {api} from '@/api/duoyu'
import {ElMessage,ElMessageBox} from 'element-plus'
const store=useUserStore(),router=useRouter()
async function command(v:string){if(v==='profile'){router.push('/account/profile');return}try{await ElMessageBox.confirm('退出账号中心；接入项目的已有会话按各项目规则失效。','退出登录');await api('/api/auth/logout','POST',{});localStorage.clear();window.location.assign('/login')}catch(e:any){if(e instanceof Error)ElMessage.error(e.message)}}
</script>