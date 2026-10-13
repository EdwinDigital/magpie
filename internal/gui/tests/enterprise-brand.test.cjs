const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");

test("enterprise branding preserves technical identifiers, provider metadata and non-string translations", () => {
  const context = vm.createContext({ window: { enterpriseAppName: "Acme Assistant", bootPrefs: {cliName:"msai-client"} } });
  for (const file of ["enterprise-brand.js", "i18n.js"]) {
    vm.runInContext(fs.readFileSync(path.resolve(__dirname, "../assets", file), "utf8"), context);
  }
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    vm.runInContext(`locale = ${JSON.stringify(lang)}`, context);
    assert.equal(vm.runInContext('t("Menu bar magpie icon").includes("Acme Assistant")', context), true);
    assert.equal(vm.runInContext('t("A folder named magpie is made in it.").includes("Acme Assistant")', context), false);
    assert.equal(vm.runInContext('t("In a new terminal, magpie runs this app: {path}", {path:"/Applications/magpie.app/Contents/MacOS/magpie"}).includes("Acme Assistant")', context), false);
    assert.equal(vm.runInContext('t("magpie-plugin-third-party")', context), "magpie-plugin-third-party");
    assert.equal(vm.runInContext('t("magpie://import?name=magpie")', context), "magpie://import?name=magpie");
    assert.equal(vm.runInContext('/magpie/i.test(tNotice("Restart Zed if it still asks for an API key: magpie has configured its gateway credential in the system credential store."))', context), false);
    assert.equal(vm.runInContext('tNotice("third-party magpie plugin advice")', context), "third-party magpie plugin advice");
    assert.equal(vm.runInContext("t(undefined)", context), undefined);
    assert.equal(vm.runInContext("t(null)", context), null);
    const intro = vm.runInContext('t("Switch an agent on and the models you set up in magpie show up as the {magpie} provider in its own model list; or pick its model right here.", {magpie:"Acme Assistant"})', context);
    assert.equal(/magpie/i.test(intro), false, lang + ": the Agent introduction still contains the public brand");
    assert.equal(/[{}]/.test(intro), false, lang + ": branding corrupted a placeholder");
    assert.equal(vm.runInContext('/magpie/i.test(t("Switch an agent on and your models show up as the magpie provider in its own model list, to change in the agent. Or pick a model right here: the agent is connected and starts on it. Switch it off and it goes back to how it was."))', context), false, lang + ": the empty-model introduction still contains the public brand");
    const quota = vm.runInContext('t("When I ask: opening or refreshing the Usage page, a card\'s refresh, and magpie quota or accounts read them; routing, the menu bar and alerts go by the last reading till then. An account a vendor turns away for its quota is still read.")', context);
    assert.equal(quota.includes("msai-client"), true, lang + ": CLI guidance must use the immutable executable name");
    assert.equal(quota.includes("Acme Assistant quota"), false);
  }
});

test("all enterprise form labels and action text have four non-English translations", () => {
  const context = vm.createContext({ window: {} });
  vm.runInContext(fs.readFileSync(path.resolve(__dirname, "../assets/i18n.js"), "utf8"), context);
  const source = fs.readFileSync(path.resolve(__dirname, "../assets/enterprise.js"), "utf8");
  const keys = new Set();
  for (const match of source.matchAll(/\bt\("([^"\n]+)"/g)) keys.add(match[1]);
  for (const match of source.matchAll(/\brow\("([^"\n]+)", "([^"\n]+)"/g)) { keys.add(match[1]); keys.add(match[2]); }
  for (const match of source.matchAll(/\bcheck\("[^"\n]+", "([^"\n]+)", "([^"\n]+)"/g)) { keys.add(match[1]); keys.add(match[2]); }
  assert(keys.size >= 25, "literal UI text audit must cover the form, not an empty match");
  for (const lang of ["zh", "zh-TW", "ja", "de"]) for (const key of keys) {
    assert.equal(vm.runInContext(`Object.hasOwn(I18N[${JSON.stringify(lang)}], ${JSON.stringify(key)})`, context), true, `${lang}: ${key}`);
  }
});
