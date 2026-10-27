<template>
  <div class="dy-page" v-loading="loading"
    ><div class="dy-heading"
      ><div><h1>邮件与系统设置</h1><p>注册规则、发送渠道与邮件预览</p></div></div
    ><div v-if="error" class="dy-error">{{ error }}</div
    ><div class="dy-grid"
      ><section class="dy-panel"
        ><h2>注册与运行配置</h2
        ><div class="dy-listitem"
          ><div>开放注册<p>关闭后，仅已有账号可以登录</p></div
          ><el-switch
            v-model="settings.registration"
            :disabled="saving"
            :loading="saving"
            @change="save"
            aria-label="开放注册" /></div
        ><div class="dy-listitem"
          ><div
            >认证地址<p class="dy-code">{{ settings.issuer }}</p></div
          ></div
        ><div class="dy-listitem"
          ><div
            >运行模式<p>{{
              settings.development ? '本机开发模式 · 不可直接公网部署' : '生产模式 · HTTPS'
            }}</p></div
          ></div
        ><el-alert
          type="info"
          :closable="false"
          title="认证域名、签名密钥和数据库配置由部署环境管理，避免后台误操作破坏接入。" /></section
      ><section class="dy-panel"
        ><h2>邮件服务</h2
        ><el-alert
          v-if="settings.local_mail"
          type="warning"
          :closable="false"
          title="当前为测试收件箱，不会向真实邮箱投递。"
          style="margin-bottom: 20px"
        />
        <div v-if="settings.local_mail" style="margin-bottom: 20px">
          <el-button tag="a" href="http://localhost:18025" target="_blank" rel="noopener noreferrer"
            >打开测试收件箱 ↗</el-button
          >
        </div>
        <el-alert
          v-else
          type="info"
          :closable="false"
          title="使用 SMTP 投递；服务商接收不代表已进入收件箱。"
          style="margin-bottom: 20px"
        />
        <el-descriptions :column="1"
          ><el-descriptions-item label="服务器"
            >{{ settings.smtp_host }}:{{ settings.smtp_port }}</el-descriptions-item
          ><el-descriptions-item label="发件邮箱">{{
            settings.smtp_from || '未配置'
          }}</el-descriptions-item
          ><el-descriptions-item label="发件人">多鱼账号</el-descriptions-item
          ><el-descriptions-item label="配置状态"
            ><el-tag :type="settings.smtp_configured ? 'success' : 'warning'">{{
              settings.local_mail
                ? '本机测试'
                : settings.smtp_configured
                  ? '已配置 SMTP'
                  : '尚未填写授权码'
            }}</el-tag></el-descriptions-item
          ></el-descriptions
        ><p v-if="settings.local_mail" class="dy-muted"
          >启用 QQ 邮箱：在项目目录运行
          <code>./scripts/configure-mail.ps1</code>，隐藏输入新的授权码，然后重启服务。</p
        >
        <p class="dy-muted"
          >SMTP 授权码只保存在服务端环境变量，后台不显示或回传密钥。此处预览不会发送真实邮件。</p
        ></section
      ></div
    ><section class="dy-panel"
      ><div class="dy-heading"
        ><div><h2 style="margin: 0">邮件模板预览</h2></div
        ><el-select v-model="purpose" @change="preview" style="width: 160px"
          ><el-option label="登录验证码" value="login" /><el-option
            label="重置密码"
            value="reset" /><el-option label="邮箱变更" value="email" /></el-select></div
      ><iframe
        title="验证码邮件预览"
        :srcdoc="html"
        sandbox=""
        style="
          width: 100%;
          height: 650px;
          border: 0;
          border-radius: 6px;
          background: #fafafa;
        " /></section
  ></div>
</template>
<script setup lang="ts">
  import { ref, onMounted } from 'vue'
  import { ElMessage } from 'element-plus'
  import { api } from '@/api/duoyu'
  const settings = ref<any>({}),
    loading = ref(true),
    error = ref(''),
    purpose = ref('login'),
    html = ref(''),
    saving = ref(false)
  async function preview() {
    const response = await fetch('/api/admin/mail-preview?purpose=' + purpose.value, {
      credentials: 'same-origin'
    })
    if (!response.ok) throw Error('模板加载失败')
    html.value = await response.text()
  }
  async function save() {
    saving.value = true
    try {
      await api('/api/admin/settings', 'PUT', { registration: settings.value.registration })
      ElMessage.success('设置已保存')
    } catch (e: any) {
      settings.value.registration = !settings.value.registration
      ElMessage.error(e.message)
    } finally {
      saving.value = false
    }
  }
  onMounted(async () => {
    try {
      settings.value = await api('/api/admin/settings')
      await preview()
    } catch (e: any) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  })
</script>
