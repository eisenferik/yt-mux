import { disconnectDetail } from "./presentation.js";
import { NATIVE_HOST, NativeMessage, NativeRequest, postRequest } from "./protocol.js";

export type NativeResult =
  | { type: "message"; message: NativeMessage }
  | { type: "disconnected"; detail: string };

export function sendNative(request: NativeRequest): Promise<NativeResult> {
  return new Promise((resolve) => {
    const port = chrome.runtime.connectNative(NATIVE_HOST);
    port.onMessage.addListener((message: NativeMessage) => {
      resolve({ type: "message", message });
      port.disconnect();
    });
    port.onDisconnect.addListener(() => {
      resolve({ type: "disconnected", detail: disconnectDetail(chrome.runtime.lastError?.message) });
    });
    postRequest(port, request);
  });
}
