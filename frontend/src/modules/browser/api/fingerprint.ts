import type { FingerprintGenerateRequest, FingerprintGenerateResult, FingerprintHealthReport, FingerprintValidateRequest } from '../types'
import { deserialize, serialize } from '../utils/fingerprintSerializer'
import { getBindings, getGoApp } from './runtime'

export async function generateFingerprintProfile(request: FingerprintGenerateRequest): Promise<FingerprintGenerateResult> {
  const goApp = getGoApp()
  if (goApp?.GenerateFingerprintProfile) {
    return await goApp.GenerateFingerprintProfile(request)
  }
  const bindings: any = await getBindings()
  if (bindings?.GenerateFingerprintProfile && getGoApp()) {
    return await bindings.GenerateFingerprintProfile(request)
  }
  const platform = request.platform || 'windows'
  const fixture = localFixture(platform, request.country, request.locale, request.timezone, request.deviceClass)
  const args = serialize({
    ...deserialize(request.currentArgs || []),
    ...fixture,
  })
  const health = validateArgsLocally(args)
  return {
    args,
    warnings: health.issues,
    health,
    summary: {
      profileId: request.profileId || '',
      platform,
      brand: fixture.brand || 'Chrome',
      locale: fixture.lang || '',
      timezone: fixture.timezone || '',
      resolution: fixture.resolution || '',
      webglVendor: fixture.webglVendor || '',
      webglRenderer: fixture.webglRenderer || '',
      hardwareConcurrency: Number(fixture.hardwareConcurrency || 0),
      deviceMemory: Number(fixture.deviceMemory || 0),
      touchPoints: Number(fixture.touchPoints || 0),
    },
  }
}

export async function validateFingerprintProfile(request: FingerprintValidateRequest): Promise<FingerprintHealthReport> {
  const goApp = getGoApp()
  if (goApp?.ValidateFingerprintProfile) {
    return await goApp.ValidateFingerprintProfile(request)
  }
  const bindings: any = await getBindings()
  if (bindings?.ValidateFingerprintProfile && getGoApp()) {
    return await bindings.ValidateFingerprintProfile(request)
  }
  return validateArgsLocally(request.args || [])
}

function validateArgsLocally(args: string[]): FingerprintHealthReport {
  const parsed = deserialize(args)
  const issues = []
  if (parsed.brand === 'Firefox' || parsed.brand === 'Safari') {
    issues.push({ code: 'non_chromium_brand', severity: 'red', field: '--fingerprint-brand', message: 'Chromium 指纹不应使用 Firefox/Safari 品牌' })
  }
  if (parsed.platform === 'windows' && parsed.fonts?.toLowerCase().includes('pingfang')) {
    issues.push({ code: 'windows_mac_fonts', severity: 'red', field: '--fingerprint-fonts', message: 'Windows profile 包含 macOS marker fonts' })
  }
  if (parsed.platform === 'mac' && parsed.webglVendor && parsed.webglVendor !== 'Apple') {
    issues.push({ code: 'mac_webgl_vendor', severity: 'red', field: '--fingerprint-webgl-vendor', message: 'macOS profile 建议使用 Apple WebGL vendor' })
  }
  if (!parsed.fonts) {
    issues.push({ code: 'missing_fonts', severity: 'yellow', field: '--fingerprint-fonts', message: '缺少字体列表' })
  }
  if (!parsed.webglVendor || !parsed.webglRenderer) {
    issues.push({ code: 'missing_webgl', severity: 'yellow', field: '--fingerprint-webgl-renderer', message: '缺少 WebGL vendor/renderer' })
  }
  return {
    status: issues.some(issue => issue.severity === 'red') ? 'red' : issues.length > 0 ? 'yellow' : 'green',
    issues,
  }
}

function localFixture(platform: string, country?: string, locale?: string, timezone?: string, deviceClass?: string) {
  const region = localRegion(country, locale, timezone)
  if (platform === 'mac') {
    return {
      brand: 'Chrome',
      platform: 'mac',
      lang: region.locale,
      timezone: region.timezone,
      resolution: deviceClass === 'workstation' ? '2560,1440' : '1440,900',
      colorDepth: '24',
      hardwareConcurrency: deviceClass === 'workstation' ? '10' : '8',
      deviceMemory: deviceClass === 'workstation' ? '16' : '8',
      canvasNoise: true,
      audioNoise: true,
      webglVendor: 'Apple',
      webglRenderer: deviceClass === 'workstation' ? 'Apple M2' : 'Apple M1',
      fonts: 'Arial,Helvetica,Helvetica Neue,Menlo,Monaco,PingFang SC,Times New Roman',
      webrtcPolicy: 'disable_non_proxied_udp',
      webrtcIP: 'auto',
      doNotTrack: false,
      touchPoints: '0',
      mediaDevices: deviceClass === 'workstation' ? '1,2,2' : '1,1,1',
    }
  }
  if (platform === 'linux') {
    return {
      brand: 'Chrome',
      platform: 'linux',
      lang: region.locale,
      timezone: region.timezone,
      resolution: deviceClass === 'light_linux' ? '1366,768' : '1920,1080',
      colorDepth: '24',
      hardwareConcurrency: deviceClass === 'light_linux' ? '4' : '8',
      deviceMemory: deviceClass === 'light_linux' ? '4' : '8',
      canvasNoise: true,
      audioNoise: true,
      webglVendor: 'Intel',
      webglRenderer: deviceClass === 'light_linux' ? 'Mesa Intel(R) HD Graphics 520' : 'Mesa Intel(R) UHD Graphics 620',
      fonts: 'Arimo,Cousine,Tinos,Noto Sans,Noto Sans CJK SC,Noto Color Emoji',
      webrtcPolicy: 'disable_non_proxied_udp',
      webrtcIP: 'auto',
      doNotTrack: false,
      touchPoints: '0',
      mediaDevices: deviceClass === 'light_linux' ? '0,1,1' : '0,1,1',
    }
  }
  return {
    brand: 'Chrome',
    platform: 'windows',
    lang: region.locale,
    timezone: region.timezone,
    resolution: deviceClass === 'gaming' ? '2560,1440' : deviceClass === 'laptop' ? '1366,768' : '1920,1080',
    colorDepth: '24',
    hardwareConcurrency: deviceClass === 'gaming' ? '16' : deviceClass === 'laptop' ? '4' : '8',
    deviceMemory: deviceClass === 'gaming' ? '16' : deviceClass === 'laptop' ? '4' : '8',
    canvasNoise: true,
    audioNoise: true,
    webglVendor: deviceClass === 'gaming' ? 'NVIDIA' : 'Intel',
    webglRenderer: deviceClass === 'gaming' ? 'NVIDIA GeForce RTX 3060' : deviceClass === 'laptop' ? 'Intel(R) UHD Graphics 620' : 'Intel(R) UHD Graphics 630',
    fonts: 'Arial,Calibri,Cambria Math,Consolas,Segoe UI,Tahoma,Times New Roman,Verdana,Microsoft YaHei',
    webrtcPolicy: 'disable_non_proxied_udp',
    webrtcIP: 'auto',
    doNotTrack: false,
    touchPoints: '0',
    mediaDevices: deviceClass === 'gaming' ? '1,2,2' : '1,1,1',
  }
}

function localRegion(country?: string, locale?: string, timezone?: string) {
  if (locale && timezone) return { locale, timezone }
  switch ((country || '').toUpperCase()) {
    case 'US':
      return { locale: locale || 'en-US', timezone: timezone || 'America/New_York' }
    case 'GB':
      return { locale: locale || 'en-GB', timezone: timezone || 'Europe/London' }
    case 'DE':
      return { locale: locale || 'de-DE', timezone: timezone || 'Europe/Berlin' }
    case 'FR':
      return { locale: locale || 'fr-FR', timezone: timezone || 'Europe/Paris' }
    case 'JP':
      return { locale: locale || 'ja-JP', timezone: timezone || 'Asia/Tokyo' }
    case 'KR':
      return { locale: locale || 'ko-KR', timezone: timezone || 'Asia/Seoul' }
    case 'SG':
      return { locale: locale || 'en-SG', timezone: timezone || 'Asia/Singapore' }
    default:
      return { locale: locale || 'zh-CN', timezone: timezone || 'Asia/Shanghai' }
  }
}
