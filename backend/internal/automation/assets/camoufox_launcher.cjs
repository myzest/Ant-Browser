/* Camoufox launcher (Node)。
 *
 * 职责：
 *   1. 从 process.argv[2] 读取 payloadPath (JSON: CamoufoxLaunchConfig + runtimeDir)
 *   2. 用 playwright-core.firefox.launchPersistentContext(userDataDir, options) 拉起 Camoufox
 *   3. 把 CAMOU_CONFIG_* 注入子进程环境
 *   4. 在 127.0.0.1 启动一个轻量 HTTP bridge，供 automation runner 控制 persistent context
 *   5. 在 stdout 打印一行协议：
 *        CAMOUFOX_READY <bridgeEndpoint>
 *      或失败时退出 1
 *   6. 保持常驻直到收到 stdin "exit" 或进程被外部 kill
 *
 * 与 runner.cjs 保持同一调用协议：payloadPath = argv[2]，runtimeDir 含 playwright-core。
 * 与 Go 侧 backend/internal/browser/camoufox_config.go:CamoufoxLaunchConfig 字段一一对应。
 */
const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const path = require('path');

function writeLine(stream, text) {
  return new Promise((resolve, reject) => {
    stream.write(text + '\n', (error) => {
      if (error) reject(error);
      else resolve();
    });
  });
}

function sendJSON(res, status, body) {
  const text = JSON.stringify(body || {});
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(text),
  });
  res.end(text);
}

function readJSONBody(req, limit = 1024 * 1024) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > limit) {
        reject(new Error('request body too large'));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => {
      const raw = Buffer.concat(chunks).toString('utf8').trim();
      if (!raw) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(raw));
      } catch (error) {
        reject(new Error(`invalid json body: ${error.message || error}`));
      }
    });
    req.on('error', reject);
  });
}

function listen(server, host, port) {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, host, () => {
      server.off('error', reject);
      resolve(server.address());
    });
  });
}

function closeServer(server) {
  if (!server || !server.listening) return Promise.resolve();
  return new Promise((resolve) => server.close(() => resolve()));
}

function evaluatePageFunction(source) {
  const text = String(source || '').trim();
  if (!text) {
    throw new Error('page function source is required');
  }
  // Source comes from local automation runner by Function.prototype.toString().
  // It mirrors Playwright's page.$$eval(pageFunction) boundary, not arbitrary web input.
  // eslint-disable-next-line no-new-func
  return new Function(`return (${text});`)();
}

function normalizeWaitForURLMatcher(raw) {
  if (!raw || typeof raw !== 'object') return raw;
  if (raw.type === 'regexp') {
    return new RegExp(String(raw.source || ''), String(raw.flags || ''));
  }
  if (raw.type === 'string') return String(raw.value || '');
  return raw.value;
}

async function createCamoufoxBridge(context) {
  const token = crypto.randomBytes(18).toString('base64url');
  const pageIds = new WeakMap();
  const pages = new Map();
  let nextPageID = 1;

  const rememberPage = (page) => {
    if (!page) return null;
    let id = pageIds.get(page);
    if (!id) {
      id = `page-${nextPageID++}`;
      pageIds.set(page, id);
      pages.set(id, page);
      page.on('close', () => pages.delete(id));
    }
    return id;
  };

  const serializePage = (page) => {
    if (!page) return null;
    const id = rememberPage(page);
    let closed = false;
    try { closed = page.isClosed(); } catch (_) { closed = true; }
    let url = '';
    if (!closed) {
      try { url = page.url(); } catch (_) { url = ''; }
    }
    return { id, url, isClosed: closed };
  };

  const getPage = (pageID) => {
    const page = pages.get(String(pageID || ''));
    if (!page) throw new Error(`unknown page: ${pageID}`);
    return page;
  };

  const getLocator = (page, params) => {
    const selector = String(params && params.selector ? params.selector : '');
    if (!selector) throw new Error('locator selector is required');
    let locator = page.locator(selector);
    const nth = Number(params && params.nth);
    if (Number.isFinite(nth) && nth >= 0 && typeof locator.nth === 'function') {
      locator = locator.nth(Math.floor(nth));
    }
    return locator;
  };

  const rpc = async (method, params) => {
    switch (method) {
      case 'context.pages':
        return { pages: context.pages().map(serializePage).filter(Boolean) };
      case 'context.newPage': {
        const page = await context.newPage();
        return { page: serializePage(page) };
      }
      case 'page.goto': {
        const page = getPage(params.pageId);
        await page.goto(String(params.url || ''), params.options || {});
        return { page: serializePage(page) };
      }
      case 'page.waitForSelector': {
        const page = getPage(params.pageId);
        await page.waitForSelector(String(params.selector || ''), params.options || {});
        return { page: serializePage(page) };
      }
      case 'page.waitForTimeout': {
        const page = getPage(params.pageId);
        await page.waitForTimeout(Number(params.ms || 0));
        return { page: serializePage(page) };
      }
      case 'page.waitForURL': {
        const page = getPage(params.pageId);
        await page.waitForURL(normalizeWaitForURLMatcher(params.matcher), params.options || {});
        return { page: serializePage(page) };
      }
      case 'page.title': {
        const page = getPage(params.pageId);
        return { value: await page.title(), page: serializePage(page) };
      }
      case 'page.screenshot': {
        const page = getPage(params.pageId);
        const result = await page.screenshot(params.options || {});
        return { value: Buffer.isBuffer(result) ? result.toString('base64') : result, page: serializePage(page) };
      }
      case 'page.close': {
        const page = getPage(params.pageId);
        await page.close(params.options || {}).catch(() => {});
        return { page: serializePage(page) };
      }
      case 'page.$$eval': {
        const page = getPage(params.pageId);
        const fn = evaluatePageFunction(params.functionSource);
        const value = await page.$$eval(String(params.selector || ''), fn, params.arg);
        return { value, page: serializePage(page) };
      }
      case 'locator.fill': {
        const page = getPage(params.pageId);
        await getLocator(page, params).fill(String(params.value || ''), params.options || {});
        return { page: serializePage(page) };
      }
      case 'locator.waitFor': {
        const page = getPage(params.pageId);
        await getLocator(page, params).waitFor(params.options || {});
        return { page: serializePage(page) };
      }
      case 'locator.press': {
        const page = getPage(params.pageId);
        await getLocator(page, params).press(String(params.key || ''), params.options || {});
        return { page: serializePage(page) };
      }
      case 'locator.click': {
        const page = getPage(params.pageId);
        await getLocator(page, params).click(params.options || {});
        return { page: serializePage(page) };
      }
      default:
        throw new Error(`unsupported camoufox bridge method: ${method}`);
    }
  };

  const server = http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url || '/', 'http://127.0.0.1');
      const basePath = `/bridge/${token}`;
      if (!url.pathname.startsWith(basePath)) {
        sendJSON(res, 404, { ok: false, error: 'not found' });
        return;
      }
      if (req.method === 'GET' && url.pathname === `${basePath}/health`) {
        sendJSON(res, 200, { ok: true });
        return;
      }
      if (req.method !== 'POST' || url.pathname !== `${basePath}/rpc`) {
        sendJSON(res, 404, { ok: false, error: 'not found' });
        return;
      }
      const body = await readJSONBody(req);
      const method = String(body.method || '').trim();
      const result = await rpc(method, body.params && typeof body.params === 'object' ? body.params : {});
      sendJSON(res, 200, { ok: true, result });
    } catch (error) {
      sendJSON(res, 500, { ok: false, error: error && error.message ? error.message : String(error) });
    }
  });

  const address = await listen(server, '127.0.0.1', 0);
  const endpoint = `http://127.0.0.1:${address.port}/bridge/${token}`;
  return { endpoint, server, rememberPage };
}

async function main() {
  const payloadPath = process.argv[2];
  if (!payloadPath) {
    throw new Error('camoufox launcher payload path is required');
  }
  const payload = JSON.parse(fs.readFileSync(payloadPath, 'utf8'));
  const runtimeDir = path.resolve(String(payload.runtimeDir || ''));
  if (!runtimeDir) {
    throw new Error('runtimeDir is required');
  }

  const playwright = require(path.join(runtimeDir, 'node_modules', 'playwright-core'));
  if (!playwright || !playwright.firefox || typeof playwright.firefox.launchPersistentContext !== 'function') {
    throw new Error('当前 Playwright runtime 缺少 firefox.launchPersistentContext 能力；请确认 automation runtime 已安装 playwright-core');
  }

  const config = payload.config || {};
  const executablePath = String(config.executablePath || '');
  if (!executablePath) {
    throw new Error('Camoufox executablePath is required');
  }
  const userDataDir = String(config.userDataDir || '');
  if (!userDataDir) {
    throw new Error('Camoufox userDataDir is required for persistent context');
  }
  const headless = Boolean(config.headless);
  const proxy = config.proxy && typeof config.proxy === 'object' ? config.proxy : null;
  const startUrls = Array.isArray(config.startUrls) ? config.startUrls.filter(u => typeof u === 'string' && u) : [];
  const extraArgs = Array.isArray(config.extraArgs) ? config.extraArgs.filter(a => typeof a === 'string' && a.trim()) : [];
  const camouConfig = config.camouConfig && typeof config.camouConfig === 'object' ? config.camouConfig : {};
  const firefoxUserPrefs = config.firefoxUserPrefs && typeof config.firefoxUserPrefs === 'object' ? config.firefoxUserPrefs : {};
  const osName = String(config.osName || payload.osName || 'macos');

  const env = Object.assign({}, process.env);
  for (const key of Object.keys(camouConfig)) {
    if (/^CAMOU_CONFIG_\d+$/.test(key)) {
      env[key] = camouConfig[key];
    }
  }
  if (osName === 'linux' && payload.fontconfigPath) {
    env['FONTCONFIG_PATH'] = String(payload.fontconfigPath);
  }

  const launchOptions = {
    executablePath,
    headless,
    args: extraArgs,
    env,
    firefoxUserPrefs,
  };
  if (proxy && proxy.server) {
    launchOptions.proxy = { server: proxy.server };
    if (proxy.username) launchOptions.proxy.username = String(proxy.username);
    if (proxy.password) launchOptions.proxy.password = String(proxy.password);
  }

  const context = await playwright.firefox.launchPersistentContext(userDataDir, launchOptions);
  let bridge = null;
  try {
    bridge = await createCamoufoxBridge(context);

    const initialPages = context.pages();
    for (const page of initialPages) bridge.rememberPage(page);
    if (startUrls.length > 0) {
      for (let i = 0; i < startUrls.length; i++) {
        let page = i === 0 && initialPages[0] ? initialPages[0] : null;
        if (!page) {
          page = await context.newPage().catch(() => null);
        }
        if (!page) {
          continue;
        }
        bridge.rememberPage(page);
        page.goto(startUrls[i], { waitUntil: 'domcontentloaded', timeout: 60000 }).catch(() => {});
      }
    }

    await writeLine(process.stdout, `CAMOUFOX_READY ${bridge.endpoint}`);
  } catch (error) {
    try { await closeServer(bridge && bridge.server); } catch (_) {}
    try { await context.close(); } catch (_) {}
    throw error;
  }

  process.stdin.setEncoding('utf8');
  process.stdin.resume();
  process.stdin.on('data', (chunk) => {
    const text = String(chunk || '').trim();
    if (text === 'exit' || text === 'close') {
      cleanup(0);
    }
  });

  context.on('close', () => cleanup(0));
  process.on('SIGTERM', () => cleanup(0));
  process.on('SIGINT', () => cleanup(130));

  let cleaned = false;
  async function cleanup(code) {
    if (cleaned) return;
    cleaned = true;
    try { await closeServer(bridge.server); } catch (_) {}
    try { await context.close(); } catch (_) {}
    process.exit(typeof code === 'number' ? code : 0);
  }
}

main().catch(async (error) => {
  const message = error && error.message ? error.message : String(error);
  await writeLine(process.stdout, `CAMOUFOX_FAIL ${message}`).catch(() => {});
  await writeLine(process.stderr, `CAMOUFOX_FAIL ${message}`).catch(() => {});
  process.exit(1);
});
