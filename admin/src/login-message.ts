import { ElMessage } from 'element-plus'
import 'element-plus/theme-chalk/base.css'
import 'element-plus/theme-chalk/el-message.css'
import 'element-plus/theme-chalk/el-icon.css'
import './login-message.css'

// Standalone Go login page: bundle only the message component and its runtime.
export function show(text: string, success = false) {
  ElMessage.closeAll()
  if (!text) return
  ElMessage({
    message: text,
    type: success ? 'success' : 'error',
    duration: success ? 4000 : 5000,
    showClose: true,
    offset: 24,
    customClass: 'duoyu-login-message',
    dangerouslyUseHTMLString: false
  })
}
