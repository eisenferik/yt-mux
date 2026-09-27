import assert from "node:assert/strict";
import test from "node:test";
import { badgeClearDelay } from "../dist/presentation.js";

import {
  deferred,
  flush,
  installChromeStub,
  loadServiceWorker,
  settingsReply,
  until,
} from "./chrome-stub.js";

const videoTab = () => ({ id: 7, url: "https://www.youtube.com/watch?v=abc" });
const messageTypes = (port) => port.messages.map((message) => message.type);
const sent = (port, type) => port.messages.find((message) => message.type === type);
const notified = (state, title) => state.notifications.find((notification) => notification.title === title);

async function requestedSettings(state) {
  await until(() => state.ports.length === 1);
  const port = state.ports[0];
  await until(() => sent(port, "settings.get") !== undefined);
  return { port, id: sent(port, "settings.get").id };
}

async function startedDownload(state, overrides = {}) {
  const { port, id } = await requestedSettings(state);
  port.emitMessage(settingsReply(id, overrides));
  await until(() => sent(port, "start") !== undefined);
  return { port, id };
}

test.beforeEach((t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
});

test("a tab that is not a YouTube video cannot start a download", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action({ id: 7, url: "https://example.com/" });
  await until(() => notified(state, "Download unavailable") !== undefined);
  await flush();

  assert.equal(state.ports.length, 0);
  assert.match(notified(state, "Download unavailable")?.message ?? "", /Open a YouTube video/);
});

test("a missing native host is reported to the user", async () => {
  const { state, listeners } = installChromeStub();
  state.connectNativeThrows = true;
  await loadServiceWorker();

  listeners.action(videoTab());
  await until(() => notified(state, "Native host unavailable") !== undefined);
  await flush();

  assert.equal(state.ports.length, 0);
  assert.match(notified(state, "Native host unavailable")?.message ?? "", /setup script/);
});

test("a finished download reports the saved file", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state);

  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\Channel\\clip.mkv" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(port.disconnected, true);
  assert.equal(notified(state, "Download complete")?.message, "clip.mkv");
});

for (const outcome of [
  { type: "done", path: "D:\\Videos\\clip.mkv", badge: "✓" },
  { type: "error", kind: "YTDLP_FAILED", badge: "!" },
]) {
  test(`a previous download cannot clear the next ${outcome.type} badge early`, async (t) => {
    const { state, listeners } = installChromeStub();
    await loadServiceWorker();
    const tab = videoTab();
    const badgeText = () => state.badges.filter((badge) => badge.text !== undefined).at(-1)?.text;

    listeners.action(tab);
    const first = await startedDownload(state);
    first.port.emitMessage({ v: 1, id: first.id, type: "done", path: "D:\\Videos\\first.mkv" });
    await flush();
    t.mock.timers.tick(1000);

    listeners.action(tab);
    const second = state.ports[1];
    await until(() => sent(second, "settings.get") !== undefined);
    const id = sent(second, "settings.get").id;
    second.emitMessage(settingsReply(id));
    await until(() => sent(second, "start") !== undefined);
    const { badge, ...message } = outcome;
    second.emitMessage({ v: 1, id, ...message });
    await flush();

    t.mock.timers.tick(badgeClearDelay - 1000);
    await flush();
    assert.equal(badgeText(), badge, "the first download's timeout must not clear the second result");

    t.mock.timers.tick(999);
    await flush();
    assert.equal(badgeText(), badge);
    t.mock.timers.tick(1);
    await flush();
    assert.equal(badgeText(), "", "the second result must clear after its own full delay");
  });
}

test("a context-menu preset is sent on start", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.menu({ menuItemId: "compat-mp4" }, tab);
  const { port } = await startedDownload(state);
  await flush();

  const start = sent(port, "start");
  assert.equal(start.preset, "compat-mp4");
  assert.equal(start.url, tab.url);
});

test("a cancelled download does not notify failure", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state);

  port.emitMessage({ v: 1, id, type: "error", kind: "CANCELLED" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(port.disconnected, true);
  assert.equal(notified(state, "Download failed"), undefined);
  assert.equal(notified(state, "Download complete"), undefined);
});

test("a settings reply starts the download once", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await requestedSettings(state);
  assert.deepEqual(messageTypes(port), ["settings.get"]);

  port.emitMessage(settingsReply(id));
  port.emitMessage(settingsReply(id));
  await until(() => sent(port, "start") !== undefined);
  await flush();
  assert.deepEqual(messageTypes(port), ["settings.get", "start"]);
  assert.deepEqual(port.messages[1], {
    v: 1,
    type: "start",
    id,
    url: tab.url,
    preset: "archive",
  });
  assert.equal(port.disconnected, false);
});

test("a second click after start sends cancel instead of disconnecting", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state);
  await flush();
  assert.deepEqual(messageTypes(port), ["settings.get", "start"]);

  listeners.action(tab);
  await until(() => sent(port, "cancel") !== undefined);
  await flush();
  assert.equal(state.ports.length, 1, "cancel must reuse the existing port");
  assert.equal(port.disconnected, false, "an in-flight download is cancelled over the port");
  assert.deepEqual(port.messages[2], { v: 1, type: "cancel", id });
});

test("another tab cannot download a video that is already downloading", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);

  listeners.action({ id: 8, url: "https://youtu.be/abc" });
  await until(() => notified(state, "Video already downloading") !== undefined);
  await flush();
  assert.equal(state.ports.length, 1);

  port.emitMessage({ v: 1, id, type: "error", kind: "CANCELLED" });
  await until(() => port.disconnected);
  listeners.action({ id: 8, url: "https://youtu.be/abc" });
  await until(() => state.ports.length === 2);
});

test("a request that cannot be sent closes the port", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await requestedSettings(state);
  state.postMessageThrows = "start";
  port.emitMessage(settingsReply(id));
  await until(() => port.disconnected);
  await flush();

  assert.equal(sent(port, "start"), undefined);
  assert.equal(port.disconnected, true, "a request the port refused must not leave the host running");
  assert.match(notified(state, "Download failed")?.message ?? "", /Could not send request/);
});

test("a second click cancels the pending session instead of opening another port", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port } = await requestedSettings(state);
  assert.deepEqual(messageTypes(port), ["settings.get"]);

  listeners.action(tab);
  await until(() => port.disconnected);
  await flush();
  assert.equal(state.ports.length, 1, "the second click must not open a second port");
  assert.equal(port.disconnected, true, "cancelling before the download starts must close the port");

  port.emitMessage(settingsReply(port.messages[0].id));
  await flush();
  assert.deepEqual(
    messageTypes(port),
    ["settings.get"],
    "a settings reply that arrives after cancellation must not start a download",
  );
});

test("cancelling while the badge update is pending never asks for settings", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();
  const gate = deferred();
  state.badgeGate = gate.promise;

  listeners.action(tab);
  await until(() => state.ports.length === 1);
  await flush();
  const port = state.ports[0];
  assert.deepEqual(port.messages, [], "this test requires the badge update to still be pending");

  listeners.action(tab);
  state.badgeGate = null;
  gate.resolve();
  await until(() => port.disconnected);
  await flush();

  assert.equal(port.disconnected, true);
  assert.deepEqual(port.messages, [], "a session cancelled mid-start must not reach the native host");
});

test("cancelling while cookies are collected prevents a late start", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.pendingContains = [];
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await requestedSettings(state);
  port.emitMessage(settingsReply(id, { cookiesEnabled: true }));
  await until(() => state.pendingContains.length === 1);
  await flush();
  assert.equal(state.pendingContains.length, 1);
  assert.deepEqual(messageTypes(port), ["settings.get"]);

  listeners.action(tab);
  await until(() => port.disconnected);
  await flush();
  assert.equal(port.disconnected, true);

  state.pendingContains.shift()(true);
  await flush();
  assert.deepEqual(
    messageTypes(port),
    ["settings.get"],
    "cookie collection that settles after cancellation must not start the download",
  );
});

test("the badge counts the download and then marks post-processing", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const badgeTexts = () => state.badges.filter((badge) => badge.text !== undefined).map((badge) => badge.text);

  listeners.action(videoTab());
  const { port, id } = await requestedSettings(state);
  port.emitMessage(settingsReply(id));
  await until(() => badgeTexts().length === 1);

  const progress = (pct) => ({ v: 1, id, type: "progress", pct, downloaded: 1, total: 2, speed: null, eta: null });
  port.emitMessage(progress(null));
  await until(() => badgeTexts().length === 2);
  port.emitMessage(progress(41.7));
  await until(() => badgeTexts().length === 3);
  port.emitMessage({ v: 1, id, type: "postprocess" });
  await until(() => badgeTexts().length === 4);
  await flush();

  assert.deepEqual(badgeTexts(), ["…", "…", "41%", "⚙"]);
});

test("a tab closed mid-download still finishes the session", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state);
  port.emitMessage({ v: 1, id, type: "accepted" });
  await flush();

  port.emitMessage({ v: 1, id, type: "progress", pct: 42, downloaded: 1, total: 2, speed: null, eta: null });
  await flush();

  state.badgeGate = Promise.reject(new Error("No tab with id: 7"));
  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\out.mkv" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(port.disconnected, true, "a badge that cannot be drawn must not hold the port open");
  assert.ok(notified(state, "Download complete"), "completion must be reported even when the tab is gone");
});

test("download failures are notified when completion notifications are disabled", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state, { notifications: false });
  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_FAILED", detail: "ERROR: disk full" });
  await until(() => notified(state, "Download failed") !== undefined);

  assert.match(notified(state, "Download failed")?.message ?? "", /disk full/);
});

test("missing cookie permission directs the user to save extension options again", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = false;
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state, { cookiesEnabled: true });
  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_LOGIN_REQUIRED" });
  await until(() => notified(state, "Download failed") !== undefined);

  const failure = notified(state, "Download failed");
  assert.match(failure?.message ?? "", /cookie access is disabled/i);
  assert.match(failure?.message ?? "", /Save extension options/);
});

test("available cookie permission without YouTube cookies directs the user to sign in to YouTube", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.cookies = [];
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state, { cookiesEnabled: true });
  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_LOGIN_REQUIRED" });
  await until(() => notified(state, "Download failed") !== undefined);

  assert.equal(notified(state, "Download failed")?.message, "Sign in to YouTube in this browser, then try again.");
});

test("an unreadable cookie store says the cookies could not be read", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.cookieReadRejects = true;
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state, { cookiesEnabled: true });
  assert.equal(sent(port, "start").cookies, undefined);

  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_LOGIN_REQUIRED" });
  await until(() => notified(state, "Download failed") !== undefined);

  assert.match(notified(state, "Download failed")?.message ?? "", /could not be read/i);
});

test("a rejected permission check does not tell the user to enable a held permission", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.permissionCheckRejects = true;
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state, { cookiesEnabled: true });
  assert.equal(sent(port, "start").cookies, undefined);

  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_LOGIN_REQUIRED" });
  await until(() => notified(state, "Download failed") !== undefined);

  const message = notified(state, "Download failed")?.message ?? "";
  assert.match(message, /could not be read/i);
  assert.doesNotMatch(message, /disabled/i);
});

test("a start request carries the cookie fields the native host decodes", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.cookies = [
    { domain: ".youtube.com", name: "SID", value: "secret", path: "/", secure: true },
    {
      domain: ".google.com",
      name: "SAPISID",
      value: "secret",
      path: "/",
      secure: true,
      httpOnly: true,
      expirationDate: 1234567890,
    },
  ];
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port } = await startedDownload(state, { cookiesEnabled: true });
  await flush();

  assert.equal(sent(port, "start").cookies.length, 2);
});

test("a host that disconnects mid-download reports the failure", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);
  port.emitMessage({ v: 1, id, type: "accepted" });
  await flush();

  chrome.runtime.lastError = { message: "Native host has exited." };
  port.emitDisconnect();
  await until(() => notified(state, "Download failed") !== undefined);
  await flush();

  assert.match(notified(state, "Download failed")?.message ?? "", /Native host has exited/);
});

test("a host that disconnects while cookies are collected ends the session", async () => {
  const { state, listeners } = installChromeStub();
  state.permissionGranted = true;
  state.pendingContains = [];
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await requestedSettings(state);
  port.emitMessage(settingsReply(id, { cookiesEnabled: true }));
  await until(() => state.pendingContains.length === 1);

  port.emitDisconnect();
  await until(() => notified(state, "Download failed") !== undefined);

  state.pendingContains.shift()(true);
  await flush();
  assert.deepEqual(
    messageTypes(port),
    ["settings.get"],
    "cookie collection that settles after the disconnection must not start the download",
  );
});

test("a settings request that cannot be sent ends the session", async () => {
  const { state, listeners } = installChromeStub();
  state.postMessageThrows = "settings.get";
  await loadServiceWorker();

  listeners.action(videoTab());
  await until(() => notified(state, "Download failed") !== undefined);
  await flush();

  const port = state.ports[0];
  assert.equal(port.disconnected, true, "the port must close when the first request cannot be sent");
  assert.deepEqual(messageTypes(port), []);
  assert.match(notified(state, "Download failed")?.message ?? "", /Could not send request/);
});
