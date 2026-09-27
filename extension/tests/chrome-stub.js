import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { afterEach } from "node:test";

const serviceWorkerURL = new URL("../dist/service-worker.js", import.meta.url);
const optionsURL = new URL("../dist/options.js", import.meta.url);

let loadCounter = 0;
let currentListeners;

function keyPaths(value, prefix = "") {
  if (Array.isArray(value)) {
    return value.flatMap((item) => keyPaths(item, `${prefix}[]`));
  }
  if (value === null || typeof value !== "object") {
    return [];
  }
  return Object.keys(value).flatMap((key) => {
    const path = `${prefix}.${key}`;
    return [path, ...keyPaths(value[key], path)];
  });
}

const describedRequestPaths = new Map();
for (const { message } of JSON.parse(
  readFileSync(new URL("../../testdata/protocol-messages.json", import.meta.url), "utf8"),
).accepted) {
  const described = describedRequestPaths.get(message.type) ?? new Set();
  keyPaths(message).forEach((path) => described.add(path));
  describedRequestPaths.set(message.type, described);
}

const undescribedRequests = [];

function recordRequestShape(message) {
  const described = describedRequestPaths.get(message.type);
  if (described === undefined) {
    undescribedRequests.push(`${message.type} is missing from the shared protocol fixture`);
    return;
  }
  for (const path of keyPaths(message)) {
    if (!described.has(path)) {
      undescribedRequests.push(`${message.type} sends ${path}, which the shared protocol fixture does not describe`);
    }
  }
}

// The worker catches everything postMessage throws, so violations are collected instead.
afterEach(() => {
  assert.deepEqual(undescribedRequests.splice(0), [], "the native host refuses fields it does not define");
});

export function installChromeStub() {
  const state = {
    ports: [],
    extensionPorts: [],
    extensionMessages: [],
    notifications: [],
    badges: [],
    permissionGranted: true,
    permissionRequestGranted: undefined,
    permissionRequestThrows: false,
    permissionRequests: 0,
    permissionRemovals: 0,
    permissionCheckRejects: false,
    cookieReadRejects: false,
    pendingContains: null,
    pendingPermission: null,
    badgeGate: null,
    cookies: [],
    connectNativeThrows: false,
    postMessageThrows: null,
  };
  const listeners = {};
  currentListeners = listeners;
  const event = (name) => ({
    addListener(handler) {
      listeners[name] = handler;
    },
  });
  const badgeResult = (detail) => {
    state.badges.push(detail);
    return state.badgeGate ?? Promise.resolve();
  };

  globalThis.chrome = {
    runtime: {
      lastError: undefined,
      onInstalled: event("installed"),
      onConnect: event("connect"),
      connect({ name }) {
        const endpoints = [0, 1].map(() => ({ messages: [], disconnects: [] }));
        let disconnected = false;
        const ports = endpoints.map((own, index) => ({
          name,
          onMessage: { addListener: (handler) => own.messages.push(handler) },
          onDisconnect: { addListener: (handler) => own.disconnects.push(handler) },
          postMessage(message) {
            if (disconnected) throw new Error("Port disconnected");
            const copy = structuredClone(message);
            state.extensionMessages.push(copy);
            queueMicrotask(() => {
              if (!disconnected) endpoints[1 - index].messages.forEach((handler) => handler(copy));
            });
          },
          disconnect() {
            if (disconnected) return;
            disconnected = true;
            queueMicrotask(() => endpoints.forEach((endpoint) => endpoint.disconnects.forEach((handler) => handler())));
          },
        }));
        listeners.connect?.(ports[1]);
        state.extensionPorts.push(ports[0]);
        return ports[0];
      },
      connectNative() {
        if (state.connectNativeThrows) {
          throw new Error("Native host unavailable");
        }
        const port = {
          messages: [],
          disconnected: false,
          emitMessage: () => {},
          emitDisconnect: () => {},
          onMessage: {
            addListener(handler) {
              port.emitMessage = handler;
            },
          },
          onDisconnect: {
            addListener(handler) {
              port.emitDisconnect = handler;
            },
          },
          postMessage(message) {
            if (state.postMessageThrows === message.type) throw new Error("Could not send request");
            recordRequestShape(message);
            port.messages.push(message);
          },
          disconnect() {
            port.disconnected = true;
          },
        };
        state.ports.push(port);
        return port;
      },
    },
    action: {
      onClicked: event("action"),
      setBadgeBackgroundColor: badgeResult,
      setBadgeText: badgeResult,
    },
    commands: { onCommand: event("command") },
    contextMenus: { onClicked: event("menu") },
    notifications: {
      create(options) {
        state.notifications.push(options);
        return Promise.resolve("notification");
      },
    },
    permissions: {
      contains(_permission) {
        if (state.permissionCheckRejects) {
          return Promise.reject(new Error("Permission is not declared in the manifest"));
        }
        if (state.pendingContains) {
          return new Promise((resolve) => state.pendingContains.push(resolve));
        }
        return Promise.resolve(state.permissionGranted);
      },
      request(_permission) {
        state.permissionRequests += 1;
        if (state.permissionRequestThrows) {
          throw new Error("This function must be called during a user gesture");
        }
        if (state.pendingPermission) {
          return new Promise((resolve) => {
            state.pendingPermission.push((granted) => {
              state.permissionGranted = granted;
              resolve(granted);
            });
          });
        }
        const granted = state.permissionRequestGranted ?? state.permissionGranted;
        state.permissionGranted = granted;
        return Promise.resolve(granted);
      },
      remove(_permission) {
        state.permissionRemovals += 1;
        state.permissionGranted = false;
        return Promise.resolve(true);
      },
    },
    cookies: {
      getAll(_query) {
        if (state.cookieReadRejects) {
          return Promise.reject(new Error("cookie read failed"));
        }
        return Promise.resolve(state.cookies);
      },
    },
    tabs: {
      query(_query) {
        return Promise.resolve([]);
      },
    },
  };

  return { state, listeners };
}

export async function loadServiceWorker() {
  loadCounter += 1;
  await import(`${serviceWorkerURL.href}?case=${loadCounter}`);
}

export async function loadOptions() {
  if (!currentListeners.connect) await loadServiceWorker();
  loadCounter += 1;
  await import(`${optionsURL.href}?case=${loadCounter}`);
}

export function installOptionsDocument() {
  globalThis.HTMLElement = FakeHTMLElement;
  globalThis.HTMLFormElement = FakeHTMLFormElement;
  globalThis.HTMLInputElement = FakeHTMLInputElement;
  globalThis.HTMLSelectElement = FakeHTMLSelectElement;
  globalThis.HTMLButtonElement = FakeHTMLButtonElement;

  const elements = new Map(Object.entries({
    "settings-form": new FakeHTMLFormElement(),
    destination: new FakeHTMLInputElement(),
    "default-preset": new FakeHTMLSelectElement(),
    "subtitle-languages": new FakeHTMLInputElement(),
    "cookies-enabled": new FakeHTMLInputElement(),
    notifications: new FakeHTMLInputElement(),
    "save-button": new FakeHTMLButtonElement(),
    "browse-button": new FakeHTMLButtonElement(),
    status: new FakeHTMLElement(),
  }));
  globalThis.document = {
    getElementById(id) {
      return elements.get(id) ?? null;
    },
  };
  return Object.fromEntries(elements);
}

class FakeHTMLElement {
  constructor() {
    this.value = "";
    this.checked = false;
    this.disabled = false;
    this.textContent = "";
    this.classList = { toggle() {} };
    this.listeners = new Map();
  }

  addEventListener(name, handler) {
    this.listeners.set(name, handler);
  }

  dispatch(name) {
    const handler = this.listeners.get(name);
    handler?.({ preventDefault() {} });
  }
}

class FakeHTMLFormElement extends FakeHTMLElement {}
class FakeHTMLInputElement extends FakeHTMLElement {}
class FakeHTMLSelectElement extends FakeHTMLElement {}
class FakeHTMLButtonElement extends FakeHTMLElement {}

export const settingsReply = (id, overrides = {}) => ({
  v: 1,
  id,
  type: "settings",
  settings: {
    destination: "D:\\Videos",
    defaultPreset: "archive",
    subtitleLanguages: ["ja"],
    cookiesEnabled: false,
    notifications: true,
    ...overrides,
  },
});

export async function flush(times = 6) {
  for (let index = 0; index < times; index += 1) {
    await new Promise((resolve) => setImmediate(resolve));
  }
}

export async function until(predicate, turns = 50) {
  for (let index = 0; index <= turns; index += 1) {
    if (predicate()) {
      return;
    }
    if (index < turns) {
      await new Promise((resolve) => setImmediate(resolve));
    }
  }
  throw new Error("expected condition never held");
}

export function deferred() {
  let resolve = () => {};
  const promise = new Promise((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
