import assert from "node:assert/strict";
import test from "node:test";

import { flush, installChromeStub, installOptionsDocument, loadOptions, settingsReply, until } from "./chrome-stub.js";

async function loadedOptions({ initialize = () => {}, settings = {} } = {}) {
  const { state } = installChromeStub();
  initialize(state);
  const elements = installOptionsDocument();
  await loadOptions();
  const port = state.ports[0];
  port.emitMessage(settingsReply(port.messages[0].id, settings));
  await until(() => elements.status.textContent === "Settings loaded.");
  return { state, elements };
}

test.beforeEach((t) => {
  t.mock.timers.enable({ apis: ["setInterval"] });
});

test("settings load fills the form", async () => {
  const { elements } = await loadedOptions({
    settings: {
      defaultPreset: "compat-mp4",
      subtitleLanguages: ["ja", "en"],
      cookiesEnabled: true,
      notifications: false,
    },
  });
  await flush();

  assert.equal(elements.destination.value, "D:\\Videos");
  assert.equal(elements["default-preset"].value, "compat-mp4");
  assert.equal(elements["subtitle-languages"].value, "ja, en");
  assert.equal(elements["cookies-enabled"].checked, true);
  assert.equal(elements.notifications.checked, false);
});

test("a missing native host is reported on the options page", async () => {
  const { state } = installChromeStub();
  state.connectNativeThrows = true;
  const elements = installOptionsDocument();
  await loadOptions();
  await until(() => elements.status.textContent === "Native host unavailable. Run the yt-mux setup script.");
  await flush();

  assert.equal(state.ports.length, 0);
});

test("a successful settings save reports Settings saved", async () => {
  const { state, elements } = await loadedOptions();

  elements.destination.value = "E:\\Archive";
  elements["settings-form"].dispatch("submit");
  await flush();
  const savePort = state.ports[1];
  const request = savePort.messages[0];
  assert.equal(request.type, "settings.set");
  assert.equal(request.settings.destination, "E:\\Archive");

  savePort.emitMessage({
    v: 1,
    id: request.id,
    type: "settings.saved",
    settings: { ...request.settings, destination: "E:\\Archive\\Saved" },
  });
  await until(() => elements.status.textContent === "Settings saved.");
  await flush();

  assert.equal(elements.status.textContent, "Settings saved.");
  assert.equal(elements.destination.value, "E:\\Archive\\Saved", "the form shows what the host saved");
  assert.equal(elements["save-button"].disabled, false);
});

test("empty subtitle languages are rejected before talking to the host", async () => {
  const { state, elements } = await loadedOptions();

  elements["subtitle-languages"].value = "  ";
  elements["settings-form"].dispatch("submit");
  await flush();

  assert.equal(state.ports.length, 1);
  assert.equal(elements.status.textContent, "Enter at least one subtitle language.");
});

test("turning signed-in YouTube cookies off removes cookie permission", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = true;
    },
    settings: { cookiesEnabled: true },
  });

  elements["cookies-enabled"].checked = false;
  elements["settings-form"].dispatch("submit");
  await flush();
  const savePort = state.ports[1];
  savePort.emitMessage({ v: 1, id: savePort.messages[0].id, type: "settings.saved", settings: savePort.messages[0].settings });
  await until(() => state.permissionRemovals === 1);
  await flush();

  assert.equal(state.permissionRemovals, 1);
  assert.equal(state.permissionGranted, false);
  assert.equal(elements.status.textContent, "Settings saved.");
});

test("settings load failures display the host detail", async () => {
  const { state } = installChromeStub();
  const elements = installOptionsDocument();
  await loadOptions();
  const port = state.ports[0];
  const id = port.messages[0].id;

  port.emitMessage({ v: 1, id, type: "error", kind: "INVALID_REQUEST", detail: "settings.json is invalid" });
  await until(() => elements.status.textContent === "settings.json is invalid");
  await flush();

  assert.equal(elements.status.textContent, "settings.json is invalid");
});

test("a matching response settles before the port disconnects", async () => {
  const { state } = installChromeStub();
  const elements = installOptionsDocument();
  await loadOptions();
  const port = state.ports[0];

  port.emitMessage(settingsReply(port.messages[0].id));
  await until(() => elements.status.textContent === "Settings loaded.");
  await flush();
  chrome.runtime.lastError = { message: "Native host disconnected" };
  port.emitDisconnect();
  await flush();

  assert.equal(port.disconnected, true);
  assert.equal(elements.status.textContent, "Settings loaded.");
});

test("folder selection completes from a single native response", async () => {
  const { state, elements } = await loadedOptions();

  elements.destination.value = "D:\\Videos";
  elements["browse-button"].dispatch("click");
  const pickPort = state.ports[1];
  const request = pickPort.messages[0];
  assert.equal(request.type, "destination.pick");
  assert.equal(request.start, "D:\\Videos");

  pickPort.emitMessage({ v: 1, id: request.id, type: "destination", path: "E:\\Archive" });
  await until(() => pickPort.disconnected);
  await flush();

  assert.equal(pickPort.disconnected, true);
  assert.equal(elements.destination.value, "E:\\Archive");
  assert.equal(elements.status.textContent, "Folder selected.");
});

test("a failed settings save removes only permission granted by that save", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.permissionRequestGranted = true;
    },
  });

  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  await flush();
  const savePort = state.ports[1];
  const id = savePort.messages[0].id;
  savePort.emitMessage({ v: 1, id, type: "error", kind: "INVALID_REQUEST", detail: "save failed" });
  await until(() => state.permissionRemovals === 1);
  await flush();

  assert.equal(state.permissionRequests, 1);
  assert.equal(state.permissionRemovals, 1);
  assert.equal(state.permissionGranted, false);
  assert.equal(elements["cookies-enabled"].checked, false);
  assert.equal(elements.status.textContent, "save failed");
});

test("a rejected cookie permission request reports the failure", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
    },
  });

  state.permissionRequestThrows = true;
  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  await until(() => elements.status.textContent === "Cookie permission was not granted.");
  await flush();

  assert.equal(state.ports.length, 1, "no settings.set may be sent without permission");
  assert.equal(elements["cookies-enabled"].checked, false);
  assert.equal(elements.status.textContent, "Cookie permission was not granted.");
});

test("cookie permission is requested before the existing-permission check settles", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.permissionRequestGranted = true;
      stub.pendingContains = [];
    },
  });
  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");

  assert.equal(state.permissionRequests, 1);
  const saveContains = state.pendingContains.shift();
  assert.ok(saveContains);
  saveContains(false);
  state.pendingContains = null;
  await until(() => state.ports.length === 2);
  await flush();
  assert.equal(state.ports[1].messages[0].type, "settings.set");
});

test("settings save uses one snapshot while permission and native work are pending", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.permissionRequestGranted = true;
      stub.pendingContains = [];
    },
  });

  elements.destination.value = "D:\\Before";
  elements["default-preset"].value = "archive-vp9";
  elements["subtitle-languages"].value = "ja, en";
  elements["cookies-enabled"].checked = true;
  elements.notifications.checked = true;
  elements["settings-form"].dispatch("submit");

  for (const id of [
    "destination",
    "default-preset",
    "subtitle-languages",
    "cookies-enabled",
    "notifications",
    "save-button",
    "browse-button",
  ]) {
    assert.equal(elements[id].disabled, true, `${id} should be disabled while saving`);
  }

  elements.destination.value = "E:\\After";
  elements["default-preset"].value = "compat-mp4";
  elements["subtitle-languages"].value = "fr";
  elements["cookies-enabled"].checked = false;
  elements.notifications.checked = false;

  const saveContains = state.pendingContains.shift();
  assert.ok(saveContains);
  saveContains(false);
  state.pendingContains = null;
  await until(() => state.ports.length === 2);
  await flush();

  const savePort = state.ports[1];
  const request = savePort.messages[0];
  assert.deepEqual(request.settings, {
    destination: "D:\\Before",
    defaultPreset: "archive-vp9",
    subtitleLanguages: ["ja", "en"],
    cookiesEnabled: true,
    notifications: true,
  });

  savePort.emitMessage({ v: 1, id: request.id, type: "settings.saved", settings: request.settings });
  await until(() => elements["save-button"].disabled === false);
  await flush();
  assert.equal(elements["save-button"].disabled, false);
  assert.equal(elements.destination.disabled, false);
});

test("a failed settings save preserves permission that was already granted", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = true;
    },
  });

  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  await flush();
  const savePort = state.ports[1];
  const id = savePort.messages[0].id;
  savePort.emitMessage({ v: 1, id, type: "error", kind: "INVALID_REQUEST", detail: "save failed" });
  await flush();

  assert.equal(state.permissionRequests, 1);
  assert.equal(state.permissionRemovals, 0);
  assert.equal(state.permissionGranted, true);
});

test("a lost save acknowledgement preserves a possibly committed cookie grant", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.permissionRequestGranted = true;
    },
  });

  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  await flush();
  const savePort = state.ports[1];
  chrome.runtime.lastError = { message: "Native host disconnected" };
  savePort.emitDisconnect();
  await until(() => elements.status.textContent === "Native host disconnected");
  await flush();

  assert.equal(state.permissionRemovals, 0);
  assert.equal(state.permissionGranted, true);
  assert.equal(elements.status.textContent, "Native host disconnected");
});

test("a failed native connection after an options grant releases new permission", async () => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.permissionRequestGranted = true;
    },
  });
  state.connectNativeThrows = true;
  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  await until(() => state.permissionRemovals === 1);
  assert.equal(state.permissionGranted, false);
  await until(() => elements.status.textContent.includes("Native host unavailable"));
});

test("the pending options transaction stays active through a long permission prompt and stops after saving", async (t) => {
  const { state, elements } = await loadedOptions({
    initialize: (stub) => {
      stub.permissionGranted = false;
      stub.pendingPermission = [];
    },
  });
  elements["cookies-enabled"].checked = true;
  elements["settings-form"].dispatch("submit");
  t.mock.timers.tick(40_000);
  await flush();
  assert.equal(state.extensionMessages.filter((message) => message.type === "pending").length, 2);
  assert.equal(state.ports.length, 1, "waiting for permission must not submit settings");
  state.pendingPermission.shift()(true);
  await until(() => state.ports.length === 2);
  const savePort = state.ports[1];
  savePort.emitMessage({ v: 1, id: savePort.messages[0].id, type: "settings.saved", settings: savePort.messages[0].settings });
  await until(() => elements.status.textContent === "Settings saved.");
  const count = state.extensionMessages.length;
  t.mock.timers.tick(60_000);
  await flush();
  assert.equal(state.extensionMessages.length, count, "completed saves must not keep the worker alive");
});

for (const success of [true, false]) {
  test(`closing options while saving lets the worker handle the ${success ? "success" : "failure"}`, async () => {
    const { state, elements } = await loadedOptions({
      initialize: (stub) => {
        stub.permissionGranted = false;
        stub.permissionRequestGranted = true;
      },
    });
    elements["cookies-enabled"].checked = true;
    elements["settings-form"].dispatch("submit");
    await until(() => state.ports.length === 2);
    const savePort = state.ports[1];
    state.extensionPorts[0].disconnect();
    await flush();
    assert.equal(savePort.disconnected, false);
    assert.equal(state.permissionRemovals, 0);
    const id = savePort.messages[0].id;
    savePort.emitMessage(success
      ? { v: 1, id, type: "settings.saved", settings: savePort.messages[0].settings }
      : { v: 1, id, type: "error", kind: "INVALID_REQUEST" });
    await until(() => savePort.disconnected);
    await flush();
    assert.equal(state.permissionGranted, success);
    assert.equal(state.permissionRemovals, success ? 0 : 1);
  });
}
