import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { errorDetail, filenameFromPath } from "../dist/presentation.js";
import { ERROR_KINDS } from "../dist/protocol.js";

const sharedErrorKinds = JSON.parse(
  readFileSync(new URL("../../testdata/error-kinds.json", import.meta.url), "utf8"),
);

function context(overrides) {
  return { settings: undefined, cookieAccess: "denied", kind: "HOST_FAILURE", ...overrides };
}

test("the error vocabulary matches the shared fixture", () => {
  assert.deepEqual([...ERROR_KINDS], sharedErrorKinds);
});

test("errorDetail names a different remedy for each login state", () => {
  const cookiesOff = errorDetail(context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: false } }));
  const permissionRevoked = errorDetail(context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true } }));
  const unreadable = errorDetail(
    context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true }, cookieAccess: "unreadable" }),
  );
  const signedOut = errorDetail(
    context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true }, cookieAccess: "granted" }),
  );
  assert.equal(new Set([cookiesOff, permissionRevoked, unreadable, signedOut]).size, 4);
  assert.match(cookiesOff, /Turn on signed-in YouTube cookies/);
  assert.match(permissionRevoked, /Save extension options/);
  assert.match(unreadable, /could not be read/);
  assert.match(signedOut, /Sign in to YouTube in this browser/);
});

test("host-internal failures collapse into one generic message", () => {
  const internal = [
    "INVALID_REQUEST",
    "HOST_FAILURE",
    "PROCESS_OUTPUT_FAILED",
    "DESTINATION_PICK_FAILED",
  ];
  const messages = new Set(internal.map((kind) => errorDetail(context({ kind }))));
  assert.equal(messages.size, 1);
});

test("filenameFromPath takes the last path segment", () => {
  assert.equal(filenameFromPath("C:\\Users\\me\\Videos\\yt-mux\\clip.mkv"), "clip.mkv");
  assert.equal(filenameFromPath("clip.mkv"), "clip.mkv");
});
