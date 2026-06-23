# 指纹回归采集模板

本模板用于 P0/P1 阶段记录检测站结果。现阶段只做采集与人工判读，不把第三方站点分数作为 CI 阻断项。

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
