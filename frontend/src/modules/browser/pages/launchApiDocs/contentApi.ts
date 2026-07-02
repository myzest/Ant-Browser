export const DOC_API_PROFILES_LAUNCH = `# 实例与启动

## 实例接口

| 方法 | 路径 | 用途 |
|------|------|------|
| \`GET\` | \`/api/profiles\` | 列实例 |
| \`POST\` | \`/api/profiles\` | 创建实例 |
| \`GET\` | \`/api/profiles/{profileId}\` | 查单个实例 |
| \`PUT\` | \`/api/profiles/{profileId}\` | 更新实例 |
| \`DELETE\` | \`/api/profiles/{profileId}\` | 删除实例 |
| \`GET\` | \`/api/profiles/{profileId}/status\` | 查实例运行态 |
| \`POST\` | \`/api/profiles/{profileId}/stop\` | 停止实例 |

## 创建实例

最小请求只需要传 \`profile\`。如果不传 \`profile.fingerprintArgs\`，后端会按实例 ID、平台默认值和绑定代理地区生成一套自洽指纹。

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/profiles \\
  -H "Content-Type: application/json" \\
  -d '{
    "profile": {
      "profileName": "buyer-001",
      "proxyId": "proxy-us",
      "keywords": ["buyer-001"],
      "tags": ["电商"]
    },
    "launchCode": "BUYER_001"
  }'
\`\`\`

## 创建实例（快速指纹配置）

如果外部系统需要像界面“创建实例配置”一样一次性指定指纹，可以在 \`profile.fingerprintArgs\` 里直接传 CLI 参数数组。下面是一个可直接复制的 Windows / Chrome / 美国用户快速配置：

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/profiles \\
  -H "Content-Type: application/json" \\
  -d '{
    "profile": {
      "profileName": "buyer-us-001",
      "proxyId": "proxy-us",
      "fingerprintArgs": [
        "--fingerprint-brand=Chrome",
        "--fingerprint-platform=windows",
        "--lang=en-US",
        "--fingerprint-locale=en-US",
        "--fingerprint-accept-language=en-US,en;q=0.9",
        "--timezone=America/Los_Angeles",
        "--fingerprint-timezone=America/Los_Angeles",
        "--window-size=1920,1080",
        "--fingerprint-screen-avail=1920,1040",
        "--fingerprint-device-pixel-ratio=1",
        "--fingerprint-color-depth=24",
        "--fingerprint-hardware-concurrency=8",
        "--fingerprint-device-memory=8",
        "--fingerprint-canvas-noise=true",
        "--fingerprint-audio-noise=true",
        "--fingerprint-webgl-vendor=Intel",
        "--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
        "--fingerprint-fonts=Segoe UI,Calibri,Cambria Math,Consolas,Arial,Tahoma,Times New Roman,Verdana",
        "--webrtc-ip-handling-policy=disable_non_proxied_udp",
        "--fingerprint-webrtc-ip=auto",
        "--fingerprint-do-not-track=false",
        "--fingerprint-touch-points=0",
        "--fingerprint-media-devices=1,1,1"
      ],
      "launchArgs": ["--disable-sync", "--no-first-run"],
      "keywords": ["buyer-us-001", "us"],
      "tags": ["电商", "北美"]
    },
    "launchCode": "BUYER_US_001"
  }'
\`\`\`

显式传入 \`fingerprintArgs\` 时，后端只保证补齐或修正 \`--fingerprint=<seed>\`，不会再自动补齐 WebGL、字体、屏幕、硬件等其它缺失项。为了降低异常风险，建议语言、时区、Accept-Language、屏幕、硬件、WebGL、字体和 WebRTC 成组配置；如果希望完全使用内置自洽默认指纹，就省略 \`fingerprintArgs\` 或传空数组。

## 创建并立即启动

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/profiles \\
  -H "Content-Type: application/json" \\
  -d '{
    "profile": {
      "profileName": "buyer-002",
      "keywords": ["buyer-002"]
    },
    "autoLaunch": true,
    "start": {
      "skipDefaultStartUrls": true
    }
  }'
\`\`\`

## 创建实例（自定义代理配置）

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/profiles \\
  -H "Content-Type: application/json" \\
  -d '{
    "profile": {
      "profileName": "buyer-003",
      "proxyConfig": "http://127.0.0.1:18080",
      "keywords": ["buyer-003"]
    }
  }'
\`\`\`

## 查询 / 更新 / 删除

\`\`\`bash
curl http://127.0.0.1:19876/api/profiles
curl http://127.0.0.1:19876/api/profiles/550e8400-e29b-41d4-a716-446655440000
curl -X PUT http://127.0.0.1:19876/api/profiles/550e8400-e29b-41d4-a716-446655440000 -H "Content-Type: application/json" -d '{ ... }'
curl -X DELETE http://127.0.0.1:19876/api/profiles/550e8400-e29b-41d4-a716-446655440000
\`\`\`

## 启动接口

| 方法 | 路径 | 用途 |
|------|------|------|
| \`GET\` | \`/api/launch/{code}\` | 按唯一 Code 启动 |
| \`POST\` | \`/api/launch\` | 按 code / selector 参数化启动 |

### 按 Code 启动

\`\`\`bash
curl http://127.0.0.1:19876/api/launch/A3F9K2
\`\`\`

### 按 selector 启动

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/launch \\
  -H "Content-Type: application/json" \\
  -d '{
    "selector": {
      "keyword": "checkout",
      "tags": ["电商", "北美"],
      "groupId": "group-sales-us",
      "matchMode": "unique"
    },
    "skipDefaultStartUrls": true
  }'
\`\`\`

### 一次性代理与地区指纹

\`POST /api/launch\` 可以通过 \`proxyId\` 或 \`proxyConfig\` 指定本次启动使用的代理，这两个字段只影响本次启动，不会覆盖实例保存的代理配置。

当一次性 \`proxyId\` 命中代理池，或一次性 \`proxyConfig\` 与代理池节点配置匹配时，如果该节点带有 country / locale / timezone，或最近的 IP 健康检测缓存里有可用地区信息，本次 launch 会临时注入 \`--lang\`、\`--fingerprint-locale\`、\`--fingerprint-accept-language\`、\`--timezone\` 和 \`--fingerprint-timezone\`。这些参数不会写回 \`profile.FingerprintArgs\`。

\`launchArgs\` 显式传入的语言、时区和 accept-language 优先级最高；纯自定义 \`proxyConfig\` 未命中代理池节点时，不会根据代理地址猜测地区。

## 启动成功响应

\`\`\`json
{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "debugReady": true,
  "cdpUrl": "http://127.0.0.1:19876"
}
\`\`\`

## 单实例状态 / 停止

| 方法 | 路径 | 示例用途 |
|------|------|----------|
| \`GET\` | \`/api/profiles/{profileId}/status\` | 查实例是否运行、是否 ready |
| \`POST\` | \`/api/profiles/{profileId}/stop\` | 任务完成后精确停止 |

## 记住这几个规则

\`\`\`text
launchCode 冲突 -> 409
PUT 是整份更新
创建实例 profile.fingerprintArgs 已接入；不传或传空数组时由后端生成默认自洽指纹
PUT 更新也是整份 profile，若不想重算指纹，请先 GET 旧 profile 并带回原 fingerprintArgs
运行中的实例不能直接 DELETE
已运行实例不会因再次 launch / session 重新应用 fingerprint 或 profile 代理
POST launch / session 中传入的一次性代理也不会作用于已运行进程
如需应用新的 fingerprint / profile proxy / temporary proxy，先 stop 再重新 launch / session
运行中实例需要打开新窗口时，只会向已有进程传递 user-data-dir、launchArgs 和 URL
matchMode=all 只在 POST /api/launch 可用
proxyId 和 proxyConfig 同时传 -> 优先 proxyId
proxyId 无效 + proxyConfig 非空 -> 使用 proxyConfig
proxyId 无效 + proxyConfig 为空 -> 400
一次性 proxyId / proxyConfig 只影响本次启动，不覆盖实例原代理
一次性代理命中代理池且有地区信息 -> 本次 launch 临时联动语言 / 时区 / accept-language
extra launchArgs 显式语言 / 时区 / accept-language -> 优先于临时代理地区
纯自定义 proxyConfig 未命中代理池 -> 不猜地区
\`\`\`
`

export const DOC_API_RUNTIME = `# 运行态与接管

## 接口

| 方法 | 路径 | 用途 |
|------|------|------|
| \`GET\` | \`/api/runtime/active\` | 查当前活动实例 |
| \`POST\` | \`/api/runtime/session\` | 准备可接管会话 |
| \`POST\` | \`/api/runtime/status\` | 按 selector 查运行态 |
| \`POST\` | \`/api/runtime/stop\` | 按 selector 停止实例 |
| \`GET\` | \`/json/version\` | 统一 CDP 入口 |
| \`GET\` | \`/json/list\` | 统一 CDP 入口 |
| \`WS\` | \`/devtools/...\` | CDP WebSocket 接管 |

## 查询当前活动实例

\`\`\`bash
curl http://127.0.0.1:19876/api/runtime/active
\`\`\`

\`\`\`json
{
  "ok": true,
  "active": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "debugReady": true,
  "cdpUrl": "http://127.0.0.1:19876"
}
\`\`\`

## 准备可接管会话

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/runtime/session \\
  -H "Content-Type: application/json" \\
  -d '{
    "selector": {
      "code": "BUYER_001"
    },
    "timeoutMs": 45000,
    "skipDefaultStartUrls": true
  }'
\`\`\`

| 返回 | 含义 |
|------|------|
| \`200 + ready=true\` | 可以直接 attach |
| \`202 + ready=false\` | 已处理，但还没 ready |

## 按 selector 查状态

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/runtime/status \\
  -H "Content-Type: application/json" \\
  -d '{
    "selector": {
      "keyword": "shop",
      "matchMode": "first"
    }
  }'
\`\`\`

## 按 selector 停止

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/runtime/stop \\
  -H "Content-Type: application/json" \\
  -d '{
    "code": "BUYER_001"
  }'
\`\`\`

## 接管示例

\`\`\`javascript
import { chromium } from "playwright";

const res = await fetch("http://127.0.0.1:19876/api/runtime/session", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    selector: { code: "BUYER_001" },
    skipDefaultStartUrls: true
  })
});

const data = await res.json();
const browser = await chromium.connectOverCDP(data.cdpUrl);
\`\`\`

## 记住这几个规则

\`\`\`text
runtime/status 和 runtime/stop 不支持 matchMode=all
attach 前先看 active / debugReady / cdpUrl
统一入口只指向一个活动实例
\`\`\`
`

export const DOC_API_AUTOMATION = `# 脚本自动化

## 接口

| 方法 | 路径 | 用途 |
|------|------|------|
| \`GET\` | \`/api/automation/scripts\` | 查脚本列表 |
| \`GET\` | \`/api/automation/scripts/{scriptId}\` | 查单个脚本详情 |
| \`POST\` | \`/api/automation/scripts/run\` | 执行脚本 |
| \`GET\` | \`/api/automation/scripts/runs\` | 查运行记录 |

## 列脚本

\`\`\`bash
curl http://127.0.0.1:19876/api/automation/scripts
\`\`\`

\`\`\`json
{
  "ok": true,
  "items": [
    {
      "id": "news-query-txt",
      "name": "查询新闻并写 TXT",
      "type": "playwright-cdp",
      "status": "ready"
    }
  ]
}
\`\`\`

## 执行脚本

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/automation/scripts/run \\
  -H "Content-Type: application/json" \\
  -d '{
    "scriptId": "news-query-txt",
    "selector": { "code": "BUYER_001" },
    "params": { "keyword": "OpenAI", "limit": 10 }
  }'
\`\`\`

\`\`\`json
{
  "ok": true,
  "run": {
    "id": "run-1",
    "status": "success",
    "summary": "已抓取 10 条新闻并写入 TXT"
  }
}
\`\`\`

如果脚本已经在界面里配置成 \`使用已有实例\` 或 \`按模板新建实例\`，也可以只传：

\`\`\`bash
curl -X POST http://127.0.0.1:19876/api/automation/scripts/run \\
  -H "Content-Type: application/json" \\
  -d '{
    "scriptId": "news-query-txt"
  }'
\`\`\`

## 查运行记录

\`\`\`bash
curl http://127.0.0.1:19876/api/automation/scripts/runs?limit=20
\`\`\`

## 记住这几个规则

\`\`\`text
scriptId 必填
推荐优先使用 selector.code，而不是 profileId
selector / params 必须是 JSON object
不传 selector / params 时，默认沿用脚本内配置
\`\`\`
`
