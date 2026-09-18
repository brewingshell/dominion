// Test harness for web/app.js. Loads index.html into jsdom, stubs the browser
// APIs xterm/app.js depend on, and evaluates app.js with a small export hook
// appended so tests can drive its internals. This lives outside web/ so it is
// never picked up by go:embed.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { JSDOM } from "jsdom";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");

export function jsonResponse(status, body) {
  return {
    status,
    ok: status >= 200 && status < 300,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function stubTerminal(window) {
  window.Terminal = class {
    constructor() {
      this.cols = 80;
      this.rows = 24;
      this.options = {};
    }
    loadAddon() {}
    open() {}
    onData() {}
    onResize() {}
    focus() {}
    dispose() {}
    write() {}
    writeln() {}
  };
  window.FitAddon = { FitAddon: class { fit() {} } };
  window.__sockets = [];
  window.WebSocket = class {
    static OPEN = 1;
    constructor() {
      this.readyState = 0;
      this.sent = [];
      window.__sockets.push(this);
    }
    close() {
      this.readyState = 3;
    }
    send(data) {
      this.sent.push(data);
    }
  };
  if (!window.TextEncoder) window.TextEncoder = TextEncoder;
}

const instances = [];

export function cleanupAll() {
  while (instances.length) {
    const inst = instances.pop();
    try {
      inst.api.stopPolling();
    } catch {}
    try {
      inst.window.close();
    } catch {}
  }
}

// load builds a fresh app instance. `routes` maps a URL predicate to a
// response factory receiving { path, opts }. `options.url` sets the page URL,
// so tests can exercise the ?session= memory.
export function load(routes = [], options = {}) {
  const html = readFileSync(join(root, "web", "index.html"), "utf8");
  const dom = new JSDOM(html, {
    url: options.url || "http://localhost/",
    runScripts: "outside-only",
    pretendToBeVisual: true,
  });
  const window = dom.window;
  if (options.userAgent) {
    Object.defineProperty(window.navigator, "userAgent", {
      configurable: true,
      get: () => options.userAgent,
    });
  }

  stubTerminal(window);
  window.matchMedia = () => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  });
  window.requestAnimationFrame = (cb) => window.setTimeout(cb, 0);

  const calls = [];
  window.fetch = async (path, opts = {}) => {
    calls.push({ path, opts });
    if (opts.signal && opts.signal.aborted) {
      throw new window.DOMException("aborted", "AbortError");
    }
    for (const route of routes) {
      if (route.match(path, opts)) return route.respond(path, opts);
    }
    if (path === "/api/sessions") return jsonResponse(200, { sessions: [] });
    if (path === "/api/login") return jsonResponse(200, { ok: true });
    if (path.startsWith("/api/")) return jsonResponse(200, { ok: true });
    return jsonResponse(404, { error: "not found" });
  };
  window.addEventListener("unhandledrejection", () => {});

  // jsdom does not implement <dialog> modal methods.
  for (const dlg of window.document.querySelectorAll("dialog")) {
    dlg.showModal = function () {
      this.open = true;
    };
    dlg.close = function () {
      this.open = false;
    };
  }

  const src = readFileSync(join(root, "web", "app.js"), "utf8");
  // Tests may inject shell globals (e.g. window.dominionChangeURL) before the
  // app is evaluated.
  if (typeof options.beforeEval === "function") options.beforeEval(window);
  const marker = "})();";
  const idx = src.lastIndexOf(marker);
  const hook =
    "globalThis.__dominion={ctrlChar,applyModifiers,activeMods,state,applySessions," +
    "renderSessions,activate,refreshActiveChrome,updateGlobalStatus,stopPolling," +
    "startPolling,poll,api,inShell,shellChangeTarget,shellSettingsTarget," +
    "applyTheme,currentTheme," +
    "canManageServers,shellVersion,changePIN,setPinError," +
    "TERM_THEMES};\n";
  window.eval(src.slice(0, idx) + hook + src.slice(idx));

  const api = window.__dominion;
  const tick = (ms = 0) => new Promise((r) => setTimeout(r, ms));

  const inst = { dom, window, document: window.document, api, calls, tick };
  instances.push(inst);
  return inst;
}
