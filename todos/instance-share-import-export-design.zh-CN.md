# 实例分享导入导出设计链路与分析思路

> 目标：支持 Ant Browser 用户跨系统分享单个或多个浏览器实例。例如用户 A 在 macOS 导出一个实例，用户 B 在 Windows 导入后得到一个可直接启动的新实例，并尽可能保留完整用户数据目录。

## 1. 背景与产品价值

当前项目已经具备“系统级全量备份/恢复”基础，但用户真实增长场景更需要“实例分享”：

- 团队成员之间共享已配置好的浏览器环境；
- 运营人员快速分发账号环境、代理配置、指纹参数；
- 测试人员复用带扩展、缓存、本地存储的环境；
- 用户跨设备、跨系统迁移单个实例；
- 降低新用户配置成本，增强浏览器生态传播能力。

因此建议将该能力定位为：

> 实例分享包 / Instance Package

而不是简单叫“备份恢复”。

## 2. 当前项目已有基础

项目中已经存在全局备份相关能力，主要文件包括：

```text
/Users/zest/myworks/Ant-Browser/backend/internal/backup/spec_scope.go
/Users/zest/myworks/Ant-Browser/backend/app_backup_entry.go
/Users/zest/myworks/Ant-Browser/backend/app_backup_archive_write.go
/Users/zest/myworks/Ant-Browser/backend/app_backup_import_flow.go
/Users/zest/myworks/Ant-Browser/backend/app_backup_file_import.go
/Users/zest/myworks/Ant-Browser/backend/app_backup_data_merge.go
/Users/zest/myworks/Ant-Browser/frontend/src/modules/settings/components/BackupSettingsCard.tsx
```

现有能力覆盖：

- `config.yaml` 导出；
- `proxies.yaml` 导出；
- `data/` 目录导出；
- 浏览器用户数据根目录导出；
- SQLite 数据库导出与合并；
- ZIP manifest 校验；
- 文件树同步；
- 前端导入导出入口与进度事件。

已确认相关测试可通过：

```bash
go test ./backend/internal/backup ./backend -run 'Backup|BuildScope'
```

结论：本需求不是从零开发，而是在现有全量备份基础上新增“实例级分享包协议”和“跨系统导入重映射”。

## 3. 当前全量备份与实例分享的区别

### 3.1 当前全量备份

适合：

- 整套 Ant Browser 数据备份；
- 当前机器恢复；
- 初始化后全量导入；
- 现有数据上合并导入。

问题：

- 粒度太大；
- 不适合用户之间只分享一个实例；
- 跨系统时 `core_path`、`user_data_root`、绝对路径等需要重写；
- 不适合直接保留原 `profileId`，否则容易冲突。

### 3.2 实例分享包

适合：

- 单实例导出；
- 多实例批量导出；
- 导入为新实例；
- 跨系统路径自动重写；
- 代理、指纹、启动参数、用户数据目录一起迁移。

核心原则：

> 分享包内部保存源实例事实，导入目标环境时重建目标实例身份。

即：

- 原 `profileId` 作为来源 ID 记录，不直接复用；
- 导入时生成新 `profileId`；
- 导入时生成新 `userDataDir`；
- 用户数据目录完整复制到目标系统当前 `UserDataRoot` 下；
- 内核使用目标系统默认内核或用户选择的内核；
- 代理配置作为快照导入或转为直连。

## 4. 用户数据完整性的准确边界

需要区分两个概念：

### 4.1 文件级完整导入

可以做到。

包括但不限于：

- 实例配置；
- 指纹参数；
- 启动参数；
- 标签、关键词、分组；
- 代理配置快照；
- 书签；
- 历史记录；
- 扩展文件；
- Preferences / Secure Preferences；
- LocalStorage；
- IndexedDB；
- Cache；
- Session Storage；
- Cookies 数据库文件；
- 整个 Chromium 用户数据目录。

### 4.2 跨系统后 100% 可解密可用

不能绝对保证。

原因是 Chromium 对部分敏感数据使用系统级加密：

- macOS：Keychain；
- Windows：DPAPI；
- Linux：Secret Service / libsecret。

所以 macOS 导出的某些 Cookie、密码、登录态 Token，在 Windows 上可能由于缺少原系统密钥而无法解密。

产品文案建议：

> 支持跨系统完整导入实例配置与用户数据目录。历史记录、书签、扩展、本地存储等数据会随实例迁移；由于不同系统浏览器加密机制差异，部分登录态、密码或加密 Cookie 可能需要重新登录。

## 5. 推荐包格式

建议新增独立格式：

```text
ant-browser-instance-package
```

文件扩展名可选：

```text
*.ant-profile.zip
*.antbrowser.zip
```

推荐目录结构：

```text
manifest.json
payload/
  profiles/
    <sourceProfileId>/
      profile.json
      launch_code.json
      proxy.json
      group.json
      user-data/
```

第一版 MVP 可以只支持单实例：

```text
manifest.json
payload/
  profile.json
  launch_code.json
  proxy.json
  group.json
  user-data/
```

## 6. manifest 设计

示例：

```json
{
  "format": "ant-browser-instance-package",
  "version": 1,
  "createdAt": "2026-07-02T12:00:00Z",
  "source": {
    "appName": "Ant Browser",
    "appVersion": "unknown",
    "os": "darwin",
    "arch": "arm64"
  },
  "profiles": [
    {
      "sourceProfileId": "...",
      "profileName": "示例实例",
      "sourceUserDataDir": "...",
      "coreId": "...",
      "coreName": "...",
      "proxyId": "...",
      "hasUserData": true,
      "userDataArchivePath": "payload/profile/user-data/"
    }
  ],
  "compatibility": {
    "crossOS": true,
    "requiresCoreRemap": true,
    "encryptedBrowserDataMayNeedRelogin": true
  }
}
```

原则：

- manifest 不保存源机器绝对路径，或只放在 debug 字段；
- 所有 archive path 使用 `/`；
- 明确 `sourceOS`，供导入时提示；
- 明确 `requiresCoreRemap`，不要跨系统硬套内核路径。

## 7. 导出链路设计

入口：实例列表 / 实例详情页新增“导出实例”。

后端接口建议：

```go
func (a *App) BrowserInstanceExportPackage(profileId string) (map[string]interface{}, error)
```

导出步骤：

1. 校验实例存在；
2. 如果实例正在运行，提示用户关闭，或后端先停止该实例；
3. 读取实例配置：`browser_profiles`；
4. 读取启动码：`launch_codes`；
5. 读取代理依赖：`browser_proxies`，优先导出绑定代理快照；
6. 读取分组依赖：`browser_groups`；
7. 解析用户数据目录：

   ```go
   userDataPath := a.browserMgr.ResolveUserDataDir(profile)
   ```

8. 构建 manifest；
9. 将 profile/proxy/group/launch_code 写成 JSON；
10. 将完整 `userDataPath` 复制进 ZIP 的 `payload/.../user-data/`；
11. 跳过符号链接；
12. 跳过明显临时锁文件，例如：
    - `SingletonLock`
    - `SingletonSocket`
    - `SingletonCookie`
    - `*.lock`
13. 写入 ZIP；
14. 前端展示导出路径、文件数、大小、耗时。

## 8. 导入链路设计

入口：实例列表新增“导入实例”。

后端接口建议：

```go
func (a *App) BrowserInstanceImportPackage() (map[string]interface{}, error)
```

导入步骤：

1. 选择 ZIP；
2. 解压到临时目录；
3. 校验 `manifest.json`：
   - format；
   - version；
   - payload 结构；
4. 读取 `profile.json`；
5. 生成新的目标身份：

   ```text
   newProfileId = uuid.NewString()
   newUserDataDir = newProfileId
   ```

6. 处理实例名称冲突：
   - 默认追加 `（导入）`；
   - 若仍冲突，追加序号；
7. 选择目标内核：
   - 默认使用当前系统默认内核；
   - 如果没有默认内核，则提示用户先导入/配置内核；
8. 处理代理策略：
   - MVP：导入代理配置快照；
   - 如果代理冲突，复用已有相同 `proxy_config`；
   - 如果不想导入代理，可转为直连；
9. 复制用户数据目录到：

   ```text
   <current UserDataRoot>/<newProfileId>/
   ```

10. 写入 `browser_profiles`；
11. 写入或重建 `launch_codes`；
12. 写入代理/分组依赖；
13. 刷新运行时配置；
14. 返回导入结果。

## 9. 路径重写规则

跨系统分享的关键是路径重写。

### 9.1 user_data_dir

导出包中记录源值，但导入时不直接使用。

导入规则：

```text
source userDataDir: 仅记录来源
目标 userDataDir: 新 profileId 或安全生成的新目录名
```

避免：

- macOS 绝对路径导入 Windows；
- Windows 盘符路径导入 macOS；
- 与本机已有实例目录冲突。

### 9.2 core_path

导出包中记录源 `coreId/coreName/corePath` 作为来源信息。

导入规则：

- 默认映射到当前系统默认内核；
- 不复用源 `corePath`；
- 后续可支持导入弹窗手动选择目标内核。

### 9.3 proxy

代理配置通常是跨系统可复用的字符串，可以导入。

规则：

- 如果目标已有相同 `proxy_config`，复用目标代理；
- 如果没有，创建新代理；
- 如果用户选择“直连导入”，清空 `proxyId/proxyConfig`。

## 10. 冲突处理策略

### 10.1 profileId 冲突

不复用源 ID，直接生成新 ID。

### 10.2 userDataDir 冲突

不复用源目录名，默认使用新 ID。

### 10.3 实例名称冲突

示例：

```text
原名称: 店铺A
导入名: 店铺A（导入）
再次冲突: 店铺A（导入 2）
```

### 10.4 代理冲突

按 `proxy_config` 去重。

### 10.5 分组冲突

按 `parent_id + group_name` 去重，存在则复用，否则创建。

### 10.6 启动码冲突

默认不保留源启动码，重新生成。

## 11. 需要新增的后端模块

建议新增：

```text
/Users/zest/myworks/Ant-Browser/backend/internal/instancepkg/types.go
/Users/zest/myworks/Ant-Browser/backend/internal/instancepkg/export.go
/Users/zest/myworks/Ant-Browser/backend/internal/instancepkg/import.go
/Users/zest/myworks/Ant-Browser/backend/internal/instancepkg/archive.go
/Users/zest/myworks/Ant-Browser/backend/app_instance_package_export.go
/Users/zest/myworks/Ant-Browser/backend/app_instance_package_import.go
```

也可以先不抽 internal 包，MVP 直接放在 `backend/app_instance_package_*.go`，稳定后再抽象。

## 12. 需要新增的前端入口

推荐位置：

```text
/Users/zest/myworks/Ant-Browser/frontend/src/modules/browser/pages/BrowserListPage.tsx
/Users/zest/myworks/Ant-Browser/frontend/src/modules/browser/pages/BrowserDetailPage.tsx
/Users/zest/myworks/Ant-Browser/frontend/src/modules/browser/api/instances.ts
```

UI 行为：

- 实例行操作新增“导出实例”；
- 页面顶部新增“导入实例”；
- 导入弹窗展示跨系统提示；
- 导入成功后刷新实例列表；
- 导入结果展示：
  - 新实例名称；
  - 新 profileId；
  - 是否导入用户数据；
  - 是否导入代理；
  - 是否进行了内核重映射。

## 13. MVP 范围

第一版建议只做：

1. 单实例导出；
2. 单实例导入为新实例；
3. 附带完整用户数据目录；
4. 附带实例配置、指纹、启动参数、标签、关键词；
5. 附带代理配置快照；
6. 导入时使用当前系统默认内核；
7. 导入时生成新 `profileId/userDataDir/launchCode`；
8. 基础 ZIP 校验与 ZipSlip 防护；
9. 前端按钮与结果提示。

暂不做：

- 多实例批量分享；
- 分享包密码加密；
- 云分享；
- 内核随包跨系统分发；
- 登录态修复；
- 用户自定义导入映射 UI。

## 14. 稳定版增强

MVP 后建议增强：

1. 导入前预览；
2. 用户选择代理策略：
   - 导入代理；
   - 改为直连；
   - 手动选择本机代理；
3. 用户选择目标内核；
4. 多实例批量导出/导入；
5. 导入失败回滚；
6. 导入后完整度报告；
7. 导出包加密；
8. 大文件进度统计；
9. 导入包版本兼容。

## 15. 测试设计

### 15.1 单元测试

新增测试覆盖：

- manifest marshal/unmarshal；
- ZIP 防穿越；
- 实例名称冲突；
- 代理去重；
- userDataDir 重写；
- 默认内核映射；
- 缺少 user-data 时的降级行为。

### 15.2 集成测试

构造临时 app root：

1. 创建测试实例；
2. 写入模拟用户数据目录；
3. 导出 ZIP；
4. 在另一个临时 app root 导入；
5. 断言：
   - 新实例存在；
   - `profileId` 已变化；
   - `userDataDir` 已变化；
   - 用户数据文件完整复制；
   - 代理按规则导入/复用；
   - 内核映射到目标默认内核。

### 15.3 手工验收

核心验收场景：

```text
macOS 用户 A：
1. 创建实例；
2. 登录网站或写入 LocalStorage；
3. 安装扩展；
4. 导出实例包。

Windows 用户 B：
1. 导入实例包；
2. 实例列表出现新实例；
3. 启动实例；
4. 检查书签、历史、本地存储、扩展是否存在；
5. 如果登录态失效，提示用户重新登录。
```

## 16. 风险与应对

### 风险 1：跨系统登录态不可用

应对：产品提示 + 导入报告，不承诺 100%。

### 风险 2：导出时实例仍在运行，文件不一致

应对：导出前要求停止实例，或自动停止目标实例。

### 风险 3：用户数据目录很大

应对：进度事件、文件计数、可取消能力后续补充。

### 风险 4：目标系统没有可用内核

应对：导入前校验默认内核，缺失时提示先配置内核。

### 风险 5：ZIP 安全问题

应对：复用现有 `unzipTo` 的 ZipSlip 校验，并对实例包再做 archive path 白名单。

## 17. 时间与把握

### 文件级完整导入导出

把握：85%～90%。

原因：已有全量备份、ZIP、文件同步、数据库 DAO、前端 Wails 绑定基础。

### 跨系统登录态 100% 可用

不能保证。

原因：Chromium 使用系统级加密，部分 Cookie/密码/Token 可能无法在异构系统解密。

### 时间预估

MVP：约 1 天。

稳定可上线版本：约 2～3 天。

产品化完整版本：约 4～5 天。

## 18. 推荐实施顺序

1. 新增实例包 types/manifest；
2. 实现单实例导出后端；
3. 实现单实例导入后端；
4. 加测试：导出 -> 导入 -> 校验新实例与用户目录；
5. 前端实例列表增加导出/导入入口；
6. 加跨系统风险提示；
7. 手工验证 macOS -> Windows 或模拟跨 root 导入；
8. 再做导入预览、代理策略、目标内核选择。

## 19. 当前结论

该能力值得做，且技术上可行。

准确表述为：

> 支持跨系统完整导入实例配置与用户数据目录；导入后会在目标系统生成一个新实例，并自动重写路径、映射内核、复制用户数据。由于 Chromium/系统加密机制差异，部分登录态、密码或加密 Cookie 跨系统后可能需要重新登录。

这是最稳妥、最符合实际的产品与技术边界。
