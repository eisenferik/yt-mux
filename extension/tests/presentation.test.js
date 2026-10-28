import assert from "node:assert/strict";
import test from "node:test";

import { errorDetail } from "../dist/presentation.js";

function context(overrides) {
  return { settings: undefined, cookieAccess: "denied", kind: "HOST_FAILURE", ...overrides };
}

test("errorDetail names a different remedy for each login state", () => {
  const cookiesOff = errorDetail(context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: false } }));
  const permissionRevoked = errorDetail(context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true } }));
  const signedOut = errorDetail(
    context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true }, cookieAccess: "granted" }),
  );
  assert.equal(new Set([cookiesOff, permissionRevoked, signedOut]).size, 3);
});

test("errorDetail says when a video is for channel members", () => {
  const membersCookiesOff = errorDetail(context({ kind: "YTDLP_MEMBERS_ONLY", settings: { cookiesEnabled: false } }));
  const membersSignedIn = errorDetail(
    context({ kind: "YTDLP_MEMBERS_ONLY", settings: { cookiesEnabled: true }, cookieAccess: "granted" }),
  );
  const loginCookiesOff = errorDetail(context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: false } }));
  const loginSignedIn = errorDetail(
    context({ kind: "YTDLP_LOGIN_REQUIRED", settings: { cookiesEnabled: true }, cookieAccess: "granted" }),
  );
  assert.notEqual(membersCookiesOff, loginCookiesOff);
  assert.notEqual(membersSignedIn, loginSignedIn);
});

test("host-internal failures collapse into one generic message", () => {
  const internal = [
    "INVALID_REQUEST",
    "HOST_FAILURE",
    "DESTINATION_PICK_FAILED",
  ];
  const messages = new Set(internal.map((kind) => errorDetail(context({ kind }))));
  assert.equal(messages.size, 1);
});
