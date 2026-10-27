<template>
  <div class="dy-page" v-loading="loading">
    <div class="dy-heading"
      ><div
        ><h1>欢迎回来，{{ user.info.userName }}</h1
        ><p>账号与认证服务概览</p></div
      ><div class="dy-toolbar"
        ><el-button :loading="loading" @click="load">刷新</el-button
        ><el-button type="primary" @click="$router.push('/manage/apps/edit')"
          >创建项目</el-button
        ></div
      ></div
    >
    <div v-if="error" class="dy-error">{{ error }}</div>
    <div class="dy-stats"
      ><div class="dy-stat" v-for="m in metrics" :key="m.label"
        ><span>{{ m.label }}</span
        ><strong>{{ m.value ?? '—' }}</strong
        ><span>{{ m.note }}</span></div
      ></div
    >
    <section class="dy-panel"
      ><div class="dy-heading"
        ><div><h2>登录趋势</h2><p>中心登录成功与失败次数 · 北京时间</p></div
        ><el-radio-group v-model="days" @change="load"
          ><el-radio-button :value="7">7 天</el-radio-button
          ><el-radio-button :value="14">14 天</el-radio-button
          ><el-radio-button :value="30">30 天</el-radio-button></el-radio-group
        ></div
      >
      <div class="dy-chart-legend"
        ><span>● 成功 {{ successTotal }}</span
        ><span class="dy-muted">● 失败 {{ failedTotal }}</span></div
      >
      <div v-if="series.length" class="dy-chart"
        ><svg viewBox="0 0 900 240" role="img" aria-label="登录成功与失败趋势柱状图">
          <g v-for="n in 5" :key="n">
            <line
              x1="40"
              x2="895"
              :y1="20 + (n - 1) * 45"
              :y2="20 + (n - 1) * 45"
              stroke="currentColor"
              opacity=".09"
            />
            <text x="0" :y="24 + (n - 1) * 45" fill="currentColor" opacity=".5" font-size="11">
              {{ Math.round((maxLogin * (5 - n)) / 4) }}
            </text>
          </g>
          <g v-for="(p, i) in series" :key="p.date">
            <rect
              :x="44 + i * step"
              :y="200 - (p.success / maxLogin) * 180"
              :width="step * 0.32"
              :height="(p.success / maxLogin) * 180"
              fill="currentColor"
              rx="2"
            >
              <title>{{ p.date }} 成功 {{ p.success }} 次</title>
            </rect>
            <rect
              :x="44 + i * step + step * 0.35"
              :y="200 - (p.failed / maxLogin) * 180"
              :width="step * 0.25"
              :height="(p.failed / maxLogin) * 180"
              fill="#aaa"
              rx="2"
            >
              <title>{{ p.date }} 失败 {{ p.failed }} 次</title>
            </rect>
            <text
              v-if="i % Math.ceil(series.length / 7) === 0 || i === series.length - 1"
              :x="44 + i * step"
              y="228"
              fill="currentColor"
              opacity=".55"
              font-size="11"
            >
              {{ p.date.slice(5) }}
            </text>
          </g>
        </svg></div
      ><p v-if="!successTotal && !failedTotal" class="dy-muted"
        >这个时间段暂无登录事件。</p
      ></section
    >
    <div class="dy-grid"
      ><section class="dy-panel"
        ><h2>新增账号</h2><p class="dy-muted">近 {{ days }} 天新增 {{ registrationTotal }} 个账号</p
        ><div class="dy-registration-chart"
          ><div
            v-for="p in series"
            :key="p.date"
            :title="p.date + '：新增 ' + p.registrations + ' 个账号'"
            ><span
              :style="{
                height: Math.max(2, (p.registrations / maxRegistration) * 130) + 'px',
                opacity: p.registrations ? 1 : 0.15
              }"
            ></span></div></div
        ><div class="dy-chart-axis"
          ><span>{{ series[0]?.date }}</span
          ><span>{{ series[series.length - 1]?.date }}</span></div
        ></section
      ><section class="dy-panel"
        ><h2>项目累计授权</h2><p class="dy-muted">按授权码成功兑换次数排序 · 从记录功能启用起累计</p
        ><div v-if="!projects.length" class="dy-empty">还没有接入项目</div
        ><div class="dy-rank" v-for="(p, i) in projects" :key="p.name"
          ><div
            ><span>{{ i + 1 }} &nbsp; {{ p.name }}</span
            ><span>{{ p.logins }} 次 · {{ p.users }} 位用户</span></div
          ><div class="dy-track"
            ><span
              :style="{ width: Math.max(0, (p.logins / maxProject) * 100) + '%' }"
            ></span></div></div></section
    ></div>
    <section class="dy-panel"
      ><div class="dy-heading"
        ><div
          ><h2>接入服务</h2><span class="dy-code">{{ stats.issuer }}</span></div
        ><el-button @click="$router.push('/manage/docs')">接入文档 →</el-button></div
      ><p class="dy-muted"
        >各项目保留自己的业务数据。中心会话统计不代表各项目的在线人数。</p
      ></section
    >
  </div>
</template>
<script setup lang="ts">
  import { ref, computed, onMounted } from 'vue'
  import { api } from '@/api/duoyu'
  import { useUserStore } from '@/store/modules/user'
  const user = useUserStore(),
    stats = ref<any>({}),
    series = ref<any[]>([]),
    projects = ref<any[]>([]),
    days = ref(14),
    loading = ref(false),
    error = ref('')
  const metrics = computed(() => [
    { label: '账号总数', value: stats.value.users, note: '统一身份账号' },
    { label: '接入项目', value: stats.value.apps, note: '独立认证配置' },
    { label: '有效中心会话', value: stats.value.sessions, note: '未过期的中心登录' },
    {
      label: '近 24 小时成功登录',
      value: stats.value.success,
      note: '失败 ' + (stats.value.failed ?? '—') + ' 次'
    }
  ])
  const successTotal = computed(() => series.value.reduce((s, p) => s + p.success, 0)),
    failedTotal = computed(() => series.value.reduce((s, p) => s + p.failed, 0)),
    registrationTotal = computed(() => series.value.reduce((s, p) => s + p.registrations, 0))
  const maxLogin = computed(() =>
      Math.max(4, ...series.value.flatMap((p) => [p.success, p.failed]))
    ),
    maxRegistration = computed(() => Math.max(1, ...series.value.map((p) => p.registrations))),
    maxProject = computed(() => Math.max(1, ...projects.value.map((p) => p.logins))),
    step = computed(() => 850 / Math.max(1, series.value.length))
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const [summary, chart] = await Promise.all([
        api('/api/admin/overview'),
        api('/api/admin/analytics?days=' + days.value)
      ])
      if (current !== generation) return
      stats.value = summary
      series.value = chart.series
      projects.value = chart.projects
    } catch (e: any) {
      if (current === generation) error.value = e.message
    } finally {
      if (current === generation) loading.value = false
    }
  }
  onMounted(load)
</script>
