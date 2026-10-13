const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(__dirname, "../assets");

function fixture(lang, theme, mode) {
  const policy = { distributionId: "acme-assistant", mode, channel: "stable", ...(mode === "platform" ? { feedUrl: "https://distribution.invalid/v1/updates/acme/acme-assistant/stable/manifest.json" } : {}) };
  const effective = { appName: "Acme", visibleProviders: [], publicPluginCatalog: false, libraryEnabled: false };
  const enterprise = { distributionId: policy.distributionId, effective, defaults: effective, revision: "one", updatePolicy: policy };
  const settings = { theme, lang, version: "1.0.2", tray: "panel", textSize: 100, lanURLs: [], fx: { rate: 7.2 }, updateEvery: 360 };
  const ctl = { web: false, checks: 0, errors: [], requests: [], reply: { state: mode === "manual" ? "manual" : "latest", current: "1.0.2", policy } };
  ctl.serve = async route => {
    const req = route.request(), url = new URL(req.url()); ctl.requests.push(url.pathname);
    const json = data => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme, web: ctl.web, enterprise })};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/enterprise") return json(enterprise);
    if (url.pathname === "/api/enterprise/providers") return json([]);
    if (url.pathname === "/api/update/check") { ctl.checks++; return json(ctl.reply); }
    if (url.pathname === "/api/update") return json(ctl.reply);
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    try { return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] }); }
    catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return ctl;
}

for (const [name, engine] of [["chromium", chromium], ["webkit", webkit]]) {
  test(`${name}: enterprise updates follow the package channel and manual packages offer no public update actions`, async () => {
    const browser = await engine.launch();
    try {
      for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) for (const theme of ["light", "dark"]) for (const mode of ["manual", "platform"]) {
        const context = await browser.newContext({ viewport: { width: 1100, height: 1000 }, colorScheme: theme });
        const page = await context.newPage(), f = fixture(lang, theme, mode);
        page.on("pageerror", e => f.errors.push(e.message));
        await page.route("**/*", f.serve);
        await page.goto("http://enterprise.test/?view=settings&tab=about");
        const version = page.locator("#about .row.pref").first();
        await version.locator(".sub").waitFor();
        await page.waitForFunction(mode => document.querySelector("#about .row.pref .sub")?.textContent === t(mode === "manual" ? "Download new versions through your enterprise distribution channel. Self-updates are not configured." : "Up to date"), mode);
        const source = page.locator(".update-mirror-row");
        assert.equal(await source.locator("button,input").count(), 0, "enterprise packages cannot select a public mirror");
        for (const cls of ["update-auto-row", "update-every-row", "update-pill-row"]) assert.equal(await page.locator("." + cls).isVisible(), mode === "platform");
        if (mode === "manual") {
          assert.equal(await version.locator("button").count(), 0, "manual packages do not offer a failing Check button");
          assert.equal(await page.locator(".whatsnew-row").count(), 0, "manual packages do not ask a missing notes feed");
          assert.equal(f.checks, 0);
          assert.equal((await version.textContent()).includes("this enterprise package uses manual updates"), false);
        } else {
          assert.equal(await source.locator("code").textContent(), "distribution.invalid");
          assert.equal(await source.locator("code").getAttribute("title"), f.reply.policy.feedUrl);
          assert((await source.textContent()).includes("stable"));
          // Even a stale mirror error cannot expose a public fallback action.
          f.reply = { ...f.reply, state: "error", error: "enterprise channel unavailable", mirror: "https://public-mirror.invalid/" };
          await version.locator("button").click();
          await page.waitForFunction(() => document.querySelector("#about .row.pref .sub")?.textContent.includes("enterprise channel unavailable"));
          assert.equal(await version.locator("button").count(), 1, "only retrying the enterprise check remains");
          assert.equal(f.checks, 1);
          assert.equal(f.requests.includes("/api/settings/update-mirror"), false);
        }
        // Mobile is the browser surface, not a native window below its minimum size.
        f.web = true;
        await page.setViewportSize({ width: 440, height: 760 });
        await page.reload();
        await page.waitForFunction(() => document.querySelector("#about .row.pref .sub")?.textContent);
        if (process.env.ARTIFACT_DIR && lang === "zh") {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `enterprise-update-${name}-${theme}-${mode}.png`) });
        }
        const overflow = await page.evaluate(() => [...document.querySelectorAll("#setPage-about .row")].filter(e => e.getBoundingClientRect().right > innerWidth + 1).map(e => ({ text: e.textContent, width: e.getBoundingClientRect().width })));
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, `${lang}/${theme}/${mode}: ${JSON.stringify(overflow)}`);
        assert.deepEqual(f.errors, []);
        await context.close();
      }
    } finally { await browser.close(); }
  });
}
