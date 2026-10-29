<template>
 <div class="dy-page art-page-view" v-loading="loading">
  <div class="dy-heading"><div><el-button v-if="creating" text class="dy-back-button" @click="router.push('/manage/project-api')">← 返回开发者接入</el-button><h1>{{ creating ? (editing ? '编辑开发者凭证' : '创建开发者凭证') : '凭证与 MCP' }}</h1><p>统一管理 HTTP API 和 MCP 的凭证、权限及项目范围</p></div><el-button v-if="!creating" type="primary" @click="router.push('/manage/project-api/new')">创建开发者凭证</el-button></div>
  <div v-if="error" class="dy-error">{{ error }} <el-button text @click="load">重试</el-button></div>
  <section v-if="secret" class="dy-panel"><h2>保存开发者凭证</h2><p class="dy-muted">凭证只显示一次。复制给 AI 的内容包含该凭证，仅用于你信任的本地项目工具，不要放入前端或 Git。</p><div class="dy-secret">{{ secret }}</div><el-button @click="copySecret">复制凭证</el-button><el-button type="primary" @click="copyWithSecret">复制 AI 完整接入说明和本次凭证</el-button><el-button @click="finish">已保存，返回列表</el-button></section>
  <section v-else-if="creating" class="dy-panel"><el-form label-position="top" @submit.prevent="create">
   <el-form-item label="名称" required><el-input v-model="form.name" maxlength="100" placeholder="例如：本地项目接入 AI" /></el-form-item>
   <el-form-item label="调用权限" required><el-checkbox-group v-model="form.scopes"><el-checkbox v-for="s in scopes" :key="s.value" :value="s.value">{{ s.name }}</el-checkbox></el-checkbox-group></el-form-item>
   <p class="dy-muted">轮换客户端密钥会使旧密钥立即失效，默认不授予此权限。</p>
   <el-form-item label="项目范围"><el-checkbox v-model="form.all_projects">允许访问所有当前及未来项目</el-checkbox></el-form-item>
   <el-form-item v-if="!form.all_projects" label="额外允许管理的已有项目"><el-select v-model="form.app_ids" multiple filterable><el-option v-for="a in apps" :key="a.id" :label="a.name" :value="a.id" /></el-select></el-form-item>
   <p class="dy-muted">默认自动获得本凭证新建项目的权限，可另外选择已有项目；项目服务与 MCP 共用此凭证，用户登录仍使用 OIDC。</p>
   <el-form-item label="有效期"><el-checkbox v-model="form.permanent">永久有效，撤销前可用</el-checkbox></el-form-item>
   <el-form-item v-if="!form.permanent" label="有效天数"><el-input-number v-model="form.days" :min="1" :max="365" /></el-form-item>
   <div class="dy-form-footer"><el-button @click="router.push('/manage/project-api')">取消</el-button><el-button type="primary" native-type="submit" :loading="saving">{{ editing ? '保存修改' : '创建凭证' }}</el-button></div>
  </el-form></section>
  <template v-else>
   <section class="dy-panel"><h2>MCP 连接</h2><p class="dy-muted">支持 Streamable HTTP 与手动 Bearer 请求头的 AI 客户端，可连接下列地址。仅展示当前凭证获准的工具，每次调用重新检查权限。</p><code class="dy-code">{{ mcpURL }}</code><div class="dy-heading-actions"><el-button @click="copyMCP">复制 MCP 配置</el-button><el-button @click="copyURL">复制地址</el-button></div><p class="dy-muted">模板中的凭证为占位符；真实凭证仅在创建或轮换后显示。OAuth 自动发现授权暂不提供。</p></section>
   <section class="dy-panel"><h2>一键交给 AI 接入</h2><p class="dy-muted">说明包含当前规范 issuer、Logo 上传、项目创建/修改、角色与准入、OIDC 接入和验收命令。普通复制不包含任何密钥；创建凭证后可一次复制说明与该凭证。</p><div class="dy-heading-actions"><el-button type="primary" @click="copyGuide">复制 AI 接入说明</el-button><el-button @click="router.push('/manage/docs')">查看接入文档</el-button></div></section>
   <section class="dy-panel"><h2>开发者凭证</h2><el-table :data="keys.slice((page-1)*20,page*20)"><el-table-column type="index" label="序号" width="76" :index="(i:number)=>(page-1)*20+i+1" /><el-table-column prop="name" label="名称" min-width="170" /><el-table-column label="权限" min-width="220"><template #default="{row}">{{ row.scopes.map((s:string)=>scopes.find(x=>x.value===s)?.name||s).join('、') }}</template></el-table-column><el-table-column label="范围" min-width="180"><template #default="{row}">{{ row.all_projects ? '全部项目' : row.app_ids.length + ' 个已授权项目（含新建）' }}</template></el-table-column><el-table-column label="有效期" min-width="170"><template #default="{row}">{{ row.expires_at ? dateText(row.expires_at) : '永久有效' }}</template></el-table-column><el-table-column label="状态" width="110"><template #default="{row}"><el-tag :type="row.revoked ? 'info' : 'success'">{{ row.revoked ? '已撤销' : row.expires_at && Date.parse(row.expires_at)<=Date.now() ? '已到期' : '可用' }}</el-tag></template></el-table-column><el-table-column label="操作" width="230"><template #default="{row}"><el-button text :disabled="row.revoked" @click="router.push('/manage/project-api/edit?id='+row.id)">编辑</el-button><el-button text :disabled="row.revoked" @click="rotate(row)">轮换</el-button><el-button text type="danger" :disabled="row.revoked" @click="revoke(row)">撤销</el-button></template></el-table-column></el-table><el-pagination v-if="keys.length>20" v-model:current-page="page" :page-size="20" :total="keys.length" layout="total, prev, pager, next" /></section>
  </template>
 </div>
</template>
<script setup lang="ts">
import {api,dateText} from '@/api/duoyu'
import {copyDuoyuText} from '@/utils/duoyu-clipboard'
import {ElMessage,ElMessageBox} from 'element-plus'
import {onBeforeRouteLeave,onBeforeRouteUpdate,useRoute,useRouter} from 'vue-router'
const mcpURL=location.origin+'/mcp'
async function copyURL(){try{await copyDuoyuText(mcpURL);ElMessage.success('地址已复制')}catch(e:any){ElMessage.error(e.message)}}
async function copyMCP(){try{await copyDuoyuText(JSON.stringify({mcpServers:{duoyu:{url:mcpURL,headers:{Authorization:'Bearer YOUR_DEVELOPER_KEY'}}}},null,2));ElMessage.success('配置已复制')}catch(e:any){ElMessage.error(e.message)}}
const route=useRoute(),router=useRouter(),editing=computed(()=>route.path.endsWith('/edit')),creating=computed(()=>route.path.endsWith('/new')||editing.value)
const loading=ref(true),saving=ref(false),error=ref(''),secret=ref(''),keys=ref<any[]>([]),apps=ref<any[]>([]),page=ref(1),baseline=ref('')
const scopes=[{value:'projects:read',name:'读取项目'},{value:'projects:create',name:'新建项目'},{value:'projects:update',name:'修改项目及角色/准入'},{value:'projects:rotate',name:'轮换客户端密钥'},{value:'app:read',name:'读取项目服务资料'},{value:'mail:send',name:'发送项目邮件'},{value:'mail:read',name:'读取邮件状态'}]
const defaults=()=>({name:'',scopes:['projects:read','projects:create','projects:update'],all_projects:false,app_ids:[] as string[],permanent:true,days:30})
const form=ref(defaults())
const dirty=computed(()=>!!secret.value||creating.value&&!loading.value&&JSON.stringify(form.value)!==baseline.value)
async function leave(){if(!dirty.value)return true;try{await ElMessageBox.confirm(secret.value?'凭证尚未确认保存，离开后无法再次查看，确定离开？':'修改尚未保存，确定离开？','离开页面');return true}catch{return false}}
onBeforeRouteLeave(leave);onBeforeRouteUpdate(leave)
function unload(e:BeforeUnloadEvent){if(dirty.value){e.preventDefault();e.returnValue=''}}
window.addEventListener('beforeunload',unload);onBeforeUnmount(()=>{window.removeEventListener('beforeunload',unload);secret.value=''})
async function load(){loading.value=true;error.value='';secret.value='';form.value=defaults();try{[keys.value,apps.value]=await Promise.all([api('/api/admin/developer-keys'),api('/api/admin/apps')]);page.value=Math.min(page.value,Math.max(1,Math.ceil(keys.value.length/20)));if(editing.value){const key=keys.value.find(k=>k.id===route.query.id);if(!key)throw Error('凭证不存在');form.value={name:key.name,scopes:key.scopes,all_projects:key.all_projects,app_ids:key.app_ids,permanent:!key.expires_at,days:key.expires_at?Math.max(1,Math.ceil((Date.parse(key.expires_at)-Date.now())/86400000)):30}}baseline.value=JSON.stringify(form.value)}catch(e:any){error.value=e.message}finally{loading.value=false}}
watch(()=>route.fullPath,load,{immediate:true})
async function guide(){return await api<{guide:string;issuer:string}>('/api/admin/ai-integration')}
async function copyGuide(){try{await copyDuoyuText((await guide()).guide);ElMessage.success('AI 接入说明已复制')}catch(e:any){ElMessage.error(e.message)}}
async function copySecret(){try{await copyDuoyuText(secret.value);ElMessage.success('凭证已复制')}catch(e:any){ElMessage.error(e.message)}}
async function copyWithSecret(){try{const doc=await guide();await copyDuoyuText(doc.guide+'\n\n## 本次本地环境配置（敏感，仅保存本机，不提交 Git）\nDUOYU_ISSUER='+doc.issuer+'\nDUOYU_MANAGEMENT_API_KEY='+secret.value+'\n');ElMessage.success('说明与本次凭证已复制')}catch(e:any){ElMessage.error(e.message)}}
async function create(){saving.value=true;try{if(editing.value){await api('/api/admin/developer-keys/'+route.query.id,'PUT',form.value);baseline.value=JSON.stringify(form.value);ElMessage.success('凭证已更新');router.push('/manage/project-api')}else{const result=await api('/api/admin/developer-keys','POST',form.value);secret.value=result.secret;baseline.value=JSON.stringify(form.value)}}catch(e:any){ElMessage.error(e.message)}finally{saving.value=false}}
function finish(){secret.value='';baseline.value=JSON.stringify(form.value);router.push('/manage/project-api')}
async function rotate(row:any){try{await ElMessageBox.confirm('立即轮换该凭证，旧凭证将失效，确认继续？','轮换凭证');const result=await api('/api/admin/developer-keys/'+row.id+'/rotate','POST',{overlap_hours:0});secret.value=result.secret}catch(e:any){if(e instanceof Error)ElMessage.error(e.message)}}
async function revoke(row:any){try{await ElMessageBox.confirm('撤销后使用该凭证的自动化工具将不能继续管理项目，确认继续？','撤销凭证',{type:'warning'});await api('/api/admin/developer-keys/'+row.id+'/revoke','POST',{});ElMessage.success('凭证已撤销');await load()}catch(e:any){if(e instanceof Error)ElMessage.error(e.message)}}
</script>
