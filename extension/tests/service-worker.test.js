import assert from "node:assert/strict";
import test from "node:test";
import { badgeClearDelay, errorDetail } from "../dist/presentation.js";

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
const lastNotification = (state) => state.notifications.at(-1);

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
  await until(() => state.notifications.length === 1);

  assert.equal(state.ports.length, 0);
});

test("a finished download reports the saved file", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);

  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\Channel\\clip.mkv" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(lastNotification(state)?.message, "clip.mkv");
});

test("clicking the completion notification reveals the saved file and reports one that is gone", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);
  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\Channel\\clip.mkv" });
  await until(() => state.notifications.length === 1);

  listeners.notificationClicked(lastNotification(state).id);
  await until(() => state.ports.length === 2);
  const reveal = sent(state.ports[1], "reveal");
  assert.equal(reveal?.path, "D:\\Videos\\Channel\\clip.mkv");

  state.ports[1].emitMessage({ v: 1, id: reveal.id, type: "error", kind: "SAVED_FILE_MISSING" });
  await until(() => state.notifications.length === 2);
  assert.equal(lastNotification(state).title, "File not found");
});

test("clicking a failure that an update fixes starts the update or points to the install command", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);
  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_OUTDATED" });
  await until(() => state.notifications.length === 1);

  listeners.notificationClicked(lastNotification(state).id);
  await until(() => state.ports.length === 2);
  const update = sent(state.ports[1], "update");
  assert.notEqual(update, undefined);

  state.ports[1].emitMessage({ v: 1, id: update.id, type: "error", kind: "HOST_FAILURE" });
  await until(() => state.notifications.length === 2);
  assert.equal(lastNotification(state).message, "Run the yt-mux install command to update.");
});

test("an updated installation reloads the extension once the download ends", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  listeners.action(videoTab());
  const { port, id } = await requestedSettings(state);
  port.emitMessage({ ...settingsReply(id), version: "99.0.0" });
  await until(() => sent(port, "start") !== undefined);
  assert.equal(state.reloads, 0);

  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\clip.mkv" });
  await until(() => state.reloads === 1);
  assert.equal(state.notifications.length, 0);

  await loadServiceWorker();
  await until(() => state.notifications.length === 1);
  assert.equal(lastNotification(state).message, "clip.mkv");
});

test("a previous download cannot clear the next badge early", async (t) => {
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
  second.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\clip.mkv" });
  await flush();
  const result = badgeText();

  t.mock.timers.tick(badgeClearDelay - 1);
  await flush();
  assert.equal(badgeText(), result, "the first download's timeout must not clear the second result");
  t.mock.timers.tick(1);
  await flush();
  assert.equal(badgeText(), "", "the second result must clear after its own full delay");
});

test("a context-menu preset is sent on start", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.menu({ menuItemId: "compat-mp4" }, videoTab());
  const { port } = await startedDownload(state);

  assert.equal(sent(port, "start").preset, "compat-mp4");
});

test("a cancelled download does not notify failure", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);

  port.emitMessage({ v: 1, id, type: "error", kind: "CANCELLED" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(state.notifications.length, 0);
});

test("a settings reply starts the download with the default preset", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await requestedSettings(state);

  port.emitMessage(settingsReply(id));
  await until(() => sent(port, "start") !== undefined);
  assert.deepEqual(sent(port, "start"), {
    v: 1,
    type: "start",
    id,
    url: tab.url,
    preset: "archive",
  });
});

test("a second click after start sends cancel instead of disconnecting", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port, id } = await startedDownload(state);

  listeners.action(tab);
  await until(() => sent(port, "cancel") !== undefined);
  await flush();
  assert.equal(state.ports.length, 1, "cancel must reuse the existing port");
  assert.equal(port.disconnected, false, "an in-flight download is cancelled over the port");
  assert.deepEqual(sent(port, "cancel"), { v: 1, type: "cancel", id });
});

test("another tab cannot download a video that is already downloading", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);

  listeners.action({ id: 8, url: "https://youtu.be/abc" });
  await until(() => state.notifications.length === 1);
  assert.equal(state.ports.length, 1);

  port.emitMessage({ v: 1, id, type: "error", kind: "CANCELLED" });
  await until(() => port.disconnected);
  listeners.action({ id: 8, url: "https://youtu.be/abc" });
  await until(() => state.ports.length === 2);
});

test("a second click cancels the pending session instead of opening another port", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const tab = videoTab();

  listeners.action(tab);
  const { port } = await requestedSettings(state);

  listeners.action(tab);
  await until(() => port.disconnected);
  await flush();
  assert.equal(state.ports.length, 1, "the second click must not open a second port");
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

  listeners.action(tab);
  await until(() => port.disconnected);

  state.pendingContains.shift()(true);
  await flush();
  assert.deepEqual(
    messageTypes(port),
    ["settings.get"],
    "cookie collection that settles after cancellation must not start the download",
  );
});

test("the badge shows the whole percentage", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();
  const badgeTexts = () => state.badges.filter((badge) => badge.text !== undefined).map((badge) => badge.text);

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);
  const before = badgeTexts().length;

  port.emitMessage({ v: 1, id, type: "progress", pct: 41.7 });
  await until(() => badgeTexts().length === before + 1);

  assert.equal(badgeTexts().at(-1), "41%");
});

test("a tab closed mid-download still finishes the session", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state);

  state.badgeGate = Promise.reject(new Error("No tab with id: 7"));
  port.emitMessage({ v: 1, id, type: "done", path: "D:\\Videos\\out.mkv" });
  await until(() => port.disconnected);
  await flush();

  assert.equal(state.notifications.length, 1, "completion must be reported even when the tab is gone");
});

test("download failures are notified when completion notifications are disabled", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port, id } = await startedDownload(state, { notifications: false });
  port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_FAILED", detail: "ERROR: disk full" });
  await until(() => state.notifications.length === 1);

  assert.match(lastNotification(state).message, /disk full/);
});

for (const { name, permissionGranted, cookieAccess } of [
  { name: "missing cookie permission", permissionGranted: false, cookieAccess: "denied" },
  { name: "signed-out cookies", permissionGranted: true, cookieAccess: "granted" },
]) {
  test(`${name} get the remedy for that cookie state`, async () => {
    const { state, listeners } = installChromeStub();
    state.permissionGranted = permissionGranted;
    await loadServiceWorker();

    listeners.action(videoTab());
    const { port, id } = await startedDownload(state, { cookiesEnabled: true });
    port.emitMessage({ v: 1, id, type: "error", kind: "YTDLP_LOGIN_REQUIRED" });
    await until(() => state.notifications.length === 1);

    const remedy = errorDetail({ settings: { cookiesEnabled: true }, cookieAccess, kind: "YTDLP_LOGIN_REQUIRED" });
    assert.equal(lastNotification(state).message, remedy);
  });
}

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

  assert.equal(sent(port, "start").cookies.length, 2);
});

test("a host that disconnects mid-download reports the failure", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port } = await startedDownload(state);

  chrome.runtime.lastError = { message: "Native host has exited." };
  port.emitDisconnect();
  await until(() => state.notifications.length === 1);

  assert.match(lastNotification(state).message, /Native host has exited/);
});

test("a missing native host is reported with the setup remedy instead of Chrome's message", async () => {
  const { state, listeners } = installChromeStub();
  await loadServiceWorker();

  listeners.action(videoTab());
  const { port } = await requestedSettings(state);

  chrome.runtime.lastError = { message: "Specified native messaging host not found." };
  port.emitDisconnect();
  await until(() => state.notifications.length === 1);

  assert.notEqual(lastNotification(state).message, "Specified native messaging host not found.");
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
  await until(() => state.notifications.length === 1);

  state.pendingContains.shift()(true);
  await flush();
  assert.deepEqual(
    messageTypes(port),
    ["settings.get"],
    "cookie collection that settles after the disconnection must not start the download",
  );
});
