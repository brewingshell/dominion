import { test, after } from "node:test";
import assert from "node:assert/strict";
import { load, jsonResponse, cleanupAll } from "./harness.mjs";

after(() => cleanupAll());

function sessions(list) {
  return jsonResponse(200, { sessions: list });
}

test("ctrlChar maps control characters", () => {
  const { api } = load();
  assert.equal(api.ctrlChar("c"), "\x03");
  assert.equal(api.ctrlChar("C"), "\x03");
  assert.equal(api.ctrlChar(" "), "\x00");
  assert.equal(api.ctrlChar("?"), "\x7f");
});

test("applyModifiers applies ctrl then alt and clears them", () => {
  const { api } = load();
  api.activeMods.add("ctrl");
  assert.equal(api.applyModifiers("c"), "\x03");
  assert.equal(api.activeMods.size, 0, "ctrl should be one-shot");

  api.activeMods.add("ctrl");
  api.activeMods.add("alt");
  assert.equal(api.applyModifiers("b"), "\x1b\x02");
  assert.equal(api.activeMods.size, 0);
});

test("applyModifiers is a no-op without modifiers", () => {
  const { api } = load();
  assert.equal(api.applyModifiers("hello"), "hello");
});

test("renderSessions reconciles tabs as sessions appear and vanish", () => {
  const { api, document } = load();
  const tablist = document.getElementById("tablist");

  api.applySessions({ sessions: [
    { name: "a", windows: 1, attached: false },
    { name: "b", windows: 2, attached: false },
  ] });
  assert.equal(tablist.querySelectorAll(".tab").length, 2);
  assert.ok(api.state.tabs.has("a") && api.state.tabs.has("b"));

  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  assert.equal(tablist.querySelectorAll(".tab").length, 1);
  assert.ok(!api.state.tabs.has("b"), "removed tab should be destroyed");

  api.applySessions({ sessions: [] });
  assert.equal(tablist.querySelectorAll(".tab").length, 0);
});

test("empty state is shown only when there are no sessions", () => {
  const { api, document } = load();
  const empty = document.getElementById("empty");

  api.applySessions({ sessions: [] });
  assert.equal(empty.hidden, false);

  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  assert.equal(empty.hidden, true);
});

test("active tab exposes aria-selected and its panel a tabpanel role", () => {
  const { api, document } = load();
  api.applySessions({ sessions: [
    { name: "a", windows: 1, attached: false },
    { name: "b", windows: 1, attached: false },
  ] });

  const active = document.querySelector(".tab.active .tab-main");
  assert.ok(active, "a tab should be active");
  assert.equal(active.getAttribute("aria-selected"), "true");
  assert.equal(
    document.querySelectorAll(".tab-main[aria-selected='false']").length,
    1
  );

  const panel = document.getElementById(active.getAttribute("aria-controls"));
  assert.ok(panel, "tab should control an existing panel");
  assert.equal(panel.getAttribute("role"), "tabpanel");
  assert.equal(panel.getAttribute("aria-labelledby"), active.id);
});

test("creating a session adds and activates a tab", async () => {
  const created = [];
  const { window, document, api, tick } = load([
    {
      match: (p) => p === "/api/sessions/create",
      respond: (p, opts) => {
        created.push(JSON.parse(opts.body).name);
        return jsonResponse(200, { ok: true });
      },
    },
    { match: (p) => p === "/api/sessions", respond: () => sessions([{ name: "fresh", windows: 1, attached: false }]) },
  ]);

  document.getElementById("new-session").click();
  assert.equal(document.getElementById("new-dialog").open, true);

  document.getElementById("new-name").value = "fresh";
  document.getElementById("new-form").dispatchEvent(
    new window.Event("submit", { bubbles: true, cancelable: true })
  );
  await tick(10);

  assert.deepEqual(created, ["fresh"]);
  assert.ok(api.state.tabs.has("fresh"), "new session should have a tab");
  assert.equal(api.state.active, "fresh", "new session should be active");
  api.stopPolling();
});

test("killing a session removes its tab", async () => {
  const killed = [];
  let list = [
    { name: "keep", windows: 1, attached: false },
    { name: "doomed", windows: 1, attached: false },
  ];
  const { window, document, api, tick } = load([
    {
      match: (p) => p === "/api/sessions/kill",
      respond: (p, opts) => {
        const name = JSON.parse(opts.body).name;
        killed.push(name);
        list = list.filter((s) => s.name !== name);
        return jsonResponse(200, { ok: true });
      },
    },
    { match: (p) => p === "/api/sessions", respond: () => sessions(list) },
  ]);

  api.applySessions({ sessions: list });
  assert.equal(document.querySelectorAll(".tab").length, 2);

  const doomed = api.state.tabs.get("doomed");
  doomed.tabEl.querySelector(".tab-kill").click();
  assert.equal(document.getElementById("confirm-dialog").open, true);
  assert.match(document.getElementById("confirm-text").textContent, /doomed/);

  document.getElementById("confirm-form").dispatchEvent(
    new window.Event("submit", { bubbles: true, cancelable: true })
  );
  await tick(10);

  assert.deepEqual(killed, ["doomed"]);
  assert.equal(document.querySelectorAll(".tab").length, 1);
  assert.ok(!api.state.tabs.has("doomed"));
  api.stopPolling();
});

test("re-render does not clear armed modifiers", () => {
  const { api } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  api.activeMods.add("ctrl");
  api.renderSessions();
  assert.equal(api.activeMods.has("ctrl"), true, "poll must not disarm modifiers");
});

test("re-render does not steal focus from the new-session dialog", () => {
  const { api, document } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const input = document.getElementById("new-name");
  input.focus();
  api.renderSessions();
  assert.equal(document.activeElement, input, "poll must not steal focus");
});

test("hiding the document stops polling; showing it resumes", () => {
  const { window, document, api } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  api.state.loggedIn = true;
  api.startPolling();
  assert.ok(api.state.poll, "polling should be running");

  Object.defineProperty(document, "hidden", { configurable: true, value: true });
  document.dispatchEvent(new window.Event("visibilitychange"));
  assert.equal(api.state.poll, null, "hidden document should not poll");

  Object.defineProperty(document, "hidden", { configurable: true, value: false });
  document.dispatchEvent(new window.Event("visibilitychange"));
  assert.ok(api.state.poll, "visible document should resume polling");
  api.stopPolling();
});

test("failed kill keeps the dialog open and shows the error", async () => {
  const { window, document, api, tick } = load([
    {
      match: (p) => p === "/api/sessions/kill",
      respond: () => jsonResponse(500, { error: "failed to kill session" }),
    },
    { match: (p) => p === "/api/sessions", respond: () => sessions([{ name: "doomed", windows: 1, attached: false }]) },
  ]);
  api.applySessions({ sessions: [{ name: "doomed", windows: 1, attached: false }] });

  api.state.tabs.get("doomed").tabEl.querySelector(".tab-kill").click();
  document.getElementById("confirm-form").dispatchEvent(
    new window.Event("submit", { bubbles: true, cancelable: true })
  );
  await tick(10);

  assert.equal(document.getElementById("confirm-dialog").open, true, "dialog should stay open");
  const err = document.getElementById("confirm-error");
  assert.equal(err.hidden, false);
  assert.match(err.textContent, /failed to kill/);
  api.stopPolling();
});

test("tablist contains only tabs and uses roving tabindex", () => {
  const { api, document } = load();
  api.applySessions({ sessions: [
    { name: "a", windows: 1, attached: false },
    { name: "b", windows: 1, attached: false },
  ] });
  const tablist = document.getElementById("tablist");
  const strays = [...tablist.children].filter(
    (el) => el.getAttribute("role") !== "presentation"
  );
  assert.equal(strays.length, 0, "tablist must hold only tabs");
  assert.equal(
    tablist.querySelectorAll(".tab-main[tabindex='0']").length,
    1,
    "exactly one tab is tabbable"
  );
});

test("api attaches an abort signal to requests", async () => {
  const { api, calls } = load();
  await api.api("/api/sessions");
  const last = calls[calls.length - 1];
  assert.ok(last.opts.signal, "fetch should receive an AbortSignal");
  assert.equal(typeof last.opts.signal.aborted, "boolean");
});

test("updateGlobalStatus does not re-announce an unchanged status", () => {
  const { window, api, document } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const el = document.getElementById("status");
  const desc = Object.getOwnPropertyDescriptor(window.Node.prototype, "textContent");
  let sets = 0;
  Object.defineProperty(el, "textContent", {
    configurable: true,
    get() {
      return desc.get.call(el);
    },
    set(v) {
      sets += 1;
      desc.set.call(el, v);
    },
  });
  api.updateGlobalStatus(); // may write once
  const afterFirst = sets;
  api.updateGlobalStatus();
  api.updateGlobalStatus();
  assert.equal(sets, afterFirst, "unchanged status should not be rewritten");
});

test("?session= reopens the remembered tab even though dominion is first", async () => {
  const { api, document, tick } = load(
    [{ match: (p) => p === "/api/sessions", respond: () => sessions([
      { name: "dominion", windows: 1, attached: false },
      { name: "alpha", windows: 1, attached: false },
      { name: "beta", windows: 1, attached: false },
    ]) }],
    { url: "http://localhost/?session=beta" }
  );
  await tick(10);
  assert.equal(api.state.active, "beta", "query param should win over the first tab");
  assert.equal(document.querySelector(".tab").classList.contains("pinned"), true);
  api.stopPolling();
});

test("activate writes ?session= to the URL", () => {
  const { window, api } = load();
  api.applySessions({ sessions: [
    { name: "a", windows: 1, attached: false },
    { name: "b", windows: 1, attached: false },
  ] });
  api.activate("b");
  assert.match(window.location.search, /session=b/);
});

test("pinned dominion tab is first, marked, and has no kill button", () => {
  const { api, document } = load();
  api.applySessions({ sessions: [
    { name: "alpha", windows: 1, attached: false },
    { name: "dominion", windows: 1, attached: false },
  ] });
  const first = document.getElementById("tablist").firstElementChild;
  assert.ok(first.classList.contains("pinned"), "dominion tab should be pinned");
  assert.equal(first.querySelector(".name").textContent, "dominion");
  assert.equal(first.querySelector(".tab-kill"), null, "pinned tab must not be killable");
  assert.ok(first.querySelector(".tab-main"), "pinned tab still selectable");
});

test("close code 4001 stops polling and returns to login", async () => {
  const { window, document, api } = load([
    { match: (p) => p === "/api/sessions", respond: () => sessions([{ name: "a", windows: 1, attached: false }]) },
  ]);
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  api.state.loggedIn = true;
  api.startPolling();
  assert.ok(api.state.poll);

  const ws = window.__sockets[window.__sockets.length - 1];
  assert.ok(ws && typeof ws.onclose === "function", "a terminal socket should exist");
  ws.onclose({ code: 4001 });

  assert.equal(api.state.poll, null, "revocation should stop polling");
  assert.equal(document.getElementById("login").hidden, false, "login overlay should show");
  assert.equal(api.state.loggedIn, false);
});

test("change-server button is hidden in a plain browser", () => {
  const { document } = load();
  assert.equal(document.getElementById("change-server").hidden, true);
});

test("change-server button shows in the Android shell and targets the prompt", () => {
  const { api, document } = load([], {
    url: "https://localhost/",
    userAgent: "Mozilla/5.0 dominion-shell/1.0",
  });
  const el = document.getElementById("change-server");
  assert.equal(el.hidden, false);
  assert.equal(api.inShell(), true);
  assert.equal(api.shellChangeTarget(), "https://localhost/?change=1");
});

test("change-server button calls the desktop binding", () => {
  let called = 0;
  const { document } = load([], { beforeEval: (w) => { w.dominionChangeURL = () => { called += 1; }; } });
  const el = document.getElementById("change-server");
  assert.equal(el.hidden, false);
  el.click();
  assert.equal(called, 1);
});
