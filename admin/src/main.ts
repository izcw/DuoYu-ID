import App from './App.vue'
import { createApp } from 'vue'
import { initStore } from './store'
import { initRouter } from './router'
import language from './locales'
import '@styles/core/tailwind.css'
import '@styles/index.scss'
import './duoyu.css'
import { setupGlobDirectives } from './directives'
import { setupErrorHandle } from './utils/sys/error-handle'
import { useUserStore } from './store/modules/user'
async function start() {
 const app=createApp(App); initStore(app)
 const response=await fetch('/api/user/info',{credentials:'same-origin'})
 if(!response.ok){window.location.replace('/login');return}
 const {data}=await response.json()
 const store=useUserStore();store.setUserInfo(data);store.setLoginStatus(true);store.checkAndClearWorktabs()
 initRouter(app);setupGlobDirectives(app);setupErrorHandle(app);app.use(language);app.mount('#app')
}
start().catch(()=>{document.body.textContent='暂时无法连接多鱼账号，请刷新页面重试。'})