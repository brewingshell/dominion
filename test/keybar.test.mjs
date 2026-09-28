// Tests for the mobile terminal key toolbar: the main row and the separate,
// independently toggled control-key row (^C ^D ^Z ...).
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { load, cleanupAll } from "./harness.mjs";

after(() => cleanupAll());

test("the control row starts collapsed and off the stored default", () => {
  const { api, document } = load();
  assert.equal(api.ctrlKeysVisible(), false, "default is collapsed");
  assert.equal(document.getElementById("keys-ctrl").hidden, true);
  assert.equal(
    document.getElementById("keys-more").getAttribute("aria-expanded"),
    "false"
  );
});

test("the more button expands and collapses the control row", () => {
  const { api, document } = load();
  const more = document.getElementById("keys-more");
  const ctrl = document.getElementById("keys-ctrl");

  more.click();
  assert.equal(ctrl.hidden, false, "first click expands");
  assert.equal(more.getAttribute("aria-expanded"), "true");
  assert.equal(api.ctrlKeysVisible(), true, "state is persisted");

  more.click();
  assert.equal(ctrl.hidden, true, "second click collapses");
  assert.equal(more.getAttribute("aria-expanded"), "false");
});

test("the control row expands automatically from the stored preference", () => {
  const { document } = load([], {
    beforeEval: (w) => w.localStorage.setItem("dominion.ctrlkeys", "1"),
  });
  assert.equal(document.getElementById("keys-ctrl").hidden, false);
  assert.equal(
    document.getElementById("keys-more").getAttribute("aria-expanded"),
    "true"
  );
});

test("control keys live in the extra row, main keys in the main row", () => {
  const { document } = load();
  const ctrl = document.getElementById("keys-ctrl");
  const main = document.getElementById("keys-main");

  for (const key of ["ctrl-c", "ctrl-d", "ctrl-r"]) {
    assert.ok(
      ctrl.querySelector(`[data-key="${key}"]`),
      `${key} belongs to the control row`
    );
  }
  for (const key of ["esc", "tab", "enter", "bs", "left"]) {
    assert.ok(
      main.querySelector(`[data-key="${key}"]`),
      `${key} stays in the main row`
    );
  }
});

test("a control key tapped in the extra row still sends its sequence", () => {
  const decoder = new TextDecoder();
  const { api, document } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const tab = api.state.tabs.get("a");
  tab.ws.readyState = 1;

  document.getElementById("keys-more").click();
  document.querySelector('#keys-ctrl [data-key="ctrl-c"]').click();

  const bytes = tab.ws.sent.map((b) => decoder.decode(b)).join("");
  assert.equal(bytes, "\x03");
  api.stopPolling();
});
