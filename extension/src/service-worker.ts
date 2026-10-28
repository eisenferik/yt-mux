import type { CookieAccess } from "./permissions.js";
import { collectCookies, CookieCollection } from "./cookie-collection.js";
import { createCookieState } from "./cookie-state.js";
import { handleSettingsSaves } from "./settings-save.js";
import {
  badgeClearDelay,
  clearBadge,
  disconnectDetail,
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
  NATIVE_HOST,
  NativeMessage,
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

chrome.commands.onCommand.addListener(() => {
  void downloadActiveTab();
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  void downloadTab(tab as chrome.tabs.Tab, info.menuItemId as PresetName);
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
    notify("Download unavailable", "Open a YouTube video first.");
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
  const port = chrome.runtime.connectNative(NATIVE_HOST);
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
  postRequest(port, { v: PROTOCOL_VERSION, type: "settings.get", id });
}

async function handleNativeMessage(session: Session, message: NativeMessage): Promise<void> {
  switch (message.type) {
    case "settings": {
      await requestStart(session, message.settings);
      return;
    }
    case "progress": {
      await showActiveBadge(session.tabID, message.pct);
      return;
    }
    case "postprocess": {
      await showProcessingBadge(session.tabID);
      return;
    }
    case "done": {
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
  if (session.phase.type !== "downloading") {
    await finishSession(session, { type: "cancelled-before-start" });
    return;
  }
  postRequest(session.port, { v: PROTOCOL_VERSION, type: "cancel", id: session.id });
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
  postRequest(session.port, {
    v: PROTOCOL_VERSION,
    type: "start",
    id: session.id,
    url: session.url,
    preset,
    ...(collection.cookies.length > 0 ? { cookies: collection.cookies } : {}),
  });
}

async function finishSession(session: Session, outcome: TerminalOutcome): Promise<void> {
  const previousPhase = session.phase;
  const settings =
    previousPhase.type === "preparing-download" || previousPhase.type === "downloading" ? previousPhase.settings : undefined;

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
      if (outcome.errorKind === "CANCELLED") {
        await clearBadge(session.tabID);
        break;
      }
      await showFailedBadge(session.tabID);
      const base = errorDetail({
        settings,
        cookieAccess: session.cookieAccess,
        kind: outcome.errorKind,
      });
      notify("Download failed", outcome.detail ? `${base}\n${outcome.detail}` : base);
      break;
    }
    case "disconnected":
      await showFailedBadge(session.tabID);
      notify("Download failed", disconnectDetail(outcome.detail));
      break;
    case "cancelled-before-start":
      await clearBadge(session.tabID);
      break;
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
