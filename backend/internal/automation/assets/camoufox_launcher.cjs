/* Camoufox launcher (Node)。
 *
 * 职责：
 *   1. 从 process.argv[2] 读取 payloadPath (JSON: CamoufoxLaunchConfig + runtimeDir)
 *   2. 用运行时 node_modules/playwright-core 的 firefox.launchServer 拉起 Camoufox 二进制
 *   3. 把 CAMOU_CONFIG_* 注入子进程环境
 *   4. 把 executable_path / firefox_user_prefs / proxy / args 透传给 Playwright
 *   5. 在 stdout 打印一行协议：
 *        CAMOUFOX_READY <wsEndpoint>
 *      或失败时退出 1
 *   6. 保持常驻直到收到 stdin "exit" 或进程被外部 kill
 *
 * 与 runner.cjs 保持同一调用协议：payloadPath = argv[2]，runtimeDir 含 playwright-core。
 * 与 Go 侧 backend/internal/browser/camoufox_config.go:CamoufoxLaunchConfig 字段一一对应。
 */
const fs = require('fs');
const path = require('path');

// 持有启动页连接，避免函数返回后被 GC/断连导致启动页上下文消失。
const startupResources = [];

function writeLine(stream, text) {
  return new Promise((resolve, reject) => {
    stream.write(text + '\n', (error) => {
      if (error) reject(error);
      else resolve();
    });
  });
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
  if (!playwright || !playwright.firefox || typeof playwright.firefox.launchServer !== 'function') {
    throw new Error('当前 Playwright runtime 缺少 firefox.launchServer 能力；请确认 automation runtime 已安装 playwright-core');
  }

  // CamoufoxLaunchConfig 字段
  const config = payload.config || {};
  const executablePath = String(config.executablePath || '');
  if (!executablePath) {
    throw new Error('Camoufox executablePath is required');
  }
  const userDataDir = String(config.userDataDir || '');
  const headless = Boolean(config.headless);
  const proxy = config.proxy && typeof config.proxy === 'object' ? config.proxy : null;
  const startUrls = Array.isArray(config.startUrls) ? config.startUrls.filter(u => typeof u === 'string' && u) : [];
  const extraArgs = Array.isArray(config.extraArgs) ? config.extraArgs.filter(a => typeof a === 'string' && a.trim()) : [];
  const camouConfig = config.camouConfig && typeof config.camouConfig === 'object' ? config.camouConfig : {};
  const firefoxUserPrefs = config.firefoxUserPrefs && typeof config.firefoxUserPrefs === 'object' ? config.firefoxUserPrefs : {};
  const osName = String(config.osName || payload.osName || 'macos');

  // 持久化用户数据目录以 Firefox -profile 形式透传
  const args = [];
  if (userDataDir) {
    args.push('-profile', userDataDir);
  }
  args.push(...extraArgs);

  // 子进程环境：继承当前 + 注入 CAMOU_CONFIG_*
  const env = Object.assign({}, process.env);
  for (const key of Object.keys(camouConfig)) {
    if (/^CAMOU_CONFIG_\d+$/.test(key)) {
      env[key] = camouConfig[key];
    }
  }
  // Linux 下 Camoufox 需要 FONTCONFIG_PATH，macOS/Windows 无需。
  if (osName === 'linux' && payload.fontconfigPath) {
    env['FONTCONFIG_PATH'] = String(payload.fontconfigPath);
  }

  const launchOptions = {
    executablePath,
    headless,
    args,
    env,
    firefoxUserPrefs,
  };
  if (proxy && proxy.server) {
    launchOptions.proxy = { server: proxy.server };
    if (proxy.username) launchOptions.proxy.username = String(proxy.username);
    if (proxy.password) launchOptions.proxy.password = String(proxy.password);
  }

  const server = await playwright.firefox.launchServer(launchOptions);
  const wsEndpoint = String(server.wsEndpoint() || '');
  if (!wsEndpoint) {
    try { await server.close(); } catch (_) {}
    throw new Error('Camoufox launchServer returned empty wsEndpoint');
  }

  // 协议一行：CAMOUFOX_READY <wsEndpoint>
  await writeLine(process.stdout, `CAMOUFOX_READY ${wsEndpoint}`);

  // 可选：若有 startUrls，逐个打开页面；失败不影响 wsEndpoint 已就绪。
  if (startUrls.length > 0) {
    try {
      const browser = await playwright.firefox.connect(wsEndpoint);
      const context = await browser.newContext();
      startupResources.push({ browser, context });
      for (const startUrl of startUrls) {
        const page = await context.newPage();
        startupResources.push({ page });
        await page.goto(startUrl, { waitUntil: 'domcontentloaded', timeout: 60000 }).catch(() => {});
      }
    } catch (_) {
      // 启动 URL 失败不影响 wsEndpoint 已就绪
    }
  }

  // 常驻直到外部 kill 或 stdin 收到 exit
  process.stdin.setEncoding('utf8');
  process.stdin.resume();
  process.stdin.on('data', (chunk) => {
    const text = String(chunk || '').trim();
    if (text === 'exit' || text === 'close') {
      cleanup(0);
    }
  });

  // 子进程意外退出时也要清理
  server.on('close', () => cleanup(0));

  process.on('SIGTERM', () => cleanup(0));
  process.on('SIGINT', () => cleanup(130));

  let cleaned = false;
  async function cleanup(code) {
    if (cleaned) return;
    cleaned = true;
    try { await server.close(); } catch (_) {}
    process.exit(typeof code === 'number' ? code : 0);
  }
}

main().catch(async (error) => {
  const message = error && error.message ? error.message : String(error);
  // Go 侧从 stdout 解析协议行；stderr 同步写一份便于日志保留。
  await writeLine(process.stdout, `CAMOUFOX_FAIL ${message}`).catch(() => {});
  await writeLine(process.stderr, `CAMOUFOX_FAIL ${message}`).catch(() => {});
  process.exit(1);
});
