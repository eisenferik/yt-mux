import type { CookieAccess } from "./permissions.js";
import { collectCookies, CookieCollection } from "./cookie-collection.js";
import { createCookieState } from "./cookie-state.js";
import { handleSettingsSaves } from "./settings-save.js";
import {
  badgeClearDelay,
  clearBadge,
  downloadMenuTitle,
  errorDetail,
  filenameFromPath,
  notify,
  presetTitles,
  showActiveBadge,
  showCancellingBadge,
  showProcessingBadge,
  showDoneBadge,
  showFailedBadge,
} from "./presentation.js";
import {
  ErrorKind,
  isPreset,
  NATIVE_HOST,
  NativeMessage,
  NativeRequest,
  postRequest,
  PRESET_NAMES,
  PresetName,
  PROTOCOL_VERSION,
  requestID,
  Settings,
} from "./protocol.js";
import { youTubeVideoID } from "./youtube-url.js";

const menuPresets = PRESET_NAMES.map((id) => ({ id, title: presetTitles[id] }));

type SessionPhase =
  | { type: "loading-settings" }
  | { type: "preparing-download"; settings: Settings }
  | { type: "downloading"; settings: Settings }
  | { type: "terminal" };

interface Session {
  id: string;
  tabID: number;
  url: string;
  videoID: string;
  requestedPreset: PresetName | undefined;
  port: chrome.runtime.Port;
  phase: SessionPhase;
  cookieAccess: CookieAccess;
}

type TerminalOutcome =
  | { type: "completed"; path: string }
  | { type: "host-error"; errorKind: ErrorKind; detail: string | undefined }
  | { type: "disconnected"; detail: string | undefined }
  | { type: "request-not-sent"; detail: string | undefined }
  | { type: "cancelled-before-start" };

interface TabClaim {
  tabID: number;
  url: string;
  videoID: string;
}

const sessions = new Map<number, Session>();
const badgeClearTimers = new Map<number, ReturnType<typeof setTimeout>>();
handleSettingsSaves(createCookieState());

chrome.runtime.onInstalled.addListener(() => createContextMenus());

chrome.action.onClicked.addListener((tab) => {
  if (tab.id !== undefined) {
    const running = sessions.get(tab.id);
    if (running) {
      void cancelSession(running);
      return;
    }
  }
  void downloadTab(tab);
});

chrome.commands.onCommand.addListener((command) => {
  if (command !== "download-video") {
    return;
  }
  void downloadActiveTab();
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (!tab || typeof info.menuItemId !== "string" || !isPreset(info.menuItemId)) {
    return;
  }
  void downloadTab(tab, info.menuItemId);
});

function createContextMenus(): void {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: "yt-mux-download", title: downloadMenuTitle, contexts: ["action"] });
    for (const preset of menuPresets) {
      chrome.contextMenus.create({
        id: preset.id,
        parentId: "yt-mux-download",
        title: preset.title,
        contexts: ["action"],
      });
    }
  });
}

async function downloadActiveTab(): Promise<void> {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab) {
    await downloadTab(tab);
  }
}

function claimTab(tab: chrome.tabs.Tab): TabClaim | undefined {
  const videoID = youTubeVideoID(tab.url);
  if (tab.id === undefined || tab.url === undefined || videoID === undefined) {
    notify("Download unavailable", "Open a YouTube video before starting yt-mux.");
    return undefined;
  }
  if (sessions.has(tab.id)) {
    notify("Download already running", "Click the toolbar action again to cancel it.");
    return undefined;
  }
  if ([...sessions.values()].some((session) => session.videoID === videoID)) {
    notify("Video already downloading", "Another tab is downloading this video.");
    return undefined;
  }
  return { tabID: tab.id, url: tab.url, videoID };
}

async function downloadTab(tab: chrome.tabs.Tab, requestedPreset?: PresetName): Promise<void> {
  const claim = claimTab(tab);
  if (!claim) {
    return;
  }
  const id = requestID();
  let port: chrome.runtime.Port;
  try {
    port = chrome.runtime.connectNative(NATIVE_HOST);
  } catch {
    notify("Native host unavailable", "Run the yt-mux setup script, then try again.");
    return;
  }

  const session: Session = {
    id,
    tabID: claim.tabID,
    url: claim.url,
    videoID: claim.videoID,
    requestedPreset,
    port,
    phase: { type: "loading-settings" },
    cookieAccess: "denied",
  };
  sessions.set(claim.tabID, session);
  cancelBadgeClear(claim.tabID);

  port.onMessage.addListener((message: NativeMessage) => {
    void handleNativeMessage(session, message);
  });
  port.onDisconnect.addListener(() => {
    const detail = chrome.runtime.lastError?.message;
    void finishSession(session, { type: "disconnected", detail });
  });

  await showActiveBadge(claim.tabID, null);
  if (session.phase.type === "terminal") {
    return;
  }
  await sendRequestOrFinishSession(session, { v: PROTOCOL_VERSION, type: "settings.get", id });
}

async function handleNativeMessage(session: Session, message: NativeMessage): Promise<void> {
  if (message.v !== PROTOCOL_VERSION || message.id !== session.id || session.phase.type === "terminal") {
    return;
  }
  switch (message.type) {
    case "settings": {
      if (session.phase.type !== "loading-settings") {
        return;
      }
      await requestStart(session, message.settings);
      return;
    }
    case "accepted": {
      return;
    }
    case "progress": {
      if (session.phase.type !== "downloading") {
        return;
      }
      await showActiveBadge(session.tabID, message.pct);
      return;
    }
    case "postprocess": {
      if (session.phase.type !== "downloading") {
        return;
      }
      await showProcessingBadge(session.tabID);
      return;
    }
    case "done": {
      if (session.phase.type !== "downloading") {
        return;
      }
      await finishSession(session, { type: "completed", path: message.path });
      return;
    }
    case "error": {
      await finishSession(session, {
        type: "host-error",
        errorKind: message.kind,
        detail: message.detail,
      });
      return;
    }
  }
}

async function cancelSession(session: Session): Promise<void> {
  if (session.phase.type === "terminal") {
    return;
  }
  if (session.phase.type !== "downloading") {
    await finishSession(session, { type: "cancelled-before-start" });
    return;
  }
  const delivered = await sendRequestOrFinishSession(session, {
    v: PROTOCOL_VERSION,
    type: "cancel",
    id: session.id,
  });
  if (!delivered) {
    return;
  }
  await showCancellingBadge(session.tabID);
}

async function requestStart(session: Session, settings: Settings): Promise<void> {
  session.phase = { type: "preparing-download", settings };
  const preset = session.requestedPreset ?? settings.defaultPreset;
  const collection: CookieCollection = settings.cookiesEnabled
    ? await collectCookies()
    : { access: "denied", cookies: [] };
  if (session.phase.type !== "preparing-download") {
    return;
  }
  session.cookieAccess = collection.access;
  session.phase = { type: "downloading", settings };
  await sendRequestOrFinishSession(session, {
    v: PROTOCOL_VERSION,
    type: "start",
    id: session.id,
    url: session.url,
    preset,
    ...(collection.cookies.length > 0 ? { cookies: collection.cookies } : {}),
  });
}

async function finishSession(session: Session, outcome: TerminalOutcome): Promise<void> {
  if (session.phase.type === "terminal") {
    return;
  }
  const previousPhase = session.phase;
  const settings = previousPhase.type === "loading-settings" ? undefined : previousPhase.settings;

  session.phase = { type: "terminal" };
  sessions.delete(session.tabID);
  // Register before awaiting badge updates so a new session can cancel this timer.
  scheduleBadgeClear(session.tabID);
  if (outcome.type !== "disconnected") {
    session.port.disconnect();
  }

  switch (outcome.type) {
    case "completed":
      await showDoneBadge(session.tabID);
      if (settings?.notifications !== false) {
        notify("Download complete", filenameFromPath(outcome.path));
      }
      break;
    case "host-error": {
      const cancelled = outcome.errorKind === "CANCELLED";
      await (cancelled ? clearBadge(session.tabID) : showFailedBadge(session.tabID));
      if (!cancelled) {
        const base = errorDetail({
          settings,
          cookieAccess: session.cookieAccess,
          kind: outcome.errorKind,
        });
        const body = outcome.detail ? `${base}\n${outcome.detail}` : base;
        notify("Download failed", body);
      }
      break;
    }
    case "disconnected":
    case "request-not-sent":
      await showFailedBadge(session.tabID);
      notify("Download failed", outcome.detail ?? "The native host disconnected unexpectedly.");
      break;
    case "cancelled-before-start":
      await clearBadge(session.tabID);
      break;
  }
}

async function sendRequestOrFinishSession(session: Session, request: NativeRequest): Promise<boolean> {
  try {
    postRequest(session.port, request);
    return true;
  } catch (error) {
    await finishSession(session, {
      type: "request-not-sent",
      detail: error instanceof Error ? error.message : undefined,
    });
    return false;
  }
}

function scheduleBadgeClear(tabID: number): void {
  cancelBadgeClear(tabID);
  const timer = setTimeout(() => {
    badgeClearTimers.delete(tabID);
    if (!sessions.has(tabID)) {
      void clearBadge(tabID);
    }
  }, badgeClearDelay);
  badgeClearTimers.set(tabID, timer);
}

function cancelBadgeClear(tabID: number): void {
  const timer = badgeClearTimers.get(tabID);
  if (timer !== undefined) {
    clearTimeout(timer);
    badgeClearTimers.delete(tabID);
  }
}
