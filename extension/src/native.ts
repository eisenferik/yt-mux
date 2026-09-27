import { NATIVE_HOST, NativeMessage, NativeRequest, postRequest, PROTOCOL_VERSION } from "./protocol.js";

const unavailable = "Native host unavailable. Run the yt-mux setup script.";

export type NativeResult =
  | { type: "message"; message: NativeMessage }
  | { type: "not-sent"; detail: string }
  | { type: "disconnected"; detail: string };

export function sendNative(request: NativeRequest): Promise<NativeResult> {
  return new Promise((resolve) => {
    let port: chrome.runtime.Port;
    try {
      port = chrome.runtime.connectNative(NATIVE_HOST);
    } catch {
      resolve({ type: "not-sent", detail: unavailable });
      return;
    }

    let settled = false;
    port.onMessage.addListener((message: NativeMessage) => {
      if (settled || message.v !== PROTOCOL_VERSION || message.id !== request.id) {
        return;
      }
      settled = true;
      resolve({ type: "message", message });
      port.disconnect();
    });
    port.onDisconnect.addListener(() => {
      if (settled) {
        return;
      }
      settled = true;
      resolve({
        type: "disconnected",
        detail: chrome.runtime.lastError?.message ?? "Native host unavailable.",
      });
    });
    try {
      postRequest(port, request);
    } catch {
      settled = true;
      port.disconnect();
      resolve({ type: "not-sent", detail: unavailable });
    }
  });
}
