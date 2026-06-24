# 指纹回归采集模板

本模板用于 P0/P1 阶段记录检测站结果。现阶段只做采集与人工判读，不把第三方站点分数作为 CI 阻断项。

## 内置采集入口

P0-D 阶段新增内置自动化脚本 `fingerprint-audit`，类型为 `playwright-cdp`。它复用现有 automation runner 与 Launch API 启动/连接 profile，默认先保存本地 probe，再按需打开检测站并保存截图/HTML。

示例请求：

```json
{
  "scriptId": "fingerprint-audit",
  "selector": { "code": "BUYER_001", "matchMode": "unique" },
  "params": {
    "detectors": ["browserscan", "creepjs"],
    "localProbeOnly": false,
    "captureScreenshot": true,
    "saveHtml": true,
    "waitAfterLoadMs": 5000,
    "timeoutMs": 120000
  }
}
```

最小离线采集可设置 `"localProbeOnly": true`，此时只生成 `local-probe.json` 与 `report.json`。检测站页面评分不解析、不作为 CI 阻断。

## UA-CH 采集边界

`fingerprint-audit` 会采集页面可见的 `navigator.userAgentData`，用于人工判读或后续与 profile / `client_hints.json` 数据比对。该采集只记录当前运行实例暴露出的 UA Client Hints，不代表 Ant-Browser 已经完成 UA-CH 伪装。

当前 UA-CH 仍是 data-only capability skeleton：只提供 Ant 自行整理的候选数据模型，不生成、不应用运行时启动参数。真正的 runtime UA-CH 生效能力需要另一个明确实现，并经过 Chromium 内核支持与检测站验证。

## 检测站点

- Fingerprint.com Playground: `https://demo.fingerprint.com/playground`
- BrowserScan: `https://www.browserscan.net/`
- CreepJS: `https://abrahamjuliot.github.io/creepjs/`
- bot.incolumitas: `https://bot.incolumitas.com/`
- Device and Browser Info: `https://deviceandbrowserinfo.com/`
- WebRTC leak test
- Canvas / Audio / WebGL 专项测试页

## 记录字段

| 字段 | 内容 |
|---|---|
| Ant 版本 |  |
| Chromium core 版本 |  |
| OS |  |
| headed/headless |  |
| profile ID |  |
| seed |  |
| fingerprintArgs |  |
| launchArgs |  |
| proxy 类型 |  |
| proxy 国家/城市 |  |
| proxy 出口 IP |  |
| detector URL |  |
| 截图路径 |  |
| 异常字段 JSON |  |
| 人工结论 |  |
| 复现步骤 |  |

## P0 验收重点

- 默认 profile 不出现 Firefox/Chrome 混合身份。
- WebRTC 不泄露真实 IP；直连时不保留随机伪造公网 IP。
- timezone / locale 与代理出口一致，或明确记录不一致原因。
- `screen` / `avail` / `inner` / `outer` / `window-size` 无物理矛盾。
- 记录 `navigator.webdriver`、plugins、PDF/Widevine、WebGL renderer、Canvas/Audio 稳定性。

## 后续 audit 命令边界

未来 `ant fingerprint audit --profile <id> --url <detector>` 只负责：

- 复用现有 profile/launch API 启动实例。
- 打开检测 URL，等待页面稳定。
- 截图并保存本地 JS probe 结果。
- 输出 `data/fingerprint-audit/<profile>/<timestamp>/report.json`。

站点页面评分、代理信誉、截图判读仍由人工确认。
