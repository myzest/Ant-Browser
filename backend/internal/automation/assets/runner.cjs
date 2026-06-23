const fs = require('fs');
const http = require('http');
const https = require('https');
const path = require('path');
const util = require('util');
const { pathToFileURL } = require('url');

const ALLOWED_WAIT_UNTIL = new Set(['load', 'domcontentloaded', 'networkidle', 'commit']);

function normalizeTimeout(value, fallback) {
  const parsed = Number(value);
  if (Number.isFinite(parsed) && parsed > 0) {
    return Math.round(parsed);
  }
  return fallback;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function writeStream(stream, text) {
  return new Promise((resolve, reject) => {
    stream.write(text, (error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}

async function closeBrowserConnection(browser) {
  if (!browser || typeof browser.close !== 'function') {
    return;
  }
  await browser.close({ reason: 'automation task finished' }).catch(() => {});
}

function normalizeEndpointCandidate(value) {
  const normalized = String(value || '').trim();
  if (!normalized) {
    return '';
  }

  try {
    const parsed = new URL(normalized);
    if (!['http:', 'https:', 'ws:', 'wss:'].includes(parsed.protocol)) {
      return '';
    }
    if (parsed.port === '0') {
      return '';
    }
    if ((parsed.protocol === 'http:' || parsed.protocol === 'https:') && (!parsed.pathname || parsed.pathname === '/') && !parsed.search && !parsed.hash) {
      return parsed.origin;
    }
    return parsed.toString();
  } catch {
    return '';
  }
}

function pushUniqueEndpoint(candidates, seen, value) {
  const endpoint = normalizeEndpointCandidate(value);
  if (!endpoint || seen.has(endpoint)) {
    return;
  }
  seen.add(endpoint);
  candidates.push(endpoint);
}

function buildCDPEndpoints(payload, session) {
  const candidates = [];
  const seen = new Set();
  pushUniqueEndpoint(candidates, seen, session && session.cdpUrl);
  const debugPort = Number(session && session.debugPort);
  if (Number.isFinite(debugPort) && debugPort > 0) {
    pushUniqueEndpoint(candidates, seen, `http://127.0.0.1:${Math.round(debugPort)}`);
  }
  pushUniqueEndpoint(candidates, seen, payload && payload.launchBaseUrl);
  return candidates;
}

function normalizePlaywrightEndpointCandidate(value) {
  const normalized = String(value || '').trim();
  if (!normalized) {
    return '';
  }
  try {
    const parsed = new URL(normalized);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      if (!parsed.pathname.includes('/bridge/')) {
        return '';
      }
      return parsed.toString().replace(/\/$/, '');
    }
    if (parsed.protocol !== 'ws:' && parsed.protocol !== 'wss:') {
      return '';
    }
    if (parsed.port === '0') {
      return '';
    }
    return parsed.toString();
  } catch {
    return '';
  }
}

function pushUniquePlaywrightEndpoint(candidates, seen, value) {
  const endpoint = normalizePlaywrightEndpointCandidate(value);
  if (!endpoint || seen.has(endpoint)) {
    return;
  }
  seen.add(endpoint);
  candidates.push(endpoint);
}

function buildPlaywrightEndpoints(_payload, session) {
  const candidates = [];
  const seen = new Set();
  pushUniquePlaywrightEndpoint(candidates, seen, session && session.playwrightWsEndpoint);
  pushUniquePlaywrightEndpoint(candidates, seen, session && session.playwrightEndpoint);
  pushUniquePlaywrightEndpoint(candidates, seen, session && session.runtimeEndpoint);
  return candidates;
}

function isCamoufoxBridgeEndpoint(endpoint) {
  try {
    const parsed = new URL(String(endpoint || ''));
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && parsed.pathname.includes('/bridge/');
  } catch {
    return false;
  }
}

function sessionRuntimeProtocol(session) {
  const protocol = String((session && session.runtimeProtocol) || '').trim().toLowerCase();
  return protocol === 'playwright' ? 'playwright' : 'cdp';
}

function normalizePathUnderRoot(rootDir, targetName) {
  const normalizedName = String(targetName || '').trim();
  const resolvedRoot = path.resolve(String(rootDir || ''));
  if (!resolvedRoot) {
    throw new Error('artifactDir is required');
  }

  const candidate = normalizedName ? path.resolve(resolvedRoot, normalizedName) : resolvedRoot;
  if (candidate !== resolvedRoot && !candidate.startsWith(`${resolvedRoot}${path.sep}`)) {
    throw new Error('artifact path escapes root directory');
  }
  return candidate;
}

async function requestJSON(method, requestURL, body, headers = {}) {
  const target = new URL(requestURL);
  const transport = target.protocol === 'https:' ? https : http;
  const payload = body == null ? '' : JSON.stringify(body);

  return await new Promise((resolve, reject) => {
    const req = transport.request(
      {
        protocol: target.protocol,
        hostname: target.hostname,
        port: target.port,
        path: `${target.pathname}${target.search}`,
        method,
        headers: {
          Accept: 'application/json',
          ...(payload
            ? {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(payload),
              }
            : {}),
          ...headers,
        },
      },
      (res) => {
        const chunks = [];
        res.on('data', (chunk) => chunks.push(chunk));
        res.on('end', () => {
          const rawText = Buffer.concat(chunks).toString('utf8').trim();
          let responseBody = {};
          if (rawText) {
            try {
              responseBody = JSON.parse(rawText);
            } catch {
              responseBody = { rawBody: rawText };
            }
          }
          resolve({
            status: res.statusCode || 0,
            body: responseBody,
          });
        });
      }
    );

    req.on('error', reject);
    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

function serializeWaitForURLMatcher(matcher) {
  if (matcher instanceof RegExp) {
    return { type: 'regexp', source: matcher.source, flags: matcher.flags };
  }
  return { type: 'string', value: String(matcher || '') };
}

class CamoufoxBridgeClient {
  constructor(endpoint) {
    this.endpoint = String(endpoint || '').replace(/\/$/, '');
  }

  async rpc(method, params = {}) {
    const response = await requestJSON('POST', `${this.endpoint}/rpc`, { method, params });
    if (!(response.status >= 200 && response.status < 300) || response.body.ok === false) {
      const errorText =
        (response.body && response.body.error && String(response.body.error).trim()) ||
        `camoufox bridge returned http ${response.status}`;
      throw new Error(errorText);
    }
    return response.body.result || {};
  }

  pageFromWire(wire) {
    if (!wire || !wire.id) return null;
    return new CamoufoxBridgePage(this, wire);
  }
}

class CamoufoxBridgeBrowser {
  constructor(client) {
    this._client = client;
    this._context = new CamoufoxBridgeContext(client);
  }

  contexts() {
    return [this._context];
  }

  async close() {
    // Persistent context 由 launcher 生命周期托管；automation task 结束时不关闭真实浏览器。
  }
}

class CamoufoxBridgeContext {
  constructor(client) {
    this._client = client;
    this._pages = new Map();
  }

  _pageFromWire(wire) {
    if (!wire || !wire.id) return null;
    const id = String(wire.id);
    const existing = this._pages.get(id);
    if (existing) {
      existing._update(wire);
      if (existing.isClosed()) {
        this._pages.delete(id);
      }
      return existing;
    }
    const page = this._client.pageFromWire(wire);
    if (page && !page.isClosed()) {
      this._pages.set(id, page);
    }
    return page;
  }

  async _wirePages() {
    const result = await this._client.rpc('context.pages');
    const wires = Array.isArray(result.pages) ? result.pages : [];
    const seen = new Set();
    const pages = [];
    for (const wire of wires) {
      const page = this._pageFromWire(wire);
      if (page && !page.isClosed()) {
        seen.add(page._id);
        pages.push(page);
      }
    }
    for (const id of Array.from(this._pages.keys())) {
      if (!seen.has(id)) {
        this._pages.delete(id);
      }
    }
    return pages;
  }

  pages() {
    return Array.from(this._pages.values()).filter((page) => !page.isClosed());
  }

  async newPage() {
    const result = await this._client.rpc('context.newPage');
    return this._pageFromWire(result.page);
  }
}

class CamoufoxBridgeLocator {
  constructor(page, selector, nth = undefined) {
    this._page = page;
    this._selector = selector;
    this._nth = nth;
  }

  first() {
    return new CamoufoxBridgeLocator(this._page, this._selector, 0);
  }

  nth(index) {
    return new CamoufoxBridgeLocator(this._page, this._selector, index);
  }

  _params(extra = {}) {
    const params = { pageId: this._page._id, selector: this._selector, ...extra };
    if (Number.isFinite(this._nth)) {
      params.nth = this._nth;
    }
    return params;
  }

  async fill(value, options = {}) {
    await this._page._client.rpc('locator.fill', this._params({ value, options }));
  }

  async waitFor(options = {}) {
    await this._page._client.rpc('locator.waitFor', this._params({ options }));
  }

  async press(key, options = {}) {
    await this._page._client.rpc('locator.press', this._params({ key, options }));
  }

  async click(options = {}) {
    await this._page._client.rpc('locator.click', this._params({ options }));
  }
}

class CamoufoxBridgePage {
  constructor(client, wire) {
    this._client = client;
    this._id = wire.id;
    this._url = wire.url || '';
    this._isClosed = Boolean(wire.isClosed);
  }

  _update(wire) {
    if (!wire) return;
    this._url = wire.url || this._url || '';
    this._isClosed = Boolean(wire.isClosed);
  }

  url() {
    return this._url || '';
  }

  isClosed() {
    return this._isClosed;
  }

  locator(selector) {
    return new CamoufoxBridgeLocator(this, selector);
  }

  async goto(url, options = {}) {
    const result = await this._client.rpc('page.goto', { pageId: this._id, url, options });
    this._update(result.page);
    return null;
  }

  async waitForSelector(selector, options = {}) {
    const result = await this._client.rpc('page.waitForSelector', { pageId: this._id, selector, options });
    this._update(result.page);
    return null;
  }

  async waitForTimeout(ms) {
    const result = await this._client.rpc('page.waitForTimeout', { pageId: this._id, ms });
    this._update(result.page);
  }

  async waitForURL(matcher, options = {}) {
    const result = await this._client.rpc('page.waitForURL', {
      pageId: this._id,
      matcher: serializeWaitForURLMatcher(matcher),
      options,
    });
    this._update(result.page);
  }

  async title() {
    const result = await this._client.rpc('page.title', { pageId: this._id });
    this._update(result.page);
    return result.value || '';
  }

  async screenshot(options = {}) {
    const result = await this._client.rpc('page.screenshot', { pageId: this._id, options });
    this._update(result.page);
    return result.value ? Buffer.from(result.value, 'base64') : Buffer.alloc(0);
  }

  async close(options = {}) {
    const result = await this._client.rpc('page.close', { pageId: this._id, options });
    this._update(result.page || { isClosed: true });
  }

  async $$eval(selector, pageFunction, arg) {
    const result = await this._client.rpc('page.$$eval', {
      pageId: this._id,
      selector,
      functionSource: typeof pageFunction === 'function' ? pageFunction.toString() : String(pageFunction || ''),
      arg,
    });
    this._update(result.page);
    return result.value;
  }
}

function inspectValue(value) {
  return util.inspect(value, {
    depth: 4,
    breakLength: 120,
    maxArrayLength: 20,
    compact: false,
  });
}

function toSerializable(value, seen = new WeakSet()) {
  if (value == null) {
    return value;
  }
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return value;
  }
  if (typeof value === 'bigint') {
    return value.toString();
  }
  if (value instanceof Date) {
    return value.toISOString();
  }
  if (value instanceof Error) {
    return {
      name: value.name,
      message: value.message,
      stack: value.stack,
    };
  }
  if (Buffer.isBuffer(value)) {
    return value.toString('utf8');
  }
  if (Array.isArray(value)) {
    return value.map((item) => toSerializable(item, seen));
  }
  if (typeof value === 'function') {
    return `[Function ${value.name || 'anonymous'}]`;
  }
  if (typeof value !== 'object') {
    return inspectValue(value);
  }
  if (seen.has(value)) {
    return '[Circular]';
  }
  seen.add(value);

  const prototype = Object.getPrototypeOf(value);
  if (prototype === Object.prototype || prototype === null) {
    const result = {};
    for (const [key, entry] of Object.entries(value)) {
      result[key] = toSerializable(entry, seen);
    }
    return result;
  }

  return inspectValue(value);
}

function buildLaunchRequestBody(defaultSelector, options) {
  const launchOptions = options && typeof options === 'object' ? options : {};
  const body = {};

  for (const key of [
    'code',
    'key',
    'profileId',
    'profileName',
    'keyword',
    'keywords',
    'tag',
    'tags',
    'groupId',
    'matchMode',
    'launchArgs',
    'startUrls',
    'skipDefaultStartUrls',
  ]) {
    if (Object.prototype.hasOwnProperty.call(launchOptions, key)) {
      body[key] = launchOptions[key];
    }
  }

  const selector =
    launchOptions.selector &&
    typeof launchOptions.selector === 'object' &&
    !Array.isArray(launchOptions.selector)
      ? launchOptions.selector
      : defaultSelector;
  if (selector && typeof selector === 'object' && !Array.isArray(selector) && Object.keys(selector).length > 0) {
    body.selector = selector;
  }

  return body;
}

async function loadScriptModule(scriptPath) {
  const resolvedPath = path.resolve(String(scriptPath || ''));
  if (!resolvedPath) {
    throw new Error('scriptPath is required');
  }

  let requiredModule = null;
  let requireError = null;
  try {
    requiredModule = require(resolvedPath);
  } catch (error) {
    requireError = error;
  }

  const imported = async () => {
    const moduleURL = pathToFileURL(resolvedPath).href;
    return await import(`${moduleURL}?t=${Date.now()}`);
  };

  if (requiredModule && typeof requiredModule.run === 'function') {
    return requiredModule;
  }
  if (typeof requiredModule === 'function') {
    return { run: requiredModule };
  }
  if (requiredModule && requiredModule.default && typeof requiredModule.default.run === 'function') {
    return requiredModule.default;
  }

  try {
    const importedModule = await imported();
    if (importedModule && typeof importedModule.run === 'function') {
      return importedModule;
    }
    if (importedModule && typeof importedModule.default === 'function') {
      return { run: importedModule.default };
    }
    if (
      importedModule &&
      importedModule.default &&
      typeof importedModule.default.run === 'function'
    ) {
      return importedModule.default;
    }
  } catch (importError) {
    if (requireError) {
      throw requireError;
    }
    throw importError;
  }

  if (requireError) {
    throw requireError;
  }
  throw new Error('script must export run()');
}

async function runScriptTask(payload, playwright) {
  const scriptModule = await loadScriptModule(payload.scriptPath);
  if (!scriptModule || typeof scriptModule.run !== 'function') {
    throw new Error('script must export run()');
  }

  const logs = [];
  const artifacts = [];
  const connectedBrowsers = new Set();
  const selector = payload.selector && typeof payload.selector === 'object' ? payload.selector : {};
  const params = payload.params && typeof payload.params === 'object' ? payload.params : {};
  const timeout = normalizeTimeout(params.timeoutMs, 30000);
  const startedAt = new Date().toISOString();

  const log = (...entries) => {
    logs.push({
      time: new Date().toISOString(),
      values: entries.map((entry) => toSerializable(entry)),
    });
  };

  const artifact = (name) => {
    const fileName = String(name || '').trim() || `artifact-${Date.now()}`;
    const targetPath = normalizePathUnderRoot(payload.artifactDir, fileName);
    fs.mkdirSync(path.dirname(targetPath), { recursive: true });
    artifacts.push(targetPath);
    return targetPath;
  };

  const launchHeaders = {};
  if (payload.launchAuthHeader && payload.launchAuthValue) {
    launchHeaders[payload.launchAuthHeader] = payload.launchAuthValue;
  }

  const launch = async (options = {}) => {
    const body = buildLaunchRequestBody(selector, options);

    const response = await requestJSON(
      'POST',
      `${String(payload.launchBaseUrl || '').replace(/\/$/, '')}/api/launch`,
      body,
      launchHeaders
    );

    if (!(response.status >= 200 && response.status < 300) || response.body.ok === false) {
      const errorText =
        (response.body && response.body.error && String(response.body.error).trim()) ||
        `launch api returned http ${response.status}`;
      throw new Error(errorText);
    }

    return response.body;
  };

  const connectCDP = async (session = {}) => {
    if (!playwright || !playwright.chromium || typeof playwright.chromium.connectOverCDP !== 'function') {
      throw new Error('当前 Playwright runtime 缺少 chromium.connectOverCDP 能力，无法通过 CDP 接管浏览器；请检查自动化运行时是否完整安装 chromium 浏览器');
    }
    const endpoints = buildCDPEndpoints(payload, session);
    if (endpoints.length === 0) {
      throw new Error(
        `launch session does not contain a valid cdp endpoint (cdpUrl=${String(
          session && session.cdpUrl ? session.cdpUrl : ''
        )}, debugPort=${String(session && session.debugPort ? session.debugPort : '')})`
      );
    }

    const deadline = Date.now() + timeout;
    let lastError = null;
    while (Date.now() <= deadline) {
      for (const endpoint of endpoints) {
        const remaining = deadline - Date.now();
        if (remaining <= 0) break;
        try {
          const browser = await playwright.chromium.connectOverCDP(endpoint, {
            timeout: Math.max(1000, Math.min(remaining, timeout)),
          });
          connectedBrowsers.add(browser);
          const context = browser.contexts()[0] || null;
          const page = context && context.pages().length > 0 ? context.pages()[0] : null;
          return { browser, context, page, session: { ...session, cdpUrl: endpoint, runtimeProtocol: 'cdp', runtimeEndpoint: endpoint } };
        } catch (error) {
          lastError = error;
        }
      }
      if (Date.now() >= deadline) break;
      await sleep(Math.min(500, Math.max(100, deadline - Date.now())));
    }
    const lastMessage = lastError && lastError.message ? lastError.message : String(lastError || 'unknown error');
    throw new Error(`cdp endpoint is not ready after ${timeout} ms (endpoints: ${endpoints.join(', ')}): ${lastMessage}`);
  };

  const connectPlaywright = async (session = {}) => {
    const endpoints = buildPlaywrightEndpoints(payload, session);
    if (endpoints.length === 0) {
      throw new Error(
        `launch session does not contain a valid playwright endpoint (runtimeEndpoint=${String(
          session && session.runtimeEndpoint ? session.runtimeEndpoint : ''
        )}, playwrightEndpoint=${String(session && session.playwrightEndpoint ? session.playwrightEndpoint : '')})`
      );
    }

    const deadline = Date.now() + timeout;
    let lastError = null;
    while (Date.now() <= deadline) {
      for (const endpoint of endpoints) {
        const remaining = deadline - Date.now();
        if (remaining <= 0) break;
        try {
          if (isCamoufoxBridgeEndpoint(endpoint)) {
            const client = new CamoufoxBridgeClient(endpoint);
            await client.rpc('context.pages');
            const browser = new CamoufoxBridgeBrowser(client);
            const context = browser.contexts()[0];
            const pages = await context._wirePages();
            const page = pages.length > 0 ? pages[0] : await context.newPage();
            return {
              browser,
              context,
              page,
              session: {
                ...session,
                runtimeProtocol: 'playwright',
                runtimeEndpoint: endpoint,
                playwrightEndpoint: endpoint,
                playwrightWsEndpoint: '',
              },
            };
          }
          if (!playwright || !playwright.firefox || typeof playwright.firefox.connect !== 'function') {
            throw new Error('当前 Playwright runtime 缺少 firefox.connect 能力，无法接管 Camoufox/Playwright 实例；请确认自动化运行时已安装 firefox 浏览器或切换为 CDP 协议脚本');
          }
          const browser = await playwright.firefox.connect(endpoint, {
            timeout: Math.max(1000, Math.min(remaining, timeout)),
          });
          connectedBrowsers.add(browser);
          const context = browser.contexts()[0] || null;
          const page = context && context.pages().length > 0 ? context.pages()[0] : null;
          return {
            browser,
            context,
            page,
            session: {
              ...session,
              runtimeProtocol: 'playwright',
              runtimeEndpoint: endpoint,
              playwrightEndpoint: endpoint,
              playwrightWsEndpoint: endpoint,
            },
          };
        } catch (error) {
          lastError = error;
        }
      }
      if (Date.now() >= deadline) break;
      await sleep(Math.min(500, Math.max(100, deadline - Date.now())));
    }
    const lastMessage = lastError && lastError.message ? lastError.message : String(lastError || 'unknown error');
    throw new Error(`playwright endpoint is not ready after ${timeout} ms (endpoints: ${endpoints.join(', ')}): ${lastMessage}`);
  };

  const connect = async (session = {}) => {
    if (sessionRuntimeProtocol(session) === 'playwright') {
      return await connectPlaywright(session);
    }
    return await connectCDP(session);
  };

  const api = {
    playwright,
    chromium: playwright.chromium,
    firefox: playwright.firefox,
    launch,
    connect,
    connectCDP,
    connectPlaywright,
    selector,
    params,
    log,
    artifact,
    artifactsDir: payload.artifactDir || '',
  };

  try {
    const rawResult = await scriptModule.run(api);
    const normalizedResult = toSerializable(rawResult);
    const ok = !(normalizedResult && typeof normalizedResult === 'object' && normalizedResult.ok === false);
    const summary =
      normalizedResult &&
      typeof normalizedResult === 'object' &&
      typeof normalizedResult.summary === 'string'
        ? normalizedResult.summary.trim()
        : ok
          ? '脚本执行完成'
          : '脚本执行失败';
    const error =
      normalizedResult &&
      typeof normalizedResult === 'object' &&
      typeof normalizedResult.error === 'string'
        ? normalizedResult.error.trim()
        : '';

    return {
      ok,
      summary,
      error,
      title:
        normalizedResult &&
        typeof normalizedResult === 'object' &&
        typeof normalizedResult.title === 'string'
          ? normalizedResult.title
          : '',
      url:
        normalizedResult &&
        typeof normalizedResult === 'object' &&
        typeof normalizedResult.url === 'string'
          ? normalizedResult.url
          : '',
      startedAt,
      finishedAt: new Date().toISOString(),
      isolatedPage: false,
      logs,
      artifacts: Array.from(new Set(artifacts)),
      result: normalizedResult,
    };
  } finally {
    await Promise.all(Array.from(connectedBrowsers, (browser) => closeBrowserConnection(browser)));
  }
}

async function main() {
  const payloadPath = process.argv[2];
  if (!payloadPath) {
    throw new Error('payload path is required');
  }

  const payload = JSON.parse(fs.readFileSync(payloadPath, 'utf8'));
  const runtimeDir = path.resolve(String(payload.runtimeDir || ''));
  if (!runtimeDir) {
    throw new Error('runtimeDir is required');
  }

  const playwright = require(path.join(runtimeDir, 'node_modules', 'playwright-core'));
  const taskType = String(payload.taskType || 'script').trim() || 'script';
  if (taskType !== 'script') {
    throw new Error(`unsupported automation task type: ${taskType}`);
  }

  const result = await runScriptTask(payload, playwright);
  await writeStream(process.stdout, JSON.stringify(result));
  process.exit(0);
}

main().catch(async (error) => {
  const message = error && error.message ? error.message : String(error);
  try {
    await writeStream(process.stderr, message);
  } finally {
    process.exit(1);
  }
});
