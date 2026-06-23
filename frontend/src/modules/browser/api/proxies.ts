import type { BrowserProxy, ProxyIPHealthResult } from '../types'
import type {
  BrowserProxyFingerprintRecommendation,
  BrowserProxyFingerprintRecommendationRequest,
} from '../types/fingerprint'
import { buildFingerprintRecommendation } from '../utils/fingerprintAdvisor'
import { serialize, type FingerprintConfig } from '../utils/fingerprintSerializer'
import { getBindings, getGoApp, getMockProxies, nowISOString, setMockProxies } from './runtime'

export interface ClashImportURLResult {
  url: string
  content: string
  proxyCount: number
  dnsServers?: string
  suggestedGroup?: string
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export async function fetchBrowserProxies(): Promise<BrowserProxy[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyList) {
    return (await bindings.BrowserProxyList()) || []
  }
  return getMockProxies()
}

export async function fetchBrowserProxyGroups(): Promise<string[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyListGroups) {
    return (await bindings.BrowserProxyListGroups()) || []
  }
  return []
}

export async function fetchBrowserProxiesByGroup(groupName: string): Promise<BrowserProxy[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyListByGroup) {
    return (await bindings.BrowserProxyListByGroup(groupName)) || []
  }
  return getMockProxies().filter((proxy) => proxy.groupName === groupName)
}

export async function fetchClashImportFromURL(targetURL: string): Promise<ClashImportURLResult> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyFetchClashByURL) {
    return (
      (await bindings.BrowserProxyFetchClashByURL(targetURL)) || {
        url: targetURL,
        content: '',
        proxyCount: 0,
      }
    )
  }

  const goApp = getGoApp()
  if (goApp?.BrowserProxyFetchClashByURL) {
    return (
      (await goApp.BrowserProxyFetchClashByURL(targetURL)) || {
        url: targetURL,
        content: '',
        proxyCount: 0,
      }
    )
  }

  throw new Error('当前环境不支持 URL 导入 Clash 配置')
}

export async function saveBrowserProxies(proxies: BrowserProxy[]): Promise<boolean> {
  const bindings: any = await getBindings()
  if (bindings?.SaveBrowserProxies) {
    await bindings.SaveBrowserProxies(proxies)
    return true
  }
  setMockProxies(proxies)
  return true
}

export async function validateProxyConfig(proxyConfig: string, proxyId: string): Promise<{ supported: boolean; errorMsg: string }> {
  const bindings: any = await getBindings()
  if (bindings?.ValidateProxyConfig) {
    return (await bindings.ValidateProxyConfig(proxyConfig, proxyId)) || { supported: true, errorMsg: '' }
  }
  return { supported: true, errorMsg: '' }
}

export async function testProxyConnectivity(proxyId: string, proxyConfig: string): Promise<{ proxyId: string; ok: boolean; latencyMs: number; error: string }> {
  const bindings: any = await getBindings()
  if (bindings?.TestProxyConnectivity) {
    return (await bindings.TestProxyConnectivity(proxyId, proxyConfig)) || { proxyId, ok: false, latencyMs: 0, error: '调用失败' }
  }
  await sleep(300 + Math.random() * 500)
  return { proxyId, ok: true, latencyMs: Math.floor(100 + Math.random() * 200), error: '' }
}

export async function testProxyRealConnectivity(proxyId: string): Promise<{ proxyId: string; ok: boolean; latencyMs: number; error: string }> {
  const bindings: any = await getBindings()
  if (bindings?.TestProxyRealConnectivity) {
    return (await bindings.TestProxyRealConnectivity(proxyId)) || { proxyId, ok: false, latencyMs: 0, error: '调用失败' }
  }
  await sleep(300 + Math.random() * 500)
  return { proxyId, ok: true, latencyMs: Math.floor(100 + Math.random() * 400), error: '' }
}

export async function browserProxyTestSpeed(proxyId: string): Promise<{ proxyId: string; ok: boolean; latencyMs: number; error: string }> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyTestSpeed) {
    return (await bindings.BrowserProxyTestSpeed(proxyId)) || { proxyId, ok: false, latencyMs: 0, error: '调用失败' }
  }
  await sleep(300 + Math.random() * 500)
  return { proxyId, ok: true, latencyMs: Math.floor(100 + Math.random() * 400), error: '' }
}

export async function browserProxyBatchTestSpeed(proxyIds: string[], concurrency: number = 20): Promise<{ proxyId: string; ok: boolean; latencyMs: number; error: string }[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyBatchTestSpeed) {
    return (await bindings.BrowserProxyBatchTestSpeed(proxyIds, concurrency)) || []
  }
  await sleep(1000)
  return proxyIds.map((proxyId) => ({ proxyId, ok: true, latencyMs: Math.floor(100 + Math.random() * 400), error: '' }))
}

export async function browserProxyCheckIPHealth(proxyId: string): Promise<ProxyIPHealthResult> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyCheckIPHealth) {
    return (
      (await bindings.BrowserProxyCheckIPHealth(proxyId)) || {
        proxyId,
        ok: false,
        source: 'ip_health',
        error: '调用失败',
        ip: '',
        fraudScore: 0,
        isResidential: false,
        isBroadcast: false,
        country: '',
        region: '',
        city: '',
        asOrganization: '',
        rawData: {},
        updatedAt: nowISOString(),
      }
    )
  }

  await sleep(600)
  return {
    proxyId,
    ok: true,
    source: 'ip_health',
    error: '',
    ip: '127.0.0.1',
    fraudScore: Math.floor(Math.random() * 100),
    isResidential: Math.random() > 0.5,
    isBroadcast: false,
    country: 'Mock',
    region: 'Mock',
    city: 'Mock',
    asOrganization: 'Mock ISP',
    rawData: {},
    updatedAt: nowISOString(),
  }
}

export async function browserProxyBatchCheckIPHealth(proxyIds: string[], concurrency: number = 10): Promise<ProxyIPHealthResult[]> {
  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyBatchCheckIPHealth) {
    return (await bindings.BrowserProxyBatchCheckIPHealth(proxyIds, concurrency)) || []
  }

  await sleep(1200)
  return proxyIds.map((proxyId) => ({
    proxyId,
    ok: true,
    source: 'ip_health',
    error: '',
    ip: '127.0.0.1',
    fraudScore: Math.floor(Math.random() * 100),
    isResidential: Math.random() > 0.5,
    isBroadcast: false,
    country: 'Mock',
    region: 'Mock',
    city: 'Mock',
    asOrganization: 'Mock ISP',
    rawData: {},
    updatedAt: nowISOString(),
  }))
}

function normalizeBrowserProxyFingerprintRecommendation(
  raw: unknown,
  fallback: BrowserProxyFingerprintRecommendationRequest,
): BrowserProxyFingerprintRecommendation {
  if (!raw || typeof raw !== 'object') {
    return buildFingerprintRecommendation(fallback)
  }

  const payload = raw as Record<string, any>
  const backendConfig: Partial<FingerprintConfig> = {}
  if (typeof payload.brand === 'string') backendConfig.brand = payload.brand
  if (typeof payload.platform === 'string') backendConfig.platform = payload.platform
  if (typeof payload.language === 'string') backendConfig.lang = payload.language
  if (typeof payload.lang === 'string') backendConfig.lang = payload.lang
  if (typeof payload.timezone === 'string') backendConfig.timezone = payload.timezone

  const recommendation = buildFingerprintRecommendation({
    ...fallback,
    proxyName: typeof payload.proxyName === 'string' ? payload.proxyName : fallback.proxyName,
    proxyConfig: typeof payload.proxyConfig === 'string' ? payload.proxyConfig : fallback.proxyConfig,
    coreType: typeof payload.coreType === 'string' ? payload.coreType : fallback.coreType,
  })

  recommendation.source = payload.source === 'backend' || Object.keys(backendConfig).length > 0 ? 'backend' : recommendation.source
  if (typeof payload.updatedAt === 'string') recommendation.updatedAt = payload.updatedAt
  if (typeof payload.note === 'string') {
    recommendation.note = payload.note
  } else if (recommendation.source === 'backend') {
    recommendation.note = typeof payload.riskLevel === 'string'
      ? `后端已根据代理健康数据生成建议，风险等级：${payload.riskLevel}`
      : '后端已根据代理健康数据生成建议'
  }

  if (payload.config && typeof payload.config === 'object') {
    recommendation.config = payload.config
  } else if (Object.keys(backendConfig).length > 0) {
    recommendation.config = backendConfig
  }
  if (Array.isArray(payload.fingerprintArgs)) {
    recommendation.fingerprintArgs = payload.fingerprintArgs.filter((item): item is string => typeof item === 'string')
  } else if (Object.keys(backendConfig).length > 0) {
    recommendation.fingerprintArgs = serialize(recommendation.config || backendConfig)
  }
  if (payload.capabilities && typeof payload.capabilities === 'object') {
    recommendation.capabilities = payload.capabilities
  }
  if (payload.capabilityNotes && typeof payload.capabilityNotes === 'object') {
    recommendation.capabilityNotes = payload.capabilityNotes
  }

  return recommendation
}

export async function recommendFingerprintByProxy(
  request: BrowserProxyFingerprintRecommendationRequest,
): Promise<BrowserProxyFingerprintRecommendation> {
  const normalizedRequest = {
    proxyId: request.proxyId.trim(),
    proxyConfig: request.proxyConfig?.trim() || '',
    proxyName: request.proxyName?.trim() || '',
    coreType: request.coreType?.trim() || '',
    proxy: request.proxy ?? null,
  } satisfies BrowserProxyFingerprintRecommendationRequest

  const bindings: any = await getBindings()
  if (bindings?.BrowserProxyRecommendFingerprint) {
    const result = await bindings.BrowserProxyRecommendFingerprint(normalizedRequest)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }
  if (bindings?.BrowserProxySuggestFingerprint) {
    const result = await bindings.BrowserProxySuggestFingerprint(normalizedRequest.proxyId)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }
  if (bindings?.SuggestFingerprintByProxy) {
    const result = await bindings.SuggestFingerprintByProxy(normalizedRequest.proxyId)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }

  const goApp = getGoApp()
  if (goApp?.BrowserProxyRecommendFingerprint) {
    const result = await goApp.BrowserProxyRecommendFingerprint(normalizedRequest)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }
  if (goApp?.BrowserProxySuggestFingerprint) {
    const result = await goApp.BrowserProxySuggestFingerprint(normalizedRequest.proxyId)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }
  if (goApp?.SuggestFingerprintByProxy) {
    const result = await goApp.SuggestFingerprintByProxy(normalizedRequest.proxyId)
    return normalizeBrowserProxyFingerprintRecommendation(result, normalizedRequest)
  }

  return buildFingerprintRecommendation(normalizedRequest)
}

export async function suggestFingerprintByProxy(proxyId: string): Promise<any> {
  const bindings: any = await getBindings()
  if (bindings?.SuggestFingerprintByProxy) {
    return (await bindings.SuggestFingerprintByProxy(proxyId)) || {}
  }
  if (bindings?.BrowserProxySuggestFingerprint) {
    return (await bindings.BrowserProxySuggestFingerprint(proxyId)) || {}
  }
  const goApp = getGoApp()
  if (goApp?.SuggestFingerprintByProxy) {
    return (await goApp.SuggestFingerprintByProxy(proxyId)) || {}
  }
  if (goApp?.BrowserProxySuggestFingerprint) {
    return (await goApp.BrowserProxySuggestFingerprint(proxyId)) || {}
  }
  return {
    proxyId,
    riskLevel: 'unknown',
    timezone: 'Asia/Shanghai',
    language: 'zh-CN',
    brand: 'Chrome',
    platform: 'windows',
    warnings: [{ id: 'fallback', message: '当前环境尚未接入代理指纹推荐 API', severity: 'info', explicit: false }],
    explicitAlerts: [],
  }
}
