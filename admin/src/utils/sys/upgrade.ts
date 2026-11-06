/**
 * 系统版本升级管理模块
 *
 * 提供完整的应用版本升级检测和处理功能
 *
 * ## 主要功能
 *
 * - 版本号比较和升级检测
 * - 首次访问识别和处理
 * - 旧版本数据自动清理
 * - 升级日志展示和通知
 * - 强制重新登录控制（根据升级日志配置）
 * - 版本号规范化处理
 * - 旧存储结构迁移和清理
 * - 升级流程延迟执行（确保应用完全加载）
 *
 * ## 使用场景
 *
 * - 应用启动时自动检测版本升级
 * - 版本更新后清理旧数据
 * - 向用户展示版本更新内容
 * - 重大更新时要求用户重新登录
 * - 防止旧版本数据污染新版本
 *
 * ## 工作流程
 *
 * 1. 检查本地存储的版本号
 * 2. 与当前应用版本对比
 * 3. 查找并清理旧版本数据
 * 4. 展示升级通知（包含更新日志）
 * 5. 根据配置决定是否强制重新登录
 * 6. 更新本地版本号
 *
 * @module utils/sys/upgrade
 * @author Art Design Pro Team
 */
import { upgradeLogList } from '@/mock/upgrade/changeLog'
import { ElNotification } from 'element-plus'
import { useUserStore } from '@/store/modules/user'
import { StorageConfig } from '@/utils/storage/storage-config'

/**
 * 版本管理器
 * 负责处理版本比较、升级检测和数据清理
 */
class VersionManager {
  /**
   * 规范化版本号字符串，移除前缀 'v'
   */
  private normalizeVersion(version: string): string {
    return version.replace(/^v/, '')
  }

  /**
   * 获取存储的版本号
   */
  private getStoredVersion(): string | null {
    return localStorage.getItem(StorageConfig.VERSION_KEY)
  }

  /**
   * 设置版本号到存储
   */
  private setStoredVersion(version: string): void {
    localStorage.setItem(StorageConfig.VERSION_KEY, version)
  }

  /**
   * 检查是否应该跳过升级处理
   */
  private shouldSkipUpgrade(): boolean {
    return StorageConfig.CURRENT_VERSION === StorageConfig.SKIP_UPGRADE_VERSION
  }

  /**
   * 检查是否为首次访问
   */
  private isFirstVisit(storedVersion: string | null): boolean {
    return !storedVersion
  }

  /**
   * 检查版本是否相同
   */
  private isSameVersion(storedVersion: string): boolean {
    return storedVersion === StorageConfig.CURRENT_VERSION
  }

  private migrateStoredVersion(version: string): void {
    const oldPrefix = StorageConfig.generateStorageKey('', version)
    const oldRoot = StorageConfig.generateLegacyKey(version)
    for (const key of Object.keys(localStorage)) {
      const target =
        key === oldRoot
          ? StorageConfig.generateLegacyKey()
          : key.startsWith(oldPrefix)
            ? StorageConfig.generateStorageKey(key.slice(oldPrefix.length))
            : null
      if (!target || localStorage.getItem(target) !== null) continue
      const value = localStorage.getItem(key)
      if (value !== null) localStorage.setItem(target, value)
    }
  }

  /**
   * 检查是否需要重新登录
   */
  private shouldRequireReLogin(storedVersion: string): boolean {
    const normalizedCurrent = this.normalizeVersion(StorageConfig.CURRENT_VERSION)
    const normalizedStored = this.normalizeVersion(storedVersion)

    return upgradeLogList.value.some((item) => {
      const itemVersion = this.normalizeVersion(item.version)
      return (
        item.requireReLogin &&
        itemVersion.localeCompare(normalizedStored, 'en', { numeric: true }) > 0 &&
        itemVersion.localeCompare(normalizedCurrent, 'en', { numeric: true }) <= 0
      )
    })
  }

  /**
   * 构建升级通知消息
   */
  private buildUpgradeMessage(requireReLogin: boolean): string {
    const { title: content } = upgradeLogList.value[0]

    const messageParts = [
      `<p style="color: var(--art-gray-800) !important; padding-bottom: 5px;">`,
      `系统已升级到 ${StorageConfig.CURRENT_VERSION} 版本，此次更新带来了以下改进：`,
      `</p>`,
      content
    ]

    if (requireReLogin) {
      messageParts.push(
        `<p style="color: var(--theme-color); padding-top: 5px;">升级完成，请重新登录后继续使用。</p>`
      )
    }

    return messageParts.join('')
  }

  /**
   * 显示升级通知
   */
  private showUpgradeNotification(message: string): void {
    ElNotification({
      title: '系统升级公告',
      message,
      duration: 0,
      type: 'success',
      dangerouslyUseHTMLString: true
    })
  }

  /**
   * 执行升级后的登出操作
   */
  private performLogout(): void {
    try {
      useUserStore().logOut()
      console.info('[Upgrade] 已执行升级后登出')
    } catch (error) {
      console.error('[Upgrade] 升级后登出失败:', error)
    }
  }

  /**
   * 执行升级流程
   */
  private async executeUpgrade(storedVersion: string): Promise<void> {
    try {
      if (this.getStoredVersion() !== storedVersion) return
      this.migrateStoredVersion(storedVersion)
      this.setStoredVersion(StorageConfig.CURRENT_VERSION)
      if (!upgradeLogList.value.length) return
      const requireReLogin = this.shouldRequireReLogin(storedVersion)
      this.showUpgradeNotification(this.buildUpgradeMessage(requireReLogin))

      if (requireReLogin) {
        this.performLogout()
      }

      console.info(`[Upgrade] 升级完成: ${storedVersion} → ${StorageConfig.CURRENT_VERSION}`)
    } catch (error) {
      console.error('[Upgrade] 系统升级处理失败:', error)
    }
  }

  /**
   * 系统升级处理主流程
   */
  async processUpgrade(): Promise<void> {
    // 跳过特定版本
    if (this.shouldSkipUpgrade()) {
      console.debug('[Upgrade] 跳过版本升级检查')
      return
    }

    const storedVersion = this.getStoredVersion()

    // 首次访问处理
    if (this.isFirstVisit(storedVersion)) {
      this.setStoredVersion(StorageConfig.CURRENT_VERSION)
      // console.info('[Upgrade] 首次访问，已设置当前版本')
      return
    }

    // 版本相同，无需升级
    if (this.isSameVersion(storedVersion!)) {
      // console.debug('[Upgrade] 版本相同，无需升级')
      return
    }

    // 延迟执行升级流程，确保应用已完全加载
    setTimeout(() => {
      this.executeUpgrade(storedVersion!)
    }, StorageConfig.UPGRADE_DELAY)
  }
}

// 创建版本管理器实例
const versionManager = new VersionManager()

/**
 * 系统升级处理入口函数
 */
export async function systemUpgrade(): Promise<void> {
  await versionManager.processUpgrade()
}
