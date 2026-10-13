// A presentation policy, not a gateway authorization policy. Shared provider
// and model data stay intact for routing and history.
(() => {
  let saved = window.bootPrefs?.enterprise || null;
  let formDraft = null, revision = "", dirty = false, busy = false;
  let choices = [], agentChoices = [], loaded = false, query = "", agentQuery = "", polling = false, snapshot = 0;
  const fields = ["appName", "logoAsset", "visibleProviders", "visibleAgents", "showPluginProviderEntry", "showCustomProviderEntry", "publicPluginCatalog", "libraryEnabled"];
  const defaultAgents = ["codex", "claude", "claude-desktop", "vscode", "copilot", "workbuddy"];
  const folded = { visibleProviders: true, visibleAgents: true };
  for (const key of Object.keys(folded)) { try { const value = localStorage.getItem("magpie.enterprise." + key + ".folded"); if (value !== null) folded[key] = value === "1"; } catch {} }
  const defaults = new Set(["subscription:copilot", "preset:ollama"]);
  const originalLogo = document.querySelector("header .logo")?.cloneNode(true);
  const originalFavicon = "icons/favicon.svg";
  let draftLogoUrls = null, uploading = false, uploadSeq = 0;
  const systemDark = matchMedia("(prefers-color-scheme: dark)");
  const theme = () => document.documentElement.dataset.theme === "dark" ? "dark" : document.documentElement.dataset.theme === "light" ? "light" : systemDark.matches ? "dark" : "light";
  const logoURL = () => saved?.logoUrls?.[theme()] || saved?.logoUrl || "";
  const effective = () => saved?.effective;
  const agentAllowed = id => !effective() || (effective().visibleAgents ?? defaultAgents).some(selected => selected === id || selected === id.split("@wsl:")[0].split("#")[0]);
  const inheritedFields = config => config?.overrideFields || (config ? fields.filter(f => f !== "visibleAgents" || config.visibleAgents != null) : []);
  // Enterprise presentation is static, including packages with no custom logo.
  const birdMotion = () => !effective() && !logoURL();
  const allow = (id) => !effective() || defaults.has(id) || effective().visibleProviders?.includes(id);
  function providerID(p) {
    if (p.move?.state === "plugin" || p.account?.builtin && (typeof providers !== "undefined" && providers?.onPlugins?.includes(p.account.builtin))) return "subscription:" + (p.account?.builtin || p.id);
    if (p.account?.builtin) return "plugin:" + p.id;
    if (p.account) return "subscription:" + p.account.agent;
    if (p.preset) return "preset:" + p.preset;
    return "custom:" + p.id;
  }
  const visible = (p) => allow(providerID(p));
  const flag = (key) => effective() ? !!effective()[key] : true;
  const catalog = () => flag("publicPluginCatalog");
  function logoImage(src, follow = src === logoURL()) {
    const img = document.createElement("img"); img.src = src; img.alt = "";
    if (follow) img.dataset.enterpriseLogo = "";
    img.onerror = () => {
      img.onerror = null; img.src = originalFavicon;
      if (typeof status === "function") status(t("Application logo is unavailable. Choose the image again."), "err");
    };
    return img;
  }
  function brand() {
    const name = effective()?.appName || "magpie";
    window.enterpriseAppName = name;
    document.title = name;
    const label = document.querySelector("header .brand > span:last-child");
    if (label) label.textContent = name;
    const logo = document.querySelector("header .logo");
    if (!birdMotion()) logo?.classList.remove("wag");
    if (logo && originalLogo) {
      if (logoURL()) {
        logo.classList.remove("wag");
        const img = logoImage(logoURL()); img.width = img.height = 18;
        logo.replaceChildren(img);
      } else if (originalLogo.querySelector("svg")) logo.replaceChildren(...[...originalLogo.cloneNode(true).childNodes]);
      else { const img = document.createElement("img"); img.src = originalFavicon; img.alt = ""; img.width = img.height = 18; logo.replaceChildren(img); }
    }
    const favicon = document.querySelector('link[rel="icon"]');
    if (favicon) { favicon.href = logoURL() || originalFavicon; favicon.type = logoURL() ? "image/png" : "image/svg+xml"; }
    document.querySelectorAll('.ic[data-icon="magpie"]').forEach((icon) => {
      icon.replaceChildren(logoImage(logoURL() || originalFavicon));
    });
    if (!birdMotion()) document.querySelectorAll(".rt-flier").forEach(node => node.remove());
    window.refreshRoutingBrand?.();
    const rendered = window.enterpriseRenderedBrands;
    if (rendered) {
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        const node = walker.currentNode;
        if (node.parentElement?.closest("script,style,code,pre,input,textarea")) continue;
        const template = rendered.get(node.nodeValue);
        if (template) { const value = template.split("{app}").join(name).split("{cli}").join(window.bootPrefs?.cliName || "magpie"); node.nodeValue = value; rendered.set(value, template); }
      }
      for (const element of document.querySelectorAll("[title],[aria-label]")) for (const attribute of ["title", "aria-label"]) {
        const template = rendered.get(element.getAttribute(attribute));
        if (template) { const value = template.split("{app}").join(name).split("{cli}").join(window.bootPrefs?.cliName || "magpie"); element.setAttribute(attribute, value); rendered.set(value, template); }
      }
    }
    document.querySelectorAll("[data-app-brand]").forEach((node) => { node.textContent = name; });
    if (typeof translateStatic === "function") translateStatic();
  }
  function themeIcons() {
    document.querySelectorAll("img[data-enterprise-logo]").forEach(img => { img.src = logoURL() || originalFavicon; });
    const favicon = document.querySelector('link[rel="icon"]');
    if (favicon) { favicon.href = logoURL() || originalFavicon; favicon.type = logoURL() ? "image/png" : "image/svg+xml"; }
    window.refreshRoutingBrand?.();
  }
  new MutationObserver(themeIcons).observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  systemDark.addEventListener("change", themeIcons);
  function navigation() {
    const b = document.querySelector('#nav [data-view="library"]');
    if (b) b.hidden = !flag("libraryEnabled") || typeof gatewayMode !== "undefined" && gatewayMode;
    if (!flag("libraryEnabled") && typeof view !== "undefined" && view === "library") {
      show("providers");
    }
  }
  function apply(next) {
    if (!next?.effective || !next.revision) return;
    const changed = saved?.revision !== next.revision;
    if (changed) snapshot++;
    saved = next; if (changed) brand(); navigation();
    if (changed && typeof providers !== "undefined" && providers && typeof renderProviders === "function") {
      if (typeof editing === "string" && !visible(providers.providers.find((p) => p.id === editing) || { id: editing })) { editing = null; }
      if (typeof view !== "undefined" && view === "providers" && !editing) renderProviders();
    }
    if (changed) window.redrawEnterprisePlugins?.();
    if (changed && typeof state !== "undefined" && typeof renderAgents === "function") {
      // Agent payloads can be unchanged: refresh this projection even while
      // its tab is hidden, rather than letting the state hash keep stale rows.
      renderAgents();
      if (typeof paintInstalls === "function") paintInstalls();
    }
    if (!dirty) { formDraft = JSON.parse(JSON.stringify(next.effective)); formDraft.visibleAgents ??= [...defaultAgents]; formDraft.overrideFields = [...inheritedFields(next.config)]; revision = next.revision; }
    if (loaded && changed && !dirty && !busy) render();
  }
  async function refresh() {
    if (polling || typeof api !== "function") return;
    polling = true;
    const started = snapshot;
    try { const next = await api("enterprise"); if (started === snapshot) apply(next); } catch {} finally { polling = false; }
  }
  function mark(key) {
    dirty = true;
    if (!formDraft.overrideFields.includes(key)) formDraft.overrideFields.push(key);
    const msg = document.querySelector("#enterpriseResult"); if (msg) msg.textContent = t("Unsaved changes");
  }
  function row(label, help, control) {
    const r = el("div", "row pref"); const who = el("div", "who");
    const title = el("label", "name", t(label));
    if (control.id) title.htmlFor = control.id;
    who.append(title); if (help) who.append(el("div", "sub", t(help)));
    r.append(who, control); return r;
  }
  function check(key, label, help) {
    const input = el("input", "enterprise-check"); input.type = "checkbox"; input.id = "enterprise-" + key; input.name = key;
    input.checked = !!formDraft[key]; input.onchange = () => { formDraft[key] = input.checked; mark(key); };
    return row(label, help, input);
  }
  function selectionSection(key, title, candidates, searchID) {
    const section = el("section", "enterprise-selection"); section.dataset.selection = key;
    const head = el("div", "row-head"), toggle = el("button", "fold"); toggle.type = "button";
    const list = el("div", "list enterprise-selection-list"); list.id = "enterprise-" + key + "-list";
    toggle.setAttribute("aria-controls", list.id);
    toggle.append(svg(CHEV_R, 11, 1.6), el("span", "label", t(title)));
    const count = el("span", "note"); toggle.append(count);
    const search = el("input", "words find enterprise-search"); search.type = "search"; search.id = searchID; search.name = key + "Search"; search.autocomplete = "off";
    search.placeholder = t(key === "visibleProviders" ? "Search providers" : "Search agents"); search.setAttribute("aria-label", search.placeholder);
    search.value = key === "visibleProviders" ? query : agentQuery;
    const help = el("p", "note enterprise-selection-help", t("Unselected items are not automatically scanned. Existing settings and sign-ins are kept."));
    help.id = list.id + "-help"; list.setAttribute("aria-describedby", help.id);
    const candidatesHere = [...candidates];
    for (const id of formDraft[key] || []) if (!candidatesHere.some(c => c.id === id)) candidatesHere.push({ id, name: id, available: false });
    function drawList() {
      const selected = new Set(formDraft[key] || []), q = search.value.trim().toLowerCase();
      count.textContent = String(candidatesHere.filter(c => c.default || selected.has(c.id)).length) + " / " + candidatesHere.length;
      list.replaceChildren();
      for (const c of candidatesHere.filter(c => !q || (c.id + " " + c.name).toLowerCase().includes(q))) {
        const label = el("label", "row enterprise-choice");
        if (c.icon) label.append(icon(c.icon));
        const who = el("span", "who"); who.append(el("span", "name", c.name));
        const kind = key === "visibleAgents" ? "Agent" : { subscription: "Subscriptions", preset: "Providers", plugin: "Plugins", custom: "Custom provider" }[c.id.split(":")[0]] || "Providers";
        who.append(el("small", "sub", c.available === false ? t("Unavailable") : c.default ? t("Default") : t(kind)));
        const input = el("input", "enterprise-check"); input.type = "checkbox"; input.name = key === "visibleProviders" ? "provider" : "agent"; input.value = c.id;
        input.checked = !!c.default || selected.has(c.id); input.disabled = !!c.default;
        input.onchange = () => { const next = new Set(formDraft[key] || []); if (input.checked) next.add(c.id); else next.delete(c.id); formDraft[key] = [...next]; mark(key); count.textContent = String(candidatesHere.filter(c => c.default || next.has(c.id)).length) + " / " + candidatesHere.length; };
        label.append(who, input); list.append(label);
      }
      if (!list.childElementCount) list.append(el("p", "note", t("No matching items")));
    }
    function fold() {
      toggle.setAttribute("aria-expanded", String(!folded[key]));
      list.hidden = search.hidden = help.hidden = folded[key];
    }
    toggle.onclick = () => { folded[key] = !folded[key]; try { localStorage.setItem("magpie.enterprise." + key + ".folded", folded[key] ? "1" : "0"); } catch {} fold(); };
    search.oninput = () => { if (key === "visibleProviders") query = search.value; else agentQuery = search.value; drawList(); };
    search.onkeydown = ev => ev.stopPropagation();
    head.append(toggle, el("span", "grow"), search); section.append(head, help, list); drawList(); fold();
    return section;
  }
  function render() {
    const page = document.querySelector("#setPage-enterprise");
    const focused = page?.contains(document.activeElement) && document.activeElement.matches("input,textarea");
    if (!page || (dirty || focused) && page.querySelector("form")) return;
    if (!loaded) {
      if (!page.childElementCount) page.append(el("p", "note", t("Loading…")));
      if (!polling) load(); return;
    }
    if (!formDraft) return;
    const scroll = page.parentElement.scrollTop;
    const form = el("form", "enterprise-form"); form.method = "post"; form.action = "/api/enterprise";
    const list = el("div", "list prefs");
    const name = el("input", "words enterprise-name"); name.type = "text"; name.id = "enterprise-appName"; name.name = "appName"; name.maxLength = 64; name.required = true; name.value = formDraft.appName; name.autocomplete = "off";
    name.oninput = () => { formDraft.appName = name.value; mark("appName"); };
    list.append(row("Application name", "System installation branding comes from the enterprise package.", name));
    const logoControl = el("div", "enterprise-logo-control");
    const urls = draftLogoUrls || (formDraft.logoAsset === saved?.effective?.logoAsset ? saved?.logoUrls : null);
    for (const mode of ["light", "dark"]) {
      const swatch = el("figure", "enterprise-theme-preview"); swatch.dataset.logoTheme = mode;
      const src = urls?.[mode] || (formDraft.logoAsset ? "/api/enterprise/logo/" + formDraft.logoAsset.replace("sha256:", "") + ".png?theme=" + mode + "&v=1" : "icons/favicon.svg");
      const image = logoImage(src, false); image.className = "enterprise-preview"; image.alt = t("Application logo preview") + " · " + t(mode === "light" ? "Light" : "Dark");
      swatch.append(image, el("figcaption", "note", t(mode === "light" ? "Light" : "Dark"))); logoControl.append(swatch);
    }
    const pick = el("input"); pick.type = "file"; pick.id = "enterprise-logo"; pick.name = "logo"; pick.accept = "image/png,image/jpeg"; pick.hidden = true; pick.setAttribute("aria-label", t("Choose a logo"));
    pick.onchange = async () => {
      const chosen = pick.files[0]; if (!chosen) return;
      const seq = ++uploadSeq; uploading = true; mark("logoAsset");
      page.querySelector('button[type="submit"]').disabled = true;
      const say = () => page.querySelector("#enterpriseResult");
      if (say()) say().textContent = t("Preparing light and dark icons…");
      try {
        const body = new FormData(); body.append("logo", chosen);
        const response = await fetch("/api/enterprise/logo", { method: "POST", body }); const payload = await response.json();
        if (!response.ok || !payload.asset || !payload.logoUrls?.light || !payload.logoUrls?.dark) throw new Error(payload.error || t("Could not prepare application icons"));
        if (seq !== uploadSeq) return;
        formDraft.logoAsset = payload.asset; draftLogoUrls = payload.logoUrls;
        for (const mode of ["light", "dark"]) {
          const image = page.querySelector(`[data-logo-theme="${mode}"] img`); if (image) image.src = payload.logoUrls[mode];
        }
        if (say()) say().textContent = t("Unsaved changes");
      } catch (error) {
        if (seq === uploadSeq) { if (say()) say().textContent = t(error.message); status(t(error.message), "err"); }
      } finally {
        if (seq === uploadSeq) { uploading = false; const button = page.querySelector('button[type="submit"]'); if (button) button.disabled = busy; }
        pick.value = "";
      }
    };
    const choose = el("button", "text", t("Choose a logo")); choose.type = "button"; choose.onclick = () => pick.click();
    logoControl.append(choose, pick);
    list.append(row("Application logo", "PNG or JPEG, at most 2 MiB. Light/dark icons are generated automatically. Transparent PNG is recommended.", logoControl));
    list.append(check("showPluginProviderEntry", "Show plugin provider entry", "The Plugins page still supports installation and management."));
    list.append(check("showCustomProviderEntry", "Show custom provider entry", "New providers stay hidden until selected here."));
    list.append(check("publicPluginCatalog", "Public plugin catalog", "Off hides discovery and search, not installation or installed plugins."));
    list.append(check("libraryEnabled", "Open resource library", "Off hides the library; existing agent resources are kept."));
    const actions = el("div", "enterprise-actions");
    const save = el("button", "text primary", t("Save changes")); save.type = "submit"; save.disabled = busy || uploading;
    const cancel = el("button", "text", t("Cancel")); cancel.type = "button"; cancel.onclick = () => { discard(); render(); };
    const reset = el("button", "text", t("Restore package defaults")); reset.type = "button";
    reset.onclick = async () => { if (dirty && !await confirmDiscard()) return; discard(); await persist(null, save); };
    const result = el("p", "note"); result.id = "enterpriseResult"; result.setAttribute("role", "status"); result.setAttribute("aria-live", "polite");
    const source = el("p", "note", saved.distributionId ? t("Enterprise package: {id} · profile {revision}", { id: saved.distributionId, revision: saved.profileRevision }) : t("Local enterprise configuration"));
    actions.append(save, cancel, reset); form.append(source, list,
      selectionSection("visibleProviders", "Visible providers", choices, "enterprise-providerSearch"),
      selectionSection("visibleAgents", "Visible agents", agentChoices, "enterprise-agentSearch"), actions, result);
    form.onsubmit = async (ev) => {
      ev.preventDefault(); if (busy || uploading || !form.reportValidity()) return;
      if (!formDraft.libraryEnabled && typeof view !== "undefined" && view === "library" && window.libraryDirty?.() && !await confirmDiscard()) return;
      await persist(formDraft, save);
    };
    page.replaceChildren(form); page.parentElement.scrollTop = scroll;
    if (saved.restartRequired?.length) {
      const prompt = el("div", "enterprise-actions");
      const restart = el("button", "text primary", t("Restart now")); restart.type = "button";
      restart.onclick = () => api("application/restart", {}).catch(e => status(t(e.message), "err"));
      const later = el("button", "text", t("Apply on next launch")); later.type = "button"; later.onclick = () => prompt.remove();
      prompt.append(el("span", "note", saved.restartRequired.map(surface => t(surface)).join(", ")), restart, later);
      page.append(prompt);
    }
  }
  function discard() {
    uploadSeq++; uploading = false; draftLogoUrls = null;
    dirty = false;
    formDraft = JSON.parse(JSON.stringify(saved.effective)); formDraft.visibleAgents ??= [...defaultAgents]; formDraft.overrideFields = [...inheritedFields(saved.config)]; revision = saved.revision;
  }
  async function persist(config, save) {
    if (busy || uploading) return; busy = true; save.disabled = true;
    const result = document.querySelector("#enterpriseResult");
    try {
      const next = await writingPrefs(api("enterprise", { config, revision }));
      dirty = false; discard(); apply(next);
      status(t("Enterprise configuration saved"), "ok");
    } catch (error) { if (result) result.textContent = t(error.message); status(t(error.message), "err"); }
    finally { busy = false; save.disabled = false; if (!dirty) render(); }
  }
  async function load() {
    if (polling) return; polling = true;
    const started = snapshot;
    try {
      const [config, candidates, agents] = await Promise.all([api("enterprise"), api("enterprise/providers"), api("enterprise/agents")]);
      if (!config?.effective) return;
      const nextChoices = Array.isArray(candidates) ? candidates : [];
      const nextAgents = Array.isArray(agents) ? agents : [];
      const changedChoices = JSON.stringify(choices) !== JSON.stringify(nextChoices) || JSON.stringify(agentChoices) !== JSON.stringify(nextAgents);
      choices = nextChoices; agentChoices = nextAgents; loaded = true;
      if (started === snapshot) apply(config);
      if (changedChoices || !document.querySelector("#setPage-enterprise form")) render();
    } catch (error) { status(error.message, "err"); } finally { polling = false; }
  }
  window.enterpriseUI = { load, logoImage, nameNode: (tag, cls) => { const node = document.createElement(tag); node.className = cls || ""; node.dataset.appBrand = ""; node.textContent = effective()?.appName || "magpie"; return node; }, effective, birdMotion, agentAllowed, allow, visible, providerID, flag, catalog, apply, render, refresh, dirty: () => dirty, discard, logoURL, updatePolicy: () => saved?.updatePolicy, permittedView: (v) => v !== "library" || flag("libraryEnabled") };
  window.receiveEnterprise = apply;
  if (saved) { brand(); navigation(); }
  setInterval(() => { if (!document.hidden && saved) refresh(); }, 2000);
  window.addEventListener("focus", refresh);
  window.addEventListener("beforeunload", e => { if (dirty) e.preventDefault(); });
})();
