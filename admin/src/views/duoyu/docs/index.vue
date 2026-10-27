<template>
  <div class="dy-page"
    ><div class="dy-heading"
      ><div><h1>接入文档</h1><p>客户端配置、标准协议与会话管理</p></div
      ><el-button @click="download">下载 Service API 文档</el-button></div
    ><div class="dy-hero"
      ><span class="dy-kicker">接入指南</span><h2>使用多鱼账号登录</h2
      ><p
        >支持 OIDC Authorization Code + S256
        PKCE、按项目启用的轮换刷新令牌，以及已授权范围内的静默认证。不提供隐式授权或密码授权。</p
      ></div
    ><section class="dy-panel"
      ><h2>01 · 创建项目</h2
      ><p class="dy-muted"
        >在项目管理中登记精确的回调地址。有后端的网站选择服务端客户端；浏览器应用选择公共客户端，不保存客户端密钥。</p
      ><el-button @click="$router.push('/manage/apps')">前往项目管理</el-button></section
    ><section class="dy-panel"
      ><h2>02 · 标准发现与认证</h2
      ><pre class="dy-code">
Issuer: {{ origin }}
Discovery: {{ origin }}/.well-known/openid-configuration
Scope: openid profile email
Response type: code
Code challenge method: S256</pre
      ><p class="dy-muted"
        >使用成熟的 OIDC 客户端库，通过 Discovery 获取端点。校验
        state、nonce、签名、issuer、audience 和有效期；服务端客户端通过 HTTP Basic
        验证客户端身份。</p
      ><p class="dy-muted"
        >使用 iss + sub 关联项目自己的用户。ID Token 用于确认身份；Access Token 用于获取
        UserInfo，不要互换用途。</p
      ></section
    ><section class="dy-panel"
      ><h2>03 · 身份资料</h2
      ><el-table :data="claims"
        ><el-table-column prop="name" label="字段" /><el-table-column
          prop="scope"
          label="范围" /><el-table-column prop="description" label="说明" /></el-table
      ><p class="dy-muted"
        >头像和昵称通过 UserInfo 更新；项目不访问中心数据库，不保存头像磁盘路径。</p
      ></section
    ><section class="dy-panel"
      ><h2>04 · 会话与退出</h2
      ><p class="dy-muted"
        >中心会话最长 12 小时，访问令牌和 ID Token 有效期 5
        分钟。已有中心会话可以在授权页面确认后继续使用，无需重新输入凭证。</p
      ><p class="dy-muted"
        >应用必须自己限制本地会话寿命。账号停用立即阻止新认证和 UserInfo
        查询，但不会凭空注销项目本地会话。若需要在 5
        分钟内生效，项目应在该周期内使用有效令牌复核身份或重新发起认证。</p
      ><p class="dy-muted"
        >退出项目、退出中心是不同操作。第一版不承诺全应用即时退出；prompt=none 暂不支持。</p
      ></section
    ><section class="dy-panel"
      ><h2>05 · 服务端 API</h2
      ><p class="dy-muted"
        >API 凭证与 OIDC 客户端密钥独立。app:read 读取项目资料，mail:send 发送项目邮件，mail:read
        查询本项目发送状态。按用途单独授予权限。</p
      ><pre class="dy-code">
GET {{ origin }}/api/service/app
Authorization: Bearer YOUR_API_KEY</pre
      ><p class="dy-muted"
        >项目邮件：POST /api/service/mail，携带 Idempotency-Key。提交 to、subject、text
        使用默认模板；提供 html 则使用项目自己的 HTML。GET /api/service/mail/{id}
        查询状态。同一项目同一幂等键不会重复发信；正文变化时必须使用新的键。</p
      ><p class="dy-muted"
        >密钥只放在后端，禁止嵌入浏览器。所有请求使用 HTTPS；错误返回对应 HTTP 状态码。</p
      ></section
    ></div
  >
</template>
<script setup lang="ts">
  import { ref, onMounted } from 'vue'
  import { api } from '@/api/duoyu'
  const diagnostics = ref<any>(null),
    diagnosticError = ref('')
  async function check() {
    try {
      diagnosticError.value = ''
      diagnostics.value = await api('/api/admin/diagnostics')
    } catch (e: any) {
      diagnosticError.value = e.message
    }
  }
  onMounted(check)

  const origin = window.location.origin
  const claims = [
    { name: 'sid', scope: 'openid', description: '中心会话标识，用于后台退出通知匹配' },
    { name: 'sub', scope: 'openid', description: '稳定的用户身份标识' },
    { name: 'name', scope: 'profile', description: '显示名称' },
    { name: 'picture', scope: 'profile', description: '头像 URL' },
    { name: 'email', scope: 'email', description: '邮箱地址' },
    { name: 'email_verified', scope: 'email', description: '邮箱已通过验证' }
  ]
  function download() {
    window.open('/api/openapi.json', '_blank', 'noopener')
  }
</script>
