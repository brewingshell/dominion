// Reproduce web/vendor/xterm.js from the pinned upstream @xterm/xterm build.
//
// xterm 6.0.0 mishandles Android soft-keyboard/IME input: keydown arrives with
// keyCode 229 and no data, which leaves the `_keyDownSeen` gate set so the
// following `input` event is dropped (upstream #5887), while the deferred
// textarea diff in _handleAnyTextareaChanges can duplicate characters on
// overlap (#5439) and committed composition text is left in the hidden textarea,
// where a later delete re-sends it (#4173/#6012).
//
// An initial fix cancelled the pending composition send whenever an `input`
// event arrived, which fixed the #4173 re-send but introduced a worse Android
// regression: committing a word and then tapping space fires
// compositionend -> input(" "), and the cancel discarded the word before it was
// ever sent (only the space reached the PTY). The current patch instead flushes
// the pending composition text synchronously from _inputEvent before sending
// the input data, subtracting the incoming data when the event already carries
// it, so the word and the space both arrive exactly once.
//
// Instead of hand-editing the minified bundle, this script starts from the
// pristine release, verifies its checksum, applies the patch table below, and
// writes the vendored file. Upgrade xterm by bumping VENDOR_VERSION/PRISTINE_
// SHA256, rerunning this script (it prints the new PATCHED_SHA256), then updating
// that constant. test/xterm-vendor.test.mjs asserts the result so CI catches any
// silent drift.
//
// Usage:
//   node test/xterm-patch.mjs              # fetch pristine, patch, write
//   node test/xterm-patch.mjs --from FILE  # patch a local pristine copy
//   node test/xterm-patch.mjs --check      # verify the vendored file only
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");

export const VENDOR_PATH = join(root, "web", "vendor", "xterm.js");
export const VENDOR_VERSION = "6.0.0";

// Cache-busting token for the vendored asset URLs in web/index.html. Bump the
// suffix whenever the bundle changes so clients that cached the old (now
// no-cache) URL still fetch the new one; test/xterm-vendor.test.mjs fails if
// index.html and this constant disagree or if it stops tracking VENDOR_VERSION.
export const ASSET_QUERY = "6.0.0p3";

const UPSTREAM_URL =
  `https://cdn.jsdelivr.net/npm/@xterm/xterm@${VENDOR_VERSION}/lib/xterm.js`;

// The pristine @xterm/xterm lib/xterm.js for VENDOR_VERSION.
export const PRISTINE_SHA256 =
  "14903579ff54664cd72f8e8699e6961a6272c21863ec1c3b118cdc8af5d4a972";

// The patched result that web/vendor/xterm.js must equal. Regenerate with
// `node test/xterm-patch.mjs` after changing PATCHES or VENDOR_VERSION.
export const PATCHED_SHA256 =
  "1910dea2db812af5914717541fef2389838de7656fdd726b28e000a0aeaa274d";

// Each entry is [before, after]; `before` must occur exactly once in the
// pristine bundle. The bundle's CompositionHelper/CoreBrowserTerminal classes
// are not name-mangled, so these anchors are stable and readable.
export const PATCHES = [
  // #5439: track the pending textarea-diff timer so overlapping keys cannot
  // schedule competing diffs, and so it can be cancelled when the input event
  // already delivered the text.
  [
    'this._compositionPosition={start:0,end:0},this._dataAlreadySent=""}',
    'this._compositionPosition={start:0,end:0},this._dataAlreadySent="",this._textareaChangeTimer=void 0}',
  ],
  [
    "_handleAnyTextareaChanges(){const e=this._textarea.value;setTimeout((()=>{if(!this._isComposing){const t=this._textarea.value,i=t.replace(e,\"\");this._dataAlreadySent=i,t.length>e.length?this._coreService.triggerDataEvent(i,!0):t.length<e.length?this._coreService.triggerDataEvent(`${a.C0.DEL}`,!0):t.length===e.length&&t!==e&&this._coreService.triggerDataEvent(t,!0)}}),0)}",
    "_handleAnyTextareaChanges(){if(this._textareaChangeTimer)return;const e=this._textarea.value;this._textareaChangeTimer=setTimeout((()=>{if(this._textareaChangeTimer=void 0,!this._isComposing){const t=this._textarea.value,i=t.replace(e,\"\");this._dataAlreadySent=i,t.length>e.length?this._coreService.triggerDataEvent(i,!0):t.length<e.length?this._coreService.triggerDataEvent(`${a.C0.DEL}`,!0):t.length===e.length&&t!==e&&this._coreService.triggerDataEvent(t,!0)}}),0)}" +
      // #6009/#4173 + Android space-to-commit: when the input event arrives while
      // a composition send is pending, flush that text synchronously (minus the
      // input event's own data, which _inputEvent sends immediately after) rather
      // than dropping it, then clear the hidden textarea so a later delete cannot
      // re-propagate the old composition. The flush must run before the pending
      // setTimeout, otherwise the deferred send is cancelled and the committed
      // word is lost. Clearing is skipped during composition and in screen-reader
      // mode.
      "cancelPendingComposition(t){if(this._isSendingComposition){this._isSendingComposition=!1;const e=this._compositionPosition.start+this._dataAlreadySent.length;let i=this._isComposing?this._textarea.value.substring(e,this._compositionPosition.start):this._textarea.value.substring(e);t&&i.endsWith(t)&&(i=i.slice(0,i.length-t.length)),i.length>0&&this._coreService.triggerDataEvent(i,!0)}this._textareaChangeTimer&&(clearTimeout(this._textareaChangeTimer),this._textareaChangeTimer=void 0),this._isComposing||this._optionsService.rawOptions.screenReaderMode||(this._textarea.value=\"\",this._dataAlreadySent=\"\")}",
  ],
  // #6009: when the composition helper consumes the keydown (keyCode 229) it
  // emits no data, so reset _keyDownSeen; otherwise the matching input event is
  // suppressed and the character is lost.
  [
    "if(!t&&!this._compositionHelper.keydown(e))return this.options.scrollOnUserInput&&this.buffer.ybase!==this.buffer.ydisp&&this.scrollToBottom(!0),!1;",
    "if(!t&&!this._compositionHelper.keydown(e))return this._keyDownSeen=!1,this.options.scrollOnUserInput&&this.buffer.ybase!==this.buffer.ydisp&&this.scrollToBottom(!0),!1;",
  ],
  // #6009: the input event now carries committed IME text; flush any pending
  // composition send first (passing the event data so it is not sent twice), then
  // send the input data. Without the flush the pending composition is dropped and
  // an Android word followed by a space tap is lost.
  [
    "this._unprocessedDeadKey=!1;const t=e.data;return this.coreService.triggerDataEvent(t,!0),this.cancel(e),!0",
    "this._unprocessedDeadKey=!1,this._compositionHelper.cancelPendingComposition(e.data);const t=e.data;return this.coreService.triggerDataEvent(t,!0),this.cancel(e),!0",
  ],
  // #4173/#6012: clear the hidden textarea after a committed composition is
  // sent, unless a new composition started or screen-reader mode is active
  // (which relies on the textarea for announcements).
  [
    "t=this._isComposing?this._textarea.value.substring(e.start,this._compositionPosition.start):this._textarea.value.substring(e.start),t.length>0&&this._coreService.triggerDataEvent(t,!0)}}),0)",
    "t=this._isComposing?this._textarea.value.substring(e.start,this._compositionPosition.start):this._textarea.value.substring(e.start),t.length>0&&this._coreService.triggerDataEvent(t,!0),this._isComposing||this._optionsService.rawOptions.screenReaderMode||(this._textarea.value=\"\",this._dataAlreadySent=\"\")}}),0)",
  ],
  [
    "this._isSendingComposition=!1;const e=this._textarea.value.substring(this._compositionPosition.start,this._compositionPosition.end);this._coreService.triggerDataEvent(e,!0)}",
    "this._isSendingComposition=!1;const e=this._textarea.value.substring(this._compositionPosition.start,this._compositionPosition.end);this._coreService.triggerDataEvent(e,!0),this._isComposing||this._optionsService.rawOptions.screenReaderMode||(this._textarea.value=\"\",this._dataAlreadySent=\"\")}",
  ],
];

export function sha256(text) {
  return createHash("sha256").update(text).digest("hex");
}

// applyPatches returns source with PATCHES applied, asserting each anchor is
// found exactly once so a changing bundle fails loudly instead of drifting.
export function applyPatches(source) {
  let out = source;
  for (const [before, after] of PATCHES) {
    const count = out.split(before).length - 1;
    if (count !== 1) {
      throw new Error(
        `patch anchor matched ${count} time(s), want 1: ${before.slice(0, 72)}...`
      );
    }
    out = out.replace(before, after);
  }
  return out;
}

async function fetchPristine() {
  const res = await fetch(UPSTREAM_URL);
  if (!res.ok) {
    throw new Error(`could not fetch ${UPSTREAM_URL}: HTTP ${res.status}`);
  }
  return res.text();
}

function check() {
  const src = readFileSync(VENDOR_PATH, "utf8");
  const got = sha256(src);
  if (got !== PATCHED_SHA256) {
    console.error(
      `web/vendor/xterm.js is not the expected patched build.\n` +
        `  got:      ${got}\n` +
        `  expected: ${PATCHED_SHA256}\n` +
        `regenerate with: node test/xterm-patch.mjs`
    );
    process.exit(1);
  }
  console.log(`web/vendor/xterm.js OK (patched @xterm/xterm ${VENDOR_VERSION})`);
}

async function main() {
  const args = process.argv.slice(2);
  if (args.includes("--check")) {
    check();
    return;
  }

  const fromIdx = args.indexOf("--from");
  let pristine;
  if (fromIdx !== -1) {
    pristine = readFileSync(args[fromIdx + 1], "utf8");
  } else {
    pristine = await fetchPristine();
  }

  const pristineHash = sha256(pristine);
  if (pristineHash !== PRISTINE_SHA256) {
    console.error(
      `pristine bundle does not match the pinned @xterm/xterm ${VENDOR_VERSION}.\n` +
        `  got:      ${pristineHash}\n` +
        `  expected: ${PRISTINE_SHA256}`
    );
    process.exit(1);
  }

  const patched = applyPatches(pristine);
  const patchedHash = sha256(patched);
  writeFileSync(VENDOR_PATH, patched);
  console.log(`wrote ${VENDOR_PATH}`);
  console.log(`  patched sha256: ${patchedHash}`);
  if (patchedHash !== PATCHED_SHA256) {
    console.log(
      `  note: PATCHED_SHA256 is ${PATCHED_SHA256}; update it to the value above`
    );
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main().catch((err) => {
    console.error(err.message);
    process.exit(1);
  });
}
