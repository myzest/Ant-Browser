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

function clampNumber(value, min, max, fallback) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  return Math.max(min, Math.min(max, parsed));
}

function createSeededRandom(seedText) {
  let seed = 2166136261;
  const text = String(seedText || 'ant-humanize');
  for (let i = 0; i < text.length; i += 1) {
    seed ^= text.charCodeAt(i);
    seed = Math.imul(seed, 16777619);
  }
  return () => {
    seed += 0x6d2b79f5;
    let value = seed;
    value = Math.imul(value ^ (value >>> 15), value | 1);
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61);
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
  };
}

function createHumanHelpers(log) {
  const presets = {
    default: { moveSteps: 24, moveDelayMs: 8, jitter: 3, overshoot: 0.18, typeDelayMs: 65, scrollSteps: 10, scrollDelayMs: 35 },
    careful: { moveSteps: 38, moveDelayMs: 14, jitter: 2, overshoot: 0.1, typeDelayMs: 110, scrollSteps: 16, scrollDelayMs: 55 },
    fast: { moveSteps: 14, moveDelayMs: 4, jitter: 4, overshoot: 0.22, typeDelayMs: 35, scrollSteps: 7, scrollDelayMs: 20 },
  };
  const random = createSeededRandom(`${Date.now()}-${process.pid}`);
  const resolvePreset = (options = {}) => {
    const presetName = String(options.preset || 'default').trim();
    const base = presets[presetName] || presets.default;
    return {
      ...base,
      ...options,
      moveSteps: clampNumber(options.moveSteps, 4, 120, base.moveSteps),
      moveDelayMs: clampNumber(options.moveDelayMs, 0, 200, base.moveDelayMs),
      jitter: clampNumber(options.jitter, 0, 40, base.jitter),
      overshoot: clampNumber(options.overshoot, 0, 0.8, base.overshoot),
      typeDelayMs: clampNumber(options.typeDelayMs, 0, 1000, base.typeDelayMs),
      scrollSteps: clampNumber(options.scrollSteps, 1, 120, base.scrollSteps),
      scrollDelayMs: clampNumber(options.scrollDelayMs, 0, 500, base.scrollDelayMs),
    };
  };
  const centerOf = async (locator, timeoutMs) => {
    if (!locator) {
      throw new Error('human action requires a locator or element handle');
    }
    if (typeof locator.waitFor === 'function') {
      await locator.waitFor({ state: 'visible', timeout: timeoutMs }).catch(() => {});
    }
    if (typeof locator.scrollIntoViewIfNeeded === 'function') {
      await locator.scrollIntoViewIfNeeded({ timeout: timeoutMs }).catch(() => {});
    }
    const box = typeof locator.boundingBox === 'function' ? await locator.boundingBox() : null;
    if (!box || box.width <= 0 || box.height <= 0) {
      throw new Error('target is not visible or has no bounding box');
    }
    return {
      x: box.x + box.width / 2,
      y: box.y + box.height / 2,
      box,
    };
  };
  const moveMouse = async (page, target, options = {}) => {
    const cfg = resolvePreset(options);
    if (!page || !page.mouse) {
      throw new Error('human mouse action requires a Playwright page');
    }
    const steps = Math.round(cfg.moveSteps);
    const start = {
      x: target.x - 80 + random() * 160,
      y: target.y - 60 + random() * 120,
    };
    const overshoot = {
      x: target.x + (random() - 0.5) * cfg.overshoot * Math.max(20, target.box ? target.box.width : 80),
      y: target.y + (random() - 0.5) * cfg.overshoot * Math.max(20, target.box ? target.box.height : 50),
    };
    await page.mouse.move(start.x, start.y);
    for (let i = 1; i <= steps; i += 1) {
      const t = i / steps;
      const ease = t * t * (3 - 2 * t);
      const midWeight = Math.sin(Math.PI * t);
      const x = start.x + (target.x - start.x) * ease + (overshoot.x - target.x) * midWeight + (random() - 0.5) * cfg.jitter;
      const y = start.y + (target.y - start.y) * ease + (overshoot.y - target.y) * midWeight + (random() - 0.5) * cfg.jitter;
      await page.mouse.move(x, y);
      if (cfg.moveDelayMs > 0) {
        await sleep(cfg.moveDelayMs + Math.round(random() * cfg.moveDelayMs));
      }
    }
    await page.mouse.move(target.x, target.y);
  };
  const click = async (locator, options = {}) => {
    const cfg = resolvePreset(options);
    const timeoutMs = clampNumber(options.timeoutMs, 250, 60000, 10000);
    const target = await centerOf(locator, timeoutMs);
    const page = options.page || (typeof locator.page === 'function' ? locator.page() : null);
    if (!page || !page.mouse) {
      throw new Error('human.click requires options.page when the target does not expose page()');
    }
    await moveMouse(page, target, cfg);
    if (typeof locator.isEnabled === 'function') {
      const enabled = await locator.isEnabled({ timeout: timeoutMs }).catch(() => true);
      if (!enabled) {
        throw new Error('target is not enabled');
      }
    }
    await sleep(clampNumber(options.beforeClickDelayMs, 0, 2000, 80 + random() * 140));
    await page.mouse.down();
    await sleep(clampNumber(options.holdMs, 20, 1000, 45 + random() * 90));
    await page.mouse.up();
    if (typeof log === 'function') {
      log('human.click', { preset: options.preset || 'default' });
    }
  };
  const type = async (locator, text, options = {}) => {
    await click(locator, { ...options, beforeClickDelayMs: options.beforeClickDelayMs || 40 });
    const page = options.page || (typeof locator.page === 'function' ? locator.page() : null);
    if (!page || !page.keyboard) {
      throw new Error('human.type requires options.page when the target does not expose page()');
    }
    const cfg = resolvePreset(options);
    const value = String(text == null ? '' : text);
    for (const char of value) {
      await page.keyboard.type(char, { delay: Math.round(cfg.typeDelayMs + random() * cfg.typeDelayMs) });
      if (random() < 0.04) {
        await sleep(Math.round(cfg.typeDelayMs * (2 + random() * 3)));
      }
    }
    if (typeof log === 'function') {
      log('human.type', { preset: options.preset || 'default', length: value.length });
    }
  };
  const scroll = async (page, options = {}) => {
    if (!page || !page.mouse) {
      throw new Error('human scroll requires a Playwright page');
    }
    const cfg = resolvePreset(options);
    const direction = String(options.direction || 'down').toLowerCase() === 'up' ? -1 : 1;
    const total = clampNumber(options.distance, 80, 8000, 600);
    const steps = Math.round(cfg.scrollSteps);
    for (let i = 1; i <= steps; i += 1) {
      const t = i / steps;
      const weight = Math.sin(Math.PI * t) || 0.2;
      const delta = direction * (total / steps) * (0.45 + weight);
      await page.mouse.wheel(0, delta);
      if (cfg.scrollDelayMs > 0) {
        await sleep(cfg.scrollDelayMs + Math.round(random() * cfg.scrollDelayMs));
      }
    }
    if (typeof log === 'function') {
      log('human.scroll', { preset: options.preset || 'default', direction, distance: total });
    }
  };
  return { click, type, scroll, moveMouse };
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

function buildConnectEndpoints(payload, session) {
  const candidates = [];
  const seen = new Set();

  const pushCandidate = (value) => {
    const endpoint = normalizeEndpointCandidate(value);
    if (!endpoint || seen.has(endpoint)) {
      return;
    }
    seen.add(endpoint);
    candidates.push(endpoint);
  };

  pushCandidate(session && session.cdpUrl);

  const debugPort = Number(session && session.debugPort);
  if (Number.isFinite(debugPort) && debugPort > 0) {
    pushCandidate(`http://127.0.0.1:${Math.round(debugPort)}`);
  }

  pushCandidate(payload && payload.launchBaseUrl);
  return candidates;
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

  if (
    !Object.prototype.hasOwnProperty.call(launchOptions, 'code') &&
    typeof launchOptions.launchCode === 'string' &&
    launchOptions.launchCode.trim()
  ) {
    body.code = launchOptions.launchCode;
  }

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

  const stealthContext = normalizeStealthContextOptions(launchOptions);
  if (stealthContext) {
    body.stealthContext = stealthContext;
  }

  const selector =
    launchOptions.selector &&
    typeof launchOptions.selector === 'object' &&
    !Array.isArray(launchOptions.selector)
      ? launchOptions.selector
      : defaultSelector;
  if (selector && typeof selector === 'object' && !Array.isArray(selector) && Object.keys(selector).length > 0) {
    body.selector = normalizeLaunchSelectorPayload(selector);
  }

  return body;
}

function normalizeLaunchSelectorPayload(selector) {
  const normalized = { ...selector };
  if (!normalized.code && typeof normalized.launchCode === 'string' && normalized.launchCode.trim()) {
    normalized.code = normalized.launchCode;
  }
  delete normalized.launchCode;
  return normalized;
}

function hasOwn(object, key) {
  return Object.prototype.hasOwnProperty.call(object, key);
}

function normalizeStealthContextOptions(options) {
  const launchOptions = options && typeof options === 'object' ? options : {};
  const contextOptions =
    launchOptions.contextOptions && typeof launchOptions.contextOptions === 'object' && !Array.isArray(launchOptions.contextOptions)
      ? launchOptions.contextOptions
      : {};
  const stealthContext =
    launchOptions.stealthContext && typeof launchOptions.stealthContext === 'object' && !Array.isArray(launchOptions.stealthContext)
      ? { ...launchOptions.stealthContext }
      : {};

  if (!hasOwn(stealthContext, 'locale') && typeof contextOptions.locale === 'string') {
    stealthContext.locale = contextOptions.locale;
  }
  if (!hasOwn(stealthContext, 'timezone') && typeof contextOptions.timezoneId === 'string') {
    stealthContext.timezone = contextOptions.timezoneId;
  }
  if (!hasOwn(stealthContext, 'timezone') && typeof contextOptions.timezone === 'string') {
    stealthContext.timezone = contextOptions.timezone;
  }
  if (!hasOwn(stealthContext, 'userAgent') && typeof contextOptions.userAgent === 'string') {
    stealthContext.userAgent = contextOptions.userAgent;
  }
  if (!hasOwn(stealthContext, 'viewport') && contextOptions.viewport && typeof contextOptions.viewport === 'object') {
    stealthContext.viewport = contextOptions.viewport;
  }
  if (!hasOwn(stealthContext, 'noViewport') && typeof contextOptions.noViewport === 'boolean') {
    stealthContext.noViewport = contextOptions.noViewport;
  }

  if (Object.keys(stealthContext).length === 0) {
    return null;
  }
  return stealthContext;
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

async function runScriptTask(payload, chromium) {
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
    const launchBaseUrl = String(payload.launchBaseUrl || '').trim().replace(/\/$/, '');
    if (!launchBaseUrl) {
      throw new Error('launchBaseUrl is required when script calls launch()');
    }

    const response = await requestJSON(
      'POST',
      `${launchBaseUrl}/api/launch`,
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

  const connect = async (session = {}) => {
    const endpoints = buildConnectEndpoints(payload, session);
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
        if (remaining <= 0) {
          break;
        }

        try {
          const browser = await chromium.connectOverCDP(endpoint, {
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
              cdpUrl: endpoint,
            },
          };
        } catch (error) {
          lastError = error;
        }
      }

      if (Date.now() >= deadline) {
        break;
      }

      await sleep(Math.min(500, Math.max(100, deadline - Date.now())));
    }

    const lastMessage =
      lastError && lastError.message ? lastError.message : String(lastError || 'unknown error');
    throw new Error(
      `cdp endpoint is not ready after ${timeout} ms (endpoints: ${endpoints.join(', ')}): ${lastMessage}`
    );
  };

  const api = {
    chromium,
    launch,
    connect,
    selector,
    params,
    log,
    human: createHumanHelpers(log),
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

  const { chromium } = require(path.join(runtimeDir, 'node_modules', 'playwright-core'));
  const taskType = String(payload.taskType || 'script').trim() || 'script';
  if (taskType !== 'script') {
    throw new Error(`unsupported automation task type: ${taskType}`);
  }

  const result = await runScriptTask(payload, chromium);
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
