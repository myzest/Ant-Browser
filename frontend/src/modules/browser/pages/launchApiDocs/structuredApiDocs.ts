export type StructuredApiMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'WS'

export type StructuredApiSectionId =
  | 'api-profiles-launch'
  | 'api-runtime'
  | 'api-automation'

export type StructuredApiDocId =
  | StructuredApiSectionId
  | 'api-profiles-list-detail'
  | 'api-profiles-create-detail'
  | 'api-profiles-get-detail'
  | 'api-profiles-update-detail'
  | 'api-profiles-delete-detail'
  | 'api-profiles-status-detail'
  | 'api-profiles-stop-detail'
  | 'api-launch-code-detail'
  | 'api-launch-body-detail'
  | 'api-runtime-active-detail'
  | 'api-runtime-session-detail'
  | 'api-runtime-status-detail'
  | 'api-runtime-stop-detail'
  | 'api-cdp-version-detail'
  | 'api-cdp-list-detail'
  | 'api-cdp-ws-detail'
  | 'api-automation-list-detail'
  | 'api-automation-script-detail'
  | 'api-automation-run-detail'
  | 'api-automation-runs-detail'

export interface StructuredApiExampleContext {
  launchBaseUrl: string
  authHeader: string
}

export interface StructuredApiExample {
  language: string
  code: (ctx: StructuredApiExampleContext) => string
}

export interface StructuredApiField {
  name: string
  type: string
  required: boolean
  location: 'Path' | 'Query' | 'Body' | 'Header'
  description: string
}

export interface StructuredApiResponseCode {
  code: string
  description: string
}

export interface StructuredApiSectionDoc {
  id: StructuredApiSectionId
  title: string
  intro: string
  highlights: string[]
}

export interface StructuredApiEndpointDoc {
  id: Exclude<StructuredApiDocId, StructuredApiSectionId>
  parentId: StructuredApiSectionId
  label: string
  method: StructuredApiMethod
  path: string
  purpose: string
  description: string
  fields: StructuredApiField[]
  requestExample?: StructuredApiExample
  responseExample?: StructuredApiExample
  responseCodes: StructuredApiResponseCode[]
  notes: string[]
}

export const STRUCTURED_API_SECTION_DOCS: StructuredApiSectionDoc[] = [
  {
    id: 'api-profiles-launch',
    title: '实例与启动',
    intro: '这页只做实例管理和启动能力的总览。先通过实例接口完成配置，再按 launchCode 或 selector 启动实例。',
    highlights: [
      '/api/profiles 负责实例增删改查。',
      '/api/launch 负责启动。',
      '详情只从表格里的“查看详情”进入。',
    ],
  },
  {
    id: 'api-runtime',
    title: '运行态与接管',
    intro: '这页只列运行态控制和统一 CDP 入口。外部编排侧先确认 active / session，再决定是否 attach 或 stop。',
    highlights: [
      'runtime/session 用于接管前准备。',
      'runtime/status / runtime/stop 都按 selector 工作。',
      'CDP 详情只从表格进入。',
    ],
  },
  {
    id: 'api-automation',
    title: '脚本自动化',
    intro: '这页只列自动化脚本相关公共入口。先查脚本列表，再按需查详情、执行脚本、回看运行记录。',
    highlights: [
      '先查列表，再按需查详情。',
      '执行接口只接受 object 形态的 selector / params。',
      '详情只从表格进入。',
    ],
  },
]

export const STRUCTURED_API_ENDPOINT_DOCS: StructuredApiEndpointDoc[] = [
  {
    id: 'api-profiles-list-detail',
    parentId: 'api-profiles-launch',
    label: '实例列表',
    method: 'GET',
    path: '/api/profiles',
    purpose: '列出当前全部实例。',
    description: '读取当前实例目录中的全部实例配置，适合做实例选择器或管理后台列表。',
    fields: [],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/profiles \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "count": 1,
  "items": [
    {
      "profileId": "550e8400-e29b-41d4-a716-446655440000",
      "profileName": "buyer-001",
      "launchCode": "BUYER_001",
      "keywords": ["buyer-001"],
      "tags": ["电商"],
      "proxyId": "proxy-us",
      "running": false,
      "debugReady": false
    }
  ]
}`,
    },
    responseCodes: [
      { code: '200', description: '返回实例列表。' },
      { code: '503', description: '实例目录当前不可用。' },
    ],
    notes: [],
  },
  {
    id: 'api-profiles-create-detail',
    parentId: 'api-profiles-launch',
    label: '创建实例',
    method: 'POST',
    path: '/api/profiles',
    purpose: '创建一个新实例，可选创建后立即启动。',
    description: '写入实例配置，必要时同时申请 launchCode，并支持通过 autoLaunch 在创建后直接启动浏览器。',
    fields: [
      { name: 'profile', type: 'object', required: true, location: 'Body', description: '实例配置主体。' },
      { name: 'profile.profileName', type: 'string', required: false, location: 'Body', description: '实例名称；建议填写并与业务账号或任务标签对应。' },
      { name: 'profile.userDataDir', type: 'string', required: false, location: 'Body', description: '用户数据目录；留空时后端自动使用新 profileId。' },
      { name: 'profile.coreId', type: 'string', required: false, location: 'Body', description: '浏览器内核 ID；留空时使用默认内核。' },
      { name: 'profile.fingerprintArgs', type: 'string[]', required: false, location: 'Body', description: '实例持久化指纹参数；不传或空数组时后端会按实例 ID、平台和代理地区生成自洽默认值。' },
      { name: 'profile.proxyId', type: 'string', required: false, location: 'Body', description: '绑定代理池节点；与 proxyConfig 同时传时优先使用 proxyId。' },
      { name: 'profile.proxyConfig', type: 'string', required: false, location: 'Body', description: '自定义代理配置；仅在 proxyId 为空或未命中且本字段非空时保存。' },
      { name: 'profile.launchArgs', type: 'string[]', required: false, location: 'Body', description: '实例持久化启动参数，每次启动都会带上。' },
      { name: 'profile.tags / profile.keywords', type: 'string[]', required: false, location: 'Body', description: '实例标签和检索关键字，用于 selector 匹配与外部编排。' },
      { name: 'profile.groupId', type: 'string', required: false, location: 'Body', description: '所属分组 ID。' },
      { name: 'launchCode', type: 'string', required: false, location: 'Body', description: '指定实例 launchCode。' },
      { name: 'autoLaunch', type: 'boolean', required: false, location: 'Body', description: '是否在创建后立即启动。' },
      { name: 'start', type: 'object', required: false, location: 'Body', description: '仅本次自动启动时附加的启动参数。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/profiles \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "profile": {
      "profileName": "buyer-001",
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
      "keywords": ["buyer-001"],
      "tags": ["电商"]
    },
    "launchCode": "BUYER_001"
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "created": true,
  "updated": false,
  "launched": false,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "profile": {
    "profileId": "550e8400-e29b-41d4-a716-446655440000",
    "profileName": "buyer-001",
    "keywords": ["buyer-001"],
    "proxyId": "proxy-us",
    "fingerprintArgs": [
      "--fingerprint=123456789",
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
    ]
  }
}`,
    },
    responseCodes: [
      { code: '201', description: '实例创建成功。' },
      { code: '400', description: '请求体非法或 profile 缺失。' },
      { code: '409', description: 'launchCode 冲突或实例数超限。' },
    ],
    notes: [
      'autoLaunch=true 时，响应会附带启动结果字段。',
      'profile.fingerprintArgs 是创建实例时已经接入的持久化指纹入口；显式传入时后端只保证补齐/修正稳定种子，响应里的 profile.fingerprintArgs 是最终保存值。',
      '快速配置可直接复制示例中的 Windows/Chrome/美国用户 fingerprintArgs；如果只想使用内置自洽默认指纹，可以省略 fingerprintArgs 或传空数组。',
      'fingerprintArgs 里的语言、时区、屏幕、硬件、WebGL、字体、WebRTC 建议成组传入，不要只改单个高风险字段。',
      'profile.proxyId 与 profile.proxyConfig 同时传时，优先使用 proxyId 对应的代理池节点。',
      '绑定的代理池节点带 country / locale / timezone 时，空 fingerprintArgs 会按代理地区生成默认语言和时区；显式 fingerprintArgs 不会被代理地区覆盖。',
      '若 proxyId 无效：提供 proxyConfig 则按自定义代理保存；未提供 proxyConfig 则返回 400。',
    ],
  },
  {
    id: 'api-profiles-get-detail',
    parentId: 'api-profiles-launch',
    label: '单个实例',
    method: 'GET',
    path: '/api/profiles/{profileId}',
    purpose: '查询单个实例配置。',
    description: '读取指定实例的完整配置快照，适合进入实例详情页或编辑页前预加载数据。',
    fields: [
      { name: 'profileId', type: 'string', required: true, location: 'Path', description: '实例 ID。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/profiles/550e8400-e29b-41d4-a716-446655440000 \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "profile": {
    "profileId": "550e8400-e29b-41d4-a716-446655440000",
    "profileName": "buyer-001",
    "keywords": ["buyer-001"],
    "tags": ["电商"],
    "proxyId": "proxy-us"
  }
}`,
    },
    responseCodes: [
      { code: '200', description: '返回实例详情。' },
      { code: '404', description: '实例不存在。' },
    ],
    notes: [],
  },
  {
    id: 'api-profiles-update-detail',
    parentId: 'api-profiles-launch',
    label: '更新实例',
    method: 'PUT',
    path: '/api/profiles/{profileId}',
    purpose: '更新指定实例配置。',
    description: '用整份 profile 配置覆盖更新实例，可选顺带更新 launchCode，并支持更新后立即启动。',
    fields: [
      { name: 'profileId', type: 'string', required: true, location: 'Path', description: '实例 ID。' },
      { name: 'profile', type: 'object', required: true, location: 'Body', description: '更新后的实例配置。' },
      { name: 'profile.fingerprintArgs', type: 'string[]', required: false, location: 'Body', description: '更新后的完整持久化指纹参数；PUT 是整份更新，未传或空数组会让后端重新生成默认指纹。' },
      { name: 'profile.launchArgs', type: 'string[]', required: false, location: 'Body', description: '更新后的完整持久化启动参数。' },
      { name: 'profile.proxyId / profile.proxyConfig', type: 'string', required: false, location: 'Body', description: '更新后的代理绑定；proxyId 优先于 proxyConfig。' },
      { name: 'launchCode', type: 'string', required: false, location: 'Body', description: '需要覆盖时传新的 launchCode。' },
      { name: 'autoLaunch', type: 'boolean', required: false, location: 'Body', description: '更新后是否直接启动。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X PUT ${launchBaseUrl}/api/profiles/550e8400-e29b-41d4-a716-446655440000 \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "profile": {
      "profileName": "buyer-001-updated",
      "proxyId": "proxy-us",
      "keywords": ["buyer-001", "checkout"]
    }
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "created": false,
  "updated": true,
  "launched": false,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001-updated",
  "launchCode": "BUYER_001"
}`,
    },
    responseCodes: [
      { code: '200', description: '更新成功。' },
      { code: '400', description: '请求体非法。' },
      { code: '404', description: '实例不存在。' },
    ],
    notes: [
      '整份更新，不是 patch。',
      '如果只想改名称、标签等字段，应先 GET /api/profiles/{profileId} 取回原 profile，再带回原 fingerprintArgs，避免意外触发默认指纹重算。',
      'profile.proxyId 与 profile.proxyConfig 同时传时，优先使用 proxyId 对应的代理池节点。',
      '若 proxyId 无效：提供 proxyConfig 则按自定义代理保存；未提供 proxyConfig 则返回 400。',
    ],
  },
  {
    id: 'api-profiles-delete-detail',
    parentId: 'api-profiles-launch',
    label: '删除实例',
    method: 'DELETE',
    path: '/api/profiles/{profileId}',
    purpose: '删除一个未运行中的实例。',
    description: '删除实例配置并移除关联 launchCode；运行中的实例会被直接拒绝删除。',
    fields: [
      { name: 'profileId', type: 'string', required: true, location: 'Path', description: '实例 ID。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X DELETE ${launchBaseUrl}/api/profiles/550e8400-e29b-41d4-a716-446655440000 \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "deleted": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001"
}`,
    },
    responseCodes: [
      { code: '200', description: '删除成功。' },
      { code: '404', description: '实例不存在。' },
      { code: '409', description: '实例仍在运行，不能直接删除。' },
    ],
    notes: [
      '运行中的实例先 stop，再 delete。',
    ],
  },
  {
    id: 'api-profiles-status-detail',
    parentId: 'api-profiles-launch',
    label: '实例状态',
    method: 'GET',
    path: '/api/profiles/{profileId}/status',
    purpose: '查询单个实例的实时运行态。',
    description: '返回运行中、debugReady、cdpUrl 等运行态字段，适合精确观察单个实例当前状态。',
    fields: [
      { name: 'profileId', type: 'string', required: true, location: 'Path', description: '实例 ID。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/profiles/550e8400-e29b-41d4-a716-446655440000/status \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "running": true,
  "debugPort": 9333,
  "debugReady": true,
  "active": true,
  "cdpUrl": "http://127.0.0.1:19876",
  "directDebugUrl": "http://127.0.0.1:9333"
}`,
    },
    responseCodes: [
      { code: '200', description: '返回实例运行态。' },
      { code: '404', description: '实例不存在。' },
    ],
    notes: [],
  },
  {
    id: 'api-profiles-stop-detail',
    parentId: 'api-profiles-launch',
    label: '停止实例',
    method: 'POST',
    path: '/api/profiles/{profileId}/stop',
    purpose: '精确停止一个指定实例。',
    description: '按 profileId 停止实例，适合任务完成后的精确回收。',
    fields: [
      { name: 'profileId', type: 'string', required: true, location: 'Path', description: '实例 ID。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/profiles/550e8400-e29b-41d4-a716-446655440000/stop \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "stopped": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "running": false,
  "debugReady": false,
  "active": false
}`,
    },
    responseCodes: [
      { code: '200', description: '停止成功。' },
      { code: '404', description: '实例不存在。' },
      { code: '503', description: '当前环境不支持运行态控制。' },
    ],
    notes: [],
  },
  {
    id: 'api-launch-code-detail',
    parentId: 'api-profiles-launch',
    label: '按 Code 启动',
    method: 'GET',
    path: '/api/launch/{code}',
    purpose: '按唯一 launchCode 启动实例。',
    description: '最短路径的启动接口，适合外部系统已经拿到唯一 launchCode 的场景。',
    fields: [
      { name: 'code', type: 'string', required: true, location: 'Path', description: '实例 launchCode。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/launch/BUYER_001 \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "pid": 10240,
  "debugPort": 9333,
  "debugReady": true,
  "cdpPort": 19876,
  "cdpUrl": "http://127.0.0.1:19876"
}`,
    },
    responseCodes: [
      { code: '200', description: '启动成功。' },
      { code: '404', description: 'launchCode 不存在。' },
    ],
    notes: [
      '如果实例已经在运行，本接口只复用已有进程，不会重新应用 profile fingerprint 或 profile 代理；如需应用新的 fingerprint/proxy，请先 stop 再重新 launch。',
    ],
  },
  {
    id: 'api-launch-body-detail',
    parentId: 'api-profiles-launch',
    label: '按 selector 启动',
    method: 'POST',
    path: '/api/launch',
    purpose: '按 selector 或兼容顶层字段启动实例。',
    description: '更灵活的启动入口，支持 selector、兼容顶层选择字段、launchArgs、startUrls 和 skipDefaultStartUrls 等临时参数。',
    fields: [
      { name: 'selector', type: 'object', required: false, location: 'Body', description: '目标实例选择条件；新接入推荐使用。' },
      { name: 'code', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.code。' },
      { name: 'profileId', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.profileId。' },
      { name: 'profileName', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.profileName。' },
      { name: 'keyword / keywords', type: 'string / string[]', required: false, location: 'Body', description: '兼容写法：等价于 selector.keyword / selector.keywords。' },
      { name: 'tag / tags', type: 'string / string[]', required: false, location: 'Body', description: '兼容写法：等价于 selector.tag / selector.tags。' },
      { name: 'groupId', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.groupId。' },
      { name: 'matchMode', type: 'unique | first | all', required: false, location: 'Body', description: '兼容写法：等价于 selector.matchMode。' },
      { name: 'launchArgs', type: 'string[]', required: false, location: 'Body', description: '本次启动的临时附加参数。' },
      { name: 'startUrls', type: 'string[]', required: false, location: 'Body', description: '本次启动后额外打开的网址。' },
      { name: 'skipDefaultStartUrls', type: 'boolean', required: false, location: 'Body', description: '是否跳过实例默认启动 URL。' },
      { name: 'proxyId', type: 'string', required: false, location: 'Body', description: '本次启动临时使用的代理池节点，不覆盖实例原代理。' },
      { name: 'proxyConfig', type: 'string', required: false, location: 'Body', description: '本次启动临时使用的代理配置；命中代理池节点时才联动地区指纹。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/launch \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "selector": {
      "keyword": "checkout",
      "tags": ["电商", "北美"],
      "matchMode": "unique"
    },
    "skipDefaultStartUrls": true
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "debugReady": true,
  "cdpUrl": "http://127.0.0.1:19876"
}`,
    },
    responseCodes: [
      { code: '200', description: '启动成功。' },
      { code: '400', description: 'selector 缺失或请求体非法。' },
      { code: '409', description: 'selector 命中多个实例。' },
    ],
    notes: [
      'selector 为空且没有任何兼容顶层选择字段时返回 400。',
      'matchMode=all 只在这个接口可用。',
      'proxyId / proxyConfig 只影响本次启动，不覆盖实例原代理。',
      '一次性 proxyId 命中代理池，或 proxyConfig 与代理池节点配置匹配，且节点或 IP 健康缓存有 country / locale / timezone 时，本次 launch 会临时注入语言、时区和 accept-language 指纹参数。',
      '临时地区参数不会写回 profile.FingerprintArgs；launchArgs 显式传入的语言、时区和 accept-language 优先。',
      '纯自定义 proxyConfig 未命中代理池节点时，不根据代理地址猜测地区。',
      '如果命中的实例已经在运行，本接口只复用已有进程；带 startUrls 时最多打开新窗口并传递 user-data-dir、launchArgs 和 URL，不会重新应用 profile fingerprint、profile 代理或一次性代理。',
      '如需应用新的 fingerprint/proxy/temporary proxy，请先 stop 再重新 launch。',
    ],
  },
  {
    id: 'api-runtime-active-detail',
    parentId: 'api-runtime',
    label: '当前活动实例',
    method: 'GET',
    path: '/api/runtime/active',
    purpose: '查看当前统一 CDP 入口挂着哪个实例。',
    description: '当外部系统只知道 LaunchServer 端口、不知道当前 active target 时，先查这个接口最直接。',
    fields: [],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/runtime/active \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "active": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "profileName": "buyer-001",
  "launchCode": "BUYER_001",
  "running": true,
  "debugReady": true,
  "cdpUrl": "http://127.0.0.1:19876",
  "directDebugUrl": "http://127.0.0.1:9333"
}`,
    },
    responseCodes: [
      { code: '200', description: '返回当前 active target 状态。' },
    ],
    notes: [
      'active=false 表示当前没有活动实例。',
    ],
  },
  {
    id: 'api-runtime-session-detail',
    parentId: 'api-runtime',
    label: '准备可接管会话',
    method: 'POST',
    path: '/api/runtime/session',
    purpose: '准备一个可 attach 的运行时会话。',
    description: '按 selector 命中实例，必要时自动启动，并在给定超时时间内等待 debugReady=true。',
    fields: [
      { name: 'selector', type: 'object', required: false, location: 'Body', description: '目标实例选择条件；新接入推荐使用。' },
      { name: 'code', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.code。' },
      { name: 'profileId', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.profileId。' },
      { name: 'profileName', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.profileName。' },
      { name: 'keyword / keywords', type: 'string / string[]', required: false, location: 'Body', description: '兼容写法：等价于 selector.keyword / selector.keywords。' },
      { name: 'tag / tags', type: 'string / string[]', required: false, location: 'Body', description: '兼容写法：等价于 selector.tag / selector.tags。' },
      { name: 'groupId', type: 'string', required: false, location: 'Body', description: '兼容写法：等价于 selector.groupId。' },
      { name: 'matchMode', type: 'unique | first', required: false, location: 'Body', description: '兼容写法：等价于 selector.matchMode。' },
      { name: 'timeoutMs', type: 'integer', required: false, location: 'Body', description: '等待 debugReady 的超时时间。' },
      { name: 'startUrls', type: 'string[]', required: false, location: 'Body', description: '本次启动时额外打开的网址。' },
      { name: 'skipDefaultStartUrls', type: 'boolean', required: false, location: 'Body', description: '是否跳过实例默认启动 URL。' },
      { name: 'launchArgs', type: 'string[]', required: false, location: 'Body', description: '本次启动时临时附加的启动参数。' },
      { name: 'proxyId', type: 'string', required: false, location: 'Body', description: '本次启动临时使用的代理池节点，不覆盖实例原代理。' },
      { name: 'proxyConfig', type: 'string', required: false, location: 'Body', description: '本次启动临时使用的代理配置；命中代理池节点时才联动地区指纹。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/runtime/session \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "selector": { "code": "BUYER_001" },
    "timeoutMs": 45000,
    "skipDefaultStartUrls": true
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "ready": true,
  "waitTimedOut": false,
  "retryable": false,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "running": true,
  "debugReady": true,
  "active": true,
  "cdpUrl": "http://127.0.0.1:19876",
  "directDebugUrl": "http://127.0.0.1:9333",
  "timeoutMs": 45000
}`,
    },
    responseCodes: [
      { code: '200', description: '实例已 ready，可直接 attach。' },
      { code: '202', description: '实例已处理但暂未 ready，可稍后重试。' },
      { code: '400', description: 'selector 缺失或 matchMode 非法。' },
      { code: '404', description: '目标实例不存在。' },
    ],
    notes: [
      'selector 为空且没有任何兼容顶层选择字段时返回 400。',
      '200 表示 ready，可直接接管。',
      '202 表示未 ready，需要重试。',
      'proxyId / proxyConfig 与 launchArgs 的本次启动语义同 POST /api/launch：不持久化代理或 profile.FingerprintArgs，且 launchArgs 显式语言、时区和 accept-language 优先。',
      '如果实例已经在运行，session 只复用已有进程，不会重新应用 profile fingerprint、profile 代理或一次性代理；如需应用新的 fingerprint/proxy，请先 stop 再重新 launch/session。',
    ],
  },
  {
    id: 'api-runtime-status-detail',
    parentId: 'api-runtime',
    label: '按 selector 查状态',
    method: 'POST',
    path: '/api/runtime/status',
    purpose: '按 selector 查询实例当前运行态。',
    description: '不启动新实例，不等待 ready，只看当前 selector 命中的实例状态。',
    fields: [
      { name: 'selector', type: 'object', required: false, location: 'Body', description: '目标实例选择条件；新接入推荐使用。' },
      { name: 'code / profileId / profileName', type: 'string', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'keyword / keywords', type: 'string / string[]', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'tag / tags / groupId', type: 'string / string[]', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'matchMode', type: 'unique | first', required: false, location: 'Body', description: '运行态控制不支持 all。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/runtime/status \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "selector": { "keyword": "checkout", "matchMode": "first" }
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "running": true,
  "debugReady": false,
  "active": false,
  "cdpUrl": ""
}`,
    },
    responseCodes: [
      { code: '200', description: '返回运行态。' },
      { code: '400', description: 'selector 缺失或 matchMode 非法。' },
      { code: '404', description: '目标实例不存在。' },
    ],
    notes: [
      '不会启动实例。',
      'selector 为空且没有任何兼容顶层选择字段时返回 400。',
    ],
  },
  {
    id: 'api-runtime-stop-detail',
    parentId: 'api-runtime',
    label: '按 selector 停止',
    method: 'POST',
    path: '/api/runtime/stop',
    purpose: '按 selector 停止实例。',
    description: '和 runtime/status 一样使用 selector，但动作改为停止实例，适合编排侧做统一回收。',
    fields: [
      { name: 'selector', type: 'object', required: false, location: 'Body', description: '目标实例选择条件；新接入推荐使用。' },
      { name: 'code / profileId / profileName', type: 'string', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'keyword / keywords', type: 'string / string[]', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'tag / tags / groupId', type: 'string / string[]', required: false, location: 'Body', description: '兼容顶层选择字段。' },
      { name: 'matchMode', type: 'unique | first', required: false, location: 'Body', description: '运行态控制不支持 all。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/runtime/stop \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "selector": { "code": "BUYER_001" }
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "stopped": true,
  "profileId": "550e8400-e29b-41d4-a716-446655440000",
  "launchCode": "BUYER_001",
  "running": false,
  "debugReady": false,
  "active": false
}`,
    },
    responseCodes: [
      { code: '200', description: '停止成功。' },
      { code: '400', description: 'selector 缺失或 matchMode 非法。' },
      { code: '404', description: '目标实例不存在。' },
    ],
    notes: [
      'selector 为空且没有任何兼容顶层选择字段时返回 400。',
      '不支持 matchMode=all。',
    ],
  },
  {
    id: 'api-cdp-version-detail',
    parentId: 'api-runtime',
    label: 'CDP 版本信息',
    method: 'GET',
    path: '/json/version',
    purpose: '读取统一 CDP 入口的版本信息。',
    description: '这个接口透传当前 active target 的 CDP 版本信息，适合 attach 前探测调试入口是否可用。',
    fields: [],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/json/version \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "Browser": "Chrome/<version>",
  "Protocol-Version": "1.3",
  "User-Agent": "Mozilla/5.0",
  "webSocketDebuggerUrl": "ws://127.0.0.1:19876/devtools/browser/active"
}`,
    },
    responseCodes: [
      { code: '200', description: '返回当前 active target 的版本信息。' },
      { code: '503', description: '当前没有可透传的 active target。' },
    ],
    notes: [
      '无 active target 时返回 503。',
    ],
  },
  {
    id: 'api-cdp-list-detail',
    parentId: 'api-runtime',
    label: 'CDP Target 列表',
    method: 'GET',
    path: '/json/list',
    purpose: '读取统一 CDP 入口当前暴露的 target 列表。',
    description: '给 Playwright、Puppeteer 或诊断工具查看当前活动 target 时使用。',
    fields: [],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/json/list \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `[
  {
    "id": "page-1",
    "type": "page",
    "title": "Checkout",
    "url": "https://example.com/checkout",
    "webSocketDebuggerUrl": "ws://127.0.0.1:19876/devtools/page/page-1"
  }
]`,
    },
    responseCodes: [
      { code: '200', description: '返回当前活动实例的 target 列表。' },
      { code: '503', description: '当前没有可透传的 active target。' },
    ],
    notes: [
      '无 active target 时返回 503。',
    ],
  },
  {
    id: 'api-cdp-ws-detail',
    parentId: 'api-runtime',
    label: 'CDP WebSocket',
    method: 'WS',
    path: '/devtools/...',
    purpose: '通过统一 WebSocket 入口接管当前活动实例。',
    description: '实际 attach 时使用的就是这个 WebSocket 入口。外部工具通常先拿 /json/version 或 /json/list，再连对应 websocketDebuggerUrl。',
    fields: [],
    requestExample: {
      language: 'javascript',
      code: ({ launchBaseUrl }) => {
        const wsBase = launchBaseUrl.replace(/^http/i, 'ws')
        return `const browser = await chromium.connectOverCDP("${wsBase}");
// 或按 /json/list 返回的 webSocketDebuggerUrl 连接具体 page target`
      },
    },
    responseExample: {
      language: 'text',
      code: () => `WebSocket 握手成功后进入标准 Chrome DevTools Protocol 消息流。`,
    },
    responseCodes: [
      { code: '101', description: 'WebSocket 升级成功。' },
      { code: '503', description: '当前没有可透传的 active target。' },
    ],
    notes: [
      '先调 runtime/session，再连 WS。',
    ],
  },
  {
    id: 'api-automation-list-detail',
    parentId: 'api-automation',
    label: '脚本列表',
    method: 'GET',
    path: '/api/automation/scripts',
    purpose: '查询可执行脚本清单。',
    description: '返回脚本元数据，用于拿 scriptId、默认 selector / params 和脚本类型。',
    fields: [],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/automation/scripts \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "count": 1,
  "items": [
    {
      "id": "news-query-txt",
      "name": "查询新闻并写 TXT",
      "type": "playwright-cdp",
      "status": "ready",
      "entryFile": "index.cjs",
      "selector": { "code": "BUYER_001" },
      "params": { "keyword": "OpenAI", "limit": 10 }
    }
  ]
}`,
    },
    responseCodes: [
      { code: '200', description: '返回脚本列表。' },
      { code: '503', description: '自动化脚本能力未启用。' },
    ],
    notes: [
      '不返回 scriptText。',
    ],
  },
  {
    id: 'api-automation-script-detail',
    parentId: 'api-automation',
    label: '脚本详情',
    method: 'GET',
    path: '/api/automation/scripts/{scriptId}',
    purpose: '按 scriptId 查询单个脚本详情。',
    description: '标准单资源读取接口，用于从脚本列表进入某个脚本时补充其来源和包格式等元数据。',
    fields: [
      { name: 'scriptId', type: 'string', required: true, location: 'Path', description: '脚本唯一 ID。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl ${launchBaseUrl}/api/automation/scripts/news-query-txt \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "item": {
    "id": "news-query-txt",
    "name": "查询新闻并写 TXT",
    "type": "playwright-cdp",
    "status": "ready",
    "entryFile": "index.cjs",
    "selector": { "code": "BUYER_001" },
    "params": { "keyword": "OpenAI", "limit": 10 },
    "packageFormat": "ant-automation-script",
    "manifestVersion": 1,
    "source": {
      "type": "git",
      "uri": "https://example.com/repo.git",
      "ref": "main"
    }
  }
}`,
    },
    responseCodes: [
      { code: '200', description: '返回脚本详情。' },
      { code: '404', description: '脚本不存在。' },
      { code: '503', description: '自动化脚本能力未启用。' },
    ],
    notes: [
      '不返回 scriptText。',
    ],
  },
  {
    id: 'api-automation-run-detail',
    parentId: 'api-automation',
    label: '执行脚本',
    method: 'POST',
    path: '/api/automation/scripts/run',
    purpose: '按 scriptId 执行脚本。',
    description: '外部调用方只需传入脚本 ID 和对象形态的 selector / params。推荐优先传 selector.code；如果脚本已在 UI 中绑定目标实例，也可以只传 scriptId 直接执行。',
    fields: [
      { name: 'scriptId', type: 'string', required: true, location: 'Body', description: '要执行的脚本 ID。' },
      { name: 'selector', type: 'object', required: false, location: 'Body', description: '覆盖脚本默认 selector。' },
      { name: 'params', type: 'object', required: false, location: 'Body', description: '覆盖脚本默认 params。' },
      { name: 'useScriptSelector', type: 'boolean', required: false, location: 'Body', description: '显式指定是否沿用脚本默认 selector。' },
      { name: 'useScriptParams', type: 'boolean', required: false, location: 'Body', description: '显式指定是否沿用脚本默认 params。' },
      { name: 'timeoutMs', type: 'integer', required: false, location: 'Body', description: '本次脚本执行超时时间。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl -X POST ${launchBaseUrl}/api/automation/scripts/run \\
  -H "Content-Type: application/json" \\
  -H "${authHeader}: <your-api-key>" \\
  -d '{
    "scriptId": "news-query-txt",
    "selector": { "code": "BUYER_001" },
    "params": { "keyword": "OpenAI", "limit": 10 }
  }'`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "run": {
    "id": "run-1",
    "scriptId": "news-query-txt",
    "scriptName": "查询新闻并写 TXT",
    "scriptType": "playwright-cdp",
    "status": "success",
    "summary": "已抓取 10 条新闻并写入 TXT",
    "durationMs": 12034
  }
}`,
    },
    responseCodes: [
      { code: '200', description: '执行成功。' },
      { code: '400', description: 'scriptId 缺失或 selector / params 不是对象。' },
      { code: '500', description: '脚本执行失败。' },
    ],
    notes: [
      'selector / params 必须是 object。',
      '不传时沿用脚本默认配置。',
    ],
  },
  {
    id: 'api-automation-runs-detail',
    parentId: 'api-automation',
    label: '运行记录',
    method: 'GET',
    path: '/api/automation/scripts/runs?limit=20',
    purpose: '查询最近脚本运行记录。',
    description: '返回最近 N 次脚本执行记录，适合调试、审计和任务结果回看。',
    fields: [
      { name: 'limit', type: 'integer', required: false, location: 'Query', description: '返回记录条数，默认 20，最小 1，最大 200。' },
    ],
    requestExample: {
      language: 'bash',
      code: ({ launchBaseUrl, authHeader }) => `curl "${launchBaseUrl}/api/automation/scripts/runs?limit=20" \\
  -H "${authHeader}: <your-api-key>"`,
    },
    responseExample: {
      language: 'json',
      code: () => `{
  "ok": true,
  "count": 1,
  "limit": 20,
  "items": [
    {
      "id": "run-1",
      "scriptId": "news-query-txt",
      "status": "success",
      "summary": "已抓取 10 条新闻并写入 TXT",
      "durationMs": 12034
    }
  ]
}`,
    },
    responseCodes: [
      { code: '200', description: '返回运行记录。' },
      { code: '503', description: '自动化脚本能力未启用。' },
    ],
    notes: [],
  },
]

export const STRUCTURED_API_SECTION_DOC_MAP = Object.fromEntries(
  STRUCTURED_API_SECTION_DOCS.map((doc) => [doc.id, doc]),
) as Record<StructuredApiSectionId, StructuredApiSectionDoc>

export const STRUCTURED_API_ENDPOINT_DOC_MAP = Object.fromEntries(
  STRUCTURED_API_ENDPOINT_DOCS.map((doc) => [doc.id, doc]),
) as Record<Exclude<StructuredApiDocId, StructuredApiSectionId>, StructuredApiEndpointDoc>

const STRUCTURED_API_DOC_IDS = new Set<StructuredApiDocId>([
  ...STRUCTURED_API_SECTION_DOCS.map((doc) => doc.id),
  ...STRUCTURED_API_ENDPOINT_DOCS.map((doc) => doc.id),
])

export function isStructuredApiDocId(id: string): id is StructuredApiDocId {
  return STRUCTURED_API_DOC_IDS.has(id as StructuredApiDocId)
}

export function isStructuredApiEndpointDocId(id: string): id is Exclude<StructuredApiDocId, StructuredApiSectionId> {
  return id in STRUCTURED_API_ENDPOINT_DOC_MAP
}

export function getStructuredApiParentDocId(id: StructuredApiDocId): StructuredApiSectionId {
  if (id in STRUCTURED_API_SECTION_DOC_MAP) {
    return id as StructuredApiSectionId
  }
  return STRUCTURED_API_ENDPOINT_DOC_MAP[id as Exclude<StructuredApiDocId, StructuredApiSectionId>].parentId
}

export function getStructuredApiHiddenDocItems() {
  return STRUCTURED_API_ENDPOINT_DOCS.map((doc) => ({
    id: doc.id,
    label: doc.label,
    summary: doc.purpose,
    content: '',
  }))
}

export function getStructuredApiSectionEndpoints(sectionId: StructuredApiSectionId) {
  return STRUCTURED_API_ENDPOINT_DOCS.filter((doc) => doc.parentId === sectionId)
}
