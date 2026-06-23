import type { BrowserProxy } from '../types'
import type {
  BrowserCoreRuntime,
  BrowserProxyFingerprintRecommendation,
  BrowserProxyFingerprintRecommendationRequest,
  FingerprintCapabilityItem,
  FingerprintCapabilityStatus,
  FingerprintFieldKey,
} from '../types/fingerprint'
import { FINGERPRINT_PRESETS, deserialize, serialize, type FingerprintConfig } from './fingerprintSerializer'

type GeoProfile = {
  country?: string
  region?: string
  city?: string
  isResidential?: boolean
  fraudScore?: number
  asOrganization?: string
}

type CapabilityDefinition = {
  key: FingerprintFieldKey
  label: string
  group: string
  notes: Record<FingerprintCapabilityStatus, string>
}

const CAPABILITY_DEFINITIONS: CapabilityDefinition[] = [
  {
    key: 'seed',
    label: '指纹种子',
    group: '基础身份',
    notes: {
      supported: '会直接进入启动流程',
      partial: '仅能间接影响启动池或被运行时接管',
      unsupported: '当前内核不直接消费该项',
    },
  },
  {
    key: 'brand',
    label: '浏览器品牌',
    group: '基础身份',
    notes: {
      supported: '可直接写入启动参数',
      partial: '可能被运行时覆盖为默认值',
      unsupported: '当前内核会忽略该项',
    },
  },
  {
    key: 'platform',
    label: '操作系统',
    group: '基础身份',
    notes: {
      supported: '可直接生效',
      partial: '可能只影响池选择或预设选择',
      unsupported: '当前内核不会直接改写平台指纹',
    },
  },
  {
    key: 'lang',
    label: '语言',
    group: '基础身份',
    notes: {
      supported: '可直接生效',
      partial: '仅部分语言字段会被同步',
      unsupported: '当前内核不支持该语言设置',
    },
  },
  {
    key: 'timezone',
    label: '时区',
    group: '基础身份',
    notes: {
      supported: '可直接生效',
      partial: '可能跟随池内预设或系统时区',
      unsupported: '当前内核不支持显式时区覆盖',
    },
  },
  {
    key: 'resolution',
    label: '分辨率',
    group: '屏幕与硬件',
    notes: {
      supported: '可直接生效',
      partial: '可能只同步外层窗口尺寸',
      unsupported: '当前内核不支持该项',
    },
  },
  {
    key: 'colorDepth',
    label: '色深',
    group: '屏幕与硬件',
    notes: {
      supported: '可直接生效',
      partial: '可能仅受运行时默认值影响',
      unsupported: '当前内核不直接覆盖色深',
    },
  },
  {
    key: 'hardwareConcurrency',
    label: 'CPU 核心数',
    group: '屏幕与硬件',
    notes: {
      supported: '可直接生效',
      partial: '可能仅作为池内静态特征',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'deviceMemory',
    label: '设备内存',
    group: '屏幕与硬件',
    notes: {
      supported: '可直接生效',
      partial: '可能仅作为池内静态特征',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'touchPoints',
    label: '触摸点数',
    group: '屏幕与硬件',
    notes: {
      supported: '可直接生效',
      partial: '可能只影响移动特征的静态模板',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'canvasNoise',
    label: 'Canvas 噪声',
    group: '渲染指纹',
    notes: {
      supported: '可直接生效',
      partial: '可能由池内运行时噪声接管',
      unsupported: '当前内核不直接注入该噪声',
    },
  },
  {
    key: 'webglVendor',
    label: 'WebGL 供应商',
    group: '渲染指纹',
    notes: {
      supported: '可直接生效',
      partial: '可能仅影响预设池的选型',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'webglRenderer',
    label: 'WebGL 渲染器',
    group: '渲染指纹',
    notes: {
      supported: '可直接生效',
      partial: '可能仅影响预设池的选型',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'audioNoise',
    label: 'Audio 噪声',
    group: '渲染指纹',
    notes: {
      supported: '可直接生效',
      partial: '可能由运行时池接管',
      unsupported: '当前内核不直接注入该噪声',
    },
  },
  {
    key: 'webrtcPolicy',
    label: 'WebRTC 策略',
    group: '网络与隐私',
    notes: {
      supported: '可直接生效',
      partial: '可能仅能降级生效',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'doNotTrack',
    label: 'Do Not Track',
    group: '网络与隐私',
    notes: {
      supported: '可直接生效',
      partial: '可能只被部分运行时采纳',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'mediaDevices',
    label: '媒体设备',
    group: '网络与隐私',
    notes: {
      supported: '可直接生效',
      partial: '可能仅影响部分设备列表',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
  {
    key: 'fonts',
    label: '字体列表',
    group: '字体',
    notes: {
      supported: '可直接生效',
      partial: '可能仅影响运行时字体池',
      unsupported: '当前内核不支持显式覆盖',
    },
  },
]

const CHROMIUM_STATUS_MAP: Record<FingerprintFieldKey, FingerprintCapabilityStatus> = {
  seed: 'supported',
  brand: 'supported',
  platform: 'supported',
  lang: 'supported',
  timezone: 'supported',
  resolution: 'supported',
  colorDepth: 'supported',
  hardwareConcurrency: 'supported',
  deviceMemory: 'supported',
  canvasNoise: 'supported',
  webglVendor: 'supported',
  webglRenderer: 'supported',
  audioNoise: 'supported',
  fonts: 'supported',
  webrtcPolicy: 'supported',
  doNotTrack: 'supported',
  mediaDevices: 'supported',
  touchPoints: 'supported',
}

const CAMOUFOX_STATUS_MAP: Record<FingerprintFieldKey, FingerprintCapabilityStatus> = {
  seed: 'partial',
  brand: 'unsupported',
  platform: 'partial',
  lang: 'supported',
  timezone: 'partial',
  resolution: 'supported',
  colorDepth: 'partial',
  hardwareConcurrency: 'partial',
  deviceMemory: 'partial',
  canvasNoise: 'partial',
  webglVendor: 'partial',
  webglRenderer: 'partial',
  audioNoise: 'partial',
  fonts: 'partial',
  webrtcPolicy: 'partial',
  doNotTrack: 'partial',
  mediaDevices: 'partial',
  touchPoints: 'partial',
}

const DEFAULT_STATUS_MAP: Record<FingerprintFieldKey, FingerprintCapabilityStatus> = {
  seed: 'partial',
  brand: 'partial',
  platform: 'partial',
  lang: 'partial',
  timezone: 'partial',
  resolution: 'partial',
  colorDepth: 'partial',
  hardwareConcurrency: 'partial',
  deviceMemory: 'partial',
  canvasNoise: 'partial',
  webglVendor: 'partial',
  webglRenderer: 'partial',
  audioNoise: 'partial',
  fonts: 'partial',
  webrtcPolicy: 'partial',
  doNotTrack: 'partial',
  mediaDevices: 'partial',
  touchPoints: 'partial',
}

const GEO_RULES: Array<{
  countries: string[]
  lang: string
  timezone: string
}> = [
  { countries: ['CN', 'HK', 'MO', 'TW', 'SG'], lang: 'zh-CN', timezone: 'Asia/Shanghai' },
  { countries: ['JP'], lang: 'ja-JP', timezone: 'Asia/Tokyo' },
  { countries: ['KR'], lang: 'ko-KR', timezone: 'Asia/Seoul' },
  { countries: ['US'], lang: 'en-US', timezone: 'America/New_York' },
  { countries: ['GB', 'UK'], lang: 'en-GB', timezone: 'Europe/London' },
  { countries: ['FR'], lang: 'fr-FR', timezone: 'Europe/Paris' },
  { countries: ['DE'], lang: 'de-DE', timezone: 'Europe/Berlin' },
  { countries: ['AU'], lang: 'en-US', timezone: 'Australia/Sydney' },
  { countries: ['CA'], lang: 'en-US', timezone: 'America/Toronto' },
]

function normalizeCoreRuntime(coreType?: string): BrowserCoreRuntime {
  const value = String(coreType || '').trim().toLowerCase()
  if (value.includes('camoufox')) return 'camoufox'
  if (value.includes('chromium') || value.includes('chrome')) return 'chromium'
  return 'unknown'
}

function getCapabilityStatus(coreType?: string, key?: FingerprintFieldKey): FingerprintCapabilityStatus {
  if (!key) return 'partial'
  const runtime = normalizeCoreRuntime(coreType)
  if (runtime === 'chromium') return CHROMIUM_STATUS_MAP[key]
  if (runtime === 'camoufox') return CAMOUFOX_STATUS_MAP[key]
  return DEFAULT_STATUS_MAP[key]
}

function asTrimmedString(value: unknown): string {
  if (value === undefined || value === null) return ''
  return String(value).trim()
}

function formatFingerprintValue(config: Partial<FingerprintConfig> | null | undefined, key: FingerprintFieldKey): string {
  if (!config) return ''
  if (key === 'resolution') {
    if (config.resolution === 'custom') {
      return asTrimmedString(config.customResolution)
    }
    return asTrimmedString(config.resolution)
  }
  const value = config[key]
  if (value === undefined || value === null || value === '') return ''
  if (typeof value === 'boolean') return value ? '启用' : '禁用'
  return String(value)
}

function parseGeoFromProxy(proxy?: BrowserProxy | null): GeoProfile {
  if (!proxy?.lastIPHealthJson) return {}
  try {
    const parsed = JSON.parse(proxy.lastIPHealthJson) as Partial<GeoProfile> & Record<string, unknown>
    return {
      country: asTrimmedString(parsed.country),
      region: asTrimmedString(parsed.region),
      city: asTrimmedString(parsed.city),
      isResidential: typeof parsed.isResidential === 'boolean' ? parsed.isResidential : undefined,
      fraudScore: typeof parsed.fraudScore === 'number' ? parsed.fraudScore : Number(parsed.fraudScore || 0) || 0,
      asOrganization: asTrimmedString(parsed.asOrganization),
    }
  } catch {
    return {}
  }
}

function findLocaleRule(country?: string): { lang: string; timezone: string } | null {
  const code = String(country || '').trim().toUpperCase()
  if (!code) return null
  for (const rule of GEO_RULES) {
    if (rule.countries.includes(code)) {
      return { lang: rule.lang, timezone: rule.timezone }
    }
  }
  return null
}

function choosePresetId(geo: GeoProfile): string {
  const country = String(geo.country || '').trim().toUpperCase()
  if (country === 'US') {
    return geo.isResidential ? 'win-chrome-us-user' : 'win-chrome-gaming'
  }
  if (country === 'GB' || country === 'UK') {
    return 'win-chrome-uk-office'
  }
  return 'win-chrome-office'
}

function buildRecommendationConfig(proxy?: BrowserProxy | null): Partial<FingerprintConfig> {
  const geo = parseGeoFromProxy(proxy)
  const locale = findLocaleRule(geo.country)
  const presetId = choosePresetId(geo)
  const preset = FINGERPRINT_PRESETS.find(item => item.id === presetId) ?? FINGERPRINT_PRESETS[0]
  const next: Partial<FingerprintConfig> = { ...preset.config }

  if (locale?.lang) {
    next.lang = locale.lang
  }
  if (locale?.timezone) {
    next.timezone = locale.timezone
  }

  // 保持默认更偏“真实桌面环境”的常见组合。
  if (next.webrtcPolicy === undefined) {
    next.webrtcPolicy = 'disable_non_proxied_udp'
  }
  if (next.doNotTrack === undefined) {
    next.doNotTrack = false
  }
  if (next.touchPoints === undefined) {
    next.touchPoints = '0'
  }

  // 当代理本身带有明显的地区信号时，尽量同步语言/时区；否则保留预设。
  if (!locale && geo.country === 'US') {
    next.lang = 'en-US'
    next.timezone = 'America/New_York'
  }

  return next
}

function recommendationToConfig(recommendation?: BrowserProxyFingerprintRecommendation | null): Partial<FingerprintConfig> {
  if (!recommendation) return {}
  if (recommendation.config && Object.keys(recommendation.config).length > 0) {
    return recommendation.config
  }
  if (recommendation.fingerprintArgs && recommendation.fingerprintArgs.length > 0) {
    return deserialize(recommendation.fingerprintArgs)
  }
  return {}
}

export function summarizeFingerprintCapabilities(coreType?: string): Record<FingerprintCapabilityStatus, number> {
  const summary: Record<FingerprintCapabilityStatus, number> = { supported: 0, partial: 0, unsupported: 0 }
  for (const item of CAPABILITY_DEFINITIONS) {
    summary[getCapabilityStatus(coreType, item.key)] += 1
  }
  return summary
}

export function buildFingerprintCapabilityMatrix(
  coreType?: string,
  currentConfig?: Partial<FingerprintConfig> | null,
  recommendation?: BrowserProxyFingerprintRecommendation | null,
): FingerprintCapabilityItem[] {
  const recommendedConfig = recommendationToConfig(recommendation)

  return CAPABILITY_DEFINITIONS.map((item) => ({
    key: item.key,
    label: item.label,
    group: item.group,
    status: getCapabilityStatus(coreType, item.key),
    currentValue: formatFingerprintValue(currentConfig, item.key) || '未设置',
    recommendedValue: formatFingerprintValue(recommendedConfig, item.key) || undefined,
    note: item.notes[getCapabilityStatus(coreType, item.key)],
  }))
}

export function buildFingerprintRecommendation(
  request: BrowserProxyFingerprintRecommendationRequest,
): BrowserProxyFingerprintRecommendation {
  const proxy = request.proxy ?? null
  const config = buildRecommendationConfig(proxy)
  const fingerprintArgs = serialize(config)
  return {
    proxyId: request.proxyId,
    proxyConfig: request.proxyConfig,
    proxyName: request.proxyName,
    coreType: request.coreType,
    source: 'heuristic',
    note: proxy
      ? '当前使用前端本地启发式推荐；后端接口未接入时会根据代理地域与历史 IP 健康信息生成建议。'
      : '当前使用前端本地启发式推荐；未提供代理明细时采用通用桌面模板。',
    config,
    fingerprintArgs,
    capabilities: Object.fromEntries(
      CAPABILITY_DEFINITIONS.map(item => [item.key, getCapabilityStatus(request.coreType, item.key)]),
    ) as Partial<Record<FingerprintFieldKey, FingerprintCapabilityStatus>>,
    capabilityNotes: Object.fromEntries(
      CAPABILITY_DEFINITIONS.map(item => [item.key, item.notes[getCapabilityStatus(request.coreType, item.key)]]),
    ) as Partial<Record<FingerprintFieldKey, string>>,
  }
}

export function mergeFingerprintRecommendation(
  currentArgs: string[],
  recommendation?: BrowserProxyFingerprintRecommendation | null,
): string[] {
  if (!recommendation) return currentArgs
  const currentConfig = deserialize(currentArgs || [])
  const recommendedConfig = recommendationToConfig(recommendation)
  const merged: FingerprintConfig = {
    ...currentConfig,
    ...recommendedConfig,
    unknownArgs: currentConfig.unknownArgs,
  }
  return serialize(merged)
}

export function formatFingerprintRecommendationProxyLabel(recommendation?: BrowserProxyFingerprintRecommendation | null): string {
  if (!recommendation) return '未推荐'
  const name = asTrimmedString(recommendation.proxyName) || asTrimmedString(recommendation.proxyId) || '当前代理'
  return name
}

export function getFingerprintRecommendationSummary(recommendation?: BrowserProxyFingerprintRecommendation | null): string {
  if (!recommendation) return ''
  const parts: string[] = []
  if (recommendation.note) parts.push(recommendation.note)
  if (recommendation.source === 'backend') {
    parts.push('已接入后端推荐')
  } else {
    parts.push('当前为本地启发式结果')
  }
  return parts.join('；')
}
