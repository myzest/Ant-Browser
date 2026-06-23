# Chromium Fingerprint Pool Design

## 1. 背景

Ant Browser 当前同时存在两类指纹配置路径：

- Chromium/fingerprint-chromium：通过 `--fingerprint-*` 启动参数生效。
- Camoufox：通过构建期预生成的 `camoufox_fingerprints.json` 池，启动时编码成 `CAMOU_CONFIG_*` 环境变量生效。

现有 Camoufox 指纹池已经证明“构建期生成、运行时稳定选取”的模式可行。但 Camoufox 池包含 Firefox/Camoufox 生态字段，不能原样喂给 Chromium，否则会出现 Chrome 身份与 Firefox 指纹混杂的问题。

本设计稿的目标是把“指纹池”能力引入 Chromium，同时保持不同浏览器内核的指纹语义隔离。

## 2. 目标

### 2.1 产品目标

- 给 Chromium 实例提供稳定、可复现、可池化的指纹配置。
- 保留用户现有 `fingerprintArgs` 的编辑习惯和兼容性。
- 让同一个 profile 在没有显式改动时，每次启动拿到同一组池化指纹。
- 后续可以按代理出口、国家、OS、硬件画像选择更贴合的指纹。

### 2.2 工程目标

- 不让 Chromium 直接消费 Camoufox 的 `CAMOU_CONFIG`。
- 建立“池条目 -> 内核导出器”的结构：
  - Chromium 导出为 `--fingerprint-*` 参数。
  - Camoufox 继续导出为 `CAMOU_CONFIG_*`。
- 将共享逻辑限制在池选择、OS 归一、seed 计算、基础画像字段。
- 让测试覆盖可重复选择、用户覆盖优先级、字段导出边界。

## 3. 非目标

- 不修改 fingerprint-chromium 内核源码。
- 不在第一版支持所有浏览器可观测字段。
- 不把 Camoufox 的 WebGL 参数全集迁移到 Chromium。
- 不改变现有 profile 数据结构的必填字段。
- 不强制迁移用户已有 `fingerprintArgs`。

## 4. 现状代码落点

### 4.1 Chromium 启动参数

Chromium 实例启动参数在 `backend/app_instance_start_prepare.go` 中构造。

当前逻辑：

- 系统参数：`--user-data-dir`、`--remote-debugging-port`、`--proxy-server`
- 如果没有 `--fingerprint=`，按 `profile.ProfileId` 计算 seed 并补充。
- 追加 `profile.FingerprintArgs`。
- 追加 profile launch args、临时 launch args、启动 URL。

因此 Chromium 指纹池最自然的接入点是：在追加 `profile.FingerprintArgs` 之前，生成一组池化 `--fingerprint-*` 参数，然后再让用户参数覆盖它。

需要注意：启动时真正参与 Chromium 的参数不只有 `profile.FingerprintArgs`。`profile.LaunchArgs` 和一次性 `ExtraLaunchArgs` 也会追加到最终命令行，并且预检逻辑已经把这三类参数一起用于指纹一致性检查。因此后续实现池化合并时，也必须把三类参数都纳入“用户显式覆盖”判断。

### 4.2 Camoufox 指纹池

Camoufox 池位于：

- `backend/internal/browser/camoufox_pool.go`
- `backend/internal/browser/camoufox_config.go`
- `backend/internal/browser/assets/camoufox_fingerprints.json`

可复用的设计经验：

- 内嵌 JSON asset。
- 按 OS 分池。
- 按 seed 取模稳定选择条目。
- 构建期生成，运行时不联网、不依赖 Python。

不能直接复用的数据内容：

- Firefox UA、Gecko 字段、`navigator.oscpu`。
- Firefox/Camoufox runtime 字段。
- Camoufox 的 WebGL 参数全集。
- 与 Chrome 行为不一致的 headers。

## 5. 总体设计

### 5.1 分层

```text
Profile
  |
  | profile id / fingerprint seed / selected OS
  v
Fingerprint Pool Selector
  |
  | generic pool entry
  v
Runtime Exporter
  |
  | ChromiumExporter -> --fingerprint-* args
  | CamoufoxExporter  -> CAMOU_CONFIG_*
  v
Browser Launch
```

### 5.2 第一版建议

第一版优先做 Chromium 专用池，不急着抽象成完全通用池。

原因：

- Camoufox 池已经稳定工作，贸然抽通用层会扩大回归面。
- Chromium 的可消费字段是 CLI 参数，不是 Camoufox config。
- 两边字段语义差异较大，先用导出器边界隔开更稳。

建议新增：

- `backend/internal/browser/chromium_pool.go`
- `backend/internal/browser/assets/chromium_fingerprints.json`
- `backend/internal/browser/chromium_fingerprint_export.go`
- 对应测试文件。

## 6. Chromium 池数据结构

### 6.1 JSON 顶层

```json
{
  "version": 1,
  "meta": {
    "countPerOS": 32,
    "osList": ["windows", "mac", "linux"],
    "generator": {
      "name": "ant-browser-static-chromium-pool",
      "revision": "v1"
    }
  },
  "pools": {
    "windows": [],
    "mac": [],
    "linux": []
  }
}
```

### 6.2 单条指纹

```json
{
  "os": "windows",
  "seed": 0,
  "hash": "stable-content-hash",
  "profile": {
    "brand": "Chrome",
    "platform": "windows",
    "lang": "zh-CN",
    "timezone": "Asia/Shanghai",
    "windowSize": "1920,1080",
    "colorDepth": 24,
    "hardwareConcurrency": 8,
    "deviceMemory": 8,
    "webglVendor": "Intel",
    "webglRenderer": "Intel(R) UHD Graphics 630",
    "fonts": ["Arial", "Microsoft YaHei", "SimSun", "SimHei", "Times New Roman"],
    "doNotTrack": false,
    "touchPoints": 0,
    "webrtcPolicy": "disable_non_proxied_udp",
    "canvasNoise": true,
    "audioNoise": true
  }
}
```

### 6.3 字段范围

第一版只导出 fingerprint-chromium 已有 UI/serializer 覆盖的字段：

- `--fingerprint`
- `--fingerprint-brand`
- `--fingerprint-platform`
- `--lang`
- `--timezone`
- `--window-size`
- `--fingerprint-color-depth`
- `--fingerprint-hardware-concurrency`
- `--fingerprint-device-memory`
- `--fingerprint-canvas-noise`
- `--fingerprint-webgl-vendor`
- `--fingerprint-webgl-renderer`
- `--fingerprint-audio-noise`
- `--fingerprint-fonts`
- `--webrtc-ip-handling-policy`
- `--fingerprint-do-not-track`
- `--fingerprint-touch-points`

暂不池化：

- UA 明文。除非确认 fingerprint-chromium 对 `--fingerprint-brand/platform` 的 UA 生成规则不足。
- `navigator.webdriver`、插件、媒体设备细节等未在当前 UI 中稳定表达的字段。
- WebGL 参数全集。

## 7. 选择规则

### 7.1 OS 归一

Chromium 池 OS key 使用：

- `windows`
- `mac`
- `linux`

兼容输入：

- `win`、`windows`、`pc` -> `windows`
- `mac`、`macos`、`darwin` -> `mac`
- `linux` -> `linux`

### 7.2 Seed 来源

优先级：

1. 启动时合并参数中的用户显式 `--fingerprint=<seed>`
2. 当前已有规则：由 `profile.ProfileId` 计算稳定 seed

这里的“启动时合并参数”包括：

- `profile.FingerprintArgs`
- 已清理 managed args 后的 `profile.LaunchArgs`
- 已清理 managed args 后的一次性 `ExtraLaunchArgs`

池选择使用同一个 seed：

```text
index = abs(seed) % len(pool[os])
```

最终导出的 Chromium args 中也包含同一个 `--fingerprint=<seed>`，保证 fingerprint-chromium 内部随机噪声与池选择稳定绑定。

### 7.3 OS 来源

优先级：

1. 启动时合并参数中的用户显式 `--fingerprint-platform=<platform>`
2. profile 已经通过 `ApplyDefaults` 落入的默认 fingerprint args
3. 宿主 OS 对应的默认 platform，与 `defaultFingerprintArgsForOS` 保持一致

第一版可以先采用更保守规则：

- 如果启动参数里已经有 platform，按该 platform 选池。
- 否则按宿主 OS 归一为 `windows`、`mac` 或 `linux`，与当前默认指纹配置保持一致。

## 8. 参数合并策略

### 8.1 原则

用户显式配置永远优先于池化默认值。

推荐顺序：

```text
system managed args
proxy args
chromium pool exported fingerprint args
profile.FingerprintArgs
profile launch args
extra launch args
urls
```

这样同一个 key 后出现的用户参数可以覆盖池参数。若 fingerprint-chromium 使用“首次出现优先”，则需要在合并前让池化 args 对用户显式 key 让位。

为了避免依赖内核对重复参数的解析顺序，建议实现为：

1. 生成池化 args。
2. 收集三类用户参数中已经设置过的 fingerprint key。
3. 从池化 args 中移除同 key 项。
4. 拼接池化剩余项 + `profile.FingerprintArgs`。
5. 保持 `profile.LaunchArgs` 和一次性 `ExtraLaunchArgs` 的原有追加顺序。

三类用户参数是：

- `profile.FingerprintArgs`
- 已清理 managed args 后的 `profile.LaunchArgs`
- 已清理 managed args 后的一次性 `ExtraLaunchArgs`

这样即使用户把 `--lang`、`--fingerprint-platform`、`--window-size` 临时放在 launch args 里，池化参数也不会提前生成同 key 默认值造成重复。

第一版不建议静默删除用户源之间已有的重复 fingerprint key，例如 `profile.FingerprintArgs` 和 `profile.LaunchArgs` 同时设置 `--lang`。这类冲突应继续交给现有预检告警提示，避免池化接入改变用户原有启动参数语义。

### 8.2 Managed args

以下参数继续由系统接管，不进入池：

- `--user-data-dir`
- `--remote-debugging-port`
- `--remote-debugging-address`
- `--remote-debugging-pipe`
- `--proxy-server`

### 8.3 兼容已有 profile

已有 profile 不做数据迁移。

启动时动态补充池化参数：

- 用户没有设置的字段由池提供。
- 用户已经设置的字段保持原值。
- 用户只设置 `--fingerprint=<seed>` 时，用该 seed 选池并补齐其他未设置字段。
- 用户把 fingerprint 相关参数放在 launch args 或一次性 extra args 中时，也视为显式覆盖。

## 9. 生成策略

### 9.1 第一版静态池

第一版可以手写或脚本生成一个小型静态池：

- 每个 OS 16 到 32 条。
- 只使用常见且自洽的桌面画像。
- Windows 占比最高。

推荐画像：

- Windows + Chrome + Intel UHD/HD + 1366x768/1920x1080/1600x900
- Windows + Chrome + NVIDIA/AMD + 1920x1080/2560x1440
- macOS + Chrome + Apple GPU + 1440x900/2560x1440
- Linux + Chrome + Intel/Mesa + 1920x1080/1366x768

### 9.2 后续生成器

后续可新增：

- `scripts/gen_chromium_fingerprints.py`

生成器职责：

- 按 OS 生成候选画像。
- 计算稳定 hash。
- 校验字段组合是否自洽。
- 输出 `backend/internal/browser/assets/chromium_fingerprints.json`。

## 10. 与代理推荐联动

当前代理推荐逻辑在 `backend/app_proxy_fingerprint_suggestion.go`，主要输出：

- timezone
- language
- brand
- platform

指纹池接入后可以分两阶段：

### 10.1 第一阶段

代理推荐仍然只改写用户可见 `fingerprintArgs`。

启动时：

- 用户推荐参数覆盖池。
- 池补齐未推荐字段。

### 10.2 第二阶段

把代理健康信息作为选池 hint：

- 国家 -> language/timezone 候选
- ASN/组织 -> brand/platform 候选
- residential/datacenter -> 风险提示，不直接改硬件

第一版不建议把代理推荐直接绑定到池选择，以免引入过多隐式行为。

## 11. 测试计划

### 11.1 Pool loader

覆盖：

- JSON 能正确加载。
- OS key 缺失时报错。
- seed 正负数取模稳定。
- 空池报错。

### 11.2 Exporter

覆盖：

- 单条 pool entry 能导出完整 Chromium args。
- 布尔、数字、数组字段序列化正确。
- fonts 数组导出为逗号字符串。
- 空字段不导出。

### 11.3 Merge

覆盖：

- 用户 `--fingerprint-platform=mac` 覆盖池默认 platform。
- 用户 `--lang=en-US` 覆盖池默认 lang。
- 用户 `--fingerprint=<seed>` 同时用于池选择和最终 args。
- 用户未设置 seed 时按 profile id 生成稳定 seed。
- 池化 args 遇到用户显式 key 时会让位，不生成同 key 默认值。
- 用户在 `profile.LaunchArgs` 或一次性 `ExtraLaunchArgs` 中设置 fingerprint key 时，池化 args 不再生成同 key。

### 11.4 Launch args

覆盖：

- Chromium 启动链包含池化 args。
- Camoufox 启动链不受 Chromium 池影响。
- managed args 仍然不会被 fingerprint pool 覆盖。

## 12. 迁移步骤

### Step 1: 新增 Chromium 池结构

新增：

- `ChromiumFingerprintPool`
- `ChromiumPoolEntry`
- `LoadChromiumFingerprintPool`
- `SelectChromiumFingerprint`
- `NormalizeChromiumFingerprintOS`

验收：

- loader 测试通过。

### Step 2: 新增 Chromium exporter

新增：

- `ExportChromiumFingerprintArgs(entry, seed)`
- 字段到 CLI 参数的序列化。

验收：

- exporter 测试通过。

### Step 3: 新增 fingerprint arg merge

新增：

- `MergeChromiumFingerprintArgs(poolArgs, fingerprintArgs, launchArgs, extraArgs)`
- 只移除会与用户显式 fingerprint key 冲突的池化参数，用户参数本身保持原顺序和原语义。

验收：

- merge 测试通过。

### Step 4: 接入 Chromium 启动链

修改：

- `buildBrowserLaunchArgs`

建议调整为：

```text
combinedUserArgs = profile.FingerprintArgs + sanitizedProfileLaunchArgs + sanitizedExtraLaunchArgs
seed = resolveFingerprintSeed(profile.ProfileId, combinedUserArgs)
os = resolveFingerprintPlatform(combinedUserArgs, hostOS)
poolArgs = buildChromiumPoolArgs(seed, os)
fingerprintArgs = merge(poolArgs, profile.FingerprintArgs, sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs)
args append fingerprintArgs
```

验收：

- 原有启动测试通过。
- 新增启动参数测试通过。

### Step 5: 后续 UI 提示

第一版可不改 UI。

后续可在指纹面板中展示：

- 当前 seed 对应池条目 hash。
- 哪些字段来自池。
- 哪些字段被用户覆盖。

## 13. 风险与应对

### 13.1 重复参数解析顺序不确定

风险：

- Chromium 或 fingerprint-chromium 对重复 key 的解析顺序可能不是预期。

应对：

- 在应用层先移除会与用户显式 key 冲突的池化参数，确保池化接入不新增重复 key。
- 用户源之间原本已经存在的重复 key 不在第一版静默改写，继续依赖启动前预检告警。

### 13.2 池字段与内核支持不一致

风险：

- 某些 `--fingerprint-*` 参数在目标内核版本中不支持。

应对：

- 第一版只使用当前 UI 已暴露的字段。
- 对不支持字段保持无害跳过。

### 13.3 指纹组合不自洽

风险：

- macOS + NVIDIA、Linux + Microsoft YaHei 等组合降低可信度。

应对：

- 按 OS 维护字体/GPU/分辨率候选。
- 生成器增加校验规则。

### 13.4 与用户现有配置冲突

风险：

- 用户已配置完整 fingerprint args，池化参数造成意外变化。

应对：

- 用户显式字段优先。
- 可增加配置开关关闭 Chromium 池化补齐。

## 14. 推荐实现结论

推荐实现方式：

- 不复用 Camoufox 指纹池数据。
- 复用 Camoufox 的“内嵌池 + OS 分组 + seed 稳定选择”模式。
- 为 Chromium 新增专用池。
- 用 exporter 将池条目转换为 `--fingerprint-*` 参数。
- 合并时用户参数优先，池化参数只补用户未显式设置的字段。

这样能把池化能力带进原 Chromium 指纹体系，同时避免 Chrome/Firefox 指纹串味，也给后续按代理出口智能选池留下空间。
