// End-to-end input regression tests for the vendored xterm.js bundle.
//
// These drive the real web/vendor/xterm.js inside jsdom through the event
// sequence an Android soft keyboard (GBoard) produces, and assert what reaches
// onData (i.e. what would be forwarded to the PTY). They exist because the IME
// patches in test/xterm-patch.mjs are easy to get subtly wrong: an earlier
// version cancelled the pending composition send on the input event, which
// dropped the committed word whenever it was followed by a space tap.
//
// The bundle expects a few browser APIs jsdom does not implement; stub them.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { JSDOM } from "jsdom";

const here = dirname(fileURLToPath(import.meta.url));
const VENDOR = join(here, "..", "web", "vendor", "xterm.js");

function make2dContext() {
  return {
    font: "",
    measureText: () => ({ width: 8, actualBoundingBoxAscent: 8, actualBoundingBoxDescent: 2 }),
    fillRect() {},
    clearRect() {},
    getImageData: () => ({ data: new Uint8ClampedArray(4) }),
    putImageData() {},
    createImageData: () => ({ data: new Uint8ClampedArray(4) }),
    save() {},
    restore() {},
    translate() {},
    scale() {},
    drawImage() {},
    beginPath() {},
    moveTo() {},
    lineTo() {},
    stroke() {},
    fill() {},
    fillText() {},
    setTransform() {},
    rect() {},
    clip() {},
    arc() {},
    closePath() {},
  };
}

// openTerminal loads the bundle into a fresh jsdom window, opens a terminal and
// returns helpers to fire input events and read what was emitted on onData.
function openTerminal() {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"t\"></div></body></html>", {
    pretendToBeVisual: true,
    runScripts: "outside-only",
  });
  const window = dom.window;
  window.matchMedia = (query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false;
    },
  });
  window.HTMLCanvasElement.prototype.getContext = (type) =>
    type === "2d" ? make2dContext() : null;
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };

  window.eval(readFileSync(VENDOR, "utf8"));

  const term = new window.Terminal({ cols: 80, rows: 24 });
  const out = [];
  term.onData((data) => out.push(data));
  term.open(window.document.getElementById("t"));
  const textarea = window.document.querySelector(".xterm-helper-textarea");
  assert.ok(textarea, "terminal did not create its helper textarea");

  const fire = (type, props = {}) => {
    const ev = new window.Event(type, { bubbles: true, cancelable: true });
    Object.assign(ev, props);
    textarea.dispatchEvent(ev);
  };
  const keydown = (keyCode, props = {}) => {
    const ev = new window.KeyboardEvent("keydown", {
      bubbles: true,
      cancelable: true,
      ...props,
    });
    Object.defineProperty(ev, "keyCode", { get: () => keyCode });
    textarea.dispatchEvent(ev);
  };
  const settle = () => new Promise((resolve) => setTimeout(resolve, 20));

  return { window, term, textarea, out, fire, keydown, settle };
}

test("Android IME: a committed word survives a following space tap", async () => {
  const { out, textarea, fire, keydown, settle } = openTerminal();

  // Compose "ls": the word is only sent when the composition finalizes.
  fire("compositionstart");
  textarea.value = "l";
  fire("compositionupdate", { data: "l" });
  textarea.value = "ls";
  fire("compositionupdate", { data: "ls" });
  await settle();

  // Tapping space: keydown 229, then compositionend, then the input event for
  // the space. The space event must not discard the pending "ls".
  keydown(229, { key: " ", code: "Space" });
  fire("compositionend", { data: "ls" });
  textarea.value = "ls ";
  fire("input", { data: " ", inputType: "insertText" });
  await settle();

  assert.equal(out.join(""), "ls ", "the committed word and the space must both be sent");
});

test("input event carrying the whole committed text sends it exactly once", async () => {
  const { out, textarea, fire, settle } = openTerminal();

  fire("compositionstart");
  textarea.value = "ls";
  fire("compositionupdate", { data: "ls" });
  await settle();
  fire("compositionend", { data: "ls" });
  textarea.value = "ls ";
  fire("input", { data: "ls ", inputType: "insertText" });
  await settle();

  assert.equal(out.join(""), "ls ", "no duplication or loss when input carries the text");
});

test("plain typing without an active composition is unchanged", async () => {
  const { out, textarea, fire, settle } = openTerminal();

  for (const value of ["l", "ls", "ls "]) {
    textarea.value = value;
    fire("input", { data: value.slice(-1), inputType: "insertText" });
    await settle();
  }

  assert.equal(out.join(""), "ls ", "plain insertText input must still forward each character");
});

test("#4173: the hidden textarea is cleared after a committed composition", async () => {
  const { textarea, fire, settle } = openTerminal();

  fire("compositionstart");
  textarea.value = "ls";
  fire("compositionupdate", { data: "ls" });
  await settle();
  fire("compositionend", { data: "ls" });
  textarea.value = "ls ";
  fire("input", { data: " ", inputType: "insertText" });
  await settle();

  assert.equal(textarea.value, "", "residue left in the textarea re-propagates on delete");
});
