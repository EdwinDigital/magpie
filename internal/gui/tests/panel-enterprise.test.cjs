// Browser plugin not available; use isolated Playwright fixtures, not employee state.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = process.env.ASSET_DIR || path.resolve(__dirname, "../assets");

function fixture(lang, enterpriseOn) {
  const effective = { schemaVersion: 1, appName: "msai-client", visibleAgents: ["workbuddy"], visibleProviders: [], libraryEnabled: false, publicPluginCatalog: false };
  const enterprise = { effective, defaults: effective, revision: "one", restartRequired: [], updatePolicy: { mode: "manual" } };
  const settings = { lang, theme: lang === "zh-TW" ? "dark" : "light", textSize: 125, guideHidden: true };
  const agent = (id, name) => ({ id, name, icon: "generic", path: "/fixture/" + id, wired: true, fields: [{ key: "model", label: "model", value: "corp/test-model", options: [{ value: "corp/test-model", label: "Test Model" }] }] });
  const ctl = { enterprise, errors: [], requests: [], fitted: [], quotaReads: [] };
  ctl.serve = async route => {
    const req = route.request(), url = new URL(req.url());
    ctl.requests.push(url.pathname);
    const json = value => route.fulfill({ json: value });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: "window.bootPrefs=" + JSON.stringify({ ...settings, web: false, enterprise: enterpriseOn ? enterprise : null }) + ";" });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [agent("workbuddy", "WorkBuddy"), agent("codex", "Codex")], profiles: [], served: 1, settings });
    if (url.pathname === "/api/enterprise") return json(enterpriseOn ? enterprise : {});
    if (url.pathname === "/api/usage/quotas") { ctl.quotaReads.push(url.search); return json([{ provider: "copilot", name: "Copilot", icon: "githubcopilot", user: "fixture", windows: [{ name: "Premium requests", used: 8 }] }]); }
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], plugins: [], gateway: { running: true, url: "http://127.0.0.1:3999", groups: [], calls: [] } });
    if (url.pathname === "/api/groups") return json({ groups: [], pools: [] });
    if (url.pathname === "/api/usage") return json({ calls: 0, agents: [], models: [], series: [] });
    if (url.pathname === "/api/window/fit") { ctl.fitted.push(Number(url.searchParams.get("h"))); return route.fulfill({ status: 204 }); }
    if (url.pathname.startsWith("/api/")) return json({});
    if (url.host !== "panel.fixture" || !/^\/[\w./-]*$/.test(url.pathname)) throw new Error("unexpected external request: " + url.href);
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    try { return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] }); }
    catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return ctl;
}

for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  test(engine + ": native panel initializes configured agents and switches allowances without closing", async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch());
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) for (const enterpriseOn of [true, false]) await t.test(lang + (enterpriseOn ? " enterprise" : " public"), async sub => {
      const context = await browser.newContext({ viewport: { width: 440, height: 560 }, reducedMotion: "reduce" });
      sub.after(() => context.close());
      const page = await context.newPage(), ctl = fixture(lang, enterpriseOn);
      page.setDefaultTimeout(5000);
      page.on("pageerror", e => ctl.errors.push(e.message));
      await page.route("**/*", ctl.serve);
      await page.goto("http://panel.fixture/?mode=panel");
      assert.deepEqual(ctl.errors, [], "missing window-only navigation must not abort panel initialization");
      const workbuddy = page.locator('.row.agent[data-id="workbuddy"]');
      await workbuddy.waitFor();
      assert.equal(await page.locator("#nav").count(), 0, "the panel intentionally has no main navigation");
      assert.equal(await page.locator(".row.agent").count(), enterpriseOn ? 1 : 2);
      assert.match(await workbuddy.innerText(), /Test Model/);
      assert.ok(ctl.requests.includes("/api/state"), "state initialization completes");
      await page.locator('[data-ptab="usage"]').click();
      await page.locator('#panelQuota .pq-card[data-card^="copilot"]').waitFor();
      assert.equal(await page.evaluate(() => document.body.dataset.ptab), "usage");
      assert.equal(await page.locator("#view-agents").isVisible(), true);
      assert.equal(await workbuddy.isVisible(), false, "allowances replaces the visible content, not the panel");
      await page.locator('[data-ptab="agents"]').click();
      await workbuddy.waitFor();
      assert.equal(await page.locator(".row.agent").count(), enterpriseOn ? 1 : 2);
      if (enterpriseOn) {
        const next = { ...ctl.enterprise, effective: { ...ctl.enterprise.effective, visibleAgents: ["codex"] }, revision: "two" };
        Object.assign(ctl.enterprise, next);
        await page.evaluate(next => window.receiveEnterprise(next), next);
        await page.locator('.row.agent[data-id="codex"]').waitFor();
        assert.equal(await workbuddy.count(), 0, "a live selection also updates the panel");
      }
      assert.deepEqual(ctl.errors, []);
      assert.equal(ctl.requests.some(p => ["/api/window/hide", "/api/window/quit", "/api/window/main"].includes(p)), false, "tab clicks never close the panel or redirect to the main window");
      assert.ok(ctl.fitted.length && ctl.fitted.every(h => Number.isFinite(h) && h > 0));
      assert.equal(ctl.requests.some(p => p.startsWith("/api/agents/")), false, "rendering and policy changes never edit agent wiring");
      if (process.env.ARTIFACT_DIR) {
        await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
        await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `panel-${engine}-${lang}-${enterpriseOn ? "enterprise" : "public"}.png`) });
      }
    });
  });
}
