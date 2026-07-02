export const DOC_TUTORIAL = `# 使用教程

## 只在应用内使用

\`\`\`text
内核管理 -> 下载 / 识别内核
代理池配置 -> 导入 / 添加代理
实例列表 -> 新建实例 -> 选择内核 / 代理 -> 启动
\`\`\`

## 最小上手顺序

1. 打开 \`指纹浏览器 > 内核管理\`
2. 准备一个可用内核
3. 打开 \`指纹浏览器 > 代理池配置\`，按需导入代理
4. 打开 \`指纹浏览器 > 实例列表\`，新建实例
5. 保存后直接启动

## 最小 HTTP 对接

\`\`\`bash
curl http://127.0.0.1:19876/api/health

curl -X POST http://127.0.0.1:19876/api/launch \\
  -H "Content-Type: application/json" \\
  -d '{
    "code": "BUYER_001",
    "skipDefaultStartUrls": true
  }'
\`\`\`

## 返回里主要看这几个字段

\`\`\`json
{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "debugReady": true,
  "cdpUrl": "http://127.0.0.1:19876"
}
\`\`\`

## Playwright 接管

\`\`\`javascript
import { chromium } from "playwright";

const res = await fetch("http://127.0.0.1:19876/api/launch", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    code: "BUYER_001",
    skipDefaultStartUrls: true
  })
});

const data = await res.json();
const browser = await chromium.connectOverCDP(data.cdpUrl);
\`\`\`

需要稳定接管时，不要自己轮询，直接用 \`POST /api/runtime/session\`。
`

export const DOC_SKILL_USAGE = `# SKILL 使用说明

## 先准备好 3 个前提

1. Ant Browser 和 OpenClaw 在同一台机器上
2. Ant Browser 的 LaunchServer 可访问，默认是 \`http://127.0.0.1:19876\`
3. OpenClaw 已安装 \`ant-chrome-openclaw\` skill，并且已有指向 Ant Browser 的远程 CDP 浏览器配置

## 安装 Skill

如果你不知道项目根目录在哪，先点上面的 \`打开根目录\`。

### Windows

直接安装：

\`\`\`powershell
pwsh -File skills/ant-chrome-openclaw/scripts/install_ant_chrome_openclaw.ps1 -SetDefaultProfile
\`\`\`

如果没探测到 OpenClaw 路径，再用：

\`\`\`powershell
pwsh -File skills/ant-chrome-openclaw/scripts/install_ant_chrome_openclaw.ps1 -TargetSkillsDir "C:\\path\\to\\openclaw\\skills" -ConfigFile "C:\\path\\to\\openclaw\\openclaw.json" -SetDefaultProfile
\`\`\`

### Linux

直接安装：

\`\`\`bash
bash skills/ant-chrome-openclaw/scripts/install_ant_chrome_openclaw.sh \
  --set-default-profile
\`\`\`

如果没探测到 OpenClaw 路径，再用：

\`\`\`bash
bash skills/ant-chrome-openclaw/scripts/install_ant_chrome_openclaw.sh \\
  --target-skills-dir /path/to/openclaw/skills \\
  --config-file /path/to/openclaw/openclaw.json \\
  --set-default-profile
\`\`\`

如果 Ant Browser 开了 API Key，安装时补上对应参数即可。

## 在对话里怎么触发

每次提问开头都明确写：

\`\`\`text
使用 ant-chrome-openclaw skill。
\`\`\`

然后直接写你的目标，不要只说“帮我打开浏览器”。

## 推荐提问模板

### 启动并接管

\`\`\`text
使用 ant-chrome-openclaw skill。
先检查 LaunchServer。
如果实例 BUYER_001 没有运行，就启动它。
确认 debugReady=true 后接管浏览器，并打开 https://example.com
\`\`\`

### 只接管当前活动实例

\`\`\`text
使用 ant-chrome-openclaw skill。
先检查当前 active 实例。
如果当前活动实例已经是 BUYER_001，就直接接管，不要切换到别的实例。
\`\`\`

### 按条件匹配实例

\`\`\`text
使用 ant-chrome-openclaw skill。
按 keyword=buyer-001 查实例状态。
如果唯一命中，就接管。
如果多命中，不要自动切换，先告诉我。
\`\`\`

### 停止实例

\`\`\`text
使用 ant-chrome-openclaw skill。
停止 launchCode=BUYER_001 对应的实例。
\`\`\`

## 稳定使用规则

- 先在 Ant Browser 前端里把实例、代理、标签和内核配置好
- 优先提供精确标识，推荐顺序是 \`launchCode\`、\`profileId\`、\`profileName\`
- 只有在 \`debugReady=true\` 且 \`cdpUrl\` 非空时才接管
- 如果 selector 命中多个实例，不要自动选，先返回结果给用户确认
- \`browser stop\` 只是断开 OpenClaw 接管，不等于关闭 Ant Browser 实例
- 当前统一 CDP 入口一次只指向一个活动实例，切换前先看 \`GET /api/runtime/active\`

## 最小排查顺序

\`\`\`text
1. GET /api/health
2. GET /api/runtime/active
3. 按 code 或 selector 调 launch / runtime/session
4. 如果失败，再看 GET /api/launch/logs?limit=20
\`\`\`
`

export const DOC_CORE_INTRO = `# 内核介绍

## 推荐目录

\`\`\`text
chrome/
  chrome-<version>/
    chrome.exe
    ...
\`\`\`

## 两种准备方式

\`\`\`text
方式 A：应用内下载
指纹浏览器 -> 内核管理 -> 下载内核

方式 B：手动下载
下载 ZIP -> 解压到 chrome/<version>/ -> 回到内核管理确认识别
\`\`\`

## 下载渠道

使用可直接下载的浏览器内核 ZIP 包地址，或从团队内部维护的发布渠道获取安装包。

## 在应用里怎么用

\`\`\`text
内核管理 -> 设默认内核
实例编辑页 -> 选择内核
\`\`\`

## 自检

\`\`\`text
1. 目录下能看到 chrome.exe
2. 内核管理页能识别到该目录
3. 实例绑定的是正确版本
\`\`\`
`

export const DOC_PROXY_INTRO = `# 代理介绍

## 直接录入示例

\`\`\`text
<proxy-url>
<proxy-url>
<proxy-url>
\`\`\`

## Clash YAML 导入示例

\`\`\`yaml
proxies:
  - name: hk-vless
    type: vless
    server: example.com
    port: 443
    uuid: your-uuid
    tls: true
    servername: example.com
\`\`\`

## 链式代理

链式代理用于需要“先通过一个前置代理，再连接目标代理节点”的场景。例如部分国外代理 IP 只允许从国外网络连接，此时可以把一个可用的 Clash 节点或 HTTP/SOCKS5 作为前置代理，再由它去连接第二层 HTTP/SOCKS5 代理。

链路顺序：

\`\`\`text
Chromium -> 本地 127.0.0.1 SOCKS5 桥接 -> 前置代理（Clash 节点或 HTTP/SOCKS5）-> 第二层/后置 HTTP/SOCKS5 -> 目标网站
\`\`\`

当前支持两种两跳链路：

- 前置 HTTP/SOCKS5 + 第二跳 HTTP/SOCKS5（兼容原有链式代理方案）
- 前置 Clash 节点 + 第二跳 HTTP/SOCKS5

前置 Clash 节点可以通过两种方式选择：

1. 填写 Clash 订阅 URL，点击获取节点，然后从订阅节点列表中选择一个作为前置代理。
2. 粘贴整份 Clash 订阅 YAML 或单个 Clash 节点 YAML，点击解析输入节点，然后从解析结果中选择一个作为前置代理。

保存链式代理时，只会保存最终选中的单个前置节点，不会把整份订阅作为链式配置保存。第二层/后置代理仍只支持 HTTP/SOCKS5。

### 链式代理 JSON 示例

\`\`\`json
{
  "name": "Clash前置链路",
  "group": "链式代理",
  "localPort": "",
  "first": {
    "type": "clash",
    "name": "前置节点名称",
    "node": {
      "name": "hk-vless",
      "type": "vless",
      "server": "example.com",
      "port": 443,
      "uuid": "your-uuid",
      "tls": true,
      "servername": "example.com"
    }
  },
  "second": {
    "protocol": "socks5",
    "server": "second-hop.example.com",
    "port": "1080",
    "username": "",
    "password": ""
  }
}
\`\`\`

## 批量 DNS 示例

\`\`\`yaml
dns:
  enable: true
  nameserver:
    - 119.29.29.29
    - 223.5.5.5
\`\`\`

## 应用内路径

\`\`\`text
代理池配置 -> 导入 Clash YAML / 录入 HTTP(S) / SOCKS5 / 链式代理
实例编辑页 -> 选择代理池节点
\`\`\`

## 在实例接口里绑定代理

\`\`\`json
{
  "profile": {
    "profileName": "buyer-001",
    "proxyId": "proxy-us",
    "keywords": ["buyer-001"]
  },
  "launchCode": "BUYER_001"
}
\`\`\`

不需要代理时，\`proxyId\` 留空即可。
\`proxyId\` 与 \`proxyConfig\` 同时传时，优先使用 \`proxyId\` 对应的代理池节点。
如果 \`proxyId\` 无效但传了 \`proxyConfig\`，会自动改为使用该 \`proxyConfig\`。
如果 \`proxyId\` 无效且 \`proxyConfig\` 也为空，请求会直接报错。
`

export const DOC_API_OVERVIEW = `# 接口总览

## 基础地址

\`\`\`text
http://127.0.0.1:19876
\`\`\`

## 认证示例

\`\`\`bash
curl -H "X-Ant-Api-Key: <your-api-key>" http://127.0.0.1:19876/api/health
\`\`\`

没开认证时，去掉这个请求头即可。

## 全部接口

| 分类 | 方法 | 路径 |
|------|------|------|
| 健康检查 | \`GET\` | \`/api/health\` |
| 实例管理 | \`GET\` | \`/api/profiles\` |
| 实例管理 | \`POST\` | \`/api/profiles\` |
| 实例管理 | \`GET\` | \`/api/profiles/{profileId}\` |
| 实例管理 | \`PUT\` | \`/api/profiles/{profileId}\` |
| 实例管理 | \`DELETE\` | \`/api/profiles/{profileId}\` |
| 实例管理 | \`GET\` | \`/api/profiles/{profileId}/status\` |
| 实例管理 | \`POST\` | \`/api/profiles/{profileId}/stop\` |
| 启动 | \`GET\` | \`/api/launch/{code}\` |
| 启动 | \`POST\` | \`/api/launch\` |
| 运行态 | \`GET\` | \`/api/runtime/active\` |
| 运行态 | \`POST\` | \`/api/runtime/session\` |
| 运行态 | \`POST\` | \`/api/runtime/status\` |
| 运行态 | \`POST\` | \`/api/runtime/stop\` |
| 自动化脚本 | \`GET\` | \`/api/automation/scripts\` |
| 自动化脚本 | \`GET\` | \`/api/automation/scripts/{scriptId}\` |
| 自动化脚本 | \`POST\` | \`/api/automation/scripts/run\` |
| 自动化脚本 | \`GET\` | \`/api/automation/scripts/runs\` |
| 调用日志 | \`GET\` | \`/api/launch/logs\` |
| CDP 统一入口 | \`GET\` | \`/json/version\` |
| CDP 统一入口 | \`GET\` | \`/json/list\` |
| CDP 统一入口 | \`WS\` | \`/devtools/...\` |

## selector 最小写法

\`\`\`json
{
  "selector": {
    "code": "BUYER_001",
    "matchMode": "unique"
  }
}
\`\`\`

也可以把 \`code / profileId / profileName / keyword / tags / groupId / matchMode\` 放在请求体顶层；新接入建议统一放进 \`selector\`。

## 怎么选接口

| 场景 | 用哪个 |
|------|--------|
| 已知唯一 Code | \`GET /api/launch/{code}\` |
| 需要参数 / selector | \`POST /api/launch\` |
| 需要 ready 后再接管 | \`POST /api/runtime/session\` |
| 只有 selector，想查状态 | \`POST /api/runtime/status\` |
| 只有 selector，想停止 | \`POST /api/runtime/stop\` |
`
