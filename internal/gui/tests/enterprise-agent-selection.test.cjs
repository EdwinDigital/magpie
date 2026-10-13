// Browser plugin not available; fixtures never inspect a real installation.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(process.env.ASSET_DIR || path.resolve(__dirname, "../assets"));
const defaults = ["codex", "claude", "claude-desktop", "vscode", "copilot", "workbuddy"];
const agents = [
  ["codex", "Codex", "codex"], ["claude", "Claude Code", "claudecode-color"],
  ["claude-desktop", "Claude Desktop", "claude-color"], ["vscode", "VS Code", "vscode"],
  ["copilot", "Copilot CLI", "githubcopilot"], ["workbuddy", "WorkBuddy", "workbuddy-color"],
  ["dsh", "DeepSeek Harness", "deepseek"],
].map(([id, name, icon]) => ({ id, name, icon, available: true }));

function fixture(lang, theme) {
  const base = { schemaVersion: 1, appName: "Acme Assistant", visibleAgents: defaults, visibleProviders: [], publicPluginCatalog: false, libraryEnabled: false };
  const ctl = { posts: [], errors: [], state: { defaults: base, effective: { ...base }, config: null, revision: "one", logoUrl: "", restartRequired: [] } };
  ctl.serve = async route => {
    const req = route.request(), u = new URL(req.url()), json = data => route.fulfill({ json: data });
    if (u.host !== "enterprise.invalid") throw new Error("unexpected external URL " + u.href);
    if (u.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme, web: true, enterprise: ctl.state })};` });
    if (u.pathname === "/api/state") return json({ agents: agents.map(a => ({ ...a, path: "/test/" + a.id, fields: [], wired: false })), clients: agents, profiles: [], settings: { lang, theme, textSize: 100, version: "1.0.5", lanURLs: [], fx: { rate: 7.2 } } });
    if (u.pathname === "/api/enterprise") {
      if (req.method() === "POST") {
        const body = req.postDataJSON(); ctl.posts.push(body);
        ctl.state = { ...ctl.state, config: body.config, effective: { ...base, ...(body.config || {}) }, revision: "saved-" + ctl.posts.length };
      }
      return json(ctl.state);
    }
    if (u.pathname === "/api/enterprise/agents") return json(agents);
    if (u.pathname === "/api/enterprise/providers") return json([
      { id: "subscription:copilot", name: "Copilot", icon: "githubcopilot", default: true },
      { id: "preset:ollama", name: "Ollama", icon: "ollama", default: true },
      { id: "preset:deepseek", name: "DeepSeek", icon: "deepseek" },
    ]);
    if (u.pathname === "/api/agents/install") return json(agents.map(a => ({ ...a, commands: [] })));
    if (u.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], plugins: [], gateway: { running: true, url: "http://127.0.0.1:3999", groups: [], calls: [] } });
    if (u.pathname === "/api/groups") return json({ groups: [], pools: [] });
    if (u.pathname === "/api/plugins") return json({ plugins: [], bun: true });
    if (u.pathname.startsWith("/api/")) return json({});
    if (!/^\/[\w./-]*$/.test(u.pathname)) throw new Error("unexpected asset path");
    const file = path.join(assets, u.pathname === "/" ? "index.html" : u.pathname);
    try { return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] }); }
    catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return ctl;
}

async function press(page, loc, method = "click") {
  // The app deliberately cancels scroll-on-click. Scroll as a person does,
  // through the outer view first, then a selection list's own scrollbar.
  const scrollers = await loc.evaluate(e => {
    const out = [];
    for (let p = e.parentElement; p; p = p.parentElement) if (p.matches(".view,.enterprise-selection-list")) out.unshift(p.id ? "#" + p.id : ".enterprise-selection-list");
    return out;
  });
  for (const selector of scrollers) {
    const s = page.locator(selector).first();
    const b = await s.boundingBox(), target = await loc.boundingBox();
    const top = Math.max(b.y, 100), bottom = Math.min(b.y + b.height, page.viewportSize().height - 36);
    if (target.y < top + 8 || target.y + target.height > bottom - 8) {
      await page.mouse.move(b.x + 3, (top + bottom) / 2);
      await page.mouse.wheel(0, target.y - (top + bottom) / 2);
      await page.waitForTimeout(300);
    }
  }
  await loc[method]();
}

for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  test(engine + ": collapsed enterprise provider/agent lists and live projection", async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch());
    t.after(() => browser.close());
    for (const [i, lang] of (process.env.LANGUAGES ? process.env.LANGUAGES.split(",") : ["en", "zh", "zh-TW", "ja", "de"]).entries()) {
      await t.test(lang, async sub => {
        const width = lang === "ja" || lang === "de" ? 390 : 980, theme = i % 2 ? "dark" : "light";
        const context = await browser.newContext({ viewport: { width, height: 920 } }); sub.after(() => context.close());
        const page = await context.newPage(), ctl = fixture(lang, theme);
        page.setDefaultTimeout(8000); page.on("pageerror", e => ctl.errors.push(e.message));
        await page.route("**/*", ctl.serve);
        await page.goto("http://enterprise.invalid/?view=settings&tab=enterprise");
        await page.locator("#enterprise-appName").waitFor();
        const provider = page.locator('[data-selection="visibleProviders"]'), agent = page.locator('[data-selection="visibleAgents"]');
        assert.deepEqual(await page.locator(".enterprise-form > section").evaluateAll(es => es.map(e => e.dataset.selection)), ["visibleProviders", "visibleAgents"]);
        assert.equal(await provider.locator(".fold").getAttribute("aria-expanded"), "false");
        assert.equal(await agent.locator(".fold").getAttribute("aria-expanded"), "false");
        assert.equal(await page.title(), "Acme Assistant");
        await press(page, agent.locator(".fold"));
        assert.deepEqual(await agent.locator('input[name="agent"]:checked').evaluateAll(es => es.map(e => e.value).sort()), [...defaults].sort());
        assert.equal(await agent.locator('input[name="agent"]:disabled').count(), 0, "the six defaults can be deselected");
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `agents-default-${engine}-${lang}.png`) });
        }
        const search = agent.locator('input[type="search"]');
        await search.fill("work"); await search.focus();
        await page.evaluate(() => window.enterpriseUI.refresh());
        assert.equal(await search.evaluate(e => document.activeElement === e), true);
        assert.equal(await agent.locator(".enterprise-choice").count(), 1);
        await search.fill("");
        await press(page, agent.locator('input[value="workbuddy"]'), "uncheck");
        await press(page, agent.locator('input[value="dsh"]'), "check");
        await press(page, agent.locator(".fold"));
        assert.equal(await agent.locator(".enterprise-selection-list").isVisible(), false);
        await press(page, provider.locator(".fold"));
        assert.equal(await provider.locator('input[value="subscription:copilot"]').isDisabled(), true);
        assert.equal(await provider.locator('input[value="preset:ollama"]').isChecked(), true);
        await press(page, provider.locator('input[value="preset:deepseek"]'), "check");
        await press(page, page.locator('.enterprise-form button[type="submit"]'));
        await page.waitForFunction(() => document.querySelector("#status")?.classList.contains("ok"));
        const config = ctl.posts.at(-1).config;
        assert.deepEqual([...config.visibleAgents].sort(), [...defaults.filter(id => id !== "workbuddy"), "dsh"].sort());
        assert.ok(config.visibleProviders.includes("preset:deepseek"));
        assert.ok(config.overrideFields.includes("visibleAgents"));
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `selection-${engine}-${lang}.png`) });
        }
        await page.locator('#nav [data-view="agents"]').click();
        await page.locator('#agents .agent[data-id="codex"]').waitFor();
        await page.waitForFunction(() => !document.querySelector("#view-agents").classList.contains("loading"));
        const shown = await page.locator("#agents .agent").evaluateAll(es => es.map(e => e.dataset.id));
        assert.ok(shown.includes("dsh")); assert.equal(shown.includes("workbuddy"), false, "a stale backend payload cannot redraw hidden agents");
        // A different window changes the policy: no reload, and empty is not default.
        ctl.state = { ...ctl.state, revision: "other-window", effective: { ...ctl.state.effective, visibleAgents: [] } };
        await page.evaluate(state => window.receiveEnterprise(state), ctl.state);
        assert.equal(await page.locator("#agents .agent").count(), 0);
        assert.equal(await page.locator("#agentsInstall").count(), 0);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
        assert.deepEqual(ctl.errors, []);
      });
    }
  });
}
