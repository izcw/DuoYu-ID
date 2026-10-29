<template>
  <section class="dy-panel" v-loading="loading">
    <el-button text class="dy-back-button" @click="router.push('/account/profile')">← 返回账号与安全</el-button>
    <p v-if="error" class="dy-error">{{ error }}</p>
    <template v-if="recoveryCodes.length">
      <h2>保存恢复码</h2>
      <p class="dy-muted">绑定成功。这些恢复码只显示一次，每个只能使用一次。请保存到安全的位置，手机丢失时可用于登录或解绑。</p>
      <div class="dy-recovery-codes"><code v-for="item in recoveryCodes" :key="item">{{ item }}</code></div>
      <el-button @click="downloadCodes">下载恢复码</el-button>
      <el-button type="primary" @click="finish">已保存，完成</el-button>
    </template>
    <template v-else>
      <h2>{{ enabled ? '已绑定身份验证器' : '绑定身份验证器' }}</h2>
      <p class="dy-muted">支持 Microsoft Authenticator。添加账号时选择“其他账号”，扫描下方二维码；登录使用应用中的 6 位动态验证码。</p>
      <p v-if="enabled" class="dy-muted">剩余 {{ remaining }} 个恢复码。解绑后所有中心会话将退出；需要更换验证器时，解绑后重新绑定。</p>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item v-if="!setup" label="当前邮箱验证码">
          <div class="dy-auth-code-row"><el-input v-model="emailCode" maxlength="6" placeholder="6 位邮箱验证码" autocomplete="off" data-lpignore="true" data-1p-ignore /><el-button :loading="sending" :disabled="countdown > 0" @click="send">{{ countdown ? countdown + ' 秒后重发' : '发送' }}</el-button></div>
        </el-form-item>
        <template v-if="setup">
          <p class="dy-muted">请在 10 分钟内完成绑定。二维码和密钥不要分享给他人。</p>
          <img class="dy-authenticator-qr" :src="setup.image" alt="身份验证器绑定二维码" />
          <el-form-item label="无法扫码时手动输入密钥"><code class="dy-authenticator-secret">{{ setup.secret }}</code></el-form-item>
        </template>
        <el-form-item v-if="setup || enabled" :label="useRecovery ? '恢复码' : '身份验证器动态码'">
          <el-input v-model="otp" :maxlength="useRecovery ? 32 : 6" :placeholder="useRecovery ? '输入一个尚未使用的恢复码' : '6 位动态验证码'" autocomplete="off" />
        </el-form-item>
        <el-checkbox v-if="enabled" v-model="useRecovery" @change="otp = ''">手机不可用，使用恢复码解绑</el-checkbox>
        <div class="dy-actions">
          <el-button @click="router.push('/account/profile')">取消</el-button>
          <el-button :type="enabled ? 'danger' : 'primary'" :loading="saving" native-type="submit">{{ enabled ? '解绑身份验证器' : setup ? '确认绑定' : '验证邮箱并继续' }}</el-button>
        </div>
      </el-form>
    </template>
  </section>
</template>
<script setup lang="ts">
import {api} from '@/api/duoyu'
import {ElMessage, ElMessageBox} from 'element-plus'
import {onBeforeRouteLeave, onBeforeRouteUpdate, useRouter} from 'vue-router'
const router=useRouter()
const loading=ref(true),saving=ref(false),sending=ref(false),error=ref(''),enabled=ref(false),remaining=ref(0),emailCode=ref(''),otp=ref(''),useRecovery=ref(false),countdown=ref(0)
const setup=ref<{image:string;secret:string}|null>(null),recoveryCodes=ref<string[]>([])
let timer:ReturnType<typeof setInterval>|undefined
onMounted(async()=>{try{const s=await api('/api/me/authenticator');enabled.value=s.enabled;remaining.value=s.recovery_remaining}catch(e:any){error.value=e.message}finally{loading.value=false}})
const dirty=computed(()=>!!setup.value||recoveryCodes.value.length>0||!!emailCode.value||!!otp.value)
async function leave(){if(!dirty.value)return true;try{await ElMessageBox.confirm(recoveryCodes.value.length?'恢复码尚未确认保存，确定离开？':'绑定或输入尚未完成，确定离开？','离开页面');return true}catch{return false}}
onBeforeRouteLeave(leave);onBeforeRouteUpdate(leave)
function unload(e:BeforeUnloadEvent){if(dirty.value){e.preventDefault();e.returnValue=''}}
window.addEventListener('beforeunload',unload)
onBeforeUnmount(()=>{clearInterval(timer);window.removeEventListener('beforeunload',unload);setup.value=null;recoveryCodes.value=[]})
async function send(){sending.value=true;try{const me=await api('/api/me');await api('/api/auth/code','POST',{email:me.email,purpose:'email'});ElMessage.success('邮件请求已提交，请查看邮箱');countdown.value=60;timer=setInterval(()=>{if(--countdown.value<=0)clearInterval(timer)},1000)}catch(e:any){ElMessage.error(e.message)}finally{sending.value=false}}
async function submit(){saving.value=true;try{
 if(enabled.value){await ElMessageBox.confirm('解绑后将退出全部中心会话，确认继续？','解绑身份验证器',{type:'warning'});await api('/api/me/authenticator','DELETE',{email_code:emailCode.value,code:otp.value,recovery:useRecovery.value});emailCode.value='';otp.value='';window.location.assign('/login')}
 else if(!setup.value){setup.value=await api('/api/me/authenticator/begin','POST',{code:emailCode.value});emailCode.value=''}
 else{const out=await api('/api/me/authenticator/finish','POST',{code:otp.value});setup.value=null;otp.value='';recoveryCodes.value=out.recovery_codes;ElMessage.success('身份验证器已绑定')}
}catch(e:any){if(e instanceof Error)ElMessage.error(e.message)}finally{saving.value=false}}
function downloadCodes(){const url=URL.createObjectURL(new Blob(['DuoYu ID 身份验证器恢复码（每个仅可用一次）\n'+recoveryCodes.value.join('\n')],{type:'text/plain;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download='duoyu-id-recovery-codes.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
function finish(){recoveryCodes.value=[];router.push('/account/profile')}
</script>
