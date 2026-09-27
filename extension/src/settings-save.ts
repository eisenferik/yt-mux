import { CookieSaveResult, CookieState } from "./cookie-state.js";
import { NativeResult, sendNative } from "./native.js";
import { checkCookiePermission, CookieGrant, grantCookiePermission } from "./permissions.js";
import { NativeMessage, PROTOCOL_VERSION, requestID, Settings } from "./protocol.js";

const channel = "yt-mux-settings-save";
interface SaveRequest {
  type: "save";
  settings: Settings;
  grant: CookieGrant;
}
type SaveChannelMessage = SaveRequest | { type: "pending" };
export type SaveReply =
  | { type: "message"; message: NativeMessage }
  | { type: "error"; error: string };

// Connect before requesting permission so the worker can protect this pending
// change. The actual request must stay on the options page's gesture stack.
export function saveOptionsSettings(settings: Settings): Promise<SaveReply> {
  const port = chrome.runtime.connect({ name: channel });
  const grant = settings.cookiesEnabled
    ? grantCookiePermission()
    : Promise.resolve({ granted: false, newlyGranted: false });
  return new Promise((resolve) => {
    let settled = false;
    // An idle runtime port alone does not keep an MV3 worker alive. Maintain
    // this in-progress transaction while the user considers the permission prompt.
    const keepAlive = setInterval(() => {
      try {
        port.postMessage({ type: "pending" } satisfies SaveChannelMessage);
      } catch (error) {
        settle({ type: "error", error: failureDetail(error) });
      }
    }, 20_000);
    const settle = (reply: SaveReply): void => {
      settled = true;
      clearInterval(keepAlive);
      resolve(reply);
      port.disconnect();
    };
    port.onMessage.addListener((reply: SaveReply) => {
      if (settled) {
        return;
      }
      settle(reply);
    });
    port.onDisconnect.addListener(() => {
      if (settled) {
        return;
      }
      settle({
        type: "error",
        error: chrome.runtime.lastError?.message ?? "Settings service disconnected.",
      });
    });
    void grant.then((result) => {
      if (settled) {
        return;
      }
      try {
        port.postMessage({ type: "save", settings, grant: result } satisfies SaveChannelMessage);
      } catch (error) {
        settle({ type: "error", error: failureDetail(error) });
      }
    });
  });
}

export function handleSettingsSaves(state: CookieState): void {
  chrome.runtime.onConnect.addListener((port) => {
    if (port.name !== channel) {
      return;
    }
    const change = state.begin();
    let received = false;
    let closed = false;
    port.onDisconnect.addListener(() => {
      closed = true;
      if (!received) {
        void change.finish("unchanged");
      }
    });
    port.onMessage.addListener((request: SaveChannelMessage) => {
      if (request.type !== "save") {
        return;
      }
      if (received || closed) {
        return;
      }
      received = true;
      change.granted(request.grant.newlyGranted);
      void save(request);
    });

    async function save(request: SaveRequest): Promise<void> {
      let result: CookieSaveResult = "unchanged";
      let reply: SaveReply | undefined;
      try {
        await change.ready;
        if (!closed && (await cookiePermissionMissing(request))) {
          reply = { type: "error", error: "Cookie permission was not granted." };
        } else if (!closed) {
          const sent = await sendNative({
            v: PROTOCOL_VERSION,
            type: "settings.set",
            id: requestID(),
            settings: request.settings,
          });
          result = saveResult(request.settings.cookiesEnabled, sent);
          reply = sent.type === "message"
            ? { type: "message", message: sent.message }
            : { type: "error", error: sent.detail };
        }
      } finally {
        await change.finish(result);
      }
      if (reply !== undefined && !closed) {
        port.postMessage(reply);
      }
    }
  });
}

async function cookiePermissionMissing(request: SaveRequest): Promise<boolean> {
  if (!request.settings.cookiesEnabled) {
    return false;
  }
  return !request.grant.granted || (await checkCookiePermission()) !== "granted";
}

function saveResult(cookiesEnabled: boolean, sent: NativeResult): CookieSaveResult {
  if (sent.type === "message") {
    if (sent.message.type !== "settings.saved") {
      return "unchanged";
    }
    return cookiesEnabled ? "enabled" : "disabled";
  }
  return sent.type === "disconnected" && cookiesEnabled ? "enabled" : "unchanged";
}

function failureDetail(error: unknown): string {
  return error instanceof Error ? error.message : "Could not communicate with the settings service.";
}
