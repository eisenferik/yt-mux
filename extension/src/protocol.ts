export const NATIVE_HOST = "io.yt_mux.host";
export const PROTOCOL_VERSION = 1 as const;

export const PRESET_NAMES = ["archive", "archive-vp9", "compat-mp4"] as const;
export type PresetName = (typeof PRESET_NAMES)[number];

const presetNameSet = new Set<string>(PRESET_NAMES);

export const ERROR_KINDS = [
  "CANCELLED",
  "INVALID_REQUEST",
  "HOST_FAILURE",
  "DESTINATION_PICK_FAILED",
  "COOKIE_FILE_FAILED",
  "PROCESS_START_FAILED",
  "PROCESS_OUTPUT_FAILED",
  "MISSING_OUTPUT_PATH",
  "YTDLP_FAILED",
  "YTDLP_LOGIN_REQUIRED",
] as const;
export type ErrorKind = (typeof ERROR_KINDS)[number];

export interface Settings {
  destination: string;
  defaultPreset: PresetName;
  subtitleLanguages: string[];
  cookiesEnabled: boolean;
  notifications: boolean;
}

export interface NativeCookie {
  domain: string;
  name: string;
  value: string;
  path: string;
  secure: boolean;
  httpOnly?: boolean;
  expirationDate?: number;
}

interface NativeMessageBase {
  v: number;
  id: string;
}

interface AcceptedMessage extends NativeMessageBase {
  type: "accepted";
}

interface ProgressMessage extends NativeMessageBase {
  type: "progress";
  downloaded: number | null;
  total: number | null;
  speed: number | null;
  eta: number | null;
  pct: number | null;
}

interface PostprocessMessage extends NativeMessageBase {
  type: "postprocess";
}

interface DoneMessage extends NativeMessageBase {
  type: "done";
  path: string;
}

interface ErrorMessage extends NativeMessageBase {
  type: "error";
  kind: ErrorKind;
  detail?: string;
}

interface SettingsMessage extends NativeMessageBase {
  type: "settings";
  settings: Settings;
}

interface SettingsSavedMessage extends NativeMessageBase {
  type: "settings.saved";
  settings: Settings;
}

interface DestinationMessage extends NativeMessageBase {
  type: "destination";
  path: string;
}

export type NativeMessage =
  | AcceptedMessage
  | ProgressMessage
  | PostprocessMessage
  | DoneMessage
  | ErrorMessage
  | SettingsMessage
  | SettingsSavedMessage
  | DestinationMessage;

interface NativeRequestBase {
  v: typeof PROTOCOL_VERSION;
  id: string;
}

interface StartRequest extends NativeRequestBase {
  type: "start";
  url: string;
  preset: PresetName;
  cookies?: NativeCookie[];
}

interface CancelRequest extends NativeRequestBase {
  type: "cancel";
}

interface SettingsGetRequest extends NativeRequestBase {
  type: "settings.get";
}

interface SettingsSetRequest extends NativeRequestBase {
  type: "settings.set";
  settings: Settings;
}

interface DestinationPickRequest extends NativeRequestBase {
  type: "destination.pick";
  start?: string;
}

export type NativeRequest =
  | StartRequest
  | CancelRequest
  | SettingsGetRequest
  | SettingsSetRequest
  | DestinationPickRequest;

export function postRequest(port: chrome.runtime.Port, request: NativeRequest): void {
  port.postMessage(request);
}

export function requestID(): string {
  return crypto.randomUUID().replaceAll("-", "");
}

export function isPreset(value: string): value is PresetName {
  return presetNameSet.has(value);
}
