(() => {
  "use strict";

  const HOST_KEY = "dominion.host";
  const SCHEME_KEY = "dominion.scheme";
  const PROBE_MS = 6000;

  const params = new URLSearchParams(window.location.search);
  // Which native shell are we in? Desktop injects a Go binding; Android is
  // marked with an appended user agent. A plain browser is neither.
  const SHELL = window.dominionConnect
    ? "desktop"
    : navigator.userAgent.includes("dominion-shell")
      ? "android"
      : "browser";
  const FORCE_PROMPT = params.get("change") === "1";
  // The desktop shell has no URL params; it inlines the error instead.
  const LOAD_ERROR = params.get("error") || window.__dominionLoadError || "";

  const formEl = document.getElementById("connect");
  const hostEl = document.getElementById("host");
  const schemeEl = document.getElementById("insecure");
  const errEl = document.getElementById("error");
  const goEl = document.getElementById("go");
  const forceEl = document.getElementById("force");

  // Persistence: the desktop shell owns it in Go and inlines the saved values;
  // Android uses Capacitor Preferences; a browser falls back to localStorage.
  const store = {
    async get(key) {
      const inline = window.__dominionSaved && window.__dominionSaved[key];
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

  function showError(msg) {
    errEl.textContent = msg;
    errEl.hidden = false;
  }

  let pending = null;

  function persistAndGo(base, host, scheme) {
    store.set(HOST_KEY, host);
    store.set(SCHEME_KEY, scheme);
    window.location.replace(`${base}/`);
  }

  async function connect(host, scheme) {
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
          showError(String((e && e.message) || e));
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
        showError(`Could not reach ${base}. Check the address and that the server is running.`);
        forceEl.hidden = false;
        return false;
      }
      persistAndGo(base, host, scheme);
      return true;
    } finally {
      goEl.disabled = false;
    }
  }

  formEl.addEventListener("submit", (e) => {
    e.preventDefault();
    const host = normalize(hostEl.value);
    if (!host) {
      showError("Enter an address like HOST:5550");
      return;
    }
    connect(host, schemeEl.checked ? "http" : "https");
  });

  forceEl.addEventListener("click", () => {
    if (pending) persistAndGo(pending.base, pending.host, pending.scheme);
  });

  (async function init() {
    const savedHost = await store.get(HOST_KEY);
    const savedScheme = await store.get(SCHEME_KEY);
    const scheme = savedScheme || (SHELL === "desktop" ? "http" : "https");

    if (savedHost) {
      hostEl.value = savedHost;
      schemeEl.checked = scheme === "http";
    }
    if (LOAD_ERROR) {
      showError(
        LOAD_ERROR === "unreachable"
          ? `Could not reach ${scheme}://${savedHost || "the server"}. Check the address and that the server is running.`
          : LOAD_ERROR
      );
    }

    // The desktop shell decides when to connect (Go probes on launch), so its
    // prompt only ever appears when input is needed.
    const autoConnect = SHELL !== "desktop" && !FORCE_PROMPT && !LOAD_ERROR && savedHost;
    if (autoConnect) {
      await connect(savedHost, scheme);
    } else {
      hostEl.focus();
    }
  })();
})();
