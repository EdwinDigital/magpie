// Browser plugin not available; use isolated Playwright/API fixtures.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(process.env.ASSET_DIR || path.resolve(__dirname, "../assets"));
const replayWord = { en: "Replay", zh: "重放", "zh-TW": "重放", ja: "リプレイ", de: "Wiedergabe" };

function fixture(lang, theme, enterprise = true) {
  const stamp = new Date().toISOString();
  const seat = { id: "copilot-seat", provider: "copilot", name: "Copilot", who: "employee@test.invalid", kind: "account", model: "test-model", known: true, used: 8, plan: "Pro" };
  let route = { id: 1, seq: 1, time: stamp, agent: "codex", model: "test-model", provider: "copilot", order: [seat], tries: [{ id: seat.id, model: seat.model, start: stamp, done: false }], done: false, status: 0, ms: 0 };
  const effective = { appName: "magpie", visibleProviders: [], publicPluginCatalog: false, libraryEnabled: false };
  const policy = { effective, defaults: effective, config: null, revision: "enterprise-one", distributionId: "enterprise-test", logoUrl: "", restartRequired: [] };
  const ctl = { policy: enterprise ? policy : null, errors: [], queued: null, pending: null, closed: false };
  ctl.complete = () => {
    route = { ...route, seq: 2, done: true, status: 200, ms: 1000, tokens: 42, tries: [{ ...route.tries[0], done: true, status: 200, ms: 1000 }] };
    if (ctl.pending) { ctl.pending(route); ctl.pending = null; } else ctl.queued = route;
  };
  ctl.serve = async intercepted => {
    const u = new URL(intercepted.request().url()), json = data => intercepted.fulfill({ json: data });
    if (u.host !== "routing.invalid") throw new Error("unexpected external request: " + u.href);
    if (u.pathname === "/boot.js") return intercepted.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme, web: true, enterprise: ctl.policy })};` });
    if (u.pathname === "/api/enterprise") return json(ctl.policy || {});
    if (u.pathname === "/api/enterprise/providers") return json([]);
    if (u.pathname === "/api/state") return json({ agents: [{ id: "codex", name: "Codex", path: "/test/codex", fields: [] }], profiles: [], settings: { lang, theme } });
    if (u.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3425" } });
    if (u.pathname === "/api/groups") return json({ groups: [], models: [], pools: [] });
    if (u.pathname === "/api/plugins") return json({ plugins: [] });
    if (u.pathname === "/api/gateway/trace") {
      let next = route;
      if (u.searchParams.get("wait")) {
        if (ctl.closed) return intercepted.abort();
        next = ctl.queued;
        ctl.queued = null;
        if (!next) next = await new Promise(resolve => { ctl.pending = resolve; });
        if (!next) return intercepted.abort();
      }
      return json({ mine: true, now: new Date().toISOString(), seq: next.seq, totals: { requests: 1, rerouted: 0, errors: 0 }, routes: [next] });
    }
    if (u.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (u.pathname.startsWith("/api/")) return json({});
    if (!/^\/[\w./-]*$/.test(u.pathname)) throw new Error("unexpected asset path: " + u.pathname);
    const file = path.join(assets, u.pathname === "/" ? "index.html" : u.pathname);
    try { return intercepted.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] }); }
    catch { return intercepted.fulfill({ status: 404, body: "" }); }
  };
  return { ctl, policy };
}

async function open(browser, lang, theme, enterprise, t) {
  const width = lang === "ja" || lang === "de" ? 390 : 980;
  const context = await browser.newContext({ viewport: { width, height: 820 }, reducedMotion: "no-preference" });
  const page = await context.newPage(), { ctl, policy } = fixture(lang, theme, enterprise);
  page.setDefaultTimeout(10000);
  page.on("pageerror", e => ctl.errors.push(e.message));
  await page.addInitScript(() => {
    window.__birds = 0; window.__packets = 0;
    new MutationObserver(records => {
      for (const record of records) for (const node of record.addedNodes) {
        if (node.nodeType !== 1) continue;
        if (node.matches(".rt-flier")) window.__birds++;
        if (node.matches(".rt-sky .pkt")) window.__packets++;
      }
    }).observe(document, { childList: true, subtree: true });
  });
  await page.route("**/*", ctl.serve);
  t.after(async () => { ctl.closed = true; ctl.pending?.(null); await context.close(); });
  await page.goto("http://routing.invalid/?view=routing");
  await page.locator(".rt-sky .pkt").waitFor();
  return { page, ctl, policy };
}

for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  test(engine + ": enterprise routing keeps packets but never creates decorative birds", async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch());
    t.after(() => browser.close());
    for (const [i, lang] of Object.keys(replayWord).entries()) {
      await t.test(lang, async sub => {
        const theme = i % 2 ? "dark" : "light";
        const { page, ctl } = await open(browser, lang, theme, true, sub);
        assert.equal(await page.title(), "magpie", "using the original name/logo must not re-enable enterprise animation");
        assert.equal(await page.locator(".rt-hub b").innerText(), "magpie");
        assert.equal(await page.locator(".rt-srcs .rt-node").count(), 1);
        assert.ok(await page.locator(".rt-wires path").count(), "routing wires remain");
        await page.evaluate(() => wag());
        assert.equal(await page.locator("header .logo").evaluate(e => e.classList.contains("wag")), false);
        await page.locator(".rt-sky .pkt.held").waitFor();
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.locator(".rt-stage").screenshot({ path: path.join(process.env.ARTIFACT_DIR, `enterprise-routing-${engine}-${lang}.png`) });
        }
        ctl.complete();
        const replay = page.locator(".rt-log-head button", { hasText: new RegExp("^" + replayWord[lang] + "$") });
        await replay.waitFor();
        await page.waitForFunction(() => !document.querySelector(".rt-sky .pkt"));
        const count = await page.evaluate(() => window.__packets);
        await replay.click();
        await page.waitForFunction(n => window.__packets > n, count);
        await page.waitForFunction(() => !document.querySelector(".rt-sky .pkt") && document.querySelector(".rt-replay")?.hidden);
        assert.equal(await page.evaluate(() => window.__birds), 0, "request, reply and replay must never construct a flier");
        assert.equal(await page.locator(".rt-flier").count(), 0);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
        assert.deepEqual(ctl.errors, []);
      });
    }
    await t.test("a late enterprise policy removes an active public flier without stopping its packet", async sub => {
      const { page, ctl, policy } = await open(browser, "en", "dark", false, sub);
      await page.locator(".rt-flier").first().waitFor();
      assert.ok(await page.evaluate(() => window.__birds), "the ordinary/public path still animates");
      const birds = await page.evaluate(() => window.__birds);
      ctl.policy = policy;
      const kept = await page.evaluate(policy => {
        const packet = document.querySelector(".rt-sky .pkt");
        window.receiveEnterprise(policy);
        wag();
        return packet.isConnected;
      }, policy);
      assert.equal(kept, true);
      assert.equal(await page.locator(".rt-flier").count(), 0);
      assert.equal(await page.locator("header .logo").evaluate(e => e.classList.contains("wag")), false);
      await page.locator(".rt-sky .pkt.held").waitFor();
      ctl.complete();
      await page.waitForFunction(() => !document.querySelector(".rt-sky .pkt"));
      assert.equal(await page.evaluate(() => window.__birds), birds, "no reply flier appears after the policy arrives");
      assert.deepEqual(ctl.errors, []);
    });
  });
}
