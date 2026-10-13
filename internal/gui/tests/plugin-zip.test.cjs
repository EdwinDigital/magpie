// Browser plugin not available; use the repository's isolated Playwright fixtures.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const http = require("node:http");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const assets = path.resolve(__dirname, "../assets");
const archive = Buffer.from("UEsDBBQAAAAAAMyeTF1/4c6CLAAAACwAAAAMAAAAcGFja2FnZS5qc29ueyJuYW1lIjoiY29tcGFueS1leGFtcGxlIiwibWFpbiI6ImluZGV4LmpzIn1QSwMEFAAAAAAAzJ5MXX+/N6QfAAAAHwAAAAgAAABpbmRleC5qc2V4cG9ydCBkZWZhdWx0IGFzeW5jICgpID0+ICh7fSlQSwECFAMUAAAAAADMnkxdf+HOgiwAAAAsAAAADAAAAAAAAAAAAAAAgAEAAAAAcGFja2FnZS5qc29uUEsBAhQDFAAAAAAAzJ5MXX+/N6QfAAAAHwAAAAgAAAAAAAAAAAAAAIABVgAAAGluZGV4LmpzUEsFBgAAAAACAAIAcAAAAJsAAAAAAA==", "base64");
const words = { en: "Upload ZIP", zh: "上传 ZIP", "zh-TW": "上傳 ZIP", ja: "ZIP をアップロード", de: "ZIP hochladen" };

function fixture(lang, theme) {
  const effective = { appName: "Acme", visibleProviders: [], publicPluginCatalog: false, libraryEnabled: false };
  const enterprise = { effective, defaults: effective, revision: "one", restartRequired: [] };
  const ctl = { uploads: [], requests: [], plugins: [], subscriptions: [], enterprise, errors: [], fail: false, release: null };
  ctl.serve = async route => {
    const req = route.request(), u = new URL(req.url());
    ctl.requests.push(u.pathname);
    const json = data => route.fulfill({ json: data });
    if (u.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs=${JSON.stringify({ lang, theme, web: true, enterprise })};` });
    if (u.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme, version: "1.0.5", textSize: 100, fx: { rate: 7.2 }, lanURLs: [] } });
    if (u.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], plugins: ctl.subscriptions, gateway: { running: true, url: "http://127.0.0.1:3999", groups: [], calls: [] } });
    if (u.pathname === "/api/enterprise") return json(enterprise);
    if (u.pathname === "/api/enterprise/providers") return json([]);
    if (u.pathname === "/api/groups") return json({ groups: [], pools: [] });
    if (u.pathname === "/api/plugins") return json({ plugins: ctl.plugins, bun: true, picker: false });
    if (u.pathname === "/api/plugins/upload") return route.continue();
    if (u.pathname.startsWith("/api/")) return json({});
    if (u.host !== ctl.host || !/^\/[\w./-]*$/.test(u.pathname)) throw new Error("unexpected external request: " + u.href);
    try {
      return route.fulfill({ body: await fs.readFile(path.join(assets, u.pathname === "/" ? "index.html" : u.pathname)), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(u.pathname === "/" ? "index.html" : u.pathname)] });
    } catch { return route.fulfill({ status: 404, body: "" }); }
  };
  return ctl;
}

for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  test(engine + ": upload a plugin ZIP with public discovery closed", async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch());
    t.after(() => browser.close());
    for (const lang of Object.keys(words)) {
      await t.test(lang, async sub => {
        const width = lang === "de" || lang === "ja" ? 390 : 980, theme = lang === "zh-TW" ? "dark" : "light";
        const context = await browser.newContext({ viewport: { width, height: 820 } });
        const page = await context.newPage(), ctl = fixture(lang, theme);
        let received;
        const started = new Promise(resolve => { received = resolve; });
        // A real local HTTP sink verifies binary bytes in both engines:
        // WebKit's intercepted postDataBuffer is a lossy UTF-8 representation.
        const sink = http.createServer(async (req, res) => {
          if (req.url !== "/api/plugins/upload") { res.writeHead(404).end(); return; }
          const chunks = [];
          for await (const chunk of req) chunks.push(chunk);
          ctl.uploads.push({ headers: req.headers, data: Buffer.concat(chunks) });
          if (ctl.hold) await new Promise(resolve => { ctl.release = resolve; received(); });
          res.setHeader("Content-Type", "application/json");
          if (ctl.fail) { res.writeHead(400).end(JSON.stringify({ error: "invalid plugin ZIP" })); return; }
          ctl.plugins = [{ spec: "/config/plugins/imports/zip-example/company-example", providers: [], version: "" }];
          res.end(JSON.stringify({ plugins: ctl.plugins }));
        });
        await new Promise(resolve => sink.listen(0, "127.0.0.1", resolve));
        ctl.host = `127.0.0.1:${sink.address().port}`;
        sub.after(async () => { ctl.release?.(); await context.close(); sink.closeAllConnections(); await new Promise(resolve => sink.close(resolve)); });
        page.setDefaultTimeout(7000);
        page.on("pageerror", e => ctl.errors.push(e.message));
        await page.route("**/*", ctl.serve);
        await page.goto(`http://${ctl.host}/?view=plugins`);
        const manual = page.locator("#view-plugins .pm-manual"), upload = manual.locator(".pm-upload"), file = manual.locator('input[type="file"]');
        await upload.waitFor();
        assert.equal(await upload.innerText(), words[lang]);
        assert.equal(await page.title(), "Acme");
        assert.equal(await manual.evaluate(e => e.parentElement.firstElementChild === e), true);
        await manual.locator(".pm-minput").fill("@company/keep-my-draft");
        // Button opens the browser/system chooser, including in the web client.
        const chooser = page.waitForEvent("filechooser");
        await upload.click();
        ctl.hold = true;
        const chosen = await chooser;
        await chosen.setFiles({ name: "company.zip", mimeType: "application/zip", buffer: archive });
        await page.waitForFunction(() => document.querySelector(".pm-manual")?.getAttribute("aria-busy") === "true");
        assert.equal(await upload.isDisabled(), true);
        assert.equal(await manual.locator('button[type="submit"]').isDisabled(), true);
        assert.equal(await file.inputValue(), "", "same archive can be selected again");
        await page.evaluate(() => window.redrawEnterprisePlugins());
        assert.equal(await manual.locator(".pm-minput").inputValue(), "@company/keep-my-draft");
        assert.equal(await upload.isDisabled(), true, "refresh cannot start a second upload");
        await started;
        assert.equal(ctl.uploads.length, 1);
        const sent = ctl.uploads[0];
        assert.match(sent.headers["content-type"], /^multipart\/form-data; boundary=/);
        assert.equal(sent.headers["x-magpie-upload"], "zip");
        assert.match(sent.data.toString(), /name="archive"; filename="company.zip"/);
        assert.equal(sent.data.includes(archive), true, "ZIP bytes are not JSON-encoded");
        ctl.hold = false; ctl.release();
        await page.locator("#view-plugins .pm-row").waitFor();
        assert.equal(await upload.isDisabled(), false);
        assert.equal(await manual.locator(".pm-minput").inputValue(), "@company/keep-my-draft");
        assert.equal(await upload.evaluate(e => document.activeElement === e), true);
        // Invalid extension fails locally; a server failure restores controls.
        await file.setInputFiles({ name: "plugin.js", mimeType: "text/javascript", buffer: Buffer.from("bad") });
        assert.equal(ctl.uploads.length, 1);
        ctl.fail = true;
        await file.setInputFiles({ name: "company.zip", mimeType: "application/zip", buffer: archive });
        await page.locator("#status", { hasText: "invalid plugin ZIP" }).waitFor();
        assert.equal(await upload.isDisabled(), false);
        assert.equal(ctl.uploads.length, 2);
        ctl.fail = false;
        await file.setInputFiles({ name: "company.zip", mimeType: "application/zip", buffer: archive });
        await page.waitForFunction(() => document.querySelector(".pm-manual")?.getAttribute("aria-busy") === "false" && document.querySelector("#status")?.classList.contains("ok"));
        assert.equal(ctl.uploads.length, 3);
        assert.equal(ctl.requests.some(p => ["/api/plugins/listings", "/api/plugins/github", "/api/plugins/search"].includes(p)), false);
        assert.deepEqual(ctl.errors, []);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "narrow layouts have no horizontal overflow");
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `plugin-zip-${engine}-${lang}.png`) });
          await manual.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `plugin-zip-entry-${engine}-${lang}.png`) });
        }
        await context.close();
      });
    }
  });
}

for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  test(engine + ": installed suppliers explain enterprise visibility and update live", async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch());
    t.after(() => browser.close());
    for (const lang of Object.keys(words)) await t.test(lang, async sub => {
      const context = await browser.newContext({ viewport: { width: lang === "de" ? 430 : 1024, height: 820 } });
      sub.after(() => context.close());
      const page = await context.newPage(), ctl = fixture(lang, lang === "zh-TW" ? "dark" : "light");
      ctl.host = "zip.fixture";
      ctl.plugins = [{ spec: "corp-auth@1.0.0", providers: ["Private Models"], hiddenProviders: ["Private Models"], version: "1.0.0", moved: [] }];
      ctl.subscriptions = [{ id: "corp", pid: "corp", name: "Private Models", icon: "generic", spec: "corp-auth@1.0.0", signedIn: false, models: 0, methods: [{ type: "api", label: "API key" }] }];
      const errors = [];
      page.on("pageerror", error => errors.push(error.message));
      await page.route("**/*", ctl.serve);
      await page.goto("http://zip.fixture/?view=plugins");
      const hint = page.locator(".pm-note").filter({ hasText: "Private Models" });
      await hint.waitFor();
      assert.equal(await hint.isVisible(), true, "a successful load is distinguished from enterprise hiding");
      if (process.env.MAGPIE_TEST_SCREENSHOTS && lang === "zh-TW") {
        await fs.mkdir(process.env.MAGPIE_TEST_SCREENSHOTS, {recursive:true});
        await page.screenshot({path:path.join(process.env.MAGPIE_TEST_SCREENSHOTS, engine+"-plugin-hidden.png")});
      }
      const next = { ...ctl.enterprise, effective: { ...ctl.enterprise.effective, visibleProviders: ["plugin:corp"] }, revision: "selected" };
      Object.assign(ctl.enterprise, next);
      await page.evaluate(next => window.receiveEnterprise(next), next);
      try { await hint.waitFor({ state: "detached", timeout: 3000 }); }
      catch (error) {
        throw new Error(JSON.stringify(await page.evaluate(() => ({ allowed: window.enterpriseUI.allow("plugin:corp"), plugins: providers?.plugins, note: document.querySelector(".pm-note")?.textContent }))) + " " + errors.join("; ") + " " + error.message);
      }
      assert.deepEqual(errors, []);
    });
  });
}
