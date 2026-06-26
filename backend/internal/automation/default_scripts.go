package automation

const DualInstanceRuntimeScriptID = "dual-instance-runtime-switch"
const FingerprintAuditScriptID = "fingerprint-audit"

func DefaultScripts() []ScriptRecord {
	return []ScriptRecord{
		{
			ID:          DualInstanceRuntimeScriptID,
			Name:        "双实例启动与 Runtime 切换",
			Description: "通过 Launch API 分别启动两个实例，切换 Runtime 会话后交给 OpenClaw 执行。",
			Type:        "launch-api",
			Status:      "ready",
			EntryFile:   "index.cjs",
			Tags:        []string{"Launch API", "OpenClaw", "双实例"},
			ParamsText: `{
	"browsers": [
	  {
	    "code": "BUYER_001",
	    "skipDefaultStartUrls": true,
	    "startUrls": ["https://finance.sina.com.cn/"]
	  },
	  {
	    "code": "BUYER_002",
	    "skipDefaultStartUrls": true,
	    "startUrls": ["https://map.baidu.com/"]
	  }
	],
	"timeoutMs": 45000
}`,
			ScriptText: `export async function run({ baseUrl, apiKey, params, log }) {
  const normalizeCode = (value, fallback) =>
    String(value || fallback || '').trim().toUpperCase()
  const normalizeStringArray = (value) =>
    Array.isArray(value)
      ? value
          .map((item) => String(item || '').trim())
          .filter(Boolean)
      : []
  const normalizeBrowserInput = (value, fallbackCode, fallbackStartUrls, defaultSkip) => {
    const raw = value && typeof value === 'object' ? value : {}
    const code = normalizeCode(raw.code || raw.launchCode, fallbackCode)
    if (!code) {
      return null
    }
    const startUrls = normalizeStringArray(raw.startUrls)
    const fallbackUrls = normalizeStringArray(fallbackStartUrls)
    const launchArgs = normalizeStringArray(raw.launchArgs)

    return {
      code,
      skipDefaultStartUrls:
        raw.skipDefaultStartUrls !== undefined
          ? raw.skipDefaultStartUrls !== false
          : defaultSkip,
      startUrls: startUrls.length > 0 ? startUrls : fallbackUrls,
      launchArgs,
    }
  }

  const timeoutMs = Number.isFinite(Number(params.timeoutMs))
    ? Math.max(1000, Math.round(Number(params.timeoutMs)))
    : 45000
  const defaultSkipDefaultStartUrls = params.skipDefaultStartUrls !== false

  let browsers = Array.isArray(params.browsers)
    ? params.browsers
        .map((item, index) =>
          normalizeBrowserInput(
            item,
            ['BUYER_001', 'BUYER_002'][index] || '',
            ['https://finance.sina.com.cn/', 'https://map.baidu.com/'][index] || [],
            defaultSkipDefaultStartUrls,
          ),
        )
        .filter(Boolean)
    : []

  if (browsers.length === 0) {
    browsers = [
      normalizeBrowserInput(
        { code: params.primaryCode, skipDefaultStartUrls: params.skipDefaultStartUrls },
        'BUYER_001',
        ['https://finance.sina.com.cn/'],
        defaultSkipDefaultStartUrls,
      ),
      normalizeBrowserInput(
        { code: params.secondaryCode, skipDefaultStartUrls: params.skipDefaultStartUrls },
        'BUYER_002',
        ['https://map.baidu.com/'],
        defaultSkipDefaultStartUrls,
      ),
    ].filter(Boolean)
  }

  if (browsers.length === 0) {
    throw new Error('params.browsers 不能为空')
  }

  const headers = {
    'Content-Type': 'application/json',
    ...(apiKey ? { 'X-Ant-Api-Key': apiKey } : {}),
  }

  const post = async (path, payload) => {
    const response = await fetch(baseUrl + path, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
    })
    const text = await response.text()
    let body = text
    try {
      body = text ? JSON.parse(text) : null
    } catch {
      body = text
    }
    if (!response.ok) {
      throw new Error(path + ' failed: ' + response.status + ' ' + text)
    }
    return body
  }

  const sessions = []

  for (const browser of browsers) {
    const sessionResult = await post('/api/runtime/session', {
      selector: { code: browser.code, matchMode: 'unique' },
      skipDefaultStartUrls: browser.skipDefaultStartUrls,
      ...(browser.startUrls.length > 0 ? { startUrls: browser.startUrls } : {}),
      ...(browser.launchArgs.length > 0 ? { launchArgs: browser.launchArgs } : {}),
      timeoutMs,
    })

    sessions.push(sessionResult)
  }

  const browserCodes = browsers.map((item) => item.code)
  log('browserCodes', browserCodes)

  return {
    ok: true,
    summary: browserCodes.length + ' 个浏览器已就绪：' + browserCodes.join(' / '),
    browserCodes,
    sessions,
  }
}`,
			Notes: "先通过接口启动两个实例并切换 Runtime 会话；随后把实例信息交给 OpenClaw 执行自动化动作。",
			Source: ScriptSource{
				Type: "builtin",
				URI:  "repo://backend/internal/automation/default_scripts.go",
				Ref:  "HEAD",
				Path: DualInstanceRuntimeScriptID,
			},
		},
		{
			ID:          "news-query-txt",
			Name:        "查询新闻并写 TXT",
			Description: "通过 Bing 搜索新闻关键词，提取结果并写入本地 txt 文件。",
			Type:        "playwright-cdp",
			Status:      "ready",
			EntryFile:   "index.cjs",
			Tags:        []string{"Playwright", "新闻", "TXT"},
			ParamsText: `{
  "keyword": "OpenAI",
  "limit": 10,
  "timeRange": "week",
  "outputFileName": "openai-news.txt",
  "timeoutMs": 30000,
  "waitAfterLoadMs": 1500,
  "captureScreenshot": false
}`,
			ScriptText: `const fs = require('fs')

const DEFAULT_EXCLUDED_DOMAINS = [
  'zhihu.com',
  'baidu.com',
  'qq.com',
  '36kr.com',
  'apifox.com',
  'chatgpt-chinese.com',
  'openwebui.cn',
  'open-openai.com',
  'xiniushu.com',
  'reddit.com',
  'quora.com',
  'tieba.baidu.com',
  'weibo.com',
  'x.com',
  'twitter.com',
  'youtube.com',
  'bilibili.com',
  'douyin.com',
  'xiaohongshu.com',
]

function normalizeInt(value, fallback, min, max) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) {
    return fallback
  }

  const rounded = Math.round(parsed)
  if (rounded < min) {
    return min
  }
  if (rounded > max) {
    return max
  }
  return rounded
}

function normalizeText(value) {
  return String(value || '').trim()
}

function normalizeDomainList(value) {
  if (!Array.isArray(value)) {
    return []
  }

  const deduped = new Set()
  for (const item of value) {
    const normalized = normalizeText(item).replace(/^https?:\/\//, '').replace(/^www\./, '').toLowerCase()
    if (normalized) {
      deduped.add(normalized)
    }
  }
  return Array.from(deduped)
}

function buildDefaultQuery(keyword) {
  const normalizedKeyword = normalizeText(keyword) || 'OpenAI'
  if (/[\u3400-\u9fff]/.test(normalizedKeyword)) {
    return normalizedKeyword + ' 新闻'
  }
  return normalizedKeyword + ' news'
}

function buildFallbackQueries(keyword, baseQuery) {
  const normalizedKeyword = normalizeText(keyword) || 'OpenAI'
  const normalizedBaseQuery = normalizeText(baseQuery)
  const candidates = [
    normalizedBaseQuery,
  ]

  if (/[\u3400-\u9fff]/.test(normalizedKeyword)) {
    candidates.push(normalizedKeyword + ' 最新新闻')
  } else {
    candidates.push(normalizedKeyword + ' latest news')
  }

  const deduped = new Set()
  for (const item of candidates) {
    const normalized = normalizeText(item)
    if (normalized) {
      deduped.add(normalized)
    }
  }
  return Array.from(deduped)
}

function buildSearchQuery(baseQuery, excludedDomains) {
  const normalizedBaseQuery = normalizeText(baseQuery)
  const normalizedDomains = normalizeDomainList(excludedDomains)
  const parts = [normalizedBaseQuery]

  for (const domain of normalizedDomains) {
    parts.push('-site:' + domain)
  }

  return parts.filter(Boolean).join(' ')
}

function mapTimeRangeToBingFilter(value) {
  switch (normalizeText(value).toLowerCase()) {
    case 'day':
    case '24h':
    case 'today':
      return 'ex1:"ez1"'
    case 'week':
      return 'ex1:"ez2"'
    case 'month':
      return 'ex1:"ez3"'
    default:
      return ''
  }
}

function buildSearchURL(query, timeRange, firstResultIndex) {
  const searchParams = new URLSearchParams({ q: query })
  const filter = mapTimeRangeToBingFilter(timeRange)
  if (filter) {
    searchParams.set('filters', filter)
  }
  if (Number.isFinite(firstResultIndex) && firstResultIndex > 1) {
    searchParams.set('first', String(firstResultIndex))
  }
  return 'https://www.bing.com/search?' + searchParams.toString()
}

function splitSnippet(snippet) {
  const normalized = normalizeText(snippet)
  if (!normalized) {
    return { publishedAt: '', summary: '' }
  }

  const match = normalized.match(/^([^·]{0,40})\s*·\s*(.+)$/)
  if (
    match &&
    /(前|分钟|小时|天前|周前|月前|昨天|\d{4}|\d{1,2}[/-]\d{1,2})/.test(match[1])
  ) {
    return {
      publishedAt: normalizeText(match[1]),
      summary: normalizeText(match[2]),
    }
  }

  return {
    publishedAt: '',
    summary: normalized,
  }
}

function parseHostname(rawUrl) {
  const normalized = normalizeText(rawUrl)
  if (!normalized) {
    return ''
  }

  try {
    return new URL(normalized).hostname.replace(/^www\./, '').toLowerCase()
  } catch {
    return ''
  }
}

function parsePathname(rawUrl) {
  const normalized = normalizeText(rawUrl)
  if (!normalized) {
    return ''
  }

  try {
    const pathname = new URL(normalized).pathname.replace(/\/+/g, '/').toLowerCase()
    if (!pathname) {
      return ''
    }
    return pathname === '/' ? pathname : pathname.replace(/\/$/, '')
  } catch {
    return ''
  }
}

function looksLikeQuestionTitle(title) {
  const normalized = normalizeText(title)
  if (!normalized) {
    return false
  }

  if (/[？?]/.test(normalized)) {
    return true
  }

  return /^(如何|为什么|怎么看|怎样|怎么|是否|有没有|谁能|请问|评价|如何评价|如何看待|为什么说)/.test(normalized)
}

function looksLikeAggregateText(text) {
  const normalized = normalizeText(text).toLowerCase()
  if (!normalized) {
    return false
  }

  return /(roundup|digest|flash report|llm news today|ai news today|daily ai news|news today|model releases)/.test(normalized)
}

function looksLikeListingPath(pathname) {
  const normalized = normalizeText(pathname).toLowerCase()
  if (!normalized || normalized === '/') {
    return false
  }

  if (/(^|\/)(tag|tags|topic|topics|category|categories|label|labels|brand|brands)(\/|$)/.test(normalized)) {
    return true
  }

  if (/(^|\/)(news|latest|headlines|insights)$/.test(normalized)) {
    return true
  }

  return /\/news\/(brand|brands|topic|topics|tag|tags)(\/|$)/.test(normalized)
}

function looksLikeListingText(text) {
  const normalized = normalizeText(text).toLowerCase()
  if (!normalized) {
    return false
  }

  return /(latest news|breaking headlines|news and insights|news and analysis|everything you need to know|get the latest|最新资讯|最新动态|实时追踪|热点快讯|快讯)/.test(normalized)
}

function isBlockedHostname(hostname) {
  const normalized = normalizeText(hostname).toLowerCase()
  if (!normalized) {
    return false
  }

  const blockedSuffixes = DEFAULT_EXCLUDED_DOMAINS
  const blockedKeywords = [
    'aitrack',
    'aitoolly',
    'aiflashreport',
    'llm-stats',
    'opentools',
  ]

  if (blockedSuffixes.some(function (suffix) {
    return normalized === suffix || normalized.endsWith('.' + suffix)
  })) {
    return true
  }

  return blockedKeywords.some(function (keyword) {
    return normalized.includes(keyword)
  })
}

function evaluateNewsItem(item) {
  const hostname = parseHostname(item.url)
  const pathname = parsePathname(item.url)
  const summary = normalizeText(item.summary)
  const source = normalizeText(item.source)
  const reasons = []

  if (!normalizeText(item.url)) {
    reasons.push('missing-url')
  }
  if (!hostname) {
    reasons.push('invalid-url')
  }
  if (hostname && isBlockedHostname(hostname)) {
    reasons.push('blocked-host')
  }
  if (!source) {
    reasons.push('missing-source')
  }
  if (summary.length < 20) {
    reasons.push('summary-too-short')
  }
  if (looksLikeQuestionTitle(item.title)) {
    reasons.push('question-title')
  }
  if (looksLikeAggregateText(item.title) || looksLikeAggregateText(summary)) {
    reasons.push('aggregate-page')
  }
  if (looksLikeListingPath(pathname) || looksLikeListingText(item.title) || looksLikeListingText(summary)) {
    reasons.push('listing-page')
  }

  return Object.assign({}, item, {
    hostname: hostname,
    pathname: pathname,
    qualityAccepted: reasons.length === 0,
    qualityReasons: reasons,
  })
}

function formatRejectedReason(reason) {
  switch (reason) {
    case 'missing-url':
      return '缺少链接'
    case 'invalid-url':
      return '链接无效'
    case 'blocked-host':
      return '来源站点已过滤'
    case 'missing-source':
      return '缺少来源'
    case 'summary-too-short':
      return '摘要过短'
    case 'question-title':
      return '标题更像问答'
    case 'aggregate-page':
      return '更像聚合页'
    case 'listing-page':
      return '更像列表页/专题页'
    default:
      return reason
  }
}

function formatReport(items, metadata) {
  const lines = [
    '新闻抓取结果',
    '查询词: ' + metadata.query,
    '抓取时间: ' + metadata.generatedAt,
    '搜索地址: ' + metadata.searchUrl,
    '原始结果: ' + metadata.rawCount,
    '通过校验: ' + items.length,
    '过滤数量: ' + metadata.rejectedItems.length,
    '',
  ]

  for (const item of items) {
    lines.push(item.rank + '. ' + item.title)
    if (item.source) {
      lines.push('来源: ' + item.source)
    }
    if (item.publishedAt) {
      lines.push('时间: ' + item.publishedAt)
    }
    lines.push('链接: ' + item.url)
    if (item.summary) {
      lines.push('摘要: ' + item.summary)
    }
    lines.push('')
  }

  if (metadata.rejectedItems.length > 0) {
    lines.push('被过滤结果（最多展示 5 条）')
    lines.push('')
    for (const item of metadata.rejectedItems.slice(0, 5)) {
      lines.push(item.rank + '. ' + item.title)
      if (item.hostname) {
        lines.push('站点: ' + item.hostname)
      }
      lines.push('原因: ' + item.qualityReasons.map(formatRejectedReason).join(' / '))
      lines.push('')
    }
  }

  return lines.join('\n')
}

function pickBestAttempt(current, candidate) {
  if (!current) {
    return candidate
  }

  if (candidate.acceptedItems.length !== current.acceptedItems.length) {
    return candidate.acceptedItems.length > current.acceptedItems.length ? candidate : current
  }

  if (candidate.distinctHostCount !== current.distinctHostCount) {
    return candidate.distinctHostCount > current.distinctHostCount ? candidate : current
  }

  if (candidate.rawItems.length !== current.rawItems.length) {
    return candidate.rawItems.length > current.rawItems.length ? candidate : current
  }

  return candidate
}

module.exports.run = async ({ launch, connect, selector, params, log, artifact }) => {
  const timeout = normalizeInt(params.timeoutMs, 30000, 1000, 120000)
  const waitAfterLoadMs = normalizeInt(params.waitAfterLoadMs, 1500, 0, 10000)
  const limit = normalizeInt(params.limit, 10, 1, 50)
  const maxPages = normalizeInt(params.maxPages, 3, 1, 5)
  const baseQuery = normalizeText(params.query) || buildDefaultQuery(params.keyword)
  const excludedDomains = normalizeDomainList(params.excludeDomains).length > 0
    ? normalizeDomainList(params.excludeDomains)
    : DEFAULT_EXCLUDED_DOMAINS
  const outputFileName = normalizeText(params.outputFileName) || 'news-results.txt'
  const scanLimit = Math.max(10, Math.min(20, limit * 2))
  const startUrls = Array.isArray(params.startUrls) && params.startUrls.length > 0
    ? params.startUrls
    : undefined

  const session = await launch({
    selector,
    startUrls,
    skipDefaultStartUrls: true,
  })

  const connection = await connect(session)
  const browser = connection.browser
  const context = connection.context || browser.contexts()[0]
  const page = await context.newPage()
  const closeRunnerPage = async function () {
    if (!page.isClosed()) {
      await page.close().catch(function () {})
    }
  }

  const searchCandidates = buildFallbackQueries(params.keyword, baseQuery)
  const minAcceptedCount = Math.min(limit, Math.max(2, Math.ceil(limit * 0.2)))
  const minDistinctHostCount = Math.min(3, minAcceptedCount)
  let bestAttempt = null

  try {
    for (const candidateQuery of searchCandidates) {
      const searchQuery = buildSearchQuery(candidateQuery, excludedDomains)
      const normalizedItems = []
      const seenUrls = new Set()
      let scannedPageCount = 0
      let firstSearchUrl = ''

      for (let pageIndex = 0; pageIndex < maxPages; pageIndex += 1) {
        const firstResultIndex = pageIndex * 10 + 1
        const searchUrl = buildSearchURL(searchQuery, params.timeRange, firstResultIndex)

        try {
          await page.goto(searchUrl, {
            waitUntil: 'domcontentloaded',
            timeout,
          })
          await page.waitForSelector('li.b_algo', { timeout })
        } catch (error) {
          if (pageIndex > 0 && normalizedItems.length > 0) {
            break
          }
          throw error
        }

        if (waitAfterLoadMs > 0) {
          await page.waitForTimeout(waitAfterLoadMs)
        }

        if (!firstSearchUrl) {
          firstSearchUrl = page.url()
        }

        const pageItems = await page.$$eval('li.b_algo', function (nodes, maxItems) {
          const clean = function (value) {
            return String(value || '').replace(/\s+/g, ' ').trim()
          }

          return nodes
            .slice(0, maxItems)
            .map(function (node) {
              const titleLink = node.querySelector('h2 a')
              const title = clean(titleLink && titleLink.textContent)
              const url = titleLink ? titleLink.href : ''
              const sourceNode = node.querySelector('.tptt')
              const source = clean(sourceNode && sourceNode.textContent)
              const citeNode = node.querySelector('.b_attribution cite')
              const cite = clean(citeNode && citeNode.textContent)
              const snippetNode = node.querySelector('.b_caption p')
              const snippet = clean(snippetNode && snippetNode.textContent)

              if (!title) {
                return null
              }

              return {
                title,
                url,
                source: source || cite,
                snippet,
              }
            })
            .filter(Boolean)
        }, scanLimit)

        let appendedCount = 0
        for (const item of pageItems) {
          const dedupeKey = normalizeText(item.url)
          if (!dedupeKey || seenUrls.has(dedupeKey)) {
            continue
          }

          seenUrls.add(dedupeKey)
          normalizedItems.push(
            evaluateNewsItem(
              Object.assign(
                {
                  rank: normalizedItems.length + 1,
                },
                item,
                splitSnippet(item.snippet)
              )
            )
          )
          appendedCount += 1
        }

        scannedPageCount += 1
        if (appendedCount === 0 || pageItems.length < 8) {
          break
        }
      }

      const acceptedItems = normalizedItems.filter(function (item) {
        return item.qualityAccepted
      }).slice(0, limit)
      const rejectedItems = normalizedItems.filter(function (item) {
        return !item.qualityAccepted
      })
      const distinctHostCount = new Set(
        acceptedItems
          .map(function (item) {
            return item.hostname
          })
          .filter(Boolean)
      ).size

      log('searchQuery', searchQuery)
      log('rawItemCount', normalizedItems.length)
      log('acceptedItemCount', acceptedItems.length)
      log('rejectedItemCount', rejectedItems.length)
      log('distinctHostCount', distinctHostCount)
      log('scannedPageCount', scannedPageCount)

      bestAttempt = pickBestAttempt(bestAttempt, {
        baseQuery: candidateQuery,
        searchQuery: searchQuery,
        searchUrl: firstSearchUrl || page.url(),
        rawItems: normalizedItems,
        acceptedItems: acceptedItems,
        rejectedItems: rejectedItems,
        distinctHostCount: distinctHostCount,
        scannedPageCount: scannedPageCount,
      })

      if (acceptedItems.length >= minAcceptedCount && distinctHostCount >= minDistinctHostCount) {
        break
      }
    }
  } catch (error) {
    await closeRunnerPage()
    throw error
  }

  if (!bestAttempt || bestAttempt.rawItems.length === 0) {
    await closeRunnerPage()
    throw new Error('未抓到新闻搜索结果，当前页面: ' + page.url())
  }

  const normalizedItems = bestAttempt.rawItems
  const acceptedItems = bestAttempt.acceptedItems
  const rejectedItems = bestAttempt.rejectedItems
  const distinctHostCount = bestAttempt.distinctHostCount
  const searchUrl = bestAttempt.searchUrl
  const scannedPageCount = bestAttempt.scannedPageCount || 1

  const outputName = outputFileName.toLowerCase().endsWith('.txt')
    ? outputFileName
    : outputFileName + '.txt'
  const outputPath = artifact(outputName)
  const reportText = formatReport(acceptedItems, {
    query: bestAttempt.baseQuery,
    generatedAt: new Date().toISOString(),
    searchUrl: searchUrl,
    rawCount: normalizedItems.length,
    rejectedItems: rejectedItems,
  })
  fs.writeFileSync(outputPath, reportText, 'utf8')

  let screenshotPath = ''
  if (params.captureScreenshot === true) {
    screenshotPath = artifact('news-search.png')
    await page.screenshot({
      path: screenshotPath,
      fullPage: true,
    })
  }

  log('outputPath', outputPath)
  await closeRunnerPage()

  if (acceptedItems.length < minAcceptedCount || distinctHostCount < minDistinctHostCount) {
    return {
      ok: false,
      summary: '新闻结果质量不足，仅 ' + acceptedItems.length + '/' + normalizedItems.length + ' 条通过校验',
      error: '搜索结果更像普通搜索、问答页或聚合页，未达到新闻抓取标准',
      query: bestAttempt.baseQuery,
      searchQuery: bestAttempt.searchQuery,
      searchUrl: searchUrl,
      outputPath,
      screenshotPath,
      rawItemCount: normalizedItems.length,
      itemCount: acceptedItems.length,
      rejectedCount: rejectedItems.length,
      distinctHostCount: distinctHostCount,
      scannedPageCount: scannedPageCount,
      firstTitle: acceptedItems[0] ? acceptedItems[0].title : '',
    }
  }

  return {
    ok: true,
    summary: '已筛出 ' + acceptedItems.length + ' 条有效新闻并写入 TXT',
    query: bestAttempt.baseQuery,
    searchQuery: bestAttempt.searchQuery,
    searchUrl: searchUrl,
    outputPath,
    screenshotPath,
    rawItemCount: normalizedItems.length,
    itemCount: acceptedItems.length,
    rejectedCount: rejectedItems.length,
    distinctHostCount: distinctHostCount,
    scannedPageCount: scannedPageCount,
    firstTitle: acceptedItems[0] ? acceptedItems[0].title : '',
  }
}`,
			Notes: "脚本会优先使用 Bing 搜索真实新闻结果，并自动追加时间过滤、排除问答/聚合站点、回退查询词和质量校验；只有达到新闻质量门槛时才会判定成功，并把结果写入本地 txt。执行时可直接点“创建 Demo 并执行”，成功后在结果里的 outputPath 查看文件。",
			Source: ScriptSource{
				Type: "builtin",
				URI:  "repo://backend/internal/automation/default_scripts.go",
				Ref:  "HEAD",
				Path: "news-query-txt",
			},
		},
		{
			ID:          FingerprintAuditScriptID,
			Name:        "指纹回归采集",
			Description: "复用 Playwright CDP 连接当前实例，采集本地指纹 probe，并可选保存检测站截图与 HTML。",
			Type:        "playwright-cdp",
			Status:      "ready",
			EntryFile:   "index.cjs",
			Tags:        []string{"Playwright", "Fingerprint", "Audit"},
			ParamsText: `{
  "detectors": [
    "browserscan",
    "creepjs"
  ],
  "localProbeOnly": false,
  "captureScreenshot": true,
  "saveHtml": true,
  "enableHeaderEcho": true,
  "headerEchoUrl": "https://httpbin.org/headers",
  "probePersistStorage": false,
  "webrtcProbeTimeoutMs": 3500,
  "waitAfterLoadMs": 5000,
  "timeoutMs": 120000
}`,
			ScriptText: `const fs = require('fs')

const DETECTOR_PRESETS = {
  browserscan: 'https://www.browserscan.net/',
  creepjs: 'https://abrahamjuliot.github.io/creepjs/',
  fingerprint: 'https://demo.fingerprint.com/playground',
  fingerprintjs: 'https://demo.fingerprint.com/playground',
  incolumitas: 'https://bot.incolumitas.com/',
  bot: 'https://bot.incolumitas.com/',
  deviceinfo: 'https://deviceandbrowserinfo.com/',
  deviceandbrowserinfo: 'https://deviceandbrowserinfo.com/',
}

function normalizeInt(value, fallback, min, max) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) {
    return fallback
  }

  const rounded = Math.round(parsed)
  if (rounded < min) {
    return min
  }
  if (rounded > max) {
    return max
  }
  return rounded
}

function normalizeBool(value, fallback) {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (['1', 'true', 'yes', 'y', 'on'].includes(normalized)) {
      return true
    }
    if (['0', 'false', 'no', 'n', 'off'].includes(normalized)) {
      return false
    }
  }
  return fallback
}

function normalizeString(value, fallback) {
  const text = String(value === undefined || value === null ? '' : value).trim()
  return text || fallback
}

function normalizeDetectorEntries(value) {
  if (!Array.isArray(value)) {
    return []
  }

  const result = []
  const seen = new Set()
  for (const item of value) {
    let id = ''
    let url = ''
    if (typeof item === 'string') {
      const raw = item.trim()
      if (!raw) {
        continue
      }
      const preset = DETECTOR_PRESETS[raw.toLowerCase()]
      id = raw.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'detector'
      url = preset || raw
    } else if (item && typeof item === 'object') {
      const rawId = String(item.id || item.name || item.key || '').trim()
      const rawUrl = String(item.url || item.href || '').trim()
      if (!rawUrl && rawId) {
        url = DETECTOR_PRESETS[rawId.toLowerCase()] || ''
      } else {
        url = rawUrl
      }
      id =
        rawId.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') ||
        String(url || 'detector').toLowerCase().replace(/^https?:\/\//, '').replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') ||
        'detector'
    }

    if (!url || !/^https?:\/\//i.test(url)) {
      continue
    }
    let normalizedUrl = url
    try {
      normalizedUrl = new URL(url).toString()
    } catch {
      continue
    }
    const dedupeKey = id + '\n' + normalizedUrl
    if (seen.has(dedupeKey)) {
      continue
    }
    seen.add(dedupeKey)
    result.push({ id, url: normalizedUrl })
  }
  return result
}

function normalizeHeaderEchoUrl(value) {
  const raw = normalizeString(value, 'https://httpbin.org/headers')
  if (!/^https?:\/\//i.test(raw)) {
    return ''
  }
  try {
    return new URL(raw).toString()
  } catch {
    return ''
  }
}

function writeJSON(filePath, value) {
  fs.writeFileSync(filePath, JSON.stringify(value, null, 2) + '\n', 'utf8')
}

function safeFilePart(value, fallback) {
  return String(value || fallback || 'item')
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, '')
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80) || String(fallback || 'item')
}

async function collectHeaderEcho(page, echoUrl, timeout) {
  if (!echoUrl) {
    return { enabled: false, error: 'headerEchoUrl is empty' }
  }

  const startedAt = Date.now()
  const result = {
    enabled: true,
    url: echoUrl,
    finalUrl: '',
    status: 0,
    requestHeaders: {},
    responseHeaders: {},
    echoedHeaders: {},
    nextHopProtocol: '',
    durationMs: 0,
    error: '',
  }

  const handler = function (request) {
    if (Object.keys(result.requestHeaders).length > 0 || request.resourceType() !== 'document') {
      return
    }
    result.requestHeaders = request.headers()
  }

  page.on('request', handler)
  try {
    const response = await page.goto(echoUrl, { waitUntil: 'domcontentloaded', timeout })
    result.finalUrl = page.url()
    if (response) {
      result.status = response.status()
      result.responseHeaders = response.headers()
    }
    const text = await page.locator('body').innerText({ timeout: Math.min(timeout, 5000) }).catch(async function () {
      return await page.content()
    })
    try {
      const parsed = JSON.parse(text)
      if (parsed && parsed.headers && typeof parsed.headers === 'object') {
        result.echoedHeaders = parsed.headers
      } else if (parsed && typeof parsed === 'object') {
        result.echoedHeaders = parsed
      }
    } catch {}
    result.nextHopProtocol = await page.evaluate(function (targetUrl) {
      const nav = performance.getEntriesByType('navigation')[0]
      if (nav && nav.nextHopProtocol) {
        return nav.nextHopProtocol
      }
      const resources = performance.getEntriesByName(targetUrl)
      return resources && resources[0] && resources[0].nextHopProtocol ? resources[0].nextHopProtocol : ''
    }, result.finalUrl).catch(function () { return '' })
  } catch (error) {
    result.error = error && error.message ? error.message : String(error)
  } finally {
    result.durationMs = Date.now() - startedAt
    page.off('request', handler)
  }

  return result
}

async function collectLocalProbe(page, options) {
  return page.evaluate(async function (options) {
    const safe = function (fn, fallback) {
      try {
        return fn()
      } catch (error) {
        return fallback
      }
    }

    const safeAsync = async function (fn, fallback) {
      try {
        return await fn()
      } catch (error) {
        if (fallback && typeof fallback === 'object') {
          return Object.assign({}, fallback, { error: error && error.message ? error.message : String(error) })
        }
        return { error: error && error.message ? error.message : String(error) }
      }
    }

    const serializePluginArray = function (items) {
      return Array.prototype.slice.call(items || []).map(function (item) {
        return {
          name: String(item && item.name || ''),
          filename: String(item && item.filename || ''),
          description: String(item && item.description || ''),
          length: Number(item && item.length || 0),
        }
      })
    }

    const serializeMimeTypeArray = function (items) {
      return Array.prototype.slice.call(items || []).map(function (item) {
        return {
          type: String(item && item.type || ''),
          suffixes: String(item && item.suffixes || ''),
          description: String(item && item.description || ''),
          enabledPlugin: String(item && item.enabledPlugin && item.enabledPlugin.name || ''),
        }
      })
    }

    const parseIceCandidate = function (candidate) {
      const raw = String(candidate && candidate.candidate || candidate || '')
      const parts = raw.split(/\s+/)
      const typIndex = parts.indexOf('typ')
      const type = typIndex >= 0 && parts[typIndex + 1] ? parts[typIndex + 1] : ''
      const address = parts.length > 4 ? parts[4] : ''
      const ipPattern = /^(?:\d{1,3}\.){3}\d{1,3}$|^[0-9a-f:]+$/i
      return {
        raw,
        type,
        address,
        port: parts.length > 5 ? parts[5] : '',
        protocol: parts.length > 2 ? parts[2].toLowerCase() : '',
        isMDNS: /\.local$/i.test(address),
        isIP: ipPattern.test(address),
      }
    }

    const isPrivateIP = function (ip) {
      if (!/^(?:\d{1,3}\.){3}\d{1,3}$/.test(ip)) {
        return /^(fc|fd|fe80:)/i.test(ip) || ip === '::1'
      }
      const parts = ip.split('.').map(function (item) { return Number(item) })
      return parts[0] === 10 ||
        (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
        (parts[0] === 192 && parts[1] === 168) ||
        parts[0] === 127 ||
        (parts[0] === 169 && parts[1] === 254)
    }

    const getUserAgentData = async function () {
      const uaData = navigator.userAgentData
      if (!uaData) {
        return null
      }
      const base = {
        brands: safe(function () { return Array.prototype.slice.call(uaData.brands || []) }, []),
        mobile: safe(function () { return Boolean(uaData.mobile) }, false),
        platform: safe(function () { return String(uaData.platform || '') }, ''),
      }
      if (typeof uaData.getHighEntropyValues !== 'function') {
        return base
      }
      try {
        const high = await uaData.getHighEntropyValues([
          'architecture',
          'bitness',
          'fullVersionList',
          'model',
          'platformVersion',
          'uaFullVersion',
          'wow64',
        ])
        return Object.assign(base, high || {})
      } catch (error) {
        return Object.assign(base, { error: error && error.message ? error.message : String(error) })
      }
    }

    const getWebGL = function (contextVersion) {
      const canvas = document.createElement('canvas')
      const gl = contextVersion === 2
        ? canvas.getContext('webgl2')
        : (canvas.getContext('webgl') || canvas.getContext('experimental-webgl'))
      if (!gl) {
        return { supported: false, context: contextVersion === 2 ? 'webgl2' : 'webgl' }
      }
      const debugInfo = safe(function () { return gl.getExtension('WEBGL_debug_renderer_info') }, null)
      return {
        supported: true,
        context: contextVersion === 2 ? 'webgl2' : 'webgl',
        vendor: debugInfo ? safe(function () { return String(gl.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) || '') }, '') : safe(function () { return String(gl.getParameter(gl.VENDOR) || '') }, ''),
        renderer: debugInfo ? safe(function () { return String(gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) || '') }, '') : safe(function () { return String(gl.getParameter(gl.RENDERER) || '') }, ''),
        version: safe(function () { return String(gl.getParameter(gl.VERSION) || '') }, ''),
        shadingLanguageVersion: safe(function () { return String(gl.getParameter(gl.SHADING_LANGUAGE_VERSION) || '') }, ''),
        extensions: safe(function () { return gl.getSupportedExtensions() || [] }, []),
        limits: {
          maxTextureSize: safe(function () { return gl.getParameter(gl.MAX_TEXTURE_SIZE) }, null),
          maxCubeMapTextureSize: safe(function () { return gl.getParameter(gl.MAX_CUBE_MAP_TEXTURE_SIZE) }, null),
          maxRenderbufferSize: safe(function () { return gl.getParameter(gl.MAX_RENDERBUFFER_SIZE) }, null),
          maxViewportDims: safe(function () { return Array.prototype.slice.call(gl.getParameter(gl.MAX_VIEWPORT_DIMS) || []) }, []),
          maxCombinedTextureImageUnits: safe(function () { return gl.getParameter(gl.MAX_COMBINED_TEXTURE_IMAGE_UNITS) }, null),
        },
      }
    }

    const getWebGPU = async function () {
      if (!navigator.gpu) {
        return { supported: false, reason: 'navigator.gpu missing' }
      }
      return safeAsync(async function () {
        const adapter = await navigator.gpu.requestAdapter()
        if (!adapter) {
          return { supported: false, reason: 'requestAdapter returned null' }
        }
        let adapterInfo = null
        if (typeof adapter.requestAdapterInfo === 'function') {
          adapterInfo = await adapter.requestAdapterInfo().catch(function () { return null })
        } else if (adapter.info) {
          adapterInfo = adapter.info
        }
        return {
          supported: true,
          features: safe(function () { return Array.prototype.slice.call(adapter.features || []) }, []),
          limits: safe(function () { return Object.assign({}, adapter.limits || {}) }, {}),
          adapterInfo,
        }
      }, { supported: true })
    }

    const testIndexedDB = async function () {
      if (!window.indexedDB) {
        return { supported: false, ok: false }
      }
      return new Promise(function (resolve) {
        const name = 'ant_fingerprint_audit_' + Date.now()
        let settled = false
        const done = function (value) {
          if (settled) {
            return
          }
          settled = true
          resolve(value)
        }
        const timer = setTimeout(function () { done({ supported: true, ok: false, error: 'timeout' }) }, 2500)
        const request = indexedDB.open(name, 1)
        request.onerror = function () {
          clearTimeout(timer)
          done({ supported: true, ok: false, error: request.error ? request.error.message : 'open failed' })
        }
        request.onupgradeneeded = function () {
          request.result.createObjectStore('items')
        }
        request.onsuccess = function () {
          const db = request.result
          db.close()
          indexedDB.deleteDatabase(name)
          clearTimeout(timer)
          done({ supported: true, ok: true })
        }
      })
    }

    const getStorage = async function () {
      const hasStorageManager = safe(function () { return Boolean(navigator.storage) }, false)
      const hasLocalStorage = safe(function () { return Boolean(window.localStorage) }, false)
      const hasCacheStorage = safe(function () { return Boolean(window.caches) }, false)
      const result = {
        estimate: null,
        persisted: null,
        persistSupported: safe(function () { return Boolean(navigator.storage && typeof navigator.storage.persist === 'function') }, false),
        persistAttempted: Boolean(options && options.probePersistStorage),
        persistResult: null,
        bucketsSupported: safe(function () { return Boolean(navigator.storageBuckets) }, false),
        localStorage: { supported: hasLocalStorage, ok: false },
        indexedDB: null,
        cacheStorage: { supported: hasCacheStorage, ok: false },
        serviceWorker: { supported: safe(function () { return Boolean(navigator.serviceWorker) }, false) },
      }
      if (hasStorageManager && typeof navigator.storage.estimate === 'function') {
        result.estimate = await safeAsync(async function () { return await navigator.storage.estimate() }, {})
      }
      if (hasStorageManager && typeof navigator.storage.persisted === 'function') {
        result.persisted = await safeAsync(async function () { return await navigator.storage.persisted() }, null)
      }
      if (result.persistAttempted && hasStorageManager && typeof navigator.storage.persist === 'function') {
        result.persistResult = await safeAsync(async function () { return await navigator.storage.persist() }, null)
      }
      result.localStorage = safe(function () {
        const key = 'ant_fingerprint_audit'
        localStorage.setItem(key, '1')
        const ok = localStorage.getItem(key) === '1'
        localStorage.removeItem(key)
        return { supported: true, ok }
      }, { supported: hasLocalStorage, ok: false })
      result.indexedDB = await testIndexedDB()
      if (hasCacheStorage) {
        result.cacheStorage = await safeAsync(async function () {
          const key = 'ant-fingerprint-audit-' + Date.now()
          await caches.open(key)
          await caches.delete(key)
          return { supported: true, ok: true }
        }, { supported: true, ok: false })
      }
      return result
    }

    const getFonts = function () {
      const candidates = [
        'Arial',
        'Calibri',
        'Cambria',
        'Consolas',
        'Courier New',
        'Georgia',
        'Helvetica Neue',
        'Menlo',
        'Microsoft YaHei',
        'PingFang SC',
        'Roboto',
        'San Francisco',
        'SimSun',
        'Times New Roman',
      ]
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        return { method: 'canvas-text-metrics', candidates, available: [], error: '2d context missing' }
      }
      const baseFonts = ['monospace', 'sans-serif', 'serif']
      const base = {}
      for (const baseFont of baseFonts) {
        ctx.font = '72px ' + baseFont
        base[baseFont] = ctx.measureText('mmmmmmmmmmlli').width
      }
      const available = []
      for (const font of candidates) {
        let found = false
        for (const baseFont of baseFonts) {
          ctx.font = '72px "' + font + '", ' + baseFont
          if (ctx.measureText('mmmmmmmmmmlli').width !== base[baseFont]) {
            found = true
            break
          }
        }
        if (found) {
          available.push(font)
        }
      }
      return { method: 'canvas-text-metrics', candidates, available }
    }

    const getPluginsWidevine = async function () {
      const plugins = serializePluginArray(navigator.plugins)
      const mimeTypes = serializeMimeTypeArray(navigator.mimeTypes)
      const mediaCapabilities = getMediaCapabilities()
      const widevine = { supported: false, error: '' }
      if (typeof navigator.requestMediaKeySystemAccess === 'function') {
        try {
          const access = await navigator.requestMediaKeySystemAccess('com.widevine.alpha', [{
            initDataTypes: ['cenc'],
            audioCapabilities: [{ contentType: 'audio/mp4; codecs="mp4a.40.2"' }],
            videoCapabilities: [{ contentType: 'video/mp4; codecs="avc1.42E01E"' }],
          }])
          widevine.supported = Boolean(access)
          widevine.keySystem = access && access.keySystem ? access.keySystem : 'com.widevine.alpha'
        } catch (error) {
          widevine.error = error && error.message ? error.message : String(error)
        }
      } else {
        widevine.error = 'requestMediaKeySystemAccess missing'
      }
      return {
        plugins,
        mimeTypes,
        pdfViewerEnabled: safe(function () { return navigator.pdfViewerEnabled }, null),
        pdfPluginNames: plugins.filter(function (item) { return /pdf/i.test(item.name + ' ' + item.description) }).map(function (item) { return item.name }),
        mediaCapabilities,
        widevine,
      }
    }

    const getMediaCapabilities = function () {
      const video = document.createElement('video')
      const audio = document.createElement('audio')
      const testCanPlay = function (element, contentType) {
        return safe(function () {
          if (!element || typeof element.canPlayType !== 'function') {
            return ''
          }
          return String(element.canPlayType(contentType) || '')
        }, '')
      }
      const testMSE = function (contentType) {
        return safe(function () {
          if (!window.MediaSource || typeof MediaSource.isTypeSupported !== 'function') {
            return null
          }
          return Boolean(MediaSource.isTypeSupported(contentType))
        }, null)
      }
      const testWebCodecs = function () {
        return {
          videoEncoder: safe(function () { return typeof window.VideoEncoder === 'function' }, false),
          videoDecoder: safe(function () { return typeof window.VideoDecoder === 'function' }, false),
          audioEncoder: safe(function () { return typeof window.AudioEncoder === 'function' }, false),
          audioDecoder: safe(function () { return typeof window.AudioDecoder === 'function' }, false),
        }
      }
      const codecs = {
        h264: {
          contentType: 'video/mp4; codecs="avc1.42E01E"',
          canPlayType: testCanPlay(video, 'video/mp4; codecs="avc1.42E01E"'),
          mediaSource: testMSE('video/mp4; codecs="avc1.42E01E"'),
        },
        aac: {
          contentType: 'audio/mp4; codecs="mp4a.40.2"',
          canPlayType: testCanPlay(audio, 'audio/mp4; codecs="mp4a.40.2"'),
          mediaSource: testMSE('audio/mp4; codecs="mp4a.40.2"'),
        },
        vp9: {
          contentType: 'video/webm; codecs="vp9"',
          canPlayType: testCanPlay(video, 'video/webm; codecs="vp9"'),
          mediaSource: testMSE('video/webm; codecs="vp9"'),
        },
        av1: {
          contentType: 'video/mp4; codecs="av01.0.05M.08"',
          canPlayType: testCanPlay(video, 'video/mp4; codecs="av01.0.05M.08"'),
          mediaSource: testMSE('video/mp4; codecs="av01.0.05M.08"'),
        },
        hevc: {
          contentType: 'video/mp4; codecs="hvc1.1.6.L93.B0"',
          canPlayType: testCanPlay(video, 'video/mp4; codecs="hvc1.1.6.L93.B0"'),
          mediaSource: testMSE('video/mp4; codecs="hvc1.1.6.L93.B0"'),
        },
      }
      return {
        codecs,
        mediaSourceSupported: safe(function () { return Boolean(window.MediaSource) }, false),
        webCodecs: testWebCodecs(),
      }
    }

    const queryPermission = async function (name) {
      if (!navigator.permissions || typeof navigator.permissions.query !== 'function') {
        return { supported: false }
      }
      try {
        const status = await navigator.permissions.query({ name })
        return { supported: true, state: status && status.state ? status.state : '' }
      } catch (error) {
        return { supported: true, error: error && error.message ? error.message : String(error) }
      }
    }

    const getGeolocationPermissions = async function () {
      return {
        geolocation: {
          supported: Boolean(navigator.geolocation),
          permission: await queryPermission('geolocation'),
        },
        permissions: {
          notifications: await queryPermission('notifications'),
          camera: await queryPermission('camera'),
          microphone: await queryPermission('microphone'),
        },
      }
    }

    const getWebRTC = async function () {
      if (!window.RTCPeerConnection) {
        return { supported: false, candidates: [], error: 'RTCPeerConnection missing' }
      }
      const timeoutMs = Math.max(500, Math.min(Number(options && options.webrtcProbeTimeoutMs) || 3500, 15000))
      return new Promise(async function (resolve) {
        const candidates = []
        let pc = null
        let settled = false
        const finish = function (error) {
          if (settled) {
            return
          }
          settled = true
          if (pc) {
            pc.close()
          }
          const parsed = candidates.map(parseIceCandidate)
          const ips = Array.from(new Set(parsed.filter(function (item) { return item.isIP }).map(function (item) { return item.address })))
          const mdnsHostnames = Array.from(new Set(parsed.filter(function (item) { return item.isMDNS }).map(function (item) { return item.address })))
          resolve({
            supported: true,
            iceGatheringState: pc ? pc.iceGatheringState : '',
            candidates: parsed,
            candidateTypes: Array.from(new Set(parsed.map(function (item) { return item.type }).filter(Boolean))),
            ips,
            privateIPs: ips.filter(isPrivateIP),
            publicIPs: ips.filter(function (ip) { return !isPrivateIP(ip) }),
            hostIPs: ips.filter(function (ip) {
              return parsed.some(function (item) { return item.address === ip && item.type === 'host' })
            }),
            srflxIPs: ips.filter(function (ip) {
              return parsed.some(function (item) { return item.address === ip && item.type === 'srflx' })
            }),
            relayIPs: ips.filter(function (ip) {
              return parsed.some(function (item) { return item.address === ip && item.type === 'relay' })
            }),
            mdnsHostnames,
            error: error || '',
          })
        }
        const timer = setTimeout(function () { finish('timeout') }, timeoutMs)
        try {
          pc = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] })
          pc.createDataChannel('audit')
          pc.onicecandidate = function (event) {
            if (event.candidate) {
              candidates.push(event.candidate.candidate)
              return
            }
            clearTimeout(timer)
            finish('')
          }
          const offer = await pc.createOffer()
          await pc.setLocalDescription(offer)
        } catch (error) {
          clearTimeout(timer)
          finish(error && error.message ? error.message : String(error))
        }
      })
    }

    const resolvedDateTime = safe(function () { return Intl.DateTimeFormat().resolvedOptions() }, {})
    const pluginsWidevine = await getPluginsWidevine()
    return {
      collectedAt: new Date().toISOString(),
      url: location.href,
      secureContext: safe(function () { return window.isSecureContext }, null),
      navigator: {
        webdriver: safe(function () { return navigator.webdriver }, null),
        userAgent: safe(function () { return navigator.userAgent }, ''),
        userAgentData: await getUserAgentData(),
        language: safe(function () { return navigator.language }, ''),
        languages: safe(function () { return Array.prototype.slice.call(navigator.languages || []) }, []),
        platform: safe(function () { return navigator.platform }, ''),
        vendor: safe(function () { return navigator.vendor }, ''),
        hardwareConcurrency: safe(function () { return navigator.hardwareConcurrency }, null),
        deviceMemory: safe(function () { return navigator.deviceMemory }, null),
        maxTouchPoints: safe(function () { return navigator.maxTouchPoints }, null),
        pdfViewerEnabled: safe(function () { return navigator.pdfViewerEnabled }, null),
      },
      intl: {
        timezone: resolvedDateTime.timeZone || '',
        locale: resolvedDateTime.locale || '',
        calendar: resolvedDateTime.calendar || '',
        numberingSystem: resolvedDateTime.numberingSystem || '',
      },
      screen: {
        width: safe(function () { return screen.width }, null),
        height: safe(function () { return screen.height }, null),
        availWidth: safe(function () { return screen.availWidth }, null),
        availHeight: safe(function () { return screen.availHeight }, null),
        colorDepth: safe(function () { return screen.colorDepth }, null),
        pixelDepth: safe(function () { return screen.pixelDepth }, null),
        devicePixelRatio: safe(function () { return window.devicePixelRatio }, null),
        innerWidth: safe(function () { return window.innerWidth }, null),
        innerHeight: safe(function () { return window.innerHeight }, null),
        outerWidth: safe(function () { return window.outerWidth }, null),
        outerHeight: safe(function () { return window.outerHeight }, null),
        visualViewport: safe(function () {
          if (!window.visualViewport) {
            return null
          }
          return {
            width: window.visualViewport.width,
            height: window.visualViewport.height,
            scale: window.visualViewport.scale,
          }
        }, null),
      },
      webgl: getWebGL(1),
      webgl2: getWebGL(2),
      webgpu: await getWebGPU(),
      fonts: getFonts(),
      plugins: pluginsWidevine.plugins,
      mimeTypes: pluginsWidevine.mimeTypes,
      pluginsWidevine,
      storage: await getStorage(),
      webrtc: await getWebRTC(),
      geolocationPermissions: await getGeolocationPermissions(),
    }
  }, options || {})
}

function headerValue(headers, name) {
  if (!headers || typeof headers !== 'object') {
    return ''
  }
  const want = String(name || '').toLowerCase()
  for (const key of Object.keys(headers)) {
    if (key.toLowerCase() === want) {
      return String(headers[key] === undefined || headers[key] === null ? '' : headers[key])
    }
  }
  return ''
}

function pushIssue(issues, severity, code, message, observed, expected) {
  issues.push({ severity, code, message, observed, expected })
}

function statusFromIssues(issues) {
  if (issues.some(function (item) { return item.severity === 'red' })) {
    return 'red'
  }
  if (issues.some(function (item) { return item.severity === 'yellow' })) {
    return 'yellow'
  }
  return 'green'
}

function matrixEntry(observed, expected, issues) {
  return {
    observed,
    expected,
    status: statusFromIssues(issues),
    issues,
  }
}

function buildUACHHeadersDimension(localProbe, headerEcho) {
  const nav = localProbe && localProbe.navigator ? localProbe.navigator : {}
  const uaData = nav.userAgentData || null
  const echoed = headerEcho && headerEcho.echoedHeaders ? headerEcho.echoedHeaders : {}
  const requestHeaders = headerEcho && headerEcho.requestHeaders ? headerEcho.requestHeaders : {}
  const issues = []
  const observed = {
    navigatorUserAgent: nav.userAgent || '',
    userAgentData: uaData,
    language: nav.language || '',
    languages: nav.languages || [],
    requestHeaders: {
      userAgent: headerValue(requestHeaders, 'user-agent'),
      secChUa: headerValue(requestHeaders, 'sec-ch-ua'),
      secChUaPlatform: headerValue(requestHeaders, 'sec-ch-ua-platform'),
      secChUaMobile: headerValue(requestHeaders, 'sec-ch-ua-mobile'),
      acceptLanguage: headerValue(requestHeaders, 'accept-language'),
    },
    echoedHeaders: {
      userAgent: headerValue(echoed, 'user-agent'),
      secChUa: headerValue(echoed, 'sec-ch-ua'),
      secChUaPlatform: headerValue(echoed, 'sec-ch-ua-platform'),
      secChUaMobile: headerValue(echoed, 'sec-ch-ua-mobile'),
      acceptLanguage: headerValue(echoed, 'accept-language'),
    },
    headerEcho,
  }
  const expected = {
    source: 'runtime navigator + HTTP echo parity',
    note: 'client_hints.json 是 data-only skeleton；本维度以 runtime UA-CH 与实际请求头为准。',
    requiredHeaders: ['user-agent', 'sec-ch-ua', 'sec-ch-ua-platform', 'sec-ch-ua-mobile', 'accept-language'],
  }
  if (!uaData) {
    pushIssue(issues, 'yellow', 'ua_ch_missing', 'navigator.userAgentData 不可用，无法验证 runtime UA-CH。', null, 'navigator.userAgentData')
  }
  if (headerEcho && headerEcho.error) {
    pushIssue(issues, 'yellow', 'header_echo_failed', 'headers echo 请求失败，UA-CH 请求头只能使用 Playwright request 侧观测。', headerEcho.error, 'echo success')
  }
  if (observed.requestHeaders.userAgent && nav.userAgent && observed.requestHeaders.userAgent !== nav.userAgent) {
    pushIssue(issues, 'red', 'user_agent_header_mismatch', '请求 User-Agent 与 navigator.userAgent 不一致。', observed.requestHeaders.userAgent, nav.userAgent)
  }
  if (uaData && observed.requestHeaders.secChUaPlatform) {
    const normalizedPlatformHeader = observed.requestHeaders.secChUaPlatform.replace(/^"|"$/g, '')
    if (uaData.platform && normalizedPlatformHeader && normalizedPlatformHeader !== uaData.platform) {
      pushIssue(issues, 'yellow', 'sec_ch_platform_mismatch', 'sec-ch-ua-platform 与 navigator.userAgentData.platform 不一致。', normalizedPlatformHeader, uaData.platform)
    }
  }
  if (uaData && observed.requestHeaders.secChUaMobile) {
    const expectedMobile = uaData.mobile ? '?1' : '?0'
    if (observed.requestHeaders.secChUaMobile !== expectedMobile) {
      pushIssue(issues, 'yellow', 'sec_ch_mobile_mismatch', 'sec-ch-ua-mobile 与 navigator.userAgentData.mobile 不一致。', observed.requestHeaders.secChUaMobile, expectedMobile)
    }
  }
  if (!observed.requestHeaders.acceptLanguage && !observed.echoedHeaders.acceptLanguage) {
    pushIssue(issues, 'yellow', 'accept_language_header_missing', '未观测到 Accept-Language 请求头。', '', 'Accept-Language')
  }
  if (uaData && Array.isArray(uaData.fullVersionList) && uaData.fullVersionList.length > 0) {
    const majorFromUA = (nav.userAgent || '').match(/(?:Chrome|Chromium)\/(\d+)/)
    const fullVersion = uaData.fullVersionList.map(function (item) { return item && item.version ? String(item.version) : '' }).find(Boolean)
    const majorFromCH = fullVersion ? fullVersion.split('.')[0] : ''
    if (majorFromUA && majorFromCH && majorFromUA[1] !== majorFromCH) {
      pushIssue(issues, 'red', 'ua_ch_major_mismatch', 'UA 与 UA-CH fullVersionList 的 Chrome major 不一致。', majorFromCH, majorFromUA[1])
    }
  }
  return matrixEntry(observed, expected, issues)
}

function buildViewportGeometryDimension(localProbe) {
  const screen = localProbe ? localProbe.screen : null
  const issues = []
  const expected = {
    geometry: 'outerWidth >= innerWidth, outerHeight >= innerHeight, avail <= screen, inner <= avail',
    mode: 'headed should track the real OS window; headless should keep deterministic coherent viewport',
  }
  if (!screen) {
    pushIssue(issues, 'red', 'viewport_probe_missing', '窗口/屏幕 probe 缺失。', null, 'screen result')
    return matrixEntry(screen, expected, issues)
  }
  const num = function (value) {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : null
  }
  const observed = {
    width: num(screen.width),
    height: num(screen.height),
    availWidth: num(screen.availWidth),
    availHeight: num(screen.availHeight),
    innerWidth: num(screen.innerWidth),
    innerHeight: num(screen.innerHeight),
    outerWidth: num(screen.outerWidth),
    outerHeight: num(screen.outerHeight),
    devicePixelRatio: num(screen.devicePixelRatio),
    visualViewport: screen.visualViewport || null,
  }
  if (observed.outerWidth !== null && observed.innerWidth !== null && observed.outerWidth < observed.innerWidth) {
    pushIssue(issues, 'red', 'outer_width_less_than_inner', 'outerWidth 小于 innerWidth，属于物理不可能窗口。', observed.outerWidth, '>= ' + observed.innerWidth)
  }
  if (observed.outerHeight !== null && observed.innerHeight !== null && observed.outerHeight < observed.innerHeight) {
    pushIssue(issues, 'red', 'outer_height_less_than_inner', 'outerHeight 小于 innerHeight，属于物理不可能窗口。', observed.outerHeight, '>= ' + observed.innerHeight)
  }
  if (observed.availWidth !== null && observed.width !== null && observed.availWidth > observed.width) {
    pushIssue(issues, 'red', 'avail_width_exceeds_screen', 'availWidth 不应大于 screen.width。', observed.availWidth, '<= ' + observed.width)
  }
  if (observed.availHeight !== null && observed.height !== null && observed.availHeight > observed.height) {
    pushIssue(issues, 'red', 'avail_height_exceeds_screen', 'availHeight 不应大于 screen.height。', observed.availHeight, '<= ' + observed.height)
  }
  if (observed.innerWidth !== null && observed.availWidth !== null && observed.innerWidth > observed.availWidth + 8) {
    pushIssue(issues, 'yellow', 'inner_width_exceeds_avail', 'innerWidth 大于可用屏幕宽度，可能是 viewport emulation 或窗口模型异常。', observed.innerWidth, '<= ' + observed.availWidth)
  }
  if (observed.innerHeight !== null && observed.availHeight !== null && observed.innerHeight > observed.availHeight + 120) {
    pushIssue(issues, 'yellow', 'inner_height_exceeds_avail', 'innerHeight 明显大于可用屏幕高度，可能是 viewport emulation 或窗口模型异常。', observed.innerHeight, '<= ' + observed.availHeight)
  }
  if (observed.devicePixelRatio !== null && (observed.devicePixelRatio <= 0 || observed.devicePixelRatio > 4)) {
    pushIssue(issues, 'yellow', 'viewport_dpr_unusual', 'devicePixelRatio 不在常见桌面范围。', observed.devicePixelRatio, '0 < dpr <= 4')
  }
  return matrixEntry(observed, expected, issues)
}

function buildGpuWebgpuDimension(localProbe) {
  const issues = []
  const observed = {
    webgl: localProbe ? localProbe.webgl : null,
    webgl2: localProbe ? localProbe.webgl2 : null,
    webgpu: localProbe ? localProbe.webgpu : null,
  }
  const expected = {
    webgl: 'supported desktop WebGL with coherent vendor/renderer',
    webgpu: 'observed when runtime exposes navigator.gpu; unsupported is informational on older runtimes',
  }
  if (!observed.webgl || !observed.webgl.supported) {
    pushIssue(issues, 'red', 'webgl_missing', 'WebGL 不可用，常见检测站会直接标记异常。', observed.webgl, 'supported')
  } else if (!observed.webgl.vendor || !observed.webgl.renderer) {
    pushIssue(issues, 'yellow', 'webgl_vendor_renderer_missing', 'WebGL vendor/renderer 为空。', observed.webgl, 'non-empty vendor/renderer')
  }
  if (!observed.webgl2 || !observed.webgl2.supported) {
    pushIssue(issues, 'yellow', 'webgl2_missing', 'WebGL2 不可用，需确认是否符合目标 runtime。', observed.webgl2, 'supported for modern Chromium')
  }
  if (observed.webgpu && observed.webgpu.error) {
    pushIssue(issues, 'yellow', 'webgpu_error', 'WebGPU probe 执行失败。', observed.webgpu.error, 'no error')
  }
  return matrixEntry(observed, expected, issues)
}

function buildStorageDimension(localProbe) {
  const storage = localProbe ? localProbe.storage : null
  const issues = []
  const expected = {
    quota: 'non-zero quota/usage estimate',
    persistence: 'persisted() observable; persist() only attempted when probePersistStorage=true',
    stores: 'localStorage, IndexedDB and CacheStorage usable in normal profiles',
  }
  if (!storage) {
    pushIssue(issues, 'red', 'storage_probe_missing', 'storage probe 缺失。', null, 'storage result')
    return matrixEntry(storage, expected, issues)
  }
  if (!storage.estimate || storage.estimate.error) {
    pushIssue(issues, 'yellow', 'storage_estimate_failed', 'navigator.storage.estimate 不可用或失败。', storage.estimate, 'quota estimate')
  } else if (!Number(storage.estimate.quota)) {
    pushIssue(issues, 'yellow', 'storage_quota_empty', 'storage quota 为空，可能暴露异常 profile 或 incognito 行为。', storage.estimate.quota, 'non-zero quota')
  }
  for (const item of [
    ['localStorage', storage.localStorage],
    ['indexedDB', storage.indexedDB],
    ['cacheStorage', storage.cacheStorage],
  ]) {
    if (!item[1] || item[1].ok !== true) {
      pushIssue(issues, 'yellow', item[0] + '_unavailable', item[0] + ' 不可用或写入测试失败。', item[1], 'ok=true')
    }
  }
  return matrixEntry(storage, expected, issues)
}

function buildWebRTCDimension(localProbe) {
  const webrtc = localProbe ? localProbe.webrtc : null
  const issues = []
  const expected = {
    policy: 'no private host IP leak; srflx should match proxy/egress when available',
    candidateTypes: ['host via mDNS or no host IP', 'srflx/relay depending network policy'],
  }
  if (!webrtc || webrtc.supported === false) {
    pushIssue(issues, 'yellow', 'webrtc_missing', 'RTCPeerConnection 不可用，无法验证 ICE 泄露。', webrtc, 'supported')
    return matrixEntry(webrtc, expected, issues)
  }
  if (webrtc.error && webrtc.error !== 'timeout') {
    pushIssue(issues, 'yellow', 'webrtc_probe_error', 'WebRTC ICE probe 执行异常。', webrtc.error, 'no error')
  }
  if (Array.isArray(webrtc.privateIPs) && webrtc.privateIPs.length > 0) {
    pushIssue(issues, 'red', 'webrtc_private_ip_leak', 'ICE candidate 暴露内网或本机地址。', webrtc.privateIPs, 'no private IPs')
  }
  if (Array.isArray(webrtc.hostIPs) && webrtc.hostIPs.length > 0) {
    pushIssue(issues, 'red', 'webrtc_host_ip_leak', 'host candidate 暴露真实 IP。', webrtc.hostIPs, 'mDNS hostname or no host IP')
  }
  if ((!webrtc.candidates || webrtc.candidates.length === 0) && webrtc.error === 'timeout') {
    pushIssue(issues, 'yellow', 'webrtc_no_candidates', 'WebRTC probe 超时且未收集到 ICE candidates。', webrtc.error, 'candidate list')
  }
  return matrixEntry(webrtc, expected, issues)
}

function buildProxyDimension(headerEcho) {
  const issues = []
  const echoed = headerEcho && headerEcho.echoedHeaders ? headerEcho.echoedHeaders : {}
  const observed = {
    headerEcho,
    proxyHeaders: {
      via: headerValue(echoed, 'via'),
      forwarded: headerValue(echoed, 'forwarded'),
      xForwardedFor: headerValue(echoed, 'x-forwarded-for'),
      xRealIP: headerValue(echoed, 'x-real-ip'),
      proxyConnection: headerValue(echoed, 'proxy-connection'),
    },
    nextHopProtocol: headerEcho ? headerEcho.nextHopProtocol : '',
    latencyMs: headerEcho ? headerEcho.durationMs : 0,
  }
  const expected = {
    headers: 'no Via/Forwarded/X-Forwarded-For/X-Real-IP leakage',
    transport: 'HTTP/2 or HTTP/3 observable when endpoint supports it; DNS/QUIC/SOCKS5 UDP require deeper network probes',
  }
  if (!headerEcho || headerEcho.enabled === false) {
    pushIssue(issues, 'yellow', 'proxy_echo_disabled', 'headers echo 未启用，无法检查代理头泄露。', headerEcho, 'enabled echo')
  } else if (headerEcho.error) {
    pushIssue(issues, 'yellow', 'proxy_echo_failed', 'headers echo 请求失败，代理 header/协议矩阵不完整。', headerEcho.error, 'echo success')
  }
  for (const key of Object.keys(observed.proxyHeaders)) {
    if (observed.proxyHeaders[key]) {
      pushIssue(issues, 'red', 'proxy_header_leak_' + key, '代理相关请求头泄露：' + key, observed.proxyHeaders[key], 'empty')
    }
  }
  if (!observed.nextHopProtocol) {
    pushIssue(issues, 'yellow', 'next_hop_protocol_missing', '未观测到 nextHopProtocol，HTTP2/3 判定不完整。', '', 'h2/h3/http/1.1')
  }
  pushIssue(issues, 'yellow', 'deep_proxy_matrix_pending', 'DNS/QUIC/SOCKS5 UDP/TLS ALPN/CONNECT 深层矩阵仍需 Chromium/network patch 或本地 echo 扩展。', 'not measured', 'measured')
  return matrixEntry(observed, expected, issues)
}

function buildFontsDimension(localProbe) {
  const fonts = localProbe ? localProbe.fonts : null
  const issues = []
  const expected = {
    method: 'canvas text metrics marker-font observation',
    note: '仅观测，不伪造 FontFace/native font APIs。',
  }
  if (!fonts || fonts.error) {
    pushIssue(issues, 'yellow', 'fonts_probe_failed', '字体 probe 失败。', fonts, 'available font markers')
  } else if (!Array.isArray(fonts.available) || fonts.available.length === 0) {
    pushIssue(issues, 'yellow', 'fonts_empty', '未检测到 marker fonts，可能是极简字体环境或 probe 不足。', fonts.available, 'some platform marker fonts')
  }
  return matrixEntry(fonts, expected, issues)
}

function buildPluginsWidevineDimension(localProbe) {
  const value = localProbe ? localProbe.pluginsWidevine : null
  const issues = []
  const expected = {
    pdf: 'Chromium PDF Viewer normally exposed',
    widevine: 'Widevine availability depends on runtime build; record support/error without JS spoofing',
    codecs: 'PDF, EME and media codec capabilities should be internally consistent',
  }
  if (!value) {
    pushIssue(issues, 'yellow', 'plugins_probe_missing', '插件/Widevine probe 缺失。', null, 'pluginsWidevine result')
    return matrixEntry(value, expected, issues)
  }
  const pdfMimeTypes = Array.isArray(value.mimeTypes) ? value.mimeTypes.filter(function (item) { return /pdf/i.test(String(item.type || item.description || '')) }) : []
  if (value.pdfViewerEnabled === true && (!value.pdfPluginNames || value.pdfPluginNames.length === 0 || pdfMimeTypes.length === 0)) {
    pushIssue(issues, 'yellow', 'pdf_viewer_incomplete', 'PDF Viewer 为 true，但 plugins/mimeTypes 未形成完整组合。', { pdfPluginNames: value.pdfPluginNames, pdfMimeTypes }, 'PDF plugin and application/pdf mimeType')
  }
  if (value.pdfViewerEnabled !== true && (!value.pdfPluginNames || value.pdfPluginNames.length === 0)) {
    pushIssue(issues, 'yellow', 'pdf_viewer_missing', '未观测到 PDF Viewer。', value.pdfViewerEnabled, 'pdfViewerEnabled=true or PDF plugin')
  }
  if (value.widevine && value.widevine.supported === true && value.widevine.error) {
    pushIssue(issues, 'yellow', 'widevine_supported_with_error', 'Widevine 同时显示 supported 和 error，EME 状态不一致。', value.widevine, 'supported without error')
  }
  if (value.widevine && value.widevine.error) {
    pushIssue(issues, 'yellow', 'widevine_unavailable', 'Widevine 不可用或 EME 探测失败。', value.widevine.error, 'supported or known unavailable by runtime')
  }
  const caps = value.mediaCapabilities || {}
  const codecs = caps.codecs || {}
  const codecSupport = function (name) {
    const item = codecs[name] || {}
    return Boolean(item.canPlayType) || item.mediaSource === true
  }
  if (value.widevine && value.widevine.supported === true && (!codecSupport('h264') || !codecSupport('aac'))) {
    pushIssue(issues, 'red', 'widevine_without_mp4_codecs', 'Widevine 可用但 H.264/AAC 能力不足，DRM/codec 矩阵矛盾。', caps, 'h264+aac support')
  }
  if (!caps.mediaSourceSupported) {
    pushIssue(issues, 'yellow', 'media_source_missing', 'MediaSource 不可用，现代 Chromium 媒体能力可能异常。', caps.mediaSourceSupported, 'MediaSource')
  }
  return matrixEntry(value, expected, issues)
}

function buildGeolocationDimension(localProbe) {
  const value = localProbe ? localProbe.geolocationPermissions : null
  const issues = []
  const expected = {
    geolocation: 'API present with permission state observable',
    permissions: 'permission states should be queryable without prompting',
  }
  if (!value || !value.geolocation || !value.geolocation.supported) {
    pushIssue(issues, 'yellow', 'geolocation_missing', 'geolocation API 不可用。', value, 'navigator.geolocation')
    return matrixEntry(value, expected, issues)
  }
  if (!value.geolocation.permission || value.geolocation.permission.supported === false || value.geolocation.permission.error) {
    pushIssue(issues, 'yellow', 'geolocation_permission_unobservable', 'geolocation 权限状态无法观测。', value.geolocation.permission, 'permissions.query geolocation')
  }
  return matrixEntry(value, expected, issues)
}

function buildFingerprintMatrix(localProbe, headerEcho) {
  const matrix = {
    uaChHeaders: buildUACHHeadersDimension(localProbe, headerEcho),
    viewportGeometry: buildViewportGeometryDimension(localProbe),
    gpuWebgpu: buildGpuWebgpuDimension(localProbe),
    storage: buildStorageDimension(localProbe),
    webrtc: buildWebRTCDimension(localProbe),
    proxy: buildProxyDimension(headerEcho),
    fonts: buildFontsDimension(localProbe),
    pluginsWidevine: buildPluginsWidevineDimension(localProbe),
    geolocation: buildGeolocationDimension(localProbe),
  }
  const issues = []
  for (const dimension of Object.keys(matrix)) {
    for (const issue of matrix[dimension].issues || []) {
      issues.push(Object.assign({ dimension }, issue))
    }
  }
  return {
    matrix,
    dimensions: Object.keys(matrix).reduce(function (acc, key) {
      acc[key] = {
        status: matrix[key].status,
        issueCount: (matrix[key].issues || []).length,
      }
      return acc
    }, {}),
    issues,
    status: statusFromIssues(issues),
  }
}

module.exports.run = async ({ launch, connect, selector, params, log, artifact, artifactsDir }) => {
  const timeout = normalizeInt(params.timeoutMs, 120000, 1000, 300000)
  const waitAfterLoadMs = normalizeInt(params.waitAfterLoadMs, 5000, 0, 60000)
  const localProbeOnly = normalizeBool(params.localProbeOnly, false)
  const captureScreenshot = normalizeBool(params.captureScreenshot, true)
  const saveHtml = normalizeBool(params.saveHtml, true)
  const enableHeaderEcho = normalizeBool(params.enableHeaderEcho, true)
  const headerEchoUrl = enableHeaderEcho ? normalizeHeaderEchoUrl(params.headerEchoUrl) : ''
  const probePersistStorage = normalizeBool(params.probePersistStorage, false)
  const webrtcProbeTimeoutMs = normalizeInt(params.webrtcProbeTimeoutMs, 3500, 500, 15000)
  const detectors = localProbeOnly ? [] : normalizeDetectorEntries(params.detectors)
  const startedAt = new Date().toISOString()

  const session = await launch({
    selector,
    skipDefaultStartUrls: true,
    startUrls: [],
  })
  const connection = await connect(session)
  const browser = connection.browser
  const context = connection.context || browser.contexts()[0]
  if (!context) {
    throw new Error('CDP 连接成功，但没有可用 browser context')
  }

  const page = await context.newPage()
  const artifacts = {
    rootDir: artifactsDir || '',
    localProbe: '',
    report: '',
    detectors: [],
  }
  const detectorResults = []
  let localProbe = null
  let headerEcho = null

  try {
    await page.goto('about:blank', { waitUntil: 'load', timeout })
    if (enableHeaderEcho) {
      headerEcho = await collectHeaderEcho(page, headerEchoUrl, timeout)
      log('headerEcho', {
        url: headerEcho.url,
        status: headerEcho.status,
        nextHopProtocol: headerEcho.nextHopProtocol,
        durationMs: headerEcho.durationMs,
        error: headerEcho.error,
      })
    } else {
      headerEcho = { enabled: false, error: 'disabled by params.enableHeaderEcho=false' }
    }

    localProbe = await collectLocalProbe(page, { probePersistStorage, webrtcProbeTimeoutMs })
    artifacts.localProbe = artifact('local-probe.json')
    writeJSON(artifacts.localProbe, localProbe)
    log('localProbePath', artifacts.localProbe)

    for (const detector of detectors) {
      const detectorId = safeFilePart(detector.id, 'detector')
      const detectorArtifact = {
        id: detectorId,
        url: detector.url,
        finalUrl: '',
        screenshotPath: '',
        htmlPath: '',
        error: '',
      }

      try {
        await page.goto(detector.url, { waitUntil: 'domcontentloaded', timeout })
        if (waitAfterLoadMs > 0) {
          await page.waitForTimeout(waitAfterLoadMs)
        }
        detectorArtifact.finalUrl = page.url()

        if (captureScreenshot) {
          detectorArtifact.screenshotPath = artifact('detectors/' + detectorId + '/screenshot.png')
          await page.screenshot({ path: detectorArtifact.screenshotPath, fullPage: true })
        }

        if (saveHtml) {
          detectorArtifact.htmlPath = artifact('detectors/' + detectorId + '/page.html')
          fs.writeFileSync(detectorArtifact.htmlPath, await page.content(), 'utf8')
        }
      } catch (error) {
        detectorArtifact.error = error && error.message ? error.message : String(error)
      }

      artifacts.detectors.push(detectorArtifact)
      detectorResults.push(detectorArtifact)
      log('detector', detectorArtifact)
    }
  } finally {
    if (!page.isClosed()) {
      await page.close().catch(function () {})
    }
  }

  const matrixResult = buildFingerprintMatrix(localProbe, headerEcho)
  const report = {
    schemaVersion: 2,
    auditId: 'fingerprint-audit-' + Date.now(),
    createdAt: startedAt,
    finishedAt: new Date().toISOString(),
    selector: selector || {},
    params: {
      detectors,
      localProbeOnly,
      captureScreenshot,
      saveHtml,
      enableHeaderEcho,
      headerEchoUrl,
      probePersistStorage,
      webrtcProbeTimeoutMs,
      waitAfterLoadMs,
      timeoutMs: timeout,
    },
    session,
    observed: {
      localProbe,
      headerEcho,
      detectorResults,
    },
    matrix: matrixResult.matrix,
    validation: {
      status: matrixResult.status,
      dimensions: matrixResult.dimensions,
      issues: matrixResult.issues,
      notes: '第三方检测站结果仅采集，不作为自动阻断；schemaVersion=2 的 matrix 用于本地能力矩阵判定。',
    },
    artifacts,
  }
  artifacts.report = artifact('report.json')
  writeJSON(artifacts.report, report)
  log('reportPath', artifacts.report)

  const detectorErrorCount = detectorResults.filter(function (item) { return item.error }).length
  return {
    ok: true,
    summary:
      '指纹采集完成：local probe 已保存' +
      (detectorResults.length > 0 ? '，检测站 ' + detectorResults.length + ' 个' : '') +
      (detectorErrorCount > 0 ? '，其中 ' + detectorErrorCount + ' 个打开失败' : ''),
    reportPath: artifacts.report,
    localProbePath: artifacts.localProbe,
    detectorCount: detectorResults.length,
    detectorErrorCount,
    validationStatus: matrixResult.status,
    matrix: matrixResult.dimensions,
    artifacts,
  }
}`,
			Notes: "采集脚本只生成 probe/report 和可选检测站截图/HTML，不解析第三方站点分数，也不会把检测站结果作为 CI 阻断。建议先用 localProbeOnly=true 跑通 profile，再按需加入 BrowserScan、CreepJS 等 detectors。",
			Source: ScriptSource{
				Type: "builtin",
				URI:  "repo://backend/internal/automation/default_scripts.go",
				Ref:  "HEAD",
				Path: FingerprintAuditScriptID,
			},
		},
	}
}
