(() => {
  "use strict";

  const HOST_KEY = "dominion.host";
  const SCHEME_KEY = "dominion.scheme";
  const PROBE_MS = 6000;

  const formEl = document.getElementById("connect");
  const hostEl = document.getElementById("host");
  const schemeEl = document.getElementById("insecure");
  const errEl = document.getElementById("error");
  const goEl = document.getElementById("go");

  // Preferences is available inside the native shell; localStorage is the
  // fallback when the page is opened in a plain browser.
  const store = {
    async get(key) {
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

  async function probe(base) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), PROBE_MS);
    try {
      const res = await fetch(`${base}/healthz`, { signal: ctrl.signal, cache: "no-store" });
      return res.ok;
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

  async function connect(host, scheme) {
    const base = `${scheme}://${host}`;
    goEl.disabled = true;
    errEl.hidden = true;
    try {
      if (!(await probe(base))) {
        showError(`Could not reach ${base}. Check the address and that the server is running.`);
        return false;
      }
      await store.set(HOST_KEY, host);
      await store.set(SCHEME_KEY, scheme);
      window.location.replace(`${base}/`);
      return true;
    } finally {
      goEl.disabled = false;
    }
  }

  formEl.addEventListener("submit", (e) => {
    e.preventDefault();
    const host = normalize(hostEl.value);
    if (!host) {
      showError("Enter an address like YOUR-HOST:5550");
      return;
    }
    connect(host, schemeEl.checked ? "http" : "https");
  });

  (async function init() {
    const [savedHost, savedScheme] = await Promise.all([
      store.get(HOST_KEY),
      store.get(SCHEME_KEY),
    ]);
    if (savedHost) {
      hostEl.value = savedHost;
      const scheme = savedScheme || "https";
      schemeEl.checked = scheme === "http";
      // Reconnect automatically; on failure the form stays for editing.
      await connect(savedHost, scheme);
    } else {
      hostEl.focus();
    }
  })();
})();
