import type { AppRouteRecord } from '@/types/router'
export const routeModules: AppRouteRecord[] = [
  {
    path: '/manage',
    name: 'Manage',
    component: '/index/index',
    meta: {
      title: '工作台',
      icon: 'ri:dashboard-line',
      roles: ['R_SUPER']
    },
    children: [
      {
        path: 'overview',
        name: 'Console',
        component: '/duoyu/overview',
        meta: {
          title: '概览',
          icon: 'ri:pie-chart-line',
          fixedTab: true
        }
      },
      {
        path: 'users',
        name: 'DuoyuUsers',
        component: '/duoyu/manage',
        meta: {
          title: '账号管理',
          icon: 'ri:user-3-line'
        }
      },
      {
        path: 'users/detail',
        name: 'DuoyuUserDetail',
        component: '/duoyu/editor',
        meta: {
          title: '账号资料',
          icon: '',
          isHide: true,
          activePath: '/manage/users'
        }
      }
    ]
  },
  {
    path: '/integrations',
    name: 'Integrations',
    component: '/index/index',
    meta: {
      title: '应用接入',
      icon: 'ri:apps-2-line',
      roles: ['R_SUPER']
    },
    children: [
      {
        path: '/manage/apps',
        name: 'DuoyuApps',
        component: '/duoyu/manage',
        meta: {
          title: '项目管理',
          icon: 'ri:apps-2-line'
        }
      },
      {
        path: '/manage/keys',
        name: 'DuoyuKeys',
        component: '/duoyu/manage',
        meta: {
          title: 'API 凭证',
          icon: 'ri:key-2-line'
        }
      },
      {
        path: '/manage/apps/edit',
        name: 'DuoyuEditor0',
        component: '/duoyu/editor',
        meta: {
          title: '项目配置',
          icon: '',
          isHide: true,
          activePath: '/manage/apps'
        }
      },
      {
        path: '/manage/apps/grants',
        name: 'DuoyuEditor1',
        component: '/duoyu/editor',
        meta: {
          title: '准入名单',
          icon: '',
          isHide: true,
          activePath: '/manage/apps'
        }
      },
      {
        path: '/manage/apps/members',
        name: 'DuoyuEditor2',
        component: '/duoyu/editor',
        meta: {
          title: '登录用户',
          icon: '',
          isHide: true,
          activePath: '/manage/apps'
        }
      },
      {
        path: '/manage/keys/edit',
        name: 'DuoyuEditor3',
        component: '/duoyu/editor',
        meta: {
          title: '凭证配置',
          icon: '',
          isHide: true,
          activePath: '/manage/keys'
        }
      },
      {
        path: '/manage/mail',
        name: 'DuoyuMail',
        component: '/duoyu/mail',
        meta: { title: '项目邮件', icon: 'ri:mail-line' }
      },
      {
        path: '/manage/docs',
        name: 'DuoyuDocs',
        component: '/duoyu/docs',
        meta: {
          title: '接入文档',
          icon: 'ri:book-open-line'
        }
      }
    ]
  },
  {
    path: '/operations',
    name: 'Operations',
    component: '/index/index',
    meta: {
      title: '安全与设置',
      icon: 'ri:shield-check-line',
      roles: ['R_SUPER']
    },
    children: [
      {
        path: '/manage/audit',
        name: 'DuoyuAudit',
        component: '/duoyu/manage',
        meta: {
          title: '安全日志',
          icon: 'ri:shield-check-line'
        }
      },
      {
        path: '/manage/settings',
        name: 'DuoyuSettings',
        component: '/duoyu/settings',
        meta: {
          title: '邮件与设置',
          icon: 'ri:settings-3-line'
        }
      }
    ]
  },
  {
    path: '/account',
    name: 'Account',
    component: '/index/index',
    meta: {
      title: '我的账号',
      icon: 'ri:account-circle-line'
    },
    children: [
      {
        path: 'profile',
        name: 'UserCenter',
        component: '/duoyu/profile',
        meta: {
          title: '账号与安全',
          icon: 'ri:user-settings-line'
        }
      },
      {
        path: 'security',
        name: 'AccountSecurity',
        component: '/duoyu/profile',
        meta: {
          title: '安全设置',
          icon: '',
          isHide: true,
          activePath: '/account/profile'
        }
      }
    ]
  }
]
