// Tests for the shell address prompt (apps/www/bootstrap.js): the settings view
// must only open when the shell can actually persist saved servers, otherwise
// it falls back to Connect with a notice instead of showing a dead screen.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { JSDOM } from "jsdom";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");
const html = readFileSync(join(root, "apps", "www", "index.html"), "utf8");
const src = readFileSync(join(root, "apps", "www", "bootstrap.js"), "utf8");

const instances = [];
after(() => {
  while (instances.length) instances.pop().window.close();
});

// load evaluates bootstrap.js with the given shell globals set before it runs.
function load({ init = () => {} } = {}) {
  const dom = new JSDOM(html, {
    url: "https://localhost/",
    runScripts: "outside-only",
    pretendToBeVisual: true,
  });
  const { window } = dom;
  window.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
  init(window);
  window.eval(src);
  const inst = { window, document: window.document };
  instances.push(inst);
  return inst;
}

test("desktop with host bindings opens the settings view", async () => {
  const { window, document } = load({
    init: (w) => {
      w.__dominionView = "settings";
      w.dominionConnect = async () => "ok";
      w.dominionListHosts = async () => "[]";
      w.dominionSaveHost = async () => "ok";
    },
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(document.body.dataset.view, "settings");
  assert.equal(window.document.getElementById("error").hidden, true);
});

test("desktop without host bindings falls back to Connect with a notice", async () => {
  const { window, document } = load({
    init: (w) => {
      w.__dominionView = "settings";
      w.dominionConnect = async () => "ok"; // inShell, but no address-book bindings
    },
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(document.body.dataset.view, "connect");
  const err = document.getElementById("error");
  assert.equal(err.hidden, false);
  assert.match(err.textContent, /cannot manage saved servers/i);
});

test("Android shell without Preferences falls back to Connect with a notice", async () => {
  const { document } = load({
    init: (w) => {
      w.__dominionView = "settings";
      Object.defineProperty(w.navigator, "userAgent", {
        configurable: true,
        get: () => "Mozilla/5.0 dominion-shell/1.1",
      });
    },
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(document.body.dataset.view, "connect");
  assert.match(document.getElementById("error").textContent, /cannot manage saved servers/i);
});

test("Android shell with Preferences opens the settings view", async () => {
  const { window, document } = load({
    init: (w) => {
      w.__dominionView = "settings";
      Object.defineProperty(w.navigator, "userAgent", {
        configurable: true,
        get: () => "Mozilla/5.0 dominion-shell/1.1",
      });
      w.Capacitor = {
        Plugins: {
          Preferences: {
            get: async () => ({ value: "[]" }),
            set: async () => {},
          },
        },
      };
    },
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(document.body.dataset.view, "settings");
  assert.equal(document.getElementById("hosts-empty").hidden, false);
});
