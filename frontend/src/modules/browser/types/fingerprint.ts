import type { BrowserProxy } from '../types'
import type { FingerprintConfig } from '../utils/fingerprintSerializer'

export type FingerprintCapabilityStatus = 'supported' | 'partial' | 'unsupported'

export type BrowserCoreRuntime = 'chromium' | 'camoufox' | 'unknown'

export type FingerprintFieldKey =
  | 'seed'
  | 'brand'
  | 'platform'
  | 'lang'
  | 'timezone'
  | 'resolution'
  | 'colorDepth'
  | 'hardwareConcurrency'
  | 'deviceMemory'
  | 'canvasNoise'
  | 'webglVendor'
  | 'webglRenderer'
  | 'audioNoise'
  | 'fonts'
  | 'webrtcPolicy'
  | 'doNotTrack'
  | 'mediaDevices'
  | 'touchPoints'

export interface BrowserProxyFingerprintRecommendationRequest {
  proxyId: string
  proxyConfig?: string
  proxyName?: string
  coreType?: string
  proxy?: BrowserProxy | null
}

export interface BrowserProxyFingerprintRecommendation {
  proxyId: string
  proxyConfig?: string
  proxyName?: string
  coreType?: string
  source: 'backend' | 'heuristic'
  updatedAt?: string
  note?: string
  config?: Partial<FingerprintConfig>
  fingerprintArgs?: string[]
  capabilities?: Partial<Record<FingerprintFieldKey, FingerprintCapabilityStatus>>
  capabilityNotes?: Partial<Record<FingerprintFieldKey, string>>
}

export interface FingerprintCapabilityItem {
  key: FingerprintFieldKey
  label: string
  group: string
  status: FingerprintCapabilityStatus
  currentValue: string
  recommendedValue?: string
  note?: string
}
