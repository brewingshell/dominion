(() => {
  "use strict";

  const HOST_KEY = "dominion.host";
  const SCHEME_KEY = "dominion.scheme";
  const HOSTS_KEY = "dominion.hosts";
  const PROBE_MS = 6000;
  const MAX_HOSTS = 20;

  const params = new URLSearchParams(window.location.search);
  // Which native shell are we in? Desktop injects a Go binding; Android is
  // marked with an appended user agent. A plain browser is neither.
  const SHELL = window.dominionConnect
    ? "desktop"
    : navigator.userAgent.includes("dominion-shell")
      ? "android"
      : "browser";
  const FORCE_PROMPT = params.get("change") === "1";
  const START_SETTINGS = params.get("settings") === "1" || window.__dominionView === "settings";
  // The desktop shell has no URL params; it inlines the error instead.
  const LOAD_ERROR = params.get("error") || window.__dominionLoadError || "";

  const el = (id) => document.getElementById(id);
  const connectForm = el("view-connect");
  const hostEl = el("host");
  const schemeEl = el("insecure");
  const errEl = el("error");
  const goEl = el("go");
  const forceEl = el("force");
  const openSettingsEl = el("open-settings");
  const settingsForm = el("view-settings");
  const sHostEl = el("s-host");
  const sSchemeEl = el("s-insecure");
  const sNameEl = el("s-name");
  const sErrEl = el("s-error");
  const closeSettingsEl = el("close-settings");
  const hostListEl = el("host-list");
  const hostsEmptyEl = el("hosts-empty");

  // ---- persistence -------------------------------------------------------

  // The desktop shell owns storage in Go and inlines the saved values; Android
  // uses Capacitor Preferences; a browser falls back to localStorage.
  const store = {
    async get(key) {
      // The desktop shell inlines short keys (host/scheme).
      const inlineKey = { "dominion.host": "host", "dominion.scheme": "scheme" }[key] || key;
      const inline = window.__dominionSaved && window.__dominionSaved[inlineKey];
      if (inline) return inline;
      try {
        if (window.Capacitor?.Plugins?.Preferences) {
          const { value } = await window.Capacitor.Plugins.Preferences.get({ key });
          return value ?? null;
        }
      } catch {}
      try {
        return localStorage.getItem(key);
      } catch {
        return null;
      }
    },
    async set(key, value) {
      try {
        if (window.Capacitor?.Plugins?.Preferences) {
          await window.Capacitor.Plugins.Preferences.set({ key, value });
          return;
        }
      } catch {}
      try {
        localStorage.setItem(key, value);
      } catch {}
    },
  };

  // List of saved servers. Desktop delegates to Go bindings; the others keep a
  // JSON array under one key.
  const hostStore = {
    async list() {
      if (SHELL === "desktop" && typeof window.dominionListHosts === "function") {
        try {
          return JSON.parse(await window.dominionListHosts()) || [];
        } catch {
          return [];
        }
      }
      try {
        return JSON.parse((await store.get(HOSTS_KEY)) || "[]") || [];
      } catch {
        return [];
      }
    },
    async save(entry) {
      if (SHELL === "desktop" && typeof window.dominionSaveHost === "function") {
        await window.dominionSaveHost(entry.name, entry.host, entry.scheme);
        return;
      }
      const list = await hostStore.list();
      const i = list.findIndex((h) => h.name === entry.name);
      if (i >= 0) list[i] = entry;
      else list.push(entry);
      const trimmed = list.slice(-MAX_HOSTS);
      await store.set(HOSTS_KEY, JSON.stringify(trimmed));
    },
    async remove(name) {
      if (SHELL === "desktop" && typeof window.dominionDeleteHost === "function") {
        await window.dominionDeleteHost(name);
        return;
      }
      const list = (await hostStore.list()).filter((h) => h.name !== name);
      await store.set(HOSTS_KEY, JSON.stringify(list));
    },
  };

  // ---- helpers -----------------------------------------------------------

  function normalize(raw) {
    let s = (raw || "").trim();
    if (!s) return null;
    s = s.replace(/^https?:\/\//i, "").replace(/\/+$/, "");
    if (!/^[^/:\s]+(:\d+)?$/.test(s)) return null;
    if (!s.includes(":")) s += ":5550";
    return s;
  }

  // probe is used by the plain browser only, to show an inline error before
  // navigating. Subresource fetches are blocked in the native shells (mixed
  // content and certificate rules), so they navigate directly instead.
  async function probe(base) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), PROBE_MS);
    try {
      await fetch(`${base}/healthz`, {
        mode: "no-cors",
        signal: ctrl.signal,
        cache: "no-store",
      });
      return true;
    } catch {
      return false;
    } finally {
      clearTimeout(timer);
    }
  }

  function showError(target, msg) {
    target.textContent = msg;
    target.hidden = false;
  }

  function setView(view) {
    document.body.dataset.view = view;
    if (view === "settings") {
      sHostEl.value = hostEl.value;
      sSchemeEl.checked = schemeEl.checked;
      sNameEl.value = "";
      sErrEl.hidden = true;
      renderHosts();
      sHostEl.focus();
    } else {
      hostEl.focus();
    }
  }

  let pending = null;

  function persistAndGo(base, host, scheme) {
    store.set(HOST_KEY, host);
    store.set(SCHEME_KEY, scheme);
    window.location.replace(`${base}/`);
  }

  async function connect(host, scheme, errTarget) {
    const base = `${scheme}://${host}`;
    pending = { base, host, scheme };
    goEl.disabled = true;
    errEl.hidden = true;
    forceEl.hidden = true;
    try {
      if (SHELL === "desktop") {
        // Go probes, persists, and navigates; on failure it rejects and we
        // show the message inline.
        try {
          await window.dominionConnect(host, scheme);
        } catch (e) {
          showError(errTarget || errEl, String((e && e.message) || e));
          return false;
        }
        return true;
      }
      if (SHELL === "android") {
        // No subresource probe: persist and navigate top-level.
        persistAndGo(base, host, scheme);
        return true;
      }
      if (!(await probe(base))) {
        showError(
          errTarget || errEl,
          `Could not reach ${base}. Check the address and that the server is running.`
        );
        if (!errTarget) forceEl.hidden = false;
        return false;
      }
      persistAndGo(base, host, scheme);
      return true;
    } finally {
      goEl.disabled = false;
    }
  }

  // ---- saved-hosts rendering --------------------------------------------

  async function renderHosts() {
    const list = await hostStore.list();
    hostListEl.textContent = "";
    hostsEmptyEl.hidden = list.length !== 0;
    for (const h of list) {
      const li = document.createElement("li");

      const meta = document.createElement("span");
      meta.className = "meta";
      const name = document.createElement("span");
      name.className = "hname";
      name.textContent = h.name;
      const url = document.createElement("span");
      url.className = "hurl";
      url.textContent = `${h.scheme}://${h.host}`;
      meta.append(name, url);

      const use = document.createElement("button");
      use.type = "button";
      use.textContent = "Connect";
      use.addEventListener("click", () => {
        connect(h.host, h.scheme, sErrEl);
      });

      const del = document.createElement("button");
      del.type = "button";
      del.className = "host-del";
      del.textContent = "Delete";
      del.addEventListener("click", async () => {
        await hostStore.remove(h.name);
        renderHosts();
      });

      li.append(meta, use, del);
      hostListEl.appendChild(li);
    }
  }

  // ---- events ------------------------------------------------------------

  connectForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const host = normalize(hostEl.value);
    if (!host) {
      showError(errEl, "Enter an address like HOST:5550");
      return;
    }
    connect(host, schemeEl.checked ? "http" : "https");
  });

  forceEl.addEventListener("click", () => {
    if (pending) persistAndGo(pending.base, pending.host, pending.scheme);
  });

  openSettingsEl.addEventListener("click", () => setView("settings"));
  closeSettingsEl.addEventListener("click", () => setView("connect"));

  settingsForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const host = normalize(sHostEl.value);
    if (!host) {
      showError(sErrEl, "Enter an address like HOST:5550");
      return;
    }
    connect(host, sSchemeEl.checked ? "http" : "https", sErrEl);
  });

  el("s-save").addEventListener("click", async () => {
    const host = normalize(sHostEl.value);
    if (!host) {
      showError(sErrEl, "Enter an address like HOST:5550");
      return;
    }
    const scheme = sSchemeEl.checked ? "http" : "https";
    const name = (sNameEl.value || "").trim() || host;
    sErrEl.hidden = true;
    await hostStore.save({ name, host, scheme });
    sNameEl.value = "";
    renderHosts();
  });

  // ---- init --------------------------------------------------------------

  (async function init() {
    const savedHost = await store.get(HOST_KEY);
    const savedScheme = await store.get(SCHEME_KEY);
    const scheme = savedScheme || (SHELL === "desktop" ? "http" : "https");

    if (savedHost) {
      hostEl.value = savedHost;
      schemeEl.checked = scheme === "http";
      sHostEl.value = savedHost;
      sSchemeEl.checked = scheme === "http";
    }
    if (LOAD_ERROR) {
      showError(
        errEl,
        LOAD_ERROR === "unreachable"
          ? `Could not reach ${scheme}://${savedHost || "the server"}. Check the address and that the server is running.`
          : LOAD_ERROR
      );
    }

    if (START_SETTINGS) {
      setView("settings");
      return;
    }

    // The desktop shell decides when to connect (Go probes on launch), so its
    // prompt only ever appears when input is needed.
    const autoConnect = SHELL !== "desktop" && !FORCE_PROMPT && !LOAD_ERROR && savedHost;
    if (autoConnect) {
      await connect(savedHost, scheme);
    } else {
      setView("connect");
    }
  })();
})();
