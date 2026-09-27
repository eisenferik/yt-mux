import assert from "node:assert/strict";
import test from "node:test";
import { createCookieState } from "../dist/cookie-state.js";
import { flush, installChromeStub } from "./chrome-stub.js";

test("a late grant result cannot revoke permission retained by a completed change", async () => {
  const { state } = installChromeStub();
  const cookies = createCookieState();
  const first = cookies.begin();
  const second = cookies.begin();
  first.granted(true);
  await first.finish("enabled");
  second.granted(true);
  await second.finish("unchanged");
  assert.equal(state.permissionRemovals, 0);
  assert.equal(state.permissionGranted, true);
});

test("a new failed grant is rolled back after an earlier grant was removed externally", async () => {
  const { state } = installChromeStub();
  const cookies = createCookieState();
  await cookies.begin().finish("enabled");
  const later = cookies.begin();
  later.granted(true);
  await later.finish("unchanged");
  assert.equal(state.permissionRemovals, 1);
  assert.equal(state.permissionGranted, false);
});

test("a queued change cannot write until the preceding change has finished", async () => {
  installChromeStub();
  const cookies = createCookieState();
  const first = cookies.begin();
  const second = cookies.begin();
  let ready = false;
  void second.ready.then(() => { ready = true; });
  await flush();
  assert.equal(ready, false);
  await first.finish("unchanged");
  await second.ready;
  assert.equal(ready, true);
  await second.finish("unchanged");
});
