# Ant-Browser 指纹优化策划方案

本文基于对 [daijro/camoufox](https://github.com/daijro/camoufox) 与 [CloakHQ/CloakBrowser](https://github.com/CloakHQ/CloakBrowser) 两个开源指纹浏览器项目的调研，整理可迁移到 Ant-Browser 的指标、实现路径、优先级与验证闭环。

当前 Ant-Browser 的浏览器内核路线是 Chromium / `fingerprint-chromium`，因此不能直接照搬 Camoufox 的 Firefox 指纹预设；正确方向是吸收其真实分布数据、指标覆盖面和生成策略，再转换成 Ant 原生的 Chromium 指纹模板。

## 1. 背景与目标

当前在 `https://demo.fingerprint.com/playground` 等检测站点上，仍可见多项异常指标。问题通常不是某一个字段“不像真浏览器”，而是多维信号之间不一致：

- 浏览器引擎、UA、平台、字体、WebGL 组合不一致。
- 代理出口 IP、时区、语言、WebRTC 候选 IP 不一致。
- 窗口尺寸、屏幕尺寸、`innerWidth/outerWidth`、viewport emulation 出现物理矛盾。
- Canvas、Audio、WebGL、字体、media devices、speech voices 等高熵指标覆盖不足。
- 自动化行为、CDP 输入、`navigator.webdriver`、插件、headless 痕迹仍可能被识别。

本方案目标：

- 建立 Ant 原生的指纹样本库和一致性生成器。
- 将 Camoufox 的真实分布数据转译为 Chromium 可用模板。
- 借鉴 CloakBrowser 的 Chromium 源码层 patch 思路和启动策略。
- 建立检测站点回归矩阵，形成持续优化闭环。

## 2. 上游项目价值判断

### 2.1 Camoufox

Camoufox 是 Firefox fork，优势在于数据和指标覆盖面：

- 内置真实指纹预设：`fingerprint-presets.json`、`fingerprint-presets-v150.json`。
- 按 OS 维护字体库：`fonts.json`。
- 按 OS 维护语音库：`voices.json`。
- 内置 WebGL 数据库：`webgl/webgl_data.db`，包含 OS 相关的 vendor/renderer 概率。
- 源码 patch 覆盖 navigator、screen、WebGL、audio、fonts、system UI font、speech voices、media devices、timezone、locale、geolocation、WebRTC 等方向。
- 生成策略强调“同一身份下的成套字段”，而非每项独立随机。

限制：

- 预设是 Firefox/Camoufox 身份，例如 `Firefox/151.0`、`rv:151.0`、`oscpu` 等。
- 不能把 Firefox UA、Firefox 平台行为直接塞进 Chromium。
- Camoufox 的 patch API 和配置字段不等于 `fingerprint-chromium` 的 CLI 参数。
- 数据文件授权需要单独核对。Camoufox 根项目为 MPL 2.0，Python 包声明为 MIT，数据文件 vendoring 前需要保留来源和许可证审查记录。

结论：Camoufox 适合作为“真实分布数据源 + 指标覆盖清单 + 生成策略参考”，不适合作为 Ant 的运行时原始指纹库直接加载。

### 2.2 CloakBrowser

CloakBrowser 是 Chromium 路线，参考价值更贴近 Ant-Browser：

- 强调 C++ 源码层修改，而不是单纯 JS 注入或配置参数。
- 默认移除 Playwright/Chromium 自动化泄露参数，如 `--enable-automation`、`--enable-unsafe-swiftshader`。
- 默认生成随机 `--fingerprint` seed。
- 根据宿主系统选择平台身份，尤其在 macOS 上避免伪装 Windows 造成字体/GPU/系统信号冲突。
- headed 模式避免 viewport emulation，防止出现 `outerWidth < innerWidth` 等物理不可能窗口。
- headless 模式使用固定、可解释的 viewport，模拟 1080p Windows 最大化窗口。
- `geoip=True` 自动根据代理出口 IP 推导 timezone、locale，并复用出口 IP 设置 WebRTC spoof。
- persistent profile 用于保留 cookies、localStorage、cache，减少 incognito/全新环境异常。
- humanize 行为层覆盖鼠标曲线、点击保持时间、键盘逐字输入、误触修正、滚动加减速和停顿。

限制：

- 部分能力在其自定义 Chromium 二进制中，Ant 不能仅靠外层参数完全复刻。
- README 中的测试结果会随时间变化，需要建立自己的回归测试，而不是直接引用为 Ant 的效果承诺。
- Pro/latest binary 模型不适合直接作为 Ant 的依赖。

结论：CloakBrowser 适合作为 Chromium 内核 patch 优先级、启动参数策略、GeoIP 一致性和行为人类化模块的参考。

## 3. Ant-Browser 当前可承接参数面

当前项目已具备或正在扩展的指纹参数包括：

- 基础身份：`--fingerprint`、`--fingerprint-brand`、`--fingerprint-platform`
- 语言与时区：`--lang`、`--timezone`、`--fingerprint-locale`、`--fingerprint-timezone`
- 屏幕与窗口：`--window-size`、`--fingerprint-color-depth`
- 硬件：`--fingerprint-hardware-concurrency`、`--fingerprint-device-memory`
- 渲染：`--fingerprint-canvas-noise`、`--fingerprint-audio-noise`
- WebGL：`--fingerprint-webgl-vendor`、`--fingerprint-webgl-renderer`
- 字体：`--fingerprint-fonts`
- 网络与隐私：`--webrtc-ip-handling-policy`、`--fingerprint-webrtc-ip`、`--fingerprint-do-not-track`
- 媒体设备：`--fingerprint-media-devices`
- 触摸：`--fingerprint-touch-points`

这说明短期可以先做“高质量参数生成器”，中长期再推动 `fingerprint-chromium` 或自研 Chromium patch 覆盖更深层指标。

## 4. 值得应用的指标清单

### 4.1 基础身份一致性

需要统一生成：

- browser brand：Chrome / Edge，暂不建议混入 Firefox。
- Chromium major version 与 UA、Client Hints、内核版本保持一致。
- platform：Windows / macOS / Linux。
- navigator platform：Win32 / MacIntel / Linux x86_64 等。
- architecture、bitness、mobile、model 等 Client Hints 相关字段。

短期落地：

- 保持 `--fingerprint-brand=Chrome` 为主。
- 按 profile 输出 `--fingerprint-platform=windows|mac|linux`。
- 不使用 Camoufox 的 Firefox UA。

中期落地：

- 增加 Chromium UA / User-Agent Client Hints 一致性生成。
- 检查 `fingerprint-chromium` 是否已有相关参数；没有则进入内核 patch 需求。

现状边界：`backend/internal/fingerprint/data/client_hints.json` 仍只是 Ant 自行整理的 data-only skeleton，不会生成或应用 runtime UA-CH 参数；真正生效需要单独实现并验证 Chromium core 支持。

### 4.2 OS 字体库

Camoufox 字体库值得重点吸收：

- Windows：Segoe UI、Calibri、Cambria Math、Nirmala UI、Consolas 等。
- macOS：Helvetica Neue、PingFang 系列、Menlo、Monaco、Lucida Grande 等。
- Linux：Arimo、Cousine、Tinos、Noto Sans 系列等。

落地策略：

- 建立 `backend/internal/fingerprint/data/fonts.json`。
- 每个 profile 按 OS 生成字体子集。
- 固定包含 OS marker fonts，避免 CreepJS 一类检测判断 OS 与字体不匹配。
- 非必要字体按比例抽样，避免所有实例字体列表完全一致。

Ant 参数映射：

- `--fingerprint-fonts=<comma-separated-fonts>`

### 4.3 WebGL vendor/renderer

Camoufox 的 WebGL DB 可作为候选来源，但必须转换为 Chromium 合法组合：

- Windows 常见：Intel、NVIDIA、AMD。
- macOS 常见：Apple、Intel、AMD，Apple Silicon 需与 macOS 身份一致。
- Linux 常见：Mesa、Intel、AMD，避免与 Windows/macOS 字体和 UA 冲突。

落地策略：

- 抽取 Camoufox DB 中 OS 权重和 vendor/renderer pair。
- 建立 Chromium allowlist，剔除 Firefox 特有或可疑 renderer。
- 与 `--fingerprint-platform`、宿主 OS、GPU 策略联动。

Ant 参数映射：

- `--fingerprint-webgl-vendor=<vendor>`
- `--fingerprint-webgl-renderer=<renderer>`

中长期补充：

- 仅 vendor/renderer 不够，检测站还会看 WebGL extensions、shader precision、unmasked renderer、canvas rendering path。需要内核层支持更完整的 WebGL profile。

### 4.4 屏幕、窗口与 viewport

需要覆盖：

- `screen.width`
- `screen.height`
- `screen.availWidth`
- `screen.availHeight`
- `colorDepth`
- `pixelDepth`
- `devicePixelRatio`
- `innerWidth/innerHeight`
- `outerWidth/outerHeight`
- 浏览器窗口 chrome 高度和最大化状态

短期落地：

- 从 Camoufox preset 抽取常见分辨率分布。
- 输出 `--window-size` 和 `--fingerprint-color-depth`。
- headed 模式避免额外 viewport emulation。
- 增加一致性校验：禁止 `outerWidth < innerWidth`、`availHeight > height` 等物理矛盾。

中期落地：

- 为不同 OS 建立窗口 chrome 差值模型。
- headless 模式使用固定 coherent viewport，例如 CloakBrowser 的 1920x947 思路。

### 4.5 硬件能力

需要覆盖：

- `hardwareConcurrency`
- `deviceMemory`
- GPU 与 CPU 档位一致性
- platform 与触摸能力一致性

短期落地：

- 按设备档位生成：office、laptop、workstation、gaming。
- Windows desktop 默认 touch points 为 0。
- macOS desktop 默认 touch points 为 0。
- 移动端暂不作为主线，除非内核支持完整 mobile emulation。

Ant 参数映射：

- `--fingerprint-hardware-concurrency=4|6|8|12|16`
- `--fingerprint-device-memory=4|8|16`
- `--fingerprint-touch-points=0|1|5`

### 4.6 语言、时区、地理位置与代理出口

这是 Fingerprint.com 很容易识别的交叉信号。

需要统一：

- 代理出口 IP 国家/城市。
- `Intl.DateTimeFormat().resolvedOptions().timeZone`
- `navigator.language`
- `navigator.languages`
- `Accept-Language`
- geolocation 权限返回值。
- WebRTC ICE candidate IP。

短期落地：

- 将现有 `--fingerprint-webrtc-ip=auto` 扩展为代理出口一致性模块。
- 代理存在时解析出口 IP。
- 根据 GeoIP 推导 timezone 和 locale。
- 同时写入 `--lang`、`--fingerprint-locale`、`--timezone`、`--fingerprint-timezone`。

中期落地：

- 引入 GeoLite2-City 或等价数据库缓存。
- 增加国家到 locale 的映射表。
- 如果用户手动指定 locale/timezone，则以用户指定为准，并提示一致性风险。

Ant 参数映射：

- `--lang=<locale>`
- `--fingerprint-locale=<locale>`
- `--timezone=<iana-timezone>`
- `--fingerprint-timezone=<iana-timezone>`
- `--fingerprint-webrtc-ip=auto|<exit-ip>`

### 4.7 WebRTC

需要覆盖：

- 公网候选 IP 不泄露真实本机 IP。
- mDNS/local candidate 行为与真实 Chrome 一致。
- 代理出口 IP 与 WebRTC 候选一致。
- `webrtc-ip-handling-policy` 不产生异常组合。

短期落地：

- 默认 `--webrtc-ip-handling-policy=disable_non_proxied_udp`。
- 有代理时默认 `--fingerprint-webrtc-ip=auto`。
- 没有代理时不要伪造一个随机公网 IP。

中长期落地：

- 在内核层处理 ICE candidate 生成，而不是只做 JS 层覆盖。

### 4.8 Canvas 与 Audio

Camoufox 和 CloakBrowser 都强调源码层处理。

短期落地：

- 保持 `--fingerprint-canvas-noise=true`。
- 保持 `--fingerprint-audio-noise=true`。
- seed 与 profile 绑定，同一 profile 多次启动保持稳定，不要每次完全漂移。

中期落地：

- 将 `--fingerprint` seed 固定到浏览器 profile，而不是每次启动都随机。
- 支持“重新生成指纹”时才更新 seed。

长期落地：

- 内核层细化 Canvas、AudioContext、OfflineAudioContext 输出，确保同 seed 下稳定、跨 API 一致。

### 4.9 Speech voices

Camoufox 的 `voices.json` 可补足语音列表：

- Windows 有 Microsoft David/Zira/Mark 等。
- macOS 有 Alex、Samantha、Victoria、Karen、Daniel 等。
- Linux 通常为空或非常少。

短期落地：

- 暂不暴露 UI，作为内核能力需求记录。

中长期落地：

- 如果 `fingerprint-chromium` 支持 speechSynthesis voices spoof，则按 OS 注入语音子集。
- 若不支持，列为 Chromium patch 项。

### 4.10 Media devices

需要覆盖：

- 摄像头、麦克风、扬声器数量。
- device label 是否在授权前为空、授权后可见。
- deviceId/groupId 稳定性。

短期落地：

- 提供常见组合：`0,1,1`、`1,1,1`、`1,2,2`、`2,1,2`。
- 与设备类型匹配：普通 desktop 不要出现过多摄像头。

Ant 参数映射：

- `--fingerprint-media-devices=cameras,microphones,speakers`

中长期落地：

- 内核层确保 enumerateDevices 行为、权限状态、label 暴露时机一致。

### 4.11 插件、MIME、PDF、Widevine

CloakBrowser 把插件列表、Widevine、persistent profile 作为重要检测面。

短期落地：

- 检查当前内核下 `navigator.plugins.length`、PDF Viewer、Chrome PDF Plugin、Widevine 是否异常。
- 在检测报告中加入插件/MIME 指标。

中期落地：

- 支持 Widevine/CDM 状态一致性。
- 支持 persistent profile 优先，避免每次都是干净新环境。

长期落地：

- 内核层补齐插件和 MIME 类型行为，避免 JS 注入痕迹。

### 4.12 Automation/CDP/headless 信号

需要覆盖：

- `navigator.webdriver`
- `window.chrome`
- permissions API
- plugins length
- CDP stack / Error stack 检测
- Playwright 默认参数
- headless UA 和 GPU path
- SwiftShader renderer

短期落地：

- 启动参数合并时移除或覆盖可疑参数。
- 避免 `--enable-automation`。
- 避免 `--enable-unsafe-swiftshader`。
- headed 优先，headless 单独做 profile。

中长期落地：

- Chromium 源码层 patch automation signals。
- CDP 输入行为和 Error stack 检测需要内核或协议层处理。

### 4.13 行为人类化

CloakBrowser humanize 模块可直接借鉴思路：

- 鼠标贝塞尔曲线。
- 移动过程 wobble。
- 点击前 aim delay。
- mouse down/up hold time。
- 输入逐字 delay。
- Shift 组合键真实事件。
- 少量误触和 Backspace 修正。
- 滚动加速、巡航、减速。
- overshoot 和 correction。
- actionability：可见、启用、稳定后再点击。

落地策略：

- 不放进普通浏览器 profile 的基础指纹配置。
- 作为自动化 API 的增强层，例如“人类化点击/输入/滚动”。
- 对普通手动使用用户不强制启用。

## 5. Ant 原生指纹库设计

### 5.1 数据目录建议

```text
backend/internal/fingerprint/
  library.go
  generator.go
  validator.go
  geoip.go
  data/
    profiles.json
    fonts.json
    webgl.json
    locales.json
    screen.json
    media_devices.json
```

### 5.2 Profile Schema 草案

```json
{
  "id": "win_chrome_office_intel_1920",
  "weight": 120,
  "platform": "windows",
  "brand": "Chrome",
  "uaFamily": "chrome",
  "locale": "en-US",
  "timezone": "America/New_York",
  "screen": {
    "width": 1920,
    "height": 1080,
    "availWidth": 1920,
    "availHeight": 1040,
    "colorDepth": 24,
    "devicePixelRatio": 1
  },
  "hardware": {
    "hardwareConcurrency": 8,
    "deviceMemory": 8,
    "touchPoints": 0
  },
  "webgl": {
    "vendor": "Intel",
    "renderer": "Intel(R) UHD Graphics 630"
  },
  "fonts": {
    "strategy": "os_subset",
    "minPercent": 30,
    "maxPercent": 78
  },
  "mediaDevices": {
    "camera": 1,
    "microphone": 1,
    "speaker": 1
  },
  "privacy": {
    "doNotTrack": false,
    "webrtcIP": "auto"
  }
}
```

### 5.3 CLI 参数输出

生成器最终输出 Ant 当前能消费的参数：

```text
--fingerprint=<stable-seed>
--fingerprint-brand=Chrome
--fingerprint-platform=windows
--lang=en-US
--fingerprint-locale=en-US
--timezone=America/New_York
--fingerprint-timezone=America/New_York
--window-size=1920,1080
--fingerprint-color-depth=24
--fingerprint-hardware-concurrency=8
--fingerprint-device-memory=8
--fingerprint-webgl-vendor=Intel
--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630
--fingerprint-fonts=Arial,Segoe UI,Calibri,...
--fingerprint-media-devices=1,1,1
--fingerprint-webrtc-ip=auto
--webrtc-ip-handling-policy=disable_non_proxied_udp
--fingerprint-touch-points=0
--fingerprint-canvas-noise=true
--fingerprint-audio-noise=true
```

## 6. 一致性校验器

新增 `validator.go`，生成 profile 前后都检查：

- Firefox UA 禁止进入 Chromium profile。
- Windows profile 禁止 macOS marker fonts 作为核心字体。
- macOS profile 禁止 Windows-only GPU/字体组合。
- Linux profile 避免 Microsoft-only 字体大量出现。
- touch points 与 desktop/mobile 类型一致。
- WebGL vendor/renderer 必须在当前 OS allowlist。
- locale 与 timezone 不一致时标记风险。
- proxy 国家与 timezone/locale 不一致时标记风险。
- WebRTC IP 不能泄露本机公网 IP。
- `outerWidth`、`innerWidth`、screen、viewport 不能出现物理矛盾。
- headless/headed 使用不同窗口策略。

UI 可展示“指纹健康度”：

- 绿色：主要维度一致。
- 黄色：用户手动指定导致部分风险。
- 红色：平台、字体、WebGL、时区/IP 明显冲突。

## 7. 分阶段实施计划

### P0：快速止血

目标：先减少明显异常组合。

- 固定 profile seed，同一浏览器 profile 不在每次启动时随机漂移。
- 有代理时默认启用 `--fingerprint-webrtc-ip=auto`。
- 默认启用代理出口 IP 与 WebRTC 一致，直连时不伪造随机公网 IP。
- headed 模式避免 viewport emulation。
- 默认参数中移除/覆盖自动化泄露参数。
- 增加 Fingerprint.com、BrowserScan、CreepJS 的手动检测清单。

验收：

- 默认 profile 不出现 Firefox/Chrome 混合身份。
- WebRTC 不泄露真实 IP。
- timezone/locale 与代理出口一致或有明确提示。
- window/screen/viewport 无物理矛盾。

### P1：Ant 原生指纹库

目标：从“固定默认参数”升级为“成套 profile 生成”。

- 引入 OS 字体库。
- 引入 Chromium WebGL allowlist。
- 引入 screen/hardware/media devices 分布。
- 实现 weighted random profile generator。
- 前端新增“随机生成指纹”“按平台生成”“按代理地区生成”。
- 保留高级参数编辑能力。

验收：

- 同平台 profile 多样化。
- 生成结果全部通过一致性校验。
- 旧 profile 可无损兼容。

### P2：GeoIP 一致性模块

目标：让代理出口驱动语言、时区、WebRTC。

- 接入 GeoIP DB 缓存。
- 代理出口 IP 解析增加超时、缓存和失败降级。
- 国家/地区到 locale/timezone 候选映射。
- 用户手动覆盖时提示风险。

验收：

- 使用美国代理时，默认生成 `en-US` 和美国 timezone。
- 使用中国代理时，默认生成 `zh-CN` 和 `Asia/Shanghai`。
- 失败时不阻塞启动，但在 UI/日志提示。

当前落地状态：

- 已落地参数层闭环：代理 IP 健康检测结果会缓存出口 IP、国家、地区、城市，并回写到代理配置；生成指纹时 `regionMode=proxy` 会优先使用该缓存推导 `--lang`、`--fingerprint-locale`、`--timezone`、`--fingerprint-timezone` 和 `--fingerprint-webrtc-ip`。
- 已落地失败降级：WebRTC 出口 IP 解析失败或直连时会移除 `--fingerprint-webrtc-ip=auto`，避免把 `auto` 直接交给内核。
- 已接入 GeoIP DB 能力：新增 `geoip` 配置，支持本地 `.mmdb` 路径，或使用 MaxMind 官方 `account_id/license_key` 下载 GeoLite2-City 到本地缓存；代理健康检测会用出口 IP 补齐国家、地区、城市、timezone，并反哺代理配置与指纹生成。Geolocation 原生 API、Storage quota 仍归入 Chromium patch / 能力探测阶段。

### P3：行为人类化

目标：降低自动化行为检测异常。

- 自动化 API 增加 humanized click/type/scroll。
- 鼠标轨迹使用贝塞尔曲线、wobble、overshoot。
- 输入增加逐字 delay、key hold、偶发误触修正。
- 滚动增加加速、减速、停顿和修正。
- 点击前等待元素 visible/enabled/stable。

验收：

- 自动化脚本可选择 humanize preset。
- 行为检测站点的明显自动化指标下降。
- 不影响用户手动浏览。

### P4：Chromium 内核 patch 需求

目标：覆盖参数层无法解决的深层检测。

候选 patch：

- navigator.webdriver / automation controlled。
- User-Agent Client Hints。
- plugins / mimeTypes / PDF Viewer / Widevine。
- WebGL extensions / shader precision / GPU consistency。
- Canvas / AudioContext / OfflineAudioContext。
- speechSynthesis voices。
- enumerateDevices 权限态和 label 暴露。
- timezone / locale / geolocation 原生 API。
- CDP stack 和 input 行为。
- headless 模式 UA、window chrome、GPU path。

验收：

- 不依赖 JS init script 覆盖关键属性。
- 检测站无法通过 descriptor、stack、toString、realm 差异识别注入。

## 8. 前端产品设计

### 8.1 指纹面板新增能力

- 随机生成：一键生成 coherent profile。
- 平台选择：Windows / macOS / Linux。
- 地区模式：跟随代理 / 手动选择国家地区 / 跟随系统。
- 设备类型：办公电脑 / 笔记本 / 高性能桌面 / 轻量 Linux。
- WebRTC：自动出口 IP / 禁用非代理 UDP / 手动指定。
- 健康检查：展示平台、字体、WebGL、语言、时区、WebRTC 的一致性状态。

### 8.2 高级模式

继续保留原始参数 textarea，但增加：

- 冲突提示。
- 未识别参数保留。
- “恢复推荐值”按钮。
- “重新生成 seed”按钮。

## 9. 检测与回归矩阵

至少覆盖：

- Fingerprint.com Playground：`https://demo.fingerprint.com/playground`
- BrowserScan：`https://www.browserscan.net/`
- CreepJS：`https://abrahamjuliot.github.io/creepjs/`
- bot.incolumitas：`https://bot.incolumitas.com/`
- deviceandbrowserinfo：`https://deviceandbrowserinfo.com/`
- WebRTC leak test。
- Audio/Canvas/WebGL 专项测试页。

每次测试记录：

- Ant 版本。
- Chromium core 版本。
- profile ID。
- proxy 类型、国家、出口 IP。
- headless/headed。
- 检测结果截图。
- 异常字段 JSON。
- 复现步骤。

建议新增内部命令：

```text
ant fingerprint audit --profile <id> --url <detector>
```

用于打开检测站并导出检测结果，后续可逐步自动化。

## 10. 风险与边界

- 不承诺“所有站点完全不可检测”。反检测是持续对抗，检测规则会变化。
- 不建议使用数据中心代理测试高级站点，代理信誉本身会影响结果。
- 不建议把 Firefox 指纹直接用于 Chromium。
- 不建议每次启动都随机全量变更指纹，稳定账号环境更重要。
- JS 注入只能作为临时补丁，关键指纹应尽量在 Chromium 源码层处理。
- vendoring Camoufox 数据前必须确认许可证和 attribution。
- CloakBrowser 的最新 binary / Pro 能力不应作为 Ant 直接依赖。

## 11. 推荐优先级

第一优先级：

- Ant 原生 coherent profile generator。
- 字体库、WebGL allowlist、screen/hardware/media 分布。
- 代理出口 IP、timezone、locale、WebRTC 一致性。
- 指纹健康检查。

第二优先级：

- GeoIP DB 缓存。
- headless/headed 窗口策略。
- persistent profile 默认策略。
- 检测站回归矩阵。

第三优先级：

- humanize 自动化行为层。
- speech voices、media devices 深度一致性。
- plugins/Widevine/PDF Viewer。

第四优先级：

- Chromium 源码层 patch 系统化建设。
- CDP/headless/automation 深层检测修复。

## 12. 结论

Camoufox 提供的是“真实指纹分布和完整指标地图”，CloakBrowser 提供的是“Chromium 源码层和产品化启动策略”。Ant-Browser 应该把二者拆开吸收：

- 用 Camoufox 的数据分布建立 Ant 自己的 Chromium 指纹样本库。
- 用 CloakBrowser 的 Chromium 策略修正启动参数、窗口、GeoIP、WebRTC、persistent profile 和行为层。
- 用一致性校验器把单项随机升级为成套身份生成。
- 用检测站回归矩阵持续衡量优化是否有效。

短期最值得做的不是继续堆更多默认参数，而是实现“按平台/地区/代理出口生成一套自洽 Chromium 指纹”的能力。
