(() => {
  "use strict";

  const loginEl = document.getElementById("login");
  const loginForm = document.getElementById("login-form");
  const pinEl = document.getElementById("pin");
  const loginError = document.getElementById("login-error");
  const loginSub = document.getElementById("login-sub");
  const appEl = document.getElementById("app");
  const drawerEl = document.getElementById("drawer");
  const tablistEl = document.getElementById("tablist");
  const terminalsEl = document.getElementById("terminals");
  const emptyEl = document.getElementById("empty");
  const statusEl = document.getElementById("status");
  const lockEl = document.getElementById("lock");
  const logoutEl = document.getElementById("logout");
  const menuToggleEl = document.getElementById("menu-toggle");
  const currentEl = document.getElementById("current");
  const scrimEl = document.getElementById("scrim");
  const drawerCloseEl = document.getElementById("drawer-close");
  const keysEl = document.getElementById("keys");
  const keysToggleEl = document.getElementById("keys-toggle");
  const newSessionEl = document.getElementById("new-session");
  const newDialog = document.getElementById("new-dialog");
  const newForm = document.getElementById("new-form");
  const newNameEl = document.getElementById("new-name");
  const newErrorEl = document.getElementById("new-error");
  const confirmDialog = document.getElementById("confirm-dialog");
  const confirmForm = document.getElementById("confirm-form");
  const confirmTextEl = document.getElementById("confirm-text");
  const confirmErrorEl = document.getElementById("confirm-error");
  const mobileQuery = window.matchMedia("(max-width: 768px)");

  const POLL_MS = 3000;
  const REQUEST_TIMEOUT_MS = 10000;
  const PINNED_SESSION = "dominion";
  const KEYS_PREF = "dominion.keys";
  const ACTIVE_PREF = "dominion.active";
  const DESKTOP_FONT = 14;
  const MOBILE_FONT = DESKTOP_FONT * 0.75;
  const KEY_SEQ = {
    esc: "\x1b",
    tab: "\x09",
    left: "\x1b[D",
    up: "\x1b[A",
    down: "\x1b[B",
    right: "\x1b[C",
  };
  const MOD_KEYS = ["ctrl", "alt"];
  const encoder = new TextEncoder();
  const activeMods = new Set();
  let tabSeq = 0;

  const state = {
    sessions: [],
    tabs: new Map(),
    active: null,
    poll: null,
    locked: false,
    loggedIn: false,
    pendingKill: null,
  };

  const TERM_THEME = {
    background: "#0b0e14",
    foreground: "#c7ccd6",
    cursor: "#7aa2f7",
    selectionBackground: "#2a3040",
    black: "#151822",
    red: "#f7768e",
    green: "#9ece6a",
    yellow: "#e0af68",
    blue: "#7aa2f7",
    magenta: "#bb9af7",
    cyan: "#7dcfff",
    white: "#c7ccd6",
  };

  async function api(path, opts = {}) {
    const { timeout = REQUEST_TIMEOUT_MS, ...rest } = opts;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeout);
    try {
      return await fetch(path, {
        credentials: "same-origin",
        signal: ctrl.signal,
        ...rest,
      });
    } finally {
      clearTimeout(timer);
    }
  }

  function apiJSON(path, body) {
    return api(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  async function errorText(res, fallback) {
    try {
      const data = await res.json();
      if (data && data.error) return data.error;
    } catch {}
    return fallback;
  }

  function showLogin(msg) {
    state.loggedIn = false;
    setMenu(false);
    appEl.hidden = true;
    loginEl.hidden = false;
    loginSub.textContent = state.locked
      ? "Locked \u2014 enter PIN to continue"
      : "Enter PIN to continue";
    if (msg) {
      loginError.textContent = msg;
      loginError.hidden = false;
    }
    pinEl.focus();
  }

  function showApp() {
    state.locked = false;
    state.loggedIn = true;
    loginEl.hidden = true;
    appEl.hidden = false;
  }

  function setMenu(open) {
    drawerEl.classList.toggle("open", open);
    scrimEl.classList.toggle("show", open);
    menuToggleEl.setAttribute("aria-expanded", open ? "true" : "false");
    menuToggleEl.setAttribute("aria-label", open ? "Hide sessions" : "Show sessions");
    if (open) {
      const active = tablistEl.querySelector(".tab-main[tabindex='0']");
      if (active) active.focus();
    }
    if (!open && mobileQuery.matches && document.activeElement === drawerCloseEl) {
      menuToggleEl.focus();
    }
  }

  function closeMenu() {
    setMenu(false);
  }

  function ctrlChar(ch) {
    if (ch === " ") return "\x00";
    if (ch === "?") return "\x7f";
    const code = ch.toUpperCase().charCodeAt(0);
    return code >= 64 && code < 128 ? String.fromCharCode(code & 0x1f) : ch;
  }

  function applyModifiers(str) {
    if (!activeMods.size || !str) return str;
    let out = str[0];
    if (activeMods.has("ctrl")) out = ctrlChar(out);
    out += str.slice(1);
    if (activeMods.has("alt")) out = "\x1b" + out;
    clearMods();
    return out;
  }

  function setMod(mod, on) {
    if (on) activeMods.add(mod);
    else activeMods.delete(mod);
    for (const btn of keysEl.querySelectorAll("button.key-mod")) {
      if (btn.dataset.mod !== mod) continue;
      btn.classList.toggle("armed", on);
      btn.setAttribute("aria-pressed", on ? "true" : "false");
    }
  }

  function clearMods() {
    for (const mod of MOD_KEYS) setMod(mod, false);
  }

  function sendData(str) {
    const tab = state.active ? state.tabs.get(state.active) : null;
    if (tab && tab.ws && tab.ws.readyState === WebSocket.OPEN) {
      tab.ws.send(encoder.encode(applyModifiers(str)));
    }
  }

  function keysAllowed() {
    return mobileQuery.matches && localStorage.getItem(KEYS_PREF) !== "0";
  }

  function appVisible() {
    return !loginEl.hidden;
  }

  function setKeysVisible(on, persist) {
    keysEl.hidden = !on;
    if (!on) clearMods();
    keysToggleEl.setAttribute("aria-pressed", on ? "true" : "false");
    keysToggleEl.setAttribute(
      "aria-label",
      on ? "Hide terminal keys" : "Show terminal keys"
    );
    if (persist) {
      try {
        localStorage.setItem(KEYS_PREF, on ? "1" : "0");
      } catch {}
    }
    requestAnimationFrame(() => {
      if (state.active) fitTab(state.tabs.get(state.active));
    });
  }

  function refitActive() {
    if (state.active) fitTab(state.tabs.get(state.active));
  }

  function termFontSize() {
    return mobileQuery.matches ? MOBILE_FONT : DESKTOP_FONT;
  }

  async function checkAuth() {
    const res = await api("/api/sessions");
    if (res.status === 401) {
      showLogin();
      return false;
    }
    if (!res.ok) {
      showLogin("server error");
      return false;
    }
    const data = await res.json();
    showApp();
    applySessions(data);
    return true;
  }

  function applySessions(data) {
    state.sessions = (data && data.sessions) || [];
    renderSessions();
  }

  loginForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    loginError.hidden = true;
    let res;
    try {
      res = await api("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ pin: pinEl.value }),
      });
    } catch {
      showLogin("network error");
      return;
    }
    if (res.ok) {
      pinEl.value = "";
      if (await checkAuth()) startPolling();
      return;
    }
    let msg = "incorrect pin";
    try {
      msg = (await res.json()).error || msg;
    } catch {}
    showLogin(msg);
  });

  menuToggleEl.addEventListener("click", () => {
    setMenu(!drawerEl.classList.contains("open"));
  });

  scrimEl.addEventListener("click", closeMenu);
  drawerCloseEl.addEventListener("click", closeMenu);

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && drawerEl.classList.contains("open")) closeMenu();
  });

  tablistEl.addEventListener("keydown", (e) => {
    const keys = ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"];
    if (!keys.includes(e.key)) return;
    const mains = [...tablistEl.querySelectorAll(".tab-main")];
    if (!mains.length) return;
    const current = mains.indexOf(document.activeElement);
    if (current === -1) return;
    e.preventDefault();
    let next = current;
    if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (current - 1 + mains.length) % mains.length;
    else if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (current + 1) % mains.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = mains.length - 1;
    const target = mains[next];
    target.focus();
    const tab = [...state.tabs.values()].find((t) => t.tabEl && t.tabEl.contains(target));
    if (tab) activate(tab.name);
  });

  mobileQuery.addEventListener("change", () => {
    closeMenu();
    clearMods();
    setKeysVisible(keysAllowed(), false);
    const size = termFontSize();
    for (const tab of state.tabs.values()) {
      if (tab.term) tab.term.options.fontSize = size;
    }
    refitActive();
  });

  keysToggleEl.addEventListener("click", () => {
    setKeysVisible(keysEl.hidden, true);
  });

  keysEl.addEventListener("pointerdown", (e) => {
    e.preventDefault();
  });

  keysEl.addEventListener("click", (e) => {
    const modBtn = e.target.closest("button.key-mod");
    if (modBtn) {
      const mod = modBtn.dataset.mod;
      setMod(mod, !activeMods.has(mod));
      return;
    }
    const btn = e.target.closest("button.key");
    if (!btn) return;
    const seq = KEY_SEQ[btn.dataset.key];
    if (seq === undefined) return;
    sendData(seq);
  });

  lockEl.addEventListener("click", async () => {
    closeMenu();
    clearMods();
    stopPolling();
    state.locked = true;
    showLogin();
    try {
      await api("/api/lock", { method: "POST" });
    } catch {}
  });

  logoutEl.addEventListener("click", async () => {
    state.locked = false;
    try {
      await api("/api/logout", { method: "POST" });
    } catch {}
    stopPolling();
    clearMods();
    for (const tab of state.tabs.values()) destroyTab(tab);
    state.tabs.clear();
    state.active = null;
    showLogin();
  });

  newSessionEl.addEventListener("click", () => {
    closeMenu();
    newErrorEl.hidden = true;
    newForm.reset();
    newDialog.showModal();
    newNameEl.focus();
  });

  newForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = newNameEl.value.trim();
    if (!name) return;
    newErrorEl.hidden = true;
    let res;
    try {
      res = await apiJSON("/api/sessions/create", { name });
    } catch {
      newErrorEl.textContent = "network error";
      newErrorEl.hidden = false;
      return;
    }
    if (!res.ok) {
      newErrorEl.textContent = await errorText(res, "failed to create session");
      newErrorEl.hidden = false;
      return;
    }
    newDialog.close();
    await poll();
    if (state.tabs.has(name)) activate(name);
  });

  confirmForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = state.pendingKill;
    if (!name) {
      confirmDialog.close();
      return;
    }
    confirmErrorEl.hidden = true;
    let res;
    try {
      res = await apiJSON("/api/sessions/kill", { name });
    } catch {
      confirmErrorEl.textContent = "network error";
      confirmErrorEl.hidden = false;
      return;
    }
    if (!res.ok && res.status !== 404) {
      confirmErrorEl.textContent = await errorText(res, "failed to kill session");
      confirmErrorEl.hidden = false;
      return;
    }
    state.pendingKill = null;
    confirmDialog.close();
    await poll();
  });

  for (const btn of document.querySelectorAll(".dialog [data-close]")) {
    btn.addEventListener("click", () => btn.closest("dialog").close());
  }

  for (const dlg of [newDialog, confirmDialog]) {
    dlg.addEventListener("click", (e) => {
      if (e.target === dlg) dlg.close();
    });
  }

  function promptKill(name) {
    const tab = state.tabs.get(name);
    state.pendingKill = name;
    confirmErrorEl.hidden = true;
    const attached = !!(tab && tab.sess && tab.sess.attached);
    confirmTextEl.innerHTML = "";
    const lead = document.createElement("span");
    lead.textContent = "Kill session ";
    const strong = document.createElement("strong");
    strong.textContent = name;
    const tail = document.createElement("span");
    tail.textContent = attached
      ? "? It is attached right now, and everything running in it will be terminated."
      : "? Everything running in it will be terminated.";
    confirmTextEl.append(lead, strong, tail);
    confirmDialog.showModal();
  }

  function renderSessions() {
    const names = new Set(state.sessions.map((s) => s.name));

    for (const [name, tab] of state.tabs) {
      if (!names.has(name)) {
        destroyTab(tab);
        state.tabs.delete(name);
        if (state.active === name) state.active = null;
      }
    }

    for (const sess of state.sessions) {
      let tab = state.tabs.get(sess.name);
      if (!tab) {
        tab = {
          name: sess.name,
          sess,
          term: null,
          fit: null,
          ws: null,
          el: null,
          tabEl: null,
          status: "idle",
          retry: 0,
          reconnectTimer: null,
        };
        createTabButton(tab);
        state.tabs.set(sess.name, tab);
      } else {
        tab.sess = sess;
        updateTabButton(tab);
      }
    }

    emptyEl.hidden = state.sessions.length !== 0;

    if (state.active && state.tabs.has(state.active)) {
      // Re-render chrome only. Polling must not clear armed modifiers,
      // write localStorage, steal focus, or refit the terminal.
      refreshActiveChrome();
    } else {
      const start = initialSession();
      if (start) activate(start);
      else updateGlobalStatus();
    }
  }

  // initialSession picks the tab to open: an explicit ?session= wins, then the
  // value remembered in localStorage, then the first non-pinned session.
  function initialSession() {
    let wanted = null;
    try {
      wanted = new URLSearchParams(location.search).get("session");
    } catch {}
    if (!wanted) {
      try {
        wanted = localStorage.getItem(ACTIVE_PREF);
      } catch {}
    }
    if (wanted && state.tabs.has(wanted)) return wanted;
    const fallback = state.sessions.find((s) => s.name !== PINNED_SESSION);
    if (fallback) return fallback.name;
    return state.sessions.length ? state.sessions[0].name : null;
  }

  function createTabButton(tab) {
    const pinned = tab.name === PINNED_SESSION;
    const wrap = document.createElement("div");
    wrap.className = "tab";
    wrap.setAttribute("role", "presentation");
    if (pinned) wrap.classList.add("pinned");

    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "tab-main";
    btn.setAttribute("role", "tab");
    btn.id = `tab-${++tabSeq}`;
    btn.setAttribute("aria-selected", "false");
    btn.setAttribute("tabindex", "-1");
    tab.tabId = btn.id;
    btn.setAttribute("aria-controls", `panel-${btn.id}`);

    const dot = document.createElement("span");
    dot.className = "dot";
    dot.dataset.status = "idle";

    const name = document.createElement("span");
    name.className = "name";
    name.textContent = tab.name;

    btn.append(dot, name);
    btn.addEventListener("click", () => {
      closeMenu();
      activate(tab.name);
    });

    wrap.append(btn);
    // The pinned session hosts the portal, so it has no kill control.
    if (!pinned) {
      const kill = document.createElement("button");
      kill.type = "button";
      kill.className = "tab-kill";
      kill.setAttribute("aria-label", `Kill session ${tab.name}`);
      kill.title = `Kill ${tab.name}`;
      kill.innerHTML = "&times;";
      kill.addEventListener("click", (e) => {
        e.stopPropagation();
        promptKill(tab.name);
      });
      wrap.append(kill);
    }

    // The pinned session is always the first tab, regardless of API order.
    if (pinned) tablistEl.prepend(wrap);
    else tablistEl.appendChild(wrap);
    tab.tabEl = wrap;
    updateTabButton(tab);
  }

  function updateTabButton(tab) {
    if (!tab.tabEl) return;
    const attached = !!(tab.sess && tab.sess.attached);
    tab.tabEl.classList.toggle("attached", attached);
    const windows = tab.sess ? tab.sess.windows : "?";
    tab.tabEl.title = `${tab.name} - ${windows} window(s)${attached ? ", attached" : ""}`;
  }

  // refreshActiveChrome updates tab styling for the current active session
  // without side effects (no modifier reset, persistence, focus, or refit).
  // Used by polling and other passive re-renders.
  function refreshActiveChrome() {
    for (const [n, t] of state.tabs) {
      if (!t.tabEl) continue;
      const on = n === state.active;
      t.tabEl.classList.toggle("active", on);
      const main = t.tabEl.querySelector(".tab-main");
      if (main) {
        main.setAttribute("aria-selected", on ? "true" : "false");
        main.setAttribute("tabindex", on ? "0" : "-1");
      }
      if (t.el) t.el.style.display = on ? "block" : "none";
    }
    updateGlobalStatus();
  }

  function activate(name) {
    const tab = state.tabs.get(name);
    if (!tab) return;
    state.active = name;
    clearMods();
    rememberActive(name);
    if (!tab.term) createTerminal(tab);
    refreshActiveChrome();
    requestAnimationFrame(() => {
      fitTab(tab);
      if (appVisible() && tab.term && !drawerEl.classList.contains("open")) tab.term.focus();
    });
  }

  // rememberActive persists the active session so a reload or app re-entry
  // reopens it: ?session= for the browser URL, localStorage for the shells.
  function rememberActive(name) {
    try {
      localStorage.setItem(ACTIVE_PREF, name);
    } catch {}
    try {
      const url = new URL(location.href);
      url.searchParams.set("session", name);
      history.replaceState(null, "", url);
    } catch {}
  }

  function createTerminal(tab) {
    const el = document.createElement("div");
    el.className = "terminal";
    if (tab.tabId) {
      el.id = `panel-${tab.tabId}`;
      el.setAttribute("role", "tabpanel");
      el.setAttribute("aria-labelledby", tab.tabId);
    }
    terminalsEl.appendChild(el);

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", "Fira Code", ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: termFontSize(),
      lineHeight: 1.15,
      scrollback: 5000,
      theme: TERM_THEME,
    });
    const fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    term.open(el);

    tab.el = el;
    tab.term = term;
    tab.fit = fit;

    term.onData((data) => {
      if (tab.ws && tab.ws.readyState === WebSocket.OPEN) {
        tab.ws.send(encoder.encode(applyModifiers(data)));
      }
    });
    term.onResize(({ cols, rows }) => sendResize(tab, cols, rows));

    connect(tab);
  }

  function connect(tab) {
    if (tab.ws) {
      const old = tab.ws;
      tab.ws = null;
      try {
        old.close();
      } catch {}
    }
    const cols = tab.term ? tab.term.cols : 80;
    const rows = tab.term ? tab.term.rows : 24;
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const url =
      `${proto}//${location.host}/api/attach?session=${encodeURIComponent(tab.name)}` +
      `&cols=${cols}&rows=${rows}`;

    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    tab.ws = ws;
    setTabStatus(tab, "connecting");

    ws.onopen = () => {
      tab.retry = 0;
      setTabStatus(tab, "connected");
      sendResize(tab, tab.term.cols, tab.term.rows);
      if (appVisible()) tab.term.focus();
    };
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        tab.term.write(new Uint8Array(ev.data));
      } else if (typeof ev.data === "string") {
        tab.term.write(ev.data);
      }
    };
    ws.onclose = (ev) => {
      if (tab.ws === ws) tab.ws = null;
      setTabStatus(tab, "disconnected");
      if (ev && ev.code === 4001) {
        // Token revoked (logout or expiry): do not reconnect, require PIN.
        stopPolling();
        showLogin();
        return;
      }
      if (state.tabs.has(tab.name) && state.sessions.some((s) => s.name === tab.name)) {
        scheduleReconnect(tab);
      }
    };
    ws.onerror = () => {};
  }

  function scheduleReconnect(tab) {
    if (tab.reconnectTimer) return;
    const delay = Math.min(1000 * Math.pow(2, tab.retry), 15000);
    tab.retry += 1;
    tab.reconnectTimer = setTimeout(() => {
      tab.reconnectTimer = null;
      if (state.tabs.has(tab.name) && state.sessions.some((s) => s.name === tab.name)) {
        connect(tab);
      }
    }, delay);
  }

  function sendResize(tab, cols, rows) {
    if (tab.ws && tab.ws.readyState === WebSocket.OPEN) {
      tab.ws.send(JSON.stringify({ type: "resize", cols, rows }));
    }
  }

  function fitTab(tab) {
    if (!tab || !tab.fit || !tab.el) return;
    if (tab.el.offsetParent === null) return;
    try {
      tab.fit.fit();
    } catch {}
  }

  function setTabStatus(tab, status) {
    tab.status = status;
    if (tab.tabEl) {
      const dot = tab.tabEl.querySelector(".dot");
      if (dot) dot.dataset.status = status;
    }
    if (state.active === tab.name) updateGlobalStatus();
  }

  function updateGlobalStatus() {
    const tab = state.active ? state.tabs.get(state.active) : null;
    const status = tab ? tab.status : "";
    const name = state.active || "";
    if (statusEl.textContent === status && statusEl.dataset.status === status) {
      // Avoid re-announcing the same status to assistive tech on each poll.
      if (currentEl.textContent !== name) currentEl.textContent = name;
      return;
    }
    statusEl.textContent = status;
    statusEl.dataset.status = status;
    currentEl.textContent = name;
  }

  function destroyTab(tab) {
    if (tab.reconnectTimer) {
      clearTimeout(tab.reconnectTimer);
      tab.reconnectTimer = null;
    }
    if (tab.ws) {
      const ws = tab.ws;
      tab.ws = null;
      try {
        ws.close();
      } catch {}
    }
    if (tab.term) {
      try {
        tab.term.dispose();
      } catch {}
      tab.term = null;
    }
    if (tab.el) tab.el.remove();
    if (tab.tabEl) tab.tabEl.remove();
  }

  async function poll() {
    let res;
    try {
      res = await api("/api/sessions");
    } catch {
      return;
    }
    if (res.status === 401) {
      stopPolling();
      showLogin();
      return;
    }
    if (!res.ok) return;
    applySessions(await res.json());
  }

  function startPolling() {
    stopPolling();
    if (document.hidden) return;
    state.poll = setInterval(poll, POLL_MS);
  }

  function stopPolling() {
    if (state.poll) {
      clearInterval(state.poll);
      state.poll = null;
    }
  }

  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      stopPolling();
    } else if (state.loggedIn) {
      poll();
      startPolling();
    }
  });

  function debounce(fn, ms) {
    let t = null;
    return (...args) => {
      if (t) clearTimeout(t);
      t = setTimeout(() => fn(...args), ms);
    };
  }

  window.addEventListener(
    "resize",
    debounce(() => {
      if (state.active) fitTab(state.tabs.get(state.active));
    }, 150)
  );

  window.addEventListener("focus", () => {
    const tab = state.active ? state.tabs.get(state.active) : null;
    if (tab && tab.term && appVisible() && !drawerEl.classList.contains("open")) tab.term.focus();
  });

  if (window.visualViewport) {
    window.visualViewport.addEventListener(
      "resize",
      debounce(refitActive, 150)
    );
  }

  setKeysVisible(keysAllowed(), false);

  (async function init() {
    if (await checkAuth()) startPolling();
  })();
})();
