const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(__dirname, "../assets");

async function pressSettings(page, selector) {
  if (selector.startsWith("#setTab-")) selector += `:visible, .nav-sub:not(.leaving) button[data-of="${selector.slice(1)}"]`;
  const control = page.locator(selector), target = await control.boundingBox(), pane = await page.locator("#view-settings").boundingBox();
  if (target.y < pane.y + 4 || target.y + target.height > pane.y + pane.height - 12) {
    await page.mouse.move(pane.x + 3, pane.y + pane.height / 2);
    await page.mouse.wheel(0, target.y - pane.y - pane.height / 2);
    await page.waitForTimeout(300);
  }
  await control.click();
}

function fixture(lang, { native = false, theme = "light" } = {}) {
  const defaults = { schemaVersion: 1, appName: "Acme Assistant", logoAsset: "", visibleProviders: [], showPluginProviderEntry: false, showCustomProviderEntry: false, publicPluginCatalog: false, libraryEnabled: false };
  let enterprise = { config: null, effective: { ...defaults }, defaults, revision: "one", logoUrl: "", restartRequired: [] };
  const asked = [], errors = [], posts = [];
  const candidates = [
    { id: "subscription:copilot", name: "Copilot", default: true, available: true },
    { id: "preset:ollama", name: "Ollama", default: true, available: true },
    { id: "preset:deepseek", name: "DeepSeek", available: true },
    { id: "preset:ollama-cloud", name: "Ollama Cloud", available: true },
    { id: "preset:qwen-cn", name: "Qwen CN", available: true },
  ];
  const settings = { theme, lang, tray: "panel", currency: "usd", textSize: 100, version: "1.0.0", visionModels: [], imageGenModels: [], workbuddyCheckins: [], lanURLs: [], fx: { rate: 7.2, at: new Date().toISOString(), stale: false } };
  const presets = [
    { id: "ollama", name: "Ollama", icon: "ollama", kind: "local", chat: "http://127.0.0.1:11434/v1", models: [], protocols: ["chat"] },
    { id: "ollama-cloud", name: "Ollama Cloud", icon: "ollama", kind: "vendor", chat: "https://cloud.invalid/v1", models: [], protocols: ["chat"] },
    { id: "deepseek", name: "DeepSeek", icon: "deepseek-color", kind: "vendor", chat: "https://deepseek.invalid/v1", models: [], protocols: ["chat"] },
    { id: "qwen", name: "Qwen", icon: "qwen-color", kind: "vendor", chat: "https://global.invalid/v1", models: [], protocols: ["chat"] },
    { id: "qwen-cn", name: "Qwen CN", icon: "qwen-color", kind: "vendor", chat: "https://china.invalid/v1", models: [], protocols: ["chat"] },
  ];
  const serve = async (route) => {
    const request = route.request(), url = new URL(request.url()); asked.push(url.pathname);
    const json = (value) => route.fulfill({ json: value });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme, web: !native, enterprise })};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/enterprise") {
      if (request.method() === "POST") {
        const body = request.postDataJSON(); posts.push(body);
        if (body.revision !== enterprise.revision) return route.fulfill({ status: 409, json: { error: "config conflict", code: "config_conflict" } });
        enterprise = { ...enterprise, config: body.config, effective: body.config ? { ...defaults, ...body.config } : { ...defaults }, revision: "revision-" + posts.length };
        enterprise.restartRequired = native && enterprise.effective.appName !== defaults.appName ? ["Native application menu entries"] : [];
      }
      return json(enterprise);
    }
    if (url.pathname === "/api/enterprise/providers") return json(candidates);
    if (url.pathname === "/api/providers") return json({ providers: [], presets, excluded: [], plugins: [], gateway: { running: true, url: "http://127.0.0.1:3999", groups: [], calls: [] } });
    if (url.pathname === "/api/plugins") return json({ plugins: [], bun: true, picker: false });
    if (url.pathname === "/api/plugins/builtin") return json([]);
    if (url.pathname === "/api/application/restart") return route.fulfill({ status: 409, json: { error: "wait for running gateway requests before restarting" } });
    if (url.pathname === "/api/plugins/add") { posts.push(request.postDataJSON()); return json({ plugins: [], bun: true }); }
    if (url.pathname === "/api/plugins/listings") return json({ listings: [] });
    if (url.pathname === "/api/plugins/github") return json({ repos: [] });
    if (url.pathname === "/api/plugins/search") return json({ hits: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/test-logo.png") return route.fulfill({ contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64") });
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    try { return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] }); }
    catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return { serve, asked, errors, posts, candidates, get: () => enterprise, set: (next) => { enterprise = next; } };
}

for (const [engineName, engine] of [["chromium", chromium], ["webkit", webkit]]) {
  test(`${engineName}: built-in plugins respect both enterprise policy and feature switches`, async () => {
    const browser = await engine.launch();
    try {
      const page = await browser.newPage({ viewport: { width: 1100, height: 900 } }), f = fixture("en");
      page.on("pageerror", e => f.errors.push(e.message));
      await page.route("**/*", f.serve);
      await page.goto("http://enterprise.invalid/?view=plugins");
      await page.locator(".pm-manual .pm-minput").waitFor();
      await page.getByRole("tab", { name: "Built-in", exact: true }).click();
      await page.locator("#builtinList").waitFor({ state: "attached" });
      assert.equal(await page.locator("#view-plugins .pm-find").isVisible(), false);
      await page.evaluate(() => applyFeatures([{ id: "library", on: true }]));
      const library = page.locator('#nav button[data-view="library"]');
      assert.equal(await library.isVisible(), false, "a built-in reconciliation cannot reopen the enterprise library");
      assert.equal(f.asked.some(p => ["/api/plugins/listings", "/api/plugins/github", "/api/plugins/search"].includes(p)), false);
      const next = { ...f.get(), effective: { ...f.get().effective, libraryEnabled: true }, revision: "library-enabled" };
      f.set(next);
      await page.evaluate(next => window.receiveEnterprise(next), next);
      assert.equal(await library.isVisible(), true);
      await page.evaluate(() => applyFeatures([{ id: "library", on: false }]));
      await page.evaluate(next => window.receiveEnterprise(next), next);
      assert.equal(await library.isVisible(), false, "enterprise polling cannot reopen a disabled built-in page");
      assert.deepEqual(f.errors, []);
    } finally { await browser.close(); }
  });

  test(`${engineName}: enterprise defaults, save, provider projection, top install, no discovery, drafts and narrow layout`, async () => {
    const browser = await engine.launch();
    try {
      for (const [lang, theme] of ["en", "zh", "zh-TW", "ja", "de"].flatMap(lang => ["light", "dark"].map(theme => [lang, theme]))) {
        const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, colorScheme: theme });
        const page = await ctx.newPage(), f = fixture(lang, { theme });
        page.on("pageerror", (e) => f.errors.push(e.message));
        await page.route("**/*", f.serve);
        await page.goto("http://enterprise.invalid/?view=settings&tab=enterprise");
        await page.locator("#enterprise-appName").waitFor();
        assert.equal(await page.locator("#enterprise-appName").evaluate(e => {
          const reference = document.createElement("span"); reference.style.background = "var(--card)"; e.parentElement.append(reference);
          const expected = getComputedStyle(reference).backgroundColor; reference.remove();
          return getComputedStyle(e).backgroundColor === expected;
        }), true, `${theme}: text inputs use the app surface, not a white native bezel`);
        assert.equal(await page.title(), "Acme Assistant");
        assert.equal(await page.locator('#nav [data-view="library"]').isVisible(), false);
        assert.equal(await page.locator('input[value="subscription:copilot"]').isDisabled(), true);
        assert.equal(await page.locator('input[value="preset:ollama"]').isDisabled(), true);
        await page.locator('[data-selection="visibleProviders"] .fold').click();
        await page.locator("#enterprise-providerSearch").fill("Deep");
        await page.locator("#enterprise-providerSearch").focus();
        await page.evaluate(() => window.enterpriseUI.refresh());
        assert.equal(await page.locator("#enterprise-providerSearch").evaluate(e => document.activeElement === e), true, "unchanged polls preserve search focus");
        await page.locator("#enterprise-providerSearch").fill("");
        await page.locator("#enterprise-appName").fill("Acme <New>");
        await page.locator('input[value="preset:deepseek"]').check();
        await page.locator('input[value="preset:qwen-cn"]').check();
        await pressSettings(page, '#setPage-enterprise button[type="submit"]');
        await page.waitForFunction(() => document.title === "Acme <New>");
        assert.equal(await page.locator(".brand > span:last-child").textContent(), "Acme <New>");
        assert.equal(await page.locator(".brand img").count(), 0, "name is text, not HTML");
        await page.locator('#nav [data-view="providers"]').click();
        await page.locator('#addSheet [data-pick="Ollama"]').waitFor();
        assert.equal(await page.locator('#addSheet [data-pick="DeepSeek"]').isVisible(), true);
        assert.equal(await page.locator('#addSheet [data-pick="Qwen CN"]').isVisible(), true, "a China-only selection is visible without its global parent");
        assert.equal(await page.locator('#addSheet [data-pick="Qwen"]').count(), 0, "an unselected global endpoint stays hidden");
        assert.equal(await page.locator('#addSheet [data-pick="Ollama Cloud"]').count(), 0);
        assert.equal(await page.locator('#addSheet [data-pick="custom"]').count(), 0);
        await page.locator('#nav [data-view="plugins"]').click();
        await page.locator(".pm-manual .pm-minput").waitFor();
        assert.equal(await page.locator("#view-plugins").evaluate((e) => e.firstElementChild.classList.contains("pm-manual")), true);
        await page.locator(".pm-manual .pm-minput").fill("@acme/plugin");
        await page.evaluate(() => window.redrawEnterprisePlugins());
        assert.equal(await page.locator(".pm-manual .pm-minput").inputValue(), "@acme/plugin");
        const installed = page.waitForResponse(r => new URL(r.url()).pathname === "/api/plugins/add");
        await page.locator('.pm-manual button[type="submit"]').click();
        await installed;
        assert.equal(f.posts.some(p => p.spec === "@acme/plugin"), true, "closed discovery retains explicit installation");
        assert.equal(f.asked.some((p) => ["/api/plugins/listings", "/api/plugins/github", "/api/plugins/search", "/api/library"].includes(p)), false);
        await page.locator("#prefs").click();
        await pressSettings(page, "#setTab-enterprise");
        await page.locator("#enterprise-appName").fill("Kept draft");
        const other = { ...f.get(), effective: { ...f.get().effective, appName: "Other window" }, revision: "other" };
        f.set(other);
        await page.evaluate((s) => window.receiveEnterprise(s), other);
        assert.equal(await page.locator("#enterprise-appName").inputValue(), "Kept draft");
        assert.equal(await page.title(), "Other window");
        await pressSettings(page, '#setPage-enterprise button[type="submit"]');
        await page.waitForFunction(() => document.querySelector("#enterpriseResult").textContent !== "" && document.querySelector("#enterpriseResult").textContent !== t("Unsaved changes"));
        assert.equal(await page.locator("#enterprise-appName").inputValue(), "Kept draft", "conflicts preserve the draft");
        await page.setViewportSize({ width: 440, height: 760 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        assert.equal(await page.locator("#setPage-enterprise select").count(), 0);
        if (process.env.ARTIFACT_DIR && lang === "zh") {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `enterprise-${engineName}-${theme}.png`) });
        }
        assert.deepEqual(f.errors, []);
        await ctx.close();
      }
    } finally { await browser.close(); }
  });

  test(`${engineName}: native restart reminder survives redraw and package reset`, async () => {
    const browser = await engine.launch();
    try {
      const page = await browser.newPage(), f = fixture("en", { native: true });
      page.on("pageerror", e => f.errors.push(e.message));
      await page.route("**/*", f.serve);
      await page.goto("http://enterprise.invalid/?view=settings&tab=enterprise");
      await page.locator("#enterprise-appName").fill("Acme Runtime");
      await page.locator('#setPage-enterprise button[type="submit"]').click();
      const restart = page.getByRole("button", { name: "Restart now", exact: true });
      await restart.waitFor();
      await restart.click();
      await page.waitForFunction(() => document.querySelector("#status").textContent.includes("running gateway"));
      assert.equal(await page.title(), "Acme Runtime", "busy restart retains saved branding");
      await page.getByRole("button", { name: "Apply on next launch", exact: true }).click();
      assert.equal(await restart.count(), 0);
      await page.getByRole("button", { name: "Restore package defaults", exact: true }).click();
      await page.waitForFunction(() => document.title === "Acme Assistant");
      assert.equal(await restart.count(), 0);
      assert.deepEqual(f.errors, []);
    } finally { await browser.close(); }
  });

  test(`${engineName}: old configuration reads cannot undo a live update and candidates refresh`, async () => {
    const browser = await engine.launch();
    try {
      const page = await browser.newPage(), f = fixture("en");
      page.on("pageerror", e => f.errors.push(e.message));
      await page.route("**/*", f.serve);
      await page.goto("http://enterprise.invalid/?view=settings&tab=enterprise");
      await page.locator("#enterprise-appName").waitFor();
      let release, started;
      const waiting = new Promise(resolve => { started = resolve; });
      const hold = new Promise(resolve => { release = resolve; });
      const stale = f.get();
      await page.route("**/api/enterprise", async route => {
        started(); await hold; await route.fulfill({ json: stale });
      }, { times: 1 });
      await page.evaluate(() => { window.enterpriseUI.refresh(); });
      await waiting;
      const latest = { ...stale, effective: { ...stale.effective, appName: "Latest enterprise" }, revision: "latest" };
      f.set(latest);
      await page.evaluate(next => window.receiveEnterprise(next), latest);
      release();
      await page.waitForFunction(() => document.title === "Latest enterprise");
      await page.waitForTimeout(100);
      assert.equal(await page.locator("#enterprise-appName").inputValue(), "Latest enterprise");
      f.candidates.push({ id: "plugin:new-plugin", name: "New plugin", available: true });
      await page.evaluate(() => window.enterpriseUI.load());
      assert.equal(await page.locator('input[value="plugin:new-plugin"]').count(), 1, "installed plugins become selectable without restarting");
      const branded = { ...f.get(), logoUrl: "/test-logo.png", revision: "logo-update" };
      f.set(branded); await page.evaluate(next => window.receiveEnterprise(next), branded);
      assert.equal(await page.locator('.rt-hub .ic[data-icon="magpie"] img').getAttribute("src"), "/test-logo.png", "routing hub receives the current logo");
      await page.locator("#enterprise-publicPluginCatalog").check();
      await page.locator("#enterprise-libraryEnabled").check();
      await page.locator('#setPage-enterprise button[type="submit"]').click();
      await page.waitForFunction(() => window.enterpriseUI.catalog());
      assert.equal(await page.locator('#nav [data-view="library"]').isVisible(), true);
      const discovery = page.waitForResponse(r => new URL(r.url()).pathname === "/api/plugins/listings");
      await page.locator('#nav [data-view="plugins"]').click();
      await discovery;
      await page.waitForFunction(() => document.querySelector(".pm-manual .pm-minput"));
      assert.equal(f.asked.includes("/api/plugins/listings"), true, "opening catalog restores discovery");
      await page.locator("#prefs").click(); await pressSettings(page, "#setTab-enterprise");
      await page.locator("#enterprise-publicPluginCatalog").uncheck();
      await page.locator("#enterprise-libraryEnabled").uncheck();
      await page.locator('#setPage-enterprise button[type="submit"]').click();
      await page.waitForFunction(() => !window.enterpriseUI.catalog());
      assert.equal(await page.locator('#nav [data-view="library"]').isVisible(), false);
      assert.deepEqual(f.errors, []);
    } finally { await browser.close(); }
  });
}
