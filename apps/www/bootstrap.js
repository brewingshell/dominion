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
  const forceEl = document.getElementById("force");

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

  // probe checks reachability only. The portal is a different origin from the
  // app bundle (https://localhost), so a plain CORS fetch would be blocked:
  // use mode "no-cors", where a resolved (opaque) response means the request
  // completed and a rejection means it did not.
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

  // pending holds the last address, so "Connect anyway" can bypass a failed
  // probe (the probe can be a false negative on some origins/certs).
  let pending = null;

  function go(base, host, scheme) {
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
      if (!(await probe(base))) {
        showError(`Could not reach ${base}. Check the address and that the server is running.`);
        forceEl.hidden = false;
        return false;
      }
      go(base, host, scheme);
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

  forceEl.addEventListener("click", () => {
    if (pending) go(pending.base, pending.host, pending.scheme);
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
