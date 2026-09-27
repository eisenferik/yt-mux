import type { CookieAccess } from "./permissions.js";
import { ErrorKind, PresetName, Settings } from "./protocol.js";

export const downloadMenuTitle = "Download";
export const badgeClearDelay = 4000;

export const presetTitles = {
  archive: "Archive",
  "archive-vp9": "Archive / VP9",
  "compat-mp4": "Compatible MP4",
} satisfies Record<PresetName, string>;

const genericHostFailureMessage = "The native host could not complete the request.";
const badgeActive = "#2563eb";
const badgeDone = "#15803d";
const badgeFailed = "#b91c1c";
const badgeCancelled = "#6b7280";
const badgeWorking = "…";
const badgeProcessing = "⚙";

export function filenameFromPath(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts.at(-1) || path;
}

export function showActiveBadge(tabID: number, pct: number | null): Promise<void> {
  const text = typeof pct === "number" ? `${Math.floor(pct)}%` : badgeWorking;
  return setBadge(tabID, text, badgeActive);
}

export function showProcessingBadge(tabID: number): Promise<void> {
  return setBadge(tabID, badgeProcessing, badgeActive);
}

export function showCancellingBadge(tabID: number): Promise<void> {
  return setBadge(tabID, badgeWorking, badgeCancelled);
}

export function showDoneBadge(tabID: number): Promise<void> {
  return setBadge(tabID, "✓", badgeDone);
}

export function showFailedBadge(tabID: number): Promise<void> {
  return setBadge(tabID, "!", badgeFailed);
}

export function clearBadge(tabID: number): Promise<void> {
  return setBadge(tabID, "", badgeCancelled);
}

async function setBadge(tabID: number, text: string, color: string): Promise<void> {
  try {
    await chrome.action.setBadgeBackgroundColor({ tabId: tabID, color });
    await chrome.action.setBadgeText({ tabId: tabID, text });
  } catch {
    return;
  }
}

export function notify(title: string, message: string): void {
  void chrome.notifications
    .create({
      type: "basic",
      iconUrl: "icons/icon128.png",
      title,
      message: message.slice(0, 240),
    })
    .catch(() => undefined);
}

function genericHostFailure(_kind: never): string {
  return genericHostFailureMessage;
}

interface ErrorDetailContext {
  settings: Settings | undefined;
  cookieAccess: CookieAccess;
  kind: ErrorKind;
}

export function errorDetail({ settings, cookieAccess, kind }: ErrorDetailContext): string {
  switch (kind) {
    case "YTDLP_LOGIN_REQUIRED":
      if (!settings?.cookiesEnabled) {
        return (
          "This video needs your YouTube sign-in. " +
          "Turn on signed-in YouTube cookies in extension options, then try again."
        );
      }
      if (cookieAccess === "denied") {
        return (
          "Cookie access is disabled. " +
          "Save extension options with signed-in YouTube cookies turned on, then try again."
        );
      }
      if (cookieAccess === "unreadable") {
        return "Your browser cookies could not be read, so the download ran signed out. Try again.";
      }
      return "Sign in to YouTube in this browser, then try again.";
    case "YTDLP_FAILED":
      return "yt-dlp could not download this video.";
    case "PROCESS_START_FAILED":
      return "A required dependency is missing. Run setup again.";
    case "COOKIE_FILE_FAILED":
      return "The temporary authentication file could not be created.";
    case "MISSING_OUTPUT_PATH":
      return "The download finished without reporting a saved file.";
    case "CANCELLED":
      return "The download was cancelled.";
    case "INVALID_REQUEST":
    case "HOST_FAILURE":
    case "PROCESS_OUTPUT_FAILED":
    case "DESTINATION_PICK_FAILED":
      return genericHostFailureMessage;
    default:
      return genericHostFailure(kind);
  }
}
