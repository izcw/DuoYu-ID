/**
 * 路由全局前置守卫模块
 *
 * 提供完整的路由导航守卫功能
 *
 * ## 主要功能
 *
 * - 登录状态验证和重定向
 * - 动态路由注册和权限控制
 * - 菜单数据获取和处理（前端/后端模式）
 * - 用户信息获取和缓存
 * - 页面标题设置
 * - 工作标签页管理
 * - 进度条和加载动画控制
 * - 静态路由识别和处理
 * - 错误处理和异常跳转
 *
 * ## 使用场景
 *
 * - 路由跳转前的权限验证
 * - 动态菜单加载和路由注册
 * - 用户登录状态管理
 * - 页面访问控制
 * - 路由级别的加载状态管理
 *
 * ## 工作流程
 *
 * 1. 检查登录状态，未登录跳转到登录页
 * 2. 首次访问时获取用户信息和菜单数据
 * 3. 根据权限动态注册路由
 * 4. 设置页面标题和工作标签页
 * 5. 处理根路径重定向到首页
 * 6. 未匹配路由跳转到 404 页面
 *
 * @module router/guards/beforeEach
 * @author Art Design Pro Team
 */
import type { Router, RouteLocationNormalized, NavigationGuardNext } from 'vue-router'
import { nextTick } from 'vue'
import NProgress from 'nprogress'
import { useSettingStore } from '@/store/modules/setting'
import { useUserStore } from '@/store/modules/user'
import { useMenuStore } from '@/store/modules/menu'
import { setWorktab } from '@/utils/navigation'
import { setPageTitle } from '@/utils/router'
import { RoutesAlias } from '../routesAlias'

import { loadingService } from '@/utils/ui'
import { useCommon } from '@/hooks/core/useCommon'
import { useWorktabStore } from '@/store/modules/worktab'
import { RouteRegistry, MenuProcessor, IframeRouteManager, RoutePermissionValidator } from '../core'

// 路由注册器实例
let routeRegistry: RouteRegistry | null = null

// 菜单处理器实例
const menuProcessor = new MenuProcessor()

// 跟踪是否需要关闭 loading
let pendingLoading = false

// Share initialization while the user navigates. A logout invalidates any pending result.
let initialization: Promise<void> | undefined
let sessionGeneration = 0
let navigation = 0
let staticNames = new Set<string | symbol | undefined | null>()

/**
 * 获取 pendingLoading 状态
 */
export function getPendingLoading(): boolean {
  return pendingLoading
}

/**
 * 重置 pendingLoading 状态
 */
export function resetPendingLoading(): void {
  pendingLoading = false
}

/**
 * 设置路由全局前置守卫
 */
export function setupBeforeEachGuard(router: Router): void {
  // 初始化路由注册器
  routeRegistry = new RouteRegistry(router)
  staticNames = new Set(router.getRoutes().map((route) => route.name))

  router.beforeEach(
    async (
      to: RouteLocationNormalized,
      from: RouteLocationNormalized,
      next: NavigationGuardNext
    ) => {
      try {
        await handleRouteGuard(to, from, next, router, ++navigation)
      } catch (error) {
        console.error('[RouteGuard] 路由守卫处理失败:', error)
        closeLoading()
        next({ name: 'Exception500' })
      }
    }
  )
}

/**
 * 关闭 loading 效果
 */
function closeLoading(): void {
  if (pendingLoading) {
    nextTick(() => {
      loadingService.hideLoading()
      pendingLoading = false
    })
  }
}

/**
 * 处理路由守卫逻辑
 */
async function handleRouteGuard(
  to: RouteLocationNormalized,
  from: RouteLocationNormalized,
  next: NavigationGuardNext,
  router: Router,
  request: number
): Promise<void> {
  const settingStore = useSettingStore()
  const userStore = useUserStore()

  // 启动进度条
  if (settingStore.showNprogress) {
    NProgress.start()
  }

  // 1. 检查登录状态
  if (!handleLoginStatus(to, userStore, next)) {
    return
  }

  if (!routeRegistry?.isRegistered() && userStore.isLogin && to.name !== 'Exception500') {
    await handleDynamicRoutes(to, next, router, request)
    return
  }

  // 4. 处理根路径重定向
  if (handleRootPathRedirect(to, next)) {
    return
  }

  // 5. 处理已匹配的路由
  if (to.matched.length > 0) {
    setWorktab(to)
    setPageTitle(to)
    next()
    return
  }

  // 6. 未匹配到路由，跳转到 404
  next({ name: 'Exception404' })
}

/**
 * 处理登录状态
 * @returns true 表示可以继续，false 表示已处理跳转
 */
function handleLoginStatus(
  to: RouteLocationNormalized,
  userStore: ReturnType<typeof useUserStore>,
  next: NavigationGuardNext
): boolean {
  // 已登录或访问登录页或静态路由，直接放行
  if (userStore.isLogin || to.path === RoutesAlias.Login || isStaticRoute(to)) {
    return true
  }

  // 未登录且访问需要权限的页面，跳转到登录页并携带 redirect 参数
  userStore.logOut()
  next({
    name: 'Login',
    query: { redirect: to.fullPath }
  })
  return false
}

/**
 * 检查路由是否为静态路由
 */
function isStaticRoute(to: RouteLocationNormalized): boolean {
  return to.name !== 'Exception404' && staticNames.has(to.name)
}

/**
 * 处理动态路由注册
 */
async function initializeRoutes(router: Router, generation: number): Promise<void> {
  const menuList = await menuProcessor.getMenuList()
  if (generation !== sessionGeneration) return
  if (!menuProcessor.validateMenuList(menuList)) throw new Error('菜单列表不可用')
  try {
    routeRegistry!.register(menuList)
    const store = useMenuStore()
    store.setMenuList(menuList)
    store.addRemoveRouteFns(routeRegistry!.getRemoveRouteFns())
    IframeRouteManager.getInstance().save()
    useWorktabStore().validateWorktabs(router)
  } catch (error) {
    clearRoutes()
    throw error
  }
}

async function handleDynamicRoutes(
  to: RouteLocationNormalized,
  next: NavigationGuardNext,
  router: Router,
  request: number
): Promise<void> {
  const generation = sessionGeneration
  if (!initialization) {
    pendingLoading = true
    loadingService.showLoading()
    initialization = initializeRoutes(router, generation).finally(() => {
      if (sessionGeneration === generation) initialization = undefined
    })
  }
  try {
    await initialization
  } catch (error) {
    if (request !== navigation || generation !== sessionGeneration) {
      next()
      return
    }
    console.error('[RouteGuard] 动态路由注册失败:', error)
    closeLoading()
    next({ name: 'Exception500', replace: true })
    return
  }
  if (request !== navigation || generation !== sessionGeneration) {
    next()
    return
  }
  if (!isStaticRoute(to)) {
    const { homePath } = useCommon()
    const target = RoutePermissionValidator.validatePath(
      to.path,
      useMenuStore().menuList,
      homePath.value || '/'
    )
    if (!target.hasPermission) {
      closeLoading()
      next({ path: target.path, replace: true })
      return
    }
  }
  next({ path: to.path, query: to.query, hash: to.hash, replace: true })
}

function clearRoutes(): void {
  routeRegistry?.unregister()
  IframeRouteManager.getInstance().clear()
  const store = useMenuStore()
  store.removeAllDynamicRoutes()
  store.setMenuList([])
}

export function resetRouterState(): void {
  sessionGeneration++
  initialization = undefined
  clearRoutes()
  closeLoading()
}

/**
 * 处理根路径重定向到首页
 * @returns true 表示已处理跳转，false 表示无需跳转
 */
function handleRootPathRedirect(to: RouteLocationNormalized, next: NavigationGuardNext): boolean {
  if (to.path !== '/') {
    return false
  }

  const { homePath } = useCommon()
  if (homePath.value && homePath.value !== '/') {
    next({ path: homePath.value, replace: true })
    return true
  }

  return false
}
