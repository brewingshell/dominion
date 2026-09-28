// Tests for swipe-to-scroll on touch devices in web/app.js. xterm 6 scrolls
// only on wheel events, so a vertical swipe has to be translated to
// scrollLines() by app.js.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { load, cleanupAll } from "./harness.mjs";

after(() => cleanupAll());

// touchEvent builds a touch event with the shape bindTouchScroll reads. jsdom
// has no TouchEvent, so the touches list is assigned directly.
function touchEvent(window, type, ys) {
  const e = new window.Event(type, { bubbles: true, cancelable: true });
  e.touches = ys.map((clientY) => ({ clientY }));
  return e;
}

// openTab loads the app, creates one session, and forces a known cell height
// (24 rows over 384px = 16px per cell) so one line is a 16px drag.
function openTab() {
  const inst = load();
  const { api, window } = inst;
  api.applySessions({ sessions: [{ name: "a", windows: 1, attached: false }] });
  const tab = api.state.tabs.get("a");
  tab.term.rows = 24;
  tab.term.element.querySelector = () => ({ clientHeight: 384 });
  return { ...inst, tab };
}

test("dragging down scrolls the scrollback up", () => {
  const { tab } = openTab();
  tab.el.dispatchEvent(touchEvent(tab.el.ownerDocument.defaultView, "touchstart", [200]));
  tab.el.dispatchEvent(touchEvent(tab.el.ownerDocument.defaultView, "touchmove", [232]));
  tab.el.dispatchEvent(touchEvent(tab.el.ownerDocument.defaultView, "touchend", []));
  assert.deepEqual(tab.term.scrolled, [-2], "a 32px drag is two lines up");
});

test("dragging up scrolls toward the prompt", () => {
  const { tab } = openTab();
  const win = tab.el.ownerDocument.defaultView;
  tab.el.dispatchEvent(touchEvent(win, "touchstart", [200]));
  tab.el.dispatchEvent(touchEvent(win, "touchmove", [168]));
  tab.el.dispatchEvent(touchEvent(win, "touchend", []));
  assert.deepEqual(tab.term.scrolled, [2], "a 32px drag is two lines down");
});

test("a slow drag accumulates sub-cell movement into whole lines", () => {
  const { tab } = openTab();
  const win = tab.el.ownerDocument.defaultView;
  tab.el.dispatchEvent(touchEvent(win, "touchstart", [100]));
  tab.el.dispatchEvent(touchEvent(win, "touchmove", [108]));
  assert.equal(tab.term.scrolled, undefined, "8px is below one cell");
  tab.el.dispatchEvent(touchEvent(win, "touchmove", [116]));
  assert.deepEqual(tab.term.scrolled, [-1], "16px total advances one line");
});

test("a multi-touch gesture is ignored", () => {
  const { tab } = openTab();
  const win = tab.el.ownerDocument.defaultView;
  tab.el.dispatchEvent(touchEvent(win, "touchstart", [200, 240]));
  tab.el.dispatchEvent(touchEvent(win, "touchmove", [232, 272]));
  assert.equal(tab.term.scrolled, undefined);
});

test("a drag without a measurable cell height does nothing", () => {
  const { tab } = openTab();
  const win = tab.el.ownerDocument.defaultView;
  tab.term.element.querySelector = () => ({ clientHeight: 0 });
  tab.el.dispatchEvent(touchEvent(win, "touchstart", [200]));
  tab.el.dispatchEvent(touchEvent(win, "touchmove", [260]));
  assert.equal(tab.term.scrolled, undefined);
});
