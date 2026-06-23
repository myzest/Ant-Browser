import type { BrowserProfile } from '../types'
import { getBindings } from '../api/runtime'

export type BrowserStartMode = 'normal' | 'direct'
export type BrowserStartRiskLevel = 'low' | 'medium' | 'high' | 'critical' | 'unknown'
export type BrowserStartWarningSeverity = 'info' | 'warning' | 'error'

export interface BrowserStartPreflightEntry {
  id: string
  message: string
  severity: BrowserStartWarningSeverity
  detail?: string
  explicit?: boolean
}

export interface BrowserStartPreflightResult {
  profileId: string
  profileName: string
  mode: BrowserStartMode
  source: 'backend' | 'frontend-fallback'
  riskLevel: BrowserStartRiskLevel
  summary: string
  warnings: BrowserStartPreflightEntry[]
  explicitAlerts: BrowserStartPreflightEntry[]
  requireConfirm: boolean
  raw?: unknown
}

const RISK_ORDER: BrowserStartRiskLevel[] = ['low', 'medium', 'high', 'critical', 'unknown']

function asTrimmedString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeRiskLevel(value: unknown): BrowserStartRiskLevel {
  const normalized = asTrimmedString(value).toLowerCase()
  if (normalized === 'low' || normalized === 'medium' || normalized === 'high' || normalized === 'critical') {
    return normalized
  }
  return 'unknown'
}

function normalizeSeverity(value: unknown, fallback: BrowserStartWarningSeverity = 'info'): BrowserStartWarningSeverity {
  const normalized = asTrimmedString(value).toLowerCase()
  if (normalized === 'error' || normalized === 'danger' || normalized === 'critical') return 'error'
  if (normalized === 'warning' || normalized === 'warn') return 'warning'
  if (normalized === 'info' || normalized === 'information') return 'info'
  return fallback
}

function pickMessage(item: any): string {
  return asTrimmedString(
    item?.message ??
    item?.text ??
    item?.title ??
    item?.detail ??
    item?.reason ??
    item?.desc ??
    item
  )
}

function normalizeEntry(item: unknown, index: number, fallbackSeverity: BrowserStartWarningSeverity): BrowserStartPreflightEntry | null {
  if (typeof item === 'string') {
    const message = item.trim()
    if (!message) return null
    return {
      id: `entry-${index}`,
      message,
      severity: fallbackSeverity,
    }
  }

  if (!item || typeof item !== 'object') {
    return null
  }

  const message = pickMessage(item)
  if (!message) {
    return null
  }

  const raw = item as any
  return {
    id: asTrimmedString(raw.id ?? raw.code ?? raw.key ?? `entry-${index}`) || `entry-${index}`,
    message,
    severity: normalizeSeverity(raw.severity ?? raw.level ?? raw.type ?? raw.kind, fallbackSeverity),
    detail: asTrimmedString(raw.detail ?? raw.hint ?? raw.advice ?? raw.suggestion) || undefined,
    explicit: Boolean(raw.explicit ?? raw.mustConfirm ?? raw.requireConfirm ?? raw.confirmRequired),
  }
}

function normalizeEntryList(value: unknown, fallbackSeverity: BrowserStartWarningSeverity): BrowserStartPreflightEntry[] {
  if (!Array.isArray(value)) {
    return []
  }

  const entries: BrowserStartPreflightEntry[] = []
  value.forEach((item, index) => {
    const entry = normalizeEntry(item, index, fallbackSeverity)
    if (entry) {
      entries.push(entry)
    }
  })
  return entries
}

function readRawList(raw: any, keys: string[]): unknown {
  for (const key of keys) {
    if (Array.isArray(raw?.[key])) {
      return raw[key]
    }
  }
  return []
}

function riskLabel(riskLevel: BrowserStartRiskLevel): string {
  switch (riskLevel) {
    case 'low':
      return '低风险'
    case 'medium':
      return '中风险'
    case 'high':
      return '高风险'
    case 'critical':
      return '严重风险'
    default:
      return '未知风险'
  }
}

function buildSummary(riskLevel: BrowserStartRiskLevel, warnings: BrowserStartPreflightEntry[], explicitAlerts: BrowserStartPreflightEntry[]): string {
  if (riskLevel === 'low' && warnings.length === 0 && explicitAlerts.length === 0) {
    return '未发现明显风险，可以继续启动。'
  }
  if (riskLevel === 'medium') {
    return '存在需要关注的中等风险，请先阅读下方提示。'
  }
  if (riskLevel === 'high') {
    return '检测到高风险项，继续启动前需要显式确认。'
  }
  if (riskLevel === 'critical') {
    return '检测到严重风险，建议先处理告警后再启动。'
  }
  return `检测到 ${riskLabel(riskLevel)} 项，请仔细确认后再继续。`
}

function buildFallbackResult(profile: BrowserProfile, mode: BrowserStartMode): BrowserStartPreflightResult {
  return {
    profileId: profile.profileId,
    profileName: profile.profileName,
    mode,
    source: 'frontend-fallback',
    riskLevel: 'low',
    summary: '当前运行环境尚未接入后端 preflight 接口；将继续按现有启动逻辑执行。',
    warnings: [],
    explicitAlerts: [],
    requireConfirm: false,
  }
}

function deriveRequireConfirm(riskLevel: BrowserStartRiskLevel, explicitAlerts: BrowserStartPreflightEntry[], raw: any): boolean {
  if (Boolean(raw?.requireConfirm ?? raw?.requiresConfirmation ?? raw?.mustConfirm ?? raw?.confirmRequired)) {
    return true
  }
  if (riskLevel === 'high' || riskLevel === 'critical') {
    return true
  }
  return explicitAlerts.some(item => item.explicit || item.severity === 'error')
}

function readPreflightPayload(raw: any): any {
  if (!raw || typeof raw !== 'object') {
    return raw
  }
  return raw.preflight ?? raw.startPreflight ?? raw.launchPreflight ?? raw.data ?? raw
}

export function normalizeBrowserStartPreflight(profile: BrowserProfile, mode: BrowserStartMode, raw: unknown): BrowserStartPreflightResult {
  const payload = readPreflightPayload(raw) as any
  if (!payload || typeof payload !== 'object') {
    return buildFallbackResult(profile, mode)
  }

  const riskLevel = normalizeRiskLevel(payload.riskLevel ?? payload.risk ?? payload.level)
  const generalWarnings = normalizeEntryList(
    readRawList(payload, ['warnings', 'warningList', 'issues', 'alerts', 'notes']),
    'warning',
  )
  const explicitAlertsFromPayload = normalizeEntryList(
    readRawList(payload, ['explicitAlerts', 'explicitWarnings', 'criticalAlerts', 'mustConfirmWarnings', 'confirmAlerts']),
    'error',
  )

  const explicitAlerts: BrowserStartPreflightEntry[] = explicitAlertsFromPayload.length > 0
    ? explicitAlertsFromPayload
    : (riskLevel === 'high' || riskLevel === 'critical')
      ? [{
          id: 'risk-confirmation',
          message: asTrimmedString(payload.explicitAlert ?? payload.explicitWarning ?? payload.confirmMessage) || `当前风险等级为 ${riskLabel(riskLevel)}，请确认后继续启动。`,
          severity: 'error',
          explicit: true,
        }]
      : []

  const warnings = generalWarnings.filter(item => !explicitAlerts.some(alert => alert.id === item.id && alert.message === item.message))
  const requireConfirm = deriveRequireConfirm(riskLevel, explicitAlerts, payload)
  const summary = asTrimmedString(payload.summary ?? payload.message ?? payload.notice) || buildSummary(riskLevel, warnings, explicitAlerts)

  return {
    profileId: profile.profileId,
    profileName: profile.profileName,
    mode,
    source: 'backend',
    riskLevel,
    summary,
    warnings,
    explicitAlerts,
    requireConfirm,
    raw,
  }
}

export async function fetchBrowserStartPreflight(profile: BrowserProfile, mode: BrowserStartMode): Promise<BrowserStartPreflightResult> {
  const bindings: any = await getBindings()
  const candidates: Array<[string, (...args: any[]) => Promise<unknown> | unknown]> = [
    ['BrowserInstanceStartPreflight', bindings?.BrowserInstanceStartPreflight],
    ['BrowserInstancePreflight', bindings?.BrowserInstancePreflight],
    ['BrowserStartPreflight', bindings?.BrowserStartPreflight],
  ]

  for (const [, fn] of candidates) {
    if (typeof fn === 'function') {
      try {
        const raw = await fn(profile.profileId, mode)
        return normalizeBrowserStartPreflight(profile, mode, raw)
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error ?? '')
        return {
          ...buildFallbackResult(profile, mode),
          warnings: message ? [{
            id: 'preflight-error',
            message: `后端 preflight 调用失败：${message}`,
            severity: 'warning',
          }] : [],
        }
      }
    }
  }

  return buildFallbackResult(profile, mode)
}

export function browserStartRiskBadgeVariant(riskLevel: BrowserStartRiskLevel): 'default' | 'success' | 'warning' | 'error' | 'info' {
  switch (riskLevel) {
    case 'low':
      return 'success'
    case 'medium':
      return 'warning'
    case 'high':
    case 'critical':
      return 'error'
    default:
      return 'info'
  }
}

export function browserStartRiskLabel(riskLevel: BrowserStartRiskLevel): string {
  switch (riskLevel) {
    case 'low':
      return '低风险'
    case 'medium':
      return '中风险'
    case 'high':
      return '高风险'
    case 'critical':
      return '严重风险'
    default:
      return '未知风险'
  }
}

export function browserStartModeLabel(mode: BrowserStartMode): string {
  return mode === 'direct' ? '直连启动' : '标准启动'
}

export function browserStartRequireExplicitConfirm(result: BrowserStartPreflightResult | null | undefined): boolean {
  return Boolean(result?.requireConfirm)
}

export function browserStartModeOrder(mode: BrowserStartMode): number {
  return mode === 'direct' ? 1 : 0
}

export function browserStartRiskOrder(riskLevel: BrowserStartRiskLevel): number {
  const index = RISK_ORDER.indexOf(riskLevel)
  return index === -1 ? RISK_ORDER.length : index
}
