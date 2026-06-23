# 指纹优化优先级任务拆解

本文是 `FINGERPRINT_OPTIMIZATION_PLAN.zh-CN.md` 的执行版任务清单，用于把 Camoufox 与 CloakBrowser 的可借鉴逻辑拆成可开发、可验收、可回归的阶段任务。

原则：

- 先修跨信号一致性，再扩大指纹覆盖面。
- 先做 Ant 当前架构能承接的参数层和启动策略，再推进 Chromium 内核 patch。
- 先建立采集和验收闭环，再用检测站分数指导下一轮优化。
- 不直接照搬 Firefox 指纹；所有外部数据必须转换成 Chromium 身份。

## P0：启动链路与基础一致性

目标：减少最明显的异常组合，确保默认实例不会因为配置冲突、代理泄露或物理矛盾被快速识别。

### P0-1：启动参数去重与覆盖顺序

任务：

- 统一管理单值参数：`--fingerprint-*`、`--lang`、`--timezone`、`--window-size`、`--webrtc-ip-handling-policy`。
- 明确覆盖顺序：自动 seed < profile fingerprint args < profile launch args < runtime extra launch args。
- 支持 `--key=value` 与 `--key value` 两种写法，最终规范成单一形式。
- 系统接管参数继续禁止外部覆盖：`--user-data-dir`、`--remote-debugging-port`、`--proxy-server` 等。

验收：

- 同一个单值参数最终只出现一次。
- 用户临时启动参数能覆盖 profile 默认参数。
- 系统接管参数不会被用户输入覆盖。
- 日志中能看出被忽略的系统接管参数。

### P0-2：稳定 seed 与 profile 生命周期

任务：

- 每个 profile 固定一个稳定 `--fingerprint=<seed>`。
- 新建 profile 时自动生成 seed。
- 复制 profile 时生成新 seed，除非用户明确要求复制完整身份。
- 提供“重新生成指纹”动作，重新生成 seed 和关联指纹字段。

验收：

- 同一 profile 多次启动 Canvas/Audio 等噪声稳定。
- 不同 profile 默认 seed 不相同。
- 用户点击重新生成后 seed 变化，并记录更新时间。

### P0-3：WebRTC 出口 IP 一致性

任务：

- 默认启用 `--fingerprint-webrtc-ip=auto`。
- 存在代理时解析代理出口 IP，并写入 `--fingerprint-webrtc-ip=<exit-ip>`。
- 直连或解析失败时移除 `auto`，避免把字符串 `auto` 直接交给内核。
- 出口 IP 解析使用总超时预算，避免启动被多个 echo 服务串行拖慢。
- 日志中的代理 URL 必须脱敏。

验收：

- 代理启动时 WebRTC 候选 IP 与代理出口一致。
- 直连启动时不出现随机伪造公网 IP。
- 代理账号密码不会出现在日志中。
- 解析失败不阻塞浏览器启动。

### P0-4：默认指纹参数扩展

任务：

- 按宿主 OS 提供基础默认参数：platform、locale、timezone、screen、hardware、WebGL、fonts、WebRTC、DNT、touch。
- 保持默认品牌为 Chrome。
- 不引入 Firefox UA 或 Firefox 专属字段。

验收：

- Windows / macOS / Linux 默认参数各自符合平台身份。
- 默认参数能在前端指纹面板中正常反序列化和编辑。
- `publish/config.init*.yaml` 与后端默认值保持一致。

### P0-5：窗口与 viewport 物理一致性

任务：

- headed 模式避免额外 viewport emulation。
- 保证 `screen`、`avail`、`inner`、`outer` 不出现明显物理矛盾。
- 为 headless 模式单独建立合理 viewport 模板。

验收：

- 不出现 `outerWidth < innerWidth`。
- 不出现 `availHeight > screen.height`。
- headed 与 headless 检测报告分开记录。

## P1：Ant 原生指纹库与一致性生成器

目标：从“固定参数模板”升级为“按平台、地区、代理出口生成成套 Chromium 身份”。

### P1-1：指纹数据目录与 schema

任务：

- 新建 `backend/internal/fingerprint/` 模块。
- 设计并实现 profile schema：platform、brand、screen、hardware、webgl、fonts、mediaDevices、locale、timezone、privacy。
- 数据文件放入 `backend/internal/fingerprint/data/`。
- 提供 Go 结构体、加载器和校验器。

验收：

- 数据文件加载失败有明确错误。
- schema 有单元测试覆盖。
- 生成结果可输出为当前浏览器启动参数。

### P1-2：字体库与字体子集生成

任务：

- 基于 Camoufox 字体数据建立 Windows / macOS / Linux 字体候选池。
- 固定 OS marker fonts。
- 非核心字体按比例抽样，避免所有 profile 字体完全一致。
- 预留字体 metrics 和真实字体文件安装策略。

验收：

- Windows profile 包含 Segoe UI、Calibri、Cambria Math 等核心字体。
- macOS profile 包含 Helvetica Neue、PingFang、Menlo/Monaco 等核心字体。
- Linux profile 包含 Noto / Liberation / DejaVu 等合理字体。
- 校验器能识别明显跨 OS 字体冲突。

### P1-3：WebGL allowlist 与权重生成

任务：

- 从 Camoufox WebGL DB 抽取 vendor/renderer 候选。
- 建立 Chromium allowlist，剔除 Firefox 特有或不可信 renderer。
- 按 OS 和设备档位加权选择 GPU。
- 输出 `--fingerprint-webgl-vendor` 与 `--fingerprint-webgl-renderer`。

验收：

- macOS 不生成明显 Windows-only GPU。
- Linux 不生成 Apple Silicon renderer。
- Windows 可按办公、游戏、高性能档位生成 Intel/NVIDIA/AMD。
- 不存在 vendor/renderer 互相冲突。

### P1-4：Screen / Hardware / MediaDevices 分布

任务：

- 建立屏幕分辨率、DPR、colorDepth、hardwareConcurrency、deviceMemory、touchPoints 分布。
- 建立 media devices 数量模板：摄像头、麦克风、扬声器。
- 与设备类型关联：desktop、laptop、workstation、gaming、light-linux。

验收：

- desktop 默认 `touchPoints=0`。
- 高配 GPU 不搭配过低 CPU/内存组合。
- media devices 数量不离谱。
- 生成结果通过一致性校验。

### P1-5：指纹健康检查

任务：

- 实现 backend validator。
- 前端显示健康状态：绿色、黄色、红色。
- 检查平台、字体、WebGL、screen、locale/timezone、WebRTC、seed。
- 对用户手动覆盖导致的不一致给出明确提示。

验收：

- Firefox UA + Chrome brand 被判红色。
- Windows platform + macOS 字体核心集被判红色。
- locale/timezone 与代理地区不一致被判黄色或红色。
- 用户仍可保存高级配置，但风险可见。

## P2：代理地理一致性与存储身份

目标：让 IP、语言、时区、地理位置、WebRTC、存储状态形成同一身份。

### P2-1：GeoIP DB 缓存

任务：

- 引入 GeoIP 数据库缓存机制。
- 支持后台更新、超时降级和失败缓存。
- 根据代理出口 IP 推导国家、城市、timezone、locale 候选。

验收：

- 美国代理默认生成 `en-US` 与美国 timezone。
- 中国代理默认生成 `zh-CN` 与 `Asia/Shanghai`。
- GeoIP 失败不阻塞启动。
- 手动覆盖优先级高于自动推导。

### P2-2：Accept-Language / navigator.languages 一致性

任务：

- 确保 `--lang`、`--fingerprint-locale`、Accept-Language、`navigator.language`、`navigator.languages` 一致。
- 支持地区候选，如 `en-US,en;q=0.9`。
- 避免只改 `navigator.language` 而请求头仍暴露真实语言。

验收：

- 检测页中 JS 语言和请求语言一致。
- 用户切换地区后语言字段成组变化。
- 不通过 CDP context emulation 做可检测的局部覆盖。

### P2-3：Geolocation 原生一致性

任务：

- 根据代理城市或用户设置生成 geolocation。
- 权限状态、成功回调、失败回调要符合真实浏览器行为。
- 支持手动关闭地理位置伪装。

验收：

- `navigator.geolocation.getCurrentPosition` 返回位置与代理地区一致。
- 权限状态和 UI 行为不互相矛盾。
- 未授权前不提前泄露地理位置。

### P2-4：Persistent Profile 默认策略

任务：

- 明确普通手动浏览优先使用 persistent profile。
- 避免长期账号每次都是全新无历史环境。
- 保存 cookies、localStorage、IndexedDB、CacheStorage、Service Worker。
- 新增 profile 暖机建议或自动暖机流程。

验收：

- 同一 profile 重启后 cookies/localStorage 保留。
- BrowserScan 等检测中 incognito/empty-profile 风险下降。
- 仍支持临时 profile，但 UI 标记风险。

### P2-5：Storage Quota 与 Incognito 检测

任务：

- 检查 `navigator.storage.estimate()` 与 StorageBuckets API。
- 研究 `fingerprint-chromium` 是否支持 `--fingerprint-storage-quota` 或同类参数。
- 若不支持，列入 Chromium patch。
- persistent 与非 persistent 模式分别建立 quota 策略。

验收：

- Storage quota 不暴露真实磁盘容量。
- 不被轻易判定为 private/incognito mode。
- quota 与 profile 类型一致。

## P3：高级浏览器 API 指纹覆盖

目标：补齐现代检测站常用但当前容易漏掉的浏览器 API 指纹。

### P3-1：User-Agent Client Hints

任务：

- 覆盖 `navigator.userAgentData`。
- 生成 `brands`、`fullVersionList`、`platform`、`platformVersion`、`architecture`、`bitness`、`mobile`。
- 与 UA、Chromium 版本、OS、brand 一致。

验收：

- UA 与 Client Hints 不冲突。
- Chrome major version 一致。
- Windows/macOS/Linux platformVersion 不混用。

### P3-2：WebGPU 指纹

任务：

- 检查 `navigator.gpu`、adapter、features、limits。
- 为 headless/Docker 建立合理策略。
- 与 WebGL GPU 身份一致。
- 不支持时以“真实不可用”方式呈现，而不是半残缺 API。

验收：

- WebGPU adapter 与 WebGL renderer 不冲突。
- headless 环境不暴露明显 Docker/SwiftShader 异常。
- 检测页中 WebGPU 状态稳定。

### P3-3：Media Codec 能力

任务：

- 覆盖 `MediaSource.isTypeSupported`。
- 覆盖 `HTMLMediaElement.canPlayType`。
- 建立 H.264、AAC、AV1、VP9、HEVC 支持矩阵。
- 与 OS、Chrome 版本、Widevine 状态一致。

验收：

- Windows/macOS/Linux codec 能力不互相串台。
- AAC/H.264 能力与 Chrome 桌面一致。
- 没有“支持 Widevine 但 codec 不支持”的矛盾。

### P3-4：Plugins / MimeTypes / PDF / Widevine

任务：

- 检查并伪装 `navigator.plugins`。
- 检查并伪装 `navigator.mimeTypes`。
- 覆盖 PDF Viewer 与 `navigator.pdfViewerEnabled`。
- 设计 Widevine CDM 检测和启用策略。

验收：

- `navigator.plugins.length` 不为异常空值。
- PDF Viewer 状态与 Chrome 桌面一致。
- Widevine 状态与 EME / codec 能力一致。

### P3-5：Network Information API

任务：

- 覆盖 `navigator.connection`。
- 生成 `rtt`、`downlink`、`effectiveType`、`saveData`、`downlinkMax`。
- 与代理类型和设备类型联动。

验收：

- 数据中心代理不伪装成移动网络。
- 移动代理可以选择 `4g` 模板。
- `connectionRTT` 不成为明显失败项。

### P3-6：Permissions API 矩阵

任务：

- 建立 permission 默认状态矩阵：notifications、geolocation、camera、microphone、clipboard、midi、persistent-storage。
- 权限查询、授权前后状态、设备 label 暴露时机保持一致。

验收：

- `navigator.permissions.query()` 返回值符合 Chrome 桌面行为。
- camera/microphone 未授权前不泄露 label。
- 授权后 enumerateDevices 行为一致。

### P3-7：Speech Voices 与 Media Devices 深度一致性

任务：

- 基于 Camoufox voices 数据建立 OS 语音池。
- Windows/macOS/Linux 返回合理 voice list。
- enumerateDevices 的 deviceId/groupId 稳定且与权限状态一致。

验收：

- Windows 有 Microsoft David/Zira/Mark 等合理语音。
- macOS 有 Alex/Samantha/Victoria 等合理语音。
- Linux 可为空或少量语音，不伪装成 macOS。

## P4：行为、人机与自动化信号

目标：降低行为检测、CDP 检测、自动化框架痕迹。

### P4-1：Humanize 自动化行为层

任务：

- 增加 humanized click/type/scroll API。
- 鼠标使用贝塞尔曲线、wobble、overshoot。
- 键盘使用逐字 delay、key hold、shift 组合、少量误触修正。
- 滚动使用加速、巡航、减速、停顿、回调修正。

验收：

- 自动化脚本可按 profile 启用 humanize。
- 支持 default/careful preset。
- 不影响普通手动浏览。

### P4-2：Actionability 与 iframe 支持

任务：

- 点击前等待元素 visible、enabled、stable。
- 检查 pointer-events。
- humanize 方法支持 iframe 和 frame locator。
- 避免坐标偏移导致异常点击轨迹。

验收：

- iframe 内点击轨迹正确。
- 隐藏元素不会被直接点击。
- 复杂页面自动化成功率不下降。

### P4-3：CDP 输入与协议使用纪律

任务：

- 减少 reCAPTCHA 前的 `page.evaluate()`、`wait_for_timeout()` 等高频 CDP 调用。
- 输入事件尽量走可信路径。
- 记录自动化脚本的 CDP 调用密度。
- 研究 CDP input stealth 是否需内核 patch。

验收：

- 自动化任务可开启“低 CDP 噪声模式”。
- 输入事件的 `isTrusted`、stack、timing 更接近真实用户。
- 文档明确哪些 API 不建议在高风控页面使用。

### P4-4：Headless 专用策略

任务：

- 建立 headless 独立 profile 模板。
- 修正 headless UA、window chrome、GPU path、taskbar、WebGPU。
- 对高风险站点默认推荐 headed。

验收：

- headless 不出现 `HeadlessChrome`。
- screen/window/viewport 自洽。
- CreepJS headless 信号下降。

## P5：Chromium 内核 patch 路线

目标：把参数层无法稳定覆盖的高风险指标移入源码层。

### P5-1：核心 navigator 与 automation patch

任务：

- `navigator.webdriver`
- `window.chrome`
- automation controlled
- plugins/mimeTypes descriptor
- Error stack / CDP detection

验收：

- 不依赖 JS init script 覆盖关键属性。
- descriptor、toString、realm 检测不暴露注入痕迹。

### P5-2：Canvas / Audio / WebGL / WebGPU 深度 patch

任务：

- Canvas 与 OffscreenCanvas。
- AudioContext 与 OfflineAudioContext。
- WebGL extensions、shader precision、format consistency。
- WebGPU adapter、limits、features。

验收：

- 同 seed 下稳定。
- 主 window、iframe、worker 中一致。
- WebGL/WebGPU/GPU 身份一致。

### P5-3：Worker / iframe / realm 全域一致性

任务：

- 覆盖 iframe。
- 覆盖 cross-origin iframe 可达部分。
- 覆盖 Worker、Service Worker、AudioWorklet、OffscreenCanvas。
- 避免只补主 window。

验收：

- 多 realm 读取同一指标结果一致。
- Worker Canvas/Audio 与主线程一致。
- 检测站不能通过 realm 差异识别。

### P5-4：网络栈与代理信号

任务：

- HTTP/2、HTTP/3、QUIC 策略。
- SOCKS5 UDP ASSOCIATE。
- DNS/connect/SSL timing。
- proxy headers 清理。
- TLS JA3/JA4/Akamai 指纹。

验收：

- 代理启用时不泄露 `Proxy-Connection` 等异常头。
- TLS/HTTP 指纹与真实 Chrome 匹配。
- QUIC/HTTP3 策略与代理能力一致。

### P5-5：WebAuthn / Passkey 能力

任务：

- `PublicKeyCredential`。
- platform authenticator。
- user verification。
- Windows Hello / Touch ID / Linux 差异。

验收：

- WebAuthn capability 与 OS 身份一致。
- 不出现 macOS 身份却返回 Windows Hello 能力。
- 不支持时行为符合真实 Chrome。

## P6：检测与回归工程化

目标：把优化效果从人工感觉变成可记录、可复现、可比较的数据。

### P6-1：指纹采集模板落地

任务：

- 使用 `FINGERPRINT_AUDIT_TEMPLATE.zh-CN.md` 记录每次检测。
- 为每次检测保存截图、JS probe、启动参数、代理信息。
- 建立 `data/fingerprint-audit/` 输出结构。

验收：

- 每次测试都能复现启动环境。
- 异常字段可被单独追踪。
- 新旧版本可横向比较。

### P6-2：内部 JS Probe

任务：

- 编写本地 JS probe，采集 navigator、screen、WebGL、WebGPU、Audio、Canvas、Storage、Permissions、Media、Network、Plugins。
- 输出 JSON。
- 不把第三方站点分数作为唯一依据。

验收：

- 本地 probe 可在任意页面执行。
- 结果结构稳定。
- 可用于比较两个 profile 的差异。

### P6-3：检测站矩阵

任务：

- 覆盖 Fingerprint.com、BrowserScan、CreepJS、bot.incolumitas、deviceandbrowserinfo。
- 分 headed/headless、直连/代理、Windows/macOS/Linux profile 记录。
- 每次优化后至少跑 P0/P1 矩阵。

验收：

- 每个版本有检测记录。
- 失败项能映射到具体任务。
- 检测结果不会只停留在截图。

## 推荐执行顺序

第一阶段：

- P0-1 启动参数去重与覆盖顺序。
- P0-2 稳定 seed。
- P0-3 WebRTC 出口 IP 一致性。
- P0-4 默认指纹参数扩展。
- P6-1 指纹采集模板落地。

第二阶段：

- P1-1 指纹库 schema。
- P1-2 字体库。
- P1-3 WebGL allowlist。
- P1-4 Screen / Hardware / MediaDevices。
- P1-5 指纹健康检查。

第三阶段：

- P2-1 GeoIP DB。
- P2-2 Accept-Language / navigator.languages。
- P2-4 Persistent Profile 默认策略。
- P2-5 Storage Quota。
- P3-1 Client Hints。

第四阶段：

- P3-2 WebGPU。
- P3-3 Media Codec。
- P3-4 Plugins / MimeTypes / PDF / Widevine。
- P3-5 Network Information API。
- P3-6 Permissions API。

第五阶段：

- P4 Humanize 与 CDP 纪律。
- P5 Chromium 内核 patch。
- P6 自动化检测矩阵。

## 任务拆分建议

- 每个 P0/P1 子任务可以单独开 issue。
- P3/P5 不建议直接大改，应先做 probe 和能力调研。
- 每个任务必须带一个“检测字段清单”和“验收截图/JSON”。
- 修改默认指纹参数时必须同步更新 publish 初始化配置和前端序列化逻辑。
- 涉及 Camoufox 数据 vendoring 前必须完成许可证确认。

## 当前可立即推进的最小闭环

1. 固定 profile seed。
2. 建立 `backend/internal/fingerprint` schema。
3. 接入字体/WebGL/screen/hardware 最小数据集。
4. 前端增加“随机生成自洽指纹”按钮。
5. 输出健康检查结果。
6. 用 `FINGERPRINT_AUDIT_TEMPLATE.zh-CN.md` 记录 Fingerprint.com、BrowserScan、CreepJS 三站结果。

## 当前落地状态（P0/P1/P2 参数层）

- P0-1 / P0-2 / P0-3 / P0-4 / P0-5：已在启动参数合并、系统接管参数剔除、profile 稳定 seed、WebRTC 出口 IP 解析、直连/失败移除 `auto`、screen/avail/DPR 校验中落地。
- P1-1 / P1-2 / P1-3 / P1-4 / P1-5：已建立 `backend/internal/fingerprint` 数据目录，`profiles/locales/fonts/webgl/screen/media_devices` 均参与生成；前端支持生成、恢复推荐值、健康检查和分维状态展示。
- P2-1 / P2-2 / P2-4：已落地参数层闭环。`geoip` 配置支持本地 `.mmdb` 或 MaxMind 官方凭证下载 GeoLite2-City 到本地缓存；代理 IP 健康结果会缓存出口 IP/国家/地区/城市/timezone/locale，并反哺 `--lang`、`--fingerprint-locale`、`--timezone`、`--fingerprint-timezone`、`--fingerprint-webrtc-ip`；普通 profile 默认使用稳定 user data dir。
- P2-3 / P2-5：当前仓库和已接入的 `fingerprint-chromium` 参数面未发现 geolocation、Storage quota 的原生启动参数。不要用无效参数假装覆盖；这两项进入 P5 Chromium patch / 能力探测后再落地。
- Bot:nodriver 与 Browser Tampering:Yes：按当前任务要求暂不处理，保留到 P4/P5 自动化和内核 patch 阶段。
