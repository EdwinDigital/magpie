const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(__dirname, "../assets");
const white = path.resolve(__dirname, "../../enterprisebuild/testdata/microsoft-white-transparent.png");

function fixture(lang) {
  const base = { appName: "Acme", logoAsset: "sha256:" + "a".repeat(64), visibleProviders: [], publicPluginCatalog: false, libraryEnabled: false };
  const urls = n => ({ light: `/logo-${n}-light.png`, dark: `/logo-${n}-dark.png` });
  let enterprise = { effective: base, defaults: base, config: null, revision: "one", logoUrl: urls(0).light, logoUrls: urls(0), restartRequired: [] };
  let settings = { theme: "light", lang, version: "1.0.4", tray: "panel", textSize: 100, lanURLs: [], fx: { rate: 7.2 } };
  const ctl = { uploads: 0, saves: [], errors: [], hold: false, failed: false, release: () => {}, get: () => enterprise };
  ctl.waitUpload = () => new Promise(resolve => { ctl.started = resolve; });
  ctl.serve = async route => {
    const req = route.request(), url = new URL(req.url());
    const json = data => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme: settings.theme, web: true, enterprise })};` });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") {
      if (req.method() === "POST") settings = { ...settings, ...req.postDataJSON() };
      return json(settings);
    }
    if (url.pathname === "/api/enterprise/providers") return json([]);
    if (url.pathname === "/api/enterprise/logo") {
      const n = ++ctl.uploads;
      if (ctl.hold) {
        const wait = new Promise(resolve => { ctl.release = resolve; });
        ctl.started?.(); ctl.started = null; await wait;
      } else { ctl.started?.(); ctl.started = null; }
      if (ctl.failed) return route.fulfill({ status: 400, json: { error: "logo has no visible pixels" } });
      return json({ asset: "sha256:" + String(n).repeat(64), logoUrls: urls(n), url: urls(n).light });
    }
    if (url.pathname === "/api/enterprise") {
      if (req.method() === "POST") {
        const body = req.postDataJSON(); ctl.saves.push(body);
        const n = Number(body.config.logoAsset.slice(-1));
        enterprise = { ...enterprise, effective: body.config, config: body.config, revision: "save-" + ctl.saves.length, logoUrl: urls(n).light, logoUrls: urls(n) };
      }
      return json(enterprise);
    }
    if (url.pathname.startsWith("/logo-")) {
      const mode = url.pathname.includes("-dark") ? "dark" : "light";
      return route.fulfill({ contentType: "image/png", body: await fs.readFile(path.join(__dirname, "fixtures", `enterprise-logo-${mode}.png`)) });
    }
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    try { return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] }); }
    catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return ctl;
}

for (const [engineName, engine] of [["chromium", chromium], ["webkit", webkit]]) {
  test(`${engineName}: themed enterprise icons, normalized upload previews and cancelled upload safety`, async () => {
    const browser = await engine.launch();
    try {
      for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
        const context = await browser.newContext({ viewport: { width: 1100, height: 950 }, colorScheme: "light" });
        const page = await context.newPage(), f = fixture(lang);
        page.on("pageerror", e => f.errors.push(e.message));
        await page.route("**/*", f.serve);
        await page.goto("http://enterprise.test/?view=settings&tab=enterprise");
        await page.locator("#enterprise-appName").waitFor();
        assert.equal(await page.locator(".enterprise-theme-preview").count(), 2);
        assert.equal(await page.locator("header .logo img").getAttribute("src"), "/logo-0-light.png");
        await page.locator("#setTab-general").click();
        await page.locator("#themeSegs .opt").nth(2).click();
        await page.waitForFunction(() => document.querySelector("header .logo img")?.getAttribute("src").includes("dark"));
        assert((await page.locator('link[rel="icon"]').getAttribute("href")).includes("dark"));
        await page.locator("#themeSegs .opt").nth(0).click();
        await page.emulateMedia({ colorScheme: "dark" });
        await page.waitForFunction(() => !document.documentElement.dataset.theme && document.querySelector("header .logo img")?.getAttribute("src").includes("dark"));
        await page.emulateMedia({ colorScheme: "light" });
        await page.waitForFunction(() => document.querySelector("header .logo img")?.getAttribute("src").includes("light"));
        await page.locator("#setTab-enterprise").click();
        f.hold = true;
        const uploading = f.waitUpload();
        await page.locator("#enterprise-logo").setInputFiles(white); await uploading;
        assert.equal(await page.locator('#setPage-enterprise button[type="submit"]').isDisabled(), true);
        await page.locator("#enterprise-appName").fill("Kept while preparing");
        await page.locator("#enterprise-appName").focus();
        f.release();
        await page.waitForFunction(() => document.querySelector('[data-logo-theme="light"] img')?.getAttribute("src") === "/logo-1-light.png");
        assert.equal(await page.locator("#enterprise-appName").evaluate(e => document.activeElement === e), true);
        assert.equal(f.saves.length, 0, "preparing previews does not save enterprise preferences");
        assert.equal(await page.locator("header .logo img").getAttribute("src"), "/logo-0-light.png", "unsaved upload does not change the active brand");
        await page.locator('#setPage-enterprise button[type="submit"]').click();
        await page.waitForFunction(() => document.title === "Kept while preparing");
        assert.equal(f.saves[0].config.logoAsset, "sha256:" + "1".repeat(64));
        const second = f.waitUpload();
        await page.locator("#enterprise-logo").setInputFiles(white); await second;
        await page.locator("#setPage-enterprise .enterprise-actions button").nth(1).click();
        const released = page.waitForResponse(r => new URL(r.url()).pathname === "/api/enterprise/logo");
        f.release(); const response = await released; await response.finished();
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        assert.equal(await page.locator('[data-logo-theme="light"] img').getAttribute("src"), "/logo-1-light.png", "late response cannot replace the cancelled preview");
        await page.locator('#setPage-enterprise button[type="submit"]').click();
        await page.waitForFunction(() => !document.querySelector('#setPage-enterprise button[type="submit"]')?.disabled);
        assert.equal(f.saves.at(-1).config.logoAsset, "sha256:" + "1".repeat(64), "late upload cannot replace the cancelled draft");
        f.hold = false; f.failed = true;
        await page.locator("#enterprise-logo").setInputFiles(white);
        await page.waitForFunction(() => document.querySelector("#enterpriseResult")?.textContent === t("logo has no visible pixels"));
        assert.equal(await page.locator("header .logo img").getAttribute("src"), "/logo-1-light.png");
        await page.setViewportSize({ width: 440, height: 760 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        if (process.env.ARTIFACT_DIR && lang === "zh") {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `enterprise-theme-icons-${engineName}.png`) });
        }
        assert.deepEqual(f.errors, []);
        await context.close();
      }
    } finally { await browser.close(); }
  });
}
