# Chromium 指纹池长期生产化计划

## 1. 背景

当前 Chromium 指纹池已经完成基础链路：

- 内嵌 `chromium_fingerprints.json`
- 按 OS + seed 稳定选池
- 导出为 fingerprint-chromium 可消费的 `--fingerprint-*` 参数
- 用户显式参数优先，池化参数只补缺

但当前池规模较小，主要用于验证接入链路，不适合作为长期生产池。长期生产版本需要具备：

- 足够大的样本空间
- 跨 OS 自洽的硬件、字体、语言、时区组合
- 可重复生成
- 可校验、可回滚、可灰度
- 与代理推荐和实例启动链路稳定协作

本计划的目标是把 Chromium 指纹池从“可用 MVP”推进到“可长期维护的生产资产”。

## 2. 生产目标

### 2.1 池规模

第一阶段生产池建议：

- Windows：128 条
- macOS：64 条
- Linux：64 条
- 总计：256 条

第二阶段扩展目标：

- Windows：256 条
- macOS：128 条
- Linux：128 条
- 总计：512 条

Windows 占比更高，符合当前默认 `--fingerprint-platform=windows` 和多数桌面运营环境。

### 2.2 稳定性目标

- 同一 `profileId` 在未修改 seed/platform 时稳定选中同一条指纹。
- 用户显式 `--fingerprint=<seed>` 后，跨版本尽量保持选池稳定。
- 发布包运行时不依赖 Python、不联网、不调用外部生成服务。
- 指纹池文件进入版本控制，可追踪 diff，可回滚。

### 2.3 可信度目标

每条 Chromium 指纹必须满足：

- OS、字体、GPU、分辨率自洽。
- 语言与时区组合合理。
- 桌面端默认 `touchPoints=0`。
- 不混入 Firefox/Camoufox/Gecko 字段。
- 不生成当前 fingerprint-chromium 内核无法消费的字段。

## 3. 生产化路线

### Phase 1: 生成器落地

新增：

- `scripts/gen_chromium_fingerprints.py`

职责：

- 根据固定候选库生成 Chromium 指纹池。
- 固定随机 seed，保证同一输入可复现同一输出。
- 计算每条记录 hash。
- 对组合进行规则校验。
- 输出紧凑 JSON 到 `backend/internal/browser/assets/chromium_fingerprints.json`。

建议命令：

```bash
python3 scripts/gen_chromium_fingerprints.py \
  --windows 128 \
  --mac 64 \
  --linux 64 \
  --out backend/internal/browser/assets/chromium_fingerprints.json
```

### Phase 2: 候选库建设

生成器不应纯随机拼字段，而应从分层候选库采样。

候选库建议放在脚本内第一版实现，后续可拆成：

- `scripts/fingerprint_data/chromium_windows.json`
- `scripts/fingerprint_data/chromium_mac.json`
- `scripts/fingerprint_data/chromium_linux.json`

每个 OS 候选库包含：

- 浏览器品牌候选
- 语言/时区区域组
- 分辨率候选
- CPU 核心数候选
- 设备内存候选
- GPU vendor/renderer 候选
- 字体集合候选
- WebRTC 策略候选

### Phase 3: 校验器落地

新增：

- `scripts/validate_chromium_fingerprints.py`

也可以先把 `--validate-only` 合并进生成器。

校验内容：

- JSON schema 完整。
- 每个 OS 数量达到目标。
- hash 唯一。
- seed 顺序连续。
- 必填字段非空。
- 所有导出字段都在 Chromium exporter 支持范围内。
- 不包含 `navigator.*`、`headers.*`、`webGl:parameters` 等 Camoufox config 字段。
- 字体/GPU/OS 组合不越界。
- 语言/时区组合属于允许矩阵。

### Phase 4: Go 侧生产测试扩展

新增或扩展测试：

- 池加载测试：数量、OS key、hash 非空。
- 池选择测试：seed 取模、负数 seed、OS alias。
- 导出测试：所有池条目都能导出合法 CLI。
- 合并测试：用户显式参数优先。
- 启动参数测试：池参数不会覆盖 managed args、proxy args、launch args。

建议增加一个全池遍历测试：

```text
for each os pool:
  for each entry:
    args = ExportChromiumFingerprintArgs(entry, seed)
    assert args contains --fingerprint
    assert no empty --key=
    assert no unsupported key
```

### Phase 5: 灰度与开关

长期生产使用建议加配置开关：

```yaml
browser:
  chromium_fingerprint_pool:
    enabled: true
    mode: supplement
    version: builtin
```

第一版已实现的行为等价于：

- `enabled=true`
- `mode=supplement`

后续可支持：

- `enabled=false`：完全回到旧行为，只补 `--fingerprint=<seed>`。
- `mode=supplement`：只补用户未设置字段。
- `mode=strict`：由池控制完整指纹，用户显式覆盖需要预检确认。

建议生产初期默认保持 `supplement`，风险最低。

## 4. 数据生成规则

### 4.1 Windows 规则

品牌：

- Chrome：80%
- Edge：20%

GPU 候选：

- Intel UHD / Iris Xe：主流办公画像
- NVIDIA GTX/RTX：高配画像
- AMD Radeon：中高配画像

字体集合：

- 中文环境：`Microsoft YaHei`、`SimSun`、`SimHei`、`Segoe UI`
- 英文环境：`Arial`、`Calibri`、`Segoe UI`、`Tahoma`、`Verdana`
- 日文环境：`Meiryo`、`Yu Gothic`、`Segoe UI`

分辨率：

- `1366,768`
- `1440,900`
- `1600,900`
- `1920,1080`
- `2560,1440`

硬件：

- CPU：4、6、8、12、16
- Memory：4、8、16
- Color depth：24
- Touch points：0

### 4.2 macOS 规则

品牌：

- Chrome：100%

GPU 候选：

- Apple M1
- Apple M2
- Apple M1 Pro
- Apple M2 Pro
- Intel Iris OpenGL Engine

字体集合：

- `Arial`
- `Helvetica`
- `Menlo`
- `PingFang SC`
- `Hiragino Sans`
- `SF Pro Text`

分辨率：

- `1280,800`
- `1440,900`
- `1512,982`
- `1680,1050`
- `1920,1080`
- `2560,1440`

硬件：

- CPU：8、10、12
- Memory：8、16
- Color depth：30
- Touch points：0

### 4.3 Linux 规则

品牌：

- Chrome：100%

GPU 候选：

- Mesa Intel UHD/HD
- Mesa Intel Iris Xe
- AMD Radeon RADV
- NVIDIA OpenGL renderer

字体集合：

- `DejaVu Sans`
- `Liberation Sans`
- `Noto Sans`
- `Ubuntu`
- `Noto Sans CJK SC`
- `Noto Sans CJK JP`

分辨率：

- `1366,768`
- `1440,900`
- `1600,900`
- `1920,1080`
- `2560,1440`

硬件：

- CPU：4、6、8、12
- Memory：4、8、16
- Color depth：24
- Touch points：0

## 5. 地区矩阵

语言和时区不要完全随机，应按地区矩阵组合。

建议第一版矩阵：

| Region | Lang | Timezone |
| --- | --- | --- |
| CN | `zh-CN` | `Asia/Shanghai` |
| US-EAST | `en-US` | `America/New_York` |
| US-CENTRAL | `en-US` | `America/Chicago` |
| US-WEST | `en-US` | `America/Los_Angeles` |
| JP | `ja-JP` | `Asia/Tokyo` |
| UK | `en-GB` | `Europe/London` |
| DE | `de-DE` | `Europe/Berlin` |

各 OS 可有不同地区权重：

- Windows：CN/US/JP 权重高。
- macOS：US/CN/JP/UK 权重高。
- Linux：US/UK/DE/CN 权重高。

## 6. 版本策略

### 6.1 JSON meta

生产池 meta 应包含：

```json
{
  "version": 2,
  "meta": {
    "countPerOS": {
      "windows": 128,
      "mac": 64,
      "linux": 64
    },
    "generator": {
      "name": "ant-browser-chromium-fingerprint-generator",
      "revision": "v1",
      "seed": 20260623
    },
    "schema": "chromium-fingerprint-pool/v2",
    "generatedAt": "2026-06-23"
  }
}
```

当前 Go 结构中的 `CountPerOS int` 可继续兼容第一版；生产化时建议改为 `map[string]int` 或新增 `Counts map[string]int`，避免 Windows/mac/Linux 数量不一致时表达不自然。

### 6.2 Seed 稳定性

池扩容会影响 `seed % len(pool)` 的结果，导致同一个 profile 选中不同条目。

长期建议：

- 第一版生产池应尽量一次性达到 256 条，避免短期内频繁扩容。
- 只要继续使用 `seed % len(pool)`，即使只 append 也可能让部分 profile 重新分布。
- 已发布池不应随意改变长度；必须扩容时应视为一次显式指纹池版本升级。
- 如果必须重排，提升 pool version，并在 release notes 中说明指纹池会重新分布。

更稳的后续方案：

```text
index = rendezvous_hash(seed/profileId, entry.hash)
```

或者在 profile 中持久化已选中的 pool entry hash。两种方式都会改变现有 seed 语义或数据模型，建议单独设计，不在本轮生产化第一阶段做。

## 7. 与代理推荐联动

生产化后可以分阶段引入代理 hint。

### 7.1 阶段一：保持现状

代理推荐仍然只生成：

- platform
- lang
- timezone
- brand

池负责补齐硬件、字体、WebGL 等未设置字段。

### 7.2 阶段二：地区 hint

在选池时根据代理健康结果选择地区子池：

```text
country=US -> lang/timezone 优先 US 矩阵
country=JP -> lang/timezone 优先 JP 矩阵
country=CN -> lang/timezone 优先 CN 矩阵
```

注意：不要让代理风险分直接改变 GPU/CPU/内存，这类联动过强，容易形成奇怪画像。

### 7.3 阶段三：可解释 UI

在指纹面板显示：

- 当前池条目 hash
- seed
- OS pool
- 来自池的字段
- 被用户覆盖的字段

这会显著降低“为什么我的参数变了”的困惑。

## 8. 发布与验收

### 8.1 PR 验收清单

每次更新生产池必须满足：

- `python3 scripts/gen_chromium_fingerprints.py ...` 可复现生成。
- `python3 scripts/validate_chromium_fingerprints.py ...` 通过。
- `jq empty backend/internal/browser/assets/chromium_fingerprints.json` 通过。
- `CGO_ENABLED=0 go test ./backend/internal/browser` 通过。
- `CGO_ENABLED=0 go test ./backend -run Chromium` 通过。
- diff 中没有 Camoufox 池、配置文件、用户数据等无关变更。

### 8.2 运行验收

至少手动启动以下实例：

- Windows platform + 默认 seed
- Windows platform + 显式 seed
- mac platform + 显式 `--fingerprint-platform=mac`
- Linux platform + 显式 `--fingerprint-platform=linux`
- 用户显式 `--lang` 覆盖池语言
- 用户显式 `--window-size` 覆盖池分辨率

检查：

- 实例能正常启动。
- 最终 args 中没有池化造成的重复 key。
- 用户显式字段优先。
- 代理参数不受影响。

## 9. 风险与控制

### 9.1 大池质量风险

问题：

- 大量随机组合可能产生不自洽画像。

控制：

- 使用候选库 + 规则矩阵，不纯随机拼字段。
- 生成后做自动校验。
- 每次发布人工抽样检查。

### 9.2 版本升级导致指纹漂移

问题：

- 池扩容或重排会导致同一 seed 选中不同条目。

控制：

- 第一版生产池一次性给足容量，减少扩容频率。
- 已发布池长度变化视为版本升级，而不是普通数据刷新。
- 修改排序必须提升池版本。
- UI 后续显示 pool hash，方便用户确认变化。

### 9.3 内核支持差异

问题：

- 不同 fingerprint-chromium 版本可能支持的 `--fingerprint-*` 参数不同。

控制：

- 第一阶段只使用当前 UI 已暴露字段。
- 新字段进入池前先在 exporter 和能力表里声明。
- 保持用户覆盖优先，未知字段不由池生成。

### 9.4 文件体积

问题：

- 512 条池会增加发布包体积。

控制：

- Chromium 池只保存 CLI 画像，不保存 Camoufox 那种 WebGL 参数全集，体积可控。
- JSON 使用紧凑格式提交。

## 10. 推荐排期

### Milestone 1: 生产生成器

产出：

- `scripts/gen_chromium_fingerprints.py`
- 256 条生产池
- 基础校验内置

验收：

- 可复现生成。
- Go 测试通过。

### Milestone 2: 独立校验器与全池测试

产出：

- `scripts/validate_chromium_fingerprints.py`
- Go 全池遍历测试
- CI/本地发布前校验命令文档

验收：

- 任意无效 OS/GPU/字体组合能被校验器拦截。

### Milestone 3: 配置开关与灰度

产出：

- `browser.chromium_fingerprint_pool.enabled`
- `browser.chromium_fingerprint_pool.mode`
- 关闭池化时回退旧行为

验收：

- enabled=false 时只补 legacy seed。
- supplement 模式保持当前行为。

### Milestone 4: 代理 hint 与 UI 可解释性

产出：

- 按国家/地区 hint 选择池条目
- 指纹面板展示 pool hash 和字段来源

验收：

- 代理推荐不会隐式改写用户显式字段。
- UI 能清楚解释哪些字段来自池。

## 11. 最终建议

短期不要手写更多 JSON 条目。正确路线是先补生成器和校验器，再把池扩到 256 条。

生产化第一版推荐目标：

- 256 条池
- 紧凑 JSON
- 规则生成
- 自动校验
- supplement 模式默认开启
- 保留用户覆盖优先

这样可以把当前 Chromium 指纹池从“链路验证”升级到“可长期维护的生产池”，同时不会引入 Camoufox/Firefox 指纹串味，也不会让用户已有启动参数突然失控。
