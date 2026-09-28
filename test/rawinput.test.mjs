// Tests for the Android/Termux-style input handling in web/app.js:
// raw composition forwarding and the control-key toolbar.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { load, cleanupAll } from "./harness.mjs";

after(() => cleanupAll());

const decoder = new TextDecoder();

// fakeTab builds the minimal tab shape sendToTab/forwardCompositionDelta use.
function fakeTab() {
  const sent = [];
  return {
    raw: true,
    rawLast: "",
    ws: {
      readyState: 1, // WebSocket.OPEN
      send(data) {
        sent.push(decoder.decode(data));
      },
    },
    sent,
  };
}

function beforeInput(inputType, data) {
  return { inputType, data, preventDefault() { this.defaultPrevented = true; } };
}

test("raw input follows the mobile default and the stored preference", () => {
  const { api } = load();
  assert.equal(api.rawInputDefault(), false, "desktop defaults to off");

  const stored = load([], { beforeEval: (w) => w.localStorage.setItem("dominion.rawinput", "1") });
  assert.equal(stored.api.rawInputEnabled(), true, "explicit on wins");

  const off = load([], { beforeEval: (w) => w.localStorage.setItem("dominion.rawinput", "0") });
  assert.equal(off.api.rawInputEnabled(), false, "explicit off wins");

  const mobile = load([], {
    beforeEval: (w) => {
      w.matchMedia = () => ({
        matches: true,
        addEventListener() {},
        removeEventListener() {},
      });
    },
  });
  assert.equal(mobile.api.rawInputEnabled(), true, "mobile defaults to on");
});

test("forwardCompositionDelta forwards only the new suffix", () => {
  const { api } = load();
  const tab = fakeTab();

  api.forwardCompositionDelta(tab, "h");
  api.forwardCompositionDelta(tab, "he");
  api.forwardCompositionDelta(tab, "hel");

  assert.deepEqual(tab.sent, ["h", "e", "l"], "each keystroke is sent once");
});

test("forwardCompositionDelta rewrites with backspaces on autocorrect", () => {
  const { api } = load();
  const tab = fakeTab();

  for (const step of ["t", "te", "teh"]) api.forwardCompositionDelta(tab, step);
  api.forwardCompositionDelta(tab, "the");

  assert.deepEqual(tab.sent, ["t", "e", "h", "\x7f\x7f", "he"]);
  assert.equal(tab.rawLast, "the");
});

test("handleRawInput sends plain text and ignores the repeating commit", () => {
  const { api } = load();
  const tab = { raw: true, rawLast: "hi" };

  // Composition already forwarded "hi"; the commit must not repeat it.
  let e = beforeInput("insertText", "hi");
  api.handleRawInput(tab, e);
  assert.equal(e.defaultPrevented, true);

  // A fresh insertText is forwarded.
  api.handleRawInput(tab, beforeInput("insertText", "x"));
});

test("handleRawInput forwards IME-generated delete, forward-delete and enter", () => {
  const { api } = load();
  const tab = fakeTab();

  api.handleRawInput(tab, beforeInput("deleteContentBackward", null));
  api.handleRawInput(tab, beforeInput("deleteContentForward", null));
  api.handleRawInput(tab, beforeInput("insertLineBreak", null));

  assert.deepEqual(tab.sent, ["\x7f", "\x1b[3~", "\r"]);
});

test("handleRawInput is inert when raw mode is off", () => {
  const { api } = load();
  const tab = fakeTab();
  tab.raw = false;

  const e = beforeInput("insertText", "x");
  api.handleRawInput(tab, e);
  assert.equal(e.defaultPrevented, undefined, "default must be left alone");
});

test("control-key toolbar sequences are wired", () => {
  const { api } = load();
  assert.equal(api.KEY_SEQ["ctrl-c"], "\x03");
  assert.equal(api.KEY_SEQ["ctrl-d"], "\x04");
  assert.equal(api.KEY_SEQ["ctrl-z"], "\x1a");
  assert.equal(api.KEY_SEQ.enter, "\r");
  assert.equal(api.KEY_SEQ.bs, "\x7f");
});

test("tapping the Ctrl+C button sends ETX to the active terminal", () => {
  const { api, document, window } = load();
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const tab = api.state.tabs.get("a");
  tab.ws.readyState = 1;

  document.querySelector('#keys [data-key="ctrl-c"]').click();

  const bytes = tab.ws.sent.map((b) => decoder.decode(b)).join("");
  assert.equal(bytes, "\x03");
  api.stopPolling();
});

test("arming Ctrl combines with the next typed character", () => {
  const { api, document, window } = load([], {
    beforeEval: (w) => w.localStorage.setItem("dominion.rawinput", "1"),
  });
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const tab = api.state.tabs.get("a");
  tab.ws.readyState = 1;

  document.querySelector('#keys [data-mod="ctrl"]').click();
  const ev = new window.Event("beforeinput", { bubbles: true, cancelable: true });
  ev.inputType = "insertText";
  ev.data = "c";
  tab.term.textarea.dispatchEvent(ev);

  const bytes = tab.ws.sent.map((b) => decoder.decode(b)).join("");
  assert.equal(bytes, "\x03", "Ctrl + c should reach the shell as ^C");
  assert.equal(api.activeMods.size, 0, "the modifier is one-shot");
  api.stopPolling();
});

test("terminal textarea disables autocomplete and raw listeners forward on Android", () => {
  const { api, window } = load([], {
    beforeEval: (w) => w.localStorage.setItem("dominion.rawinput", "1"),
  });
  try {
    api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
    const tab = api.state.tabs.get("a");
    const ta = tab.term.textarea;
    assert.ok(ta, "terminal should expose its helper textarea");
    assert.equal(ta.getAttribute("autocomplete"), "off");
    assert.equal(ta.getAttribute("spellcheck"), "false");

    // The harness socket never fires onopen, so mark it writable.
    tab.ws.readyState = 1;

    const ev = new window.Event("beforeinput", { bubbles: true, cancelable: true });
    ev.inputType = "insertText";
    ev.data = "ls";
    ta.dispatchEvent(ev);

    const bytes = tab.ws.sent.map((b) => decoder.decode(b)).join("");
    assert.equal(bytes, "ls", "a keystroke reaches the terminal immediately");
  } finally {
    api.stopPolling();
  }
});
