// Guards the vendored xterm.js bundle: it must be the pinned @xterm/xterm
// release with the local IME patches applied. See test/xterm-patch.mjs for the
// patch table and how to regenerate. If this fails after an xterm upgrade,
// update VENDOR_VERSION/PRISTINE_SHA256/PATCHED_SHA256 there, adjust PATCHES if
// an anchor moved, and rerun the script.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

import {
  VENDOR_PATH,
  VENDOR_VERSION,
  PRISTINE_SHA256,
  PATCHED_SHA256,
  PATCHES,
  applyPatches,
  sha256,
} from "./xterm-patch.mjs";

test("vendored xterm is byte-for-byte the patched pinned build", () => {
  const src = readFileSync(VENDOR_PATH, "utf8");
  assert.equal(
    sha256(src),
    PATCHED_SHA256,
    `web/vendor/xterm.js drifted from the patched @xterm/xterm ${VENDOR_VERSION} build; ` +
      "regenerate with: node test/xterm-patch.mjs"
  );
  assert.notEqual(PRISTINE_SHA256, PATCHED_SHA256);
});

test("vendored xterm carries the IME fixes and not the buggy code", () => {
  const src = readFileSync(VENDOR_PATH, "utf8");

  // The patches must be present...
  const expected = [
    // #5439: guarded textarea-diff timer.
    "if(this._textareaChangeTimer)return;",
    "this._textareaChangeTimer=void 0",
    // #6009: cancel deferred composition send when input delivers the text.
    "cancelPendingComposition(){",
    "this._compositionHelper.cancelPendingComposition();const t=e.data",
    // #6009: reset _keyDownSeen when the composition helper consumes keydown.
    "this._keyDownSeen=!1,this.options.scrollOnUserInput",
    // #4173/#6012: clear the hidden textarea after a committed composition.
    "triggerDataEvent(t,!0),this._isComposing||this._optionsService.rawOptions.screenReaderMode",
    "triggerDataEvent(e,!0),this._isComposing||this._optionsService.rawOptions.screenReaderMode",
  ];
  for (const marker of expected) {
    assert.ok(src.includes(marker), `missing patch marker: ${marker}`);
  }

  // ...and the unguarded original must be gone.
  assert.ok(
    !src.includes(
      "_handleAnyTextareaChanges(){const e=this._textarea.value;setTimeout"
    ),
    "the unguarded _handleAnyTextareaChanges is still present"
  );
});

test("patch table is exactly what produced the vendored bundle", () => {
  const src = readFileSync(VENDOR_PATH, "utf8");
  // Every patch result must be present...
  for (const [, after] of PATCHES) {
    assert.ok(
      src.includes(after),
      `patch result missing: ${after.slice(0, 60)}...`
    );
  }
  // ...and no pristine anchor may survive, which also proves applyPatches is
  // not a no-op and that the vendored file really is patched.
  assert.throws(() => applyPatches(src));
});
