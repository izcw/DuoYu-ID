<template>
 <div class="dy-page art-page-view" v-loading="loading">
  <div class="dy-heading"><div><el-button text class="dy-back-button" @click="back">← 返回{{ editing || memberMode ? '项目角色' : '项目管理' }}</el-button><h1>{{ memberMode ? '修改用户项目角色' : editing ? roleID ? '编辑角色' : '新增角色' : '项目角色' }}</h1><p>{{ appName }}</p></div></div>
  <div v-if="error" class="dy-error">{{ error }} <el-button text @click="load">重试</el-button></div>
  <section v-else-if="!loading" class="dy-panel">
   <el-form v-if="memberMode" label-position="top" @submit.prevent="saveMember"><h2>{{ member?.name }} · {{ member?.email }}</h2><el-form-item label="项目角色" required><el-select v-model="selectedRole"><el-option v-for="r in roles" :key="r.id" :value="r.id" :label="r.name + '（' + r.role_key + '）'" /></el-select></el-form-item><p class="dy-muted">仅修改当前项目角色，不授予账号中心管理权限。</p><div class="dy-form-footer"><el-button @click="back">取消</el-button><el-button native-type="submit" type="primary" :loading="saving">保存角色</el-button></div></el-form>
   <template v-else-if="editing">
    <el-form label-position="top" @submit.prevent="save">
     <el-form-item label="名称" required><el-input v-model="form.name" maxlength="40" placeholder="例如：管理员" /></el-form-item>
     <el-form-item label="字段标识" required><el-input v-model="form.role_key" maxlength="40" placeholder="例如：admin、user、editor" /><p class="dy-muted">以小写字母开头，可包含小写字母、数字、下划线。接入项目通过此字段判断角色。</p></el-form-item>
     <el-form-item label="描述" required><el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" /></el-form-item>
     <el-form-item label="默认注册角色"><el-switch v-model="form.is_default" :disabled="originalDefault" /><p class="dy-muted">首次加入项目的用户自动获得此角色。请先将另一个角色设为默认，再取消当前默认角色。</p></el-form-item>
     <div class="dy-form-footer"><el-button @click="back">取消</el-button><el-button native-type="submit" type="primary" :loading="saving">{{ roleID ? '保存修改' : '创建角色' }}</el-button></div>
    </el-form>
    <template v-if="roleID"><h2 style="margin-top:32px">删除角色</h2><p class="dy-muted">初始角色也可删除。现有用户将转移到替代角色；删除默认角色时，替代角色同时成为默认。</p><el-select v-model="replacement" placeholder="请选择替代角色"><el-option v-for="r in roles.filter(r => r.id !== roleID)" :key="r.id" :value="r.id" :label="r.name" /></el-select><el-button type="danger" :disabled="!replacement" :loading="saving" @click="remove">删除角色</el-button></template>
   </template>
   <template v-else>
    <div class="dy-toolbar"><el-button type="primary" @click="edit('')">新增角色</el-button><span class="dy-muted">共 {{ roles.length }} 个角色 · 默认：{{ roles.find(r => r.is_default)?.name }}</span></div>
    <p class="dy-muted">每个项目独立配置角色。首次加入时分配默认角色，调整默认值不会改变已有用户。</p>
    <el-table :data="roles.slice((page-1)*20,page*20)"><el-table-column type="index" label="序号" width="76" :index="(i:number)=>(page-1)*20+i+1" /><el-table-column prop="name" label="名称" min-width="140" /><el-table-column prop="role_key" label="字段" min-width="120" /><el-table-column prop="description" label="描述" min-width="180" show-overflow-tooltip /><el-table-column label="默认注册" width="120"><template #default="{row}"><el-tag v-if="row.is_default" type="info">默认角色</el-tag><span v-else>—</span></template></el-table-column><el-table-column prop="user_count" label="用户数" width="100" /><el-table-column label="操作" width="120"><template #default="{row}"><el-button text @click="edit(row.id)">编辑</el-button></template></el-table-column></el-table>
    <el-pagination v-if="roles.length>20" v-model:current-page="page" :page-size="20" :total="roles.length" layout="total, prev, pager, next" />
   </template>
  </section>
 </div>
</template>
<script setup lang="ts">
import {api} from '@/api/duoyu'
import {ElMessage,ElMessageBox} from 'element-plus'
import {onBeforeRouteLeave,onBeforeRouteUpdate,useRoute,useRouter} from 'vue-router'
const route=useRoute(),router=useRouter(),id=computed(()=>String(route.query.id||'')),roleID=computed(()=>String(route.query.role||'')),editing=computed(()=>route.path.endsWith('/roles/edit')),memberMode=computed(()=>route.path.endsWith('/member-role'))
const loading=ref(true),saving=ref(false),error=ref(''),roles=ref<any[]>([]),appName=ref(''),page=ref(1),replacement=ref(''),originalDefault=ref(false),member=ref<any>(),selectedRole=ref(''),baseline=ref('')
const form=ref({name:'',role_key:'',description:'',is_default:false})
const snapshot=()=>JSON.stringify(memberMode.value?selectedRole.value:form.value)
const dirty=computed(()=>!loading.value&&!error.value&&(editing.value||memberMode.value)&&snapshot()!==baseline.value)
async function leave(){if(!dirty.value)return true;try{await ElMessageBox.confirm('修改尚未保存，确定离开？','离开页面');return true}catch{return false}}
onBeforeRouteLeave(leave);onBeforeRouteUpdate(leave)
function unload(e:BeforeUnloadEvent){if(dirty.value){e.preventDefault();e.returnValue=''}}
window.addEventListener('beforeunload',unload);onBeforeUnmount(()=>window.removeEventListener('beforeunload',unload))
async function load(){loading.value=true;error.value='';try{const [apps,list]=await Promise.all([api<any[]>('/api/admin/apps/'+id.value),api<any[]>('/api/admin/apps/'+id.value+'/roles')]);if(!apps[0])throw Error('项目不存在');appName.value=apps[0].name;roles.value=list;page.value=Math.min(page.value,Math.max(1,Math.ceil(list.length/20)));const current=list.find(r=>r.id===roleID.value);if(editing.value&&roleID.value&&!current)throw Error('角色不存在');form.value=current?{name:current.name,role_key:current.role_key,description:current.description,is_default:current.is_default}:{name:'',role_key:'',description:'',is_default:false};originalDefault.value=!!current?.is_default;replacement.value=list.find(r=>r.is_default&&r.id!==roleID.value)?.id||'';
 if(memberMode.value){const user=String(route.query.user||'');const out=await api<any>('/api/admin/apps/'+id.value+'/users?page=1&page_size=20&q='+encodeURIComponent(user));member.value=out.items.find((u:any)=>u.id===user);if(!member.value)throw Error('项目用户不存在');selectedRole.value=member.value.role_id}
 baseline.value=snapshot()
 }catch(e:any){error.value=e.message}finally{loading.value=false}}
watch(()=>route.fullPath,load,{immediate:true})
function edit(role:string){router.push({path:'/manage/apps/roles/edit',query:{id:id.value,...(role?{role}:{})}})}
function back(){router.push(editing.value||memberMode.value?{path:'/manage/apps/roles',query:{id:id.value}}:'/manage/apps')}
async function save(){saving.value=true;try{await api('/api/admin/apps/'+id.value+'/roles'+(roleID.value?'/'+roleID.value:''),roleID.value?'PUT':'POST',form.value);baseline.value=snapshot();ElMessage.success('角色已保存');back()}catch(e:any){ElMessage.error(e.message)}finally{saving.value=false}}
async function remove(){try{await ElMessageBox.confirm('删除后将把现有用户转移到替代角色，确认继续？','删除角色',{type:'warning'});saving.value=true;await api('/api/admin/apps/'+id.value+'/roles/'+roleID.value,'DELETE',{replacement_id:replacement.value});baseline.value=snapshot();ElMessage.success('角色已删除');back()}catch(e:any){if(e instanceof Error)ElMessage.error(e.message)}finally{saving.value=false}}
async function saveMember(){saving.value=true;try{await api('/api/admin/apps/'+id.value+'/users/'+String(route.query.user)+'/role','PUT',{role_id:selectedRole.value});baseline.value=snapshot();ElMessage.success('项目角色已更新');router.push({path:'/manage/apps/members',query:{id:id.value}})}catch(e:any){ElMessage.error(e.message)}finally{saving.value=false}}
</script>
