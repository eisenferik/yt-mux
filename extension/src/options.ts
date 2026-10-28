import { sendNative } from "./native.js";
import { SaveReply, saveOptionsSettings } from "./settings-save.js";
import { PresetName, PROTOCOL_VERSION, requestID, Settings } from "./protocol.js";

const form = element<HTMLFormElement>("settings-form");
const destination = element<HTMLInputElement>("destination");
const defaultPreset = element<HTMLSelectElement>("default-preset");
const subtitleLanguages = element<HTMLInputElement>("subtitle-languages");
const cookiesEnabled = element<HTMLInputElement>("cookies-enabled");
const notifications = element<HTMLInputElement>("notifications");
const saveButton = element<HTMLButtonElement>("save-button");
const browseButton = element<HTMLButtonElement>("browse-button");
const status = element<HTMLElement>("status");
const formControls = [
  destination,
  defaultPreset,
  subtitleLanguages,
  cookiesEnabled,
  notifications,
  saveButton,
  browseButton,
];

let loadedSettings: Settings | undefined;
void loadSettings();

form.addEventListener("submit", (event) => {
  event.preventDefault();
  void saveSettings();
});
browseButton.addEventListener("click", () => {
  void pickDestination();
});

async function loadSettings(): Promise<void> {
  busy("Loading settings…");
  const id = requestID();
  const sent = await sendNative({ v: PROTOCOL_VERSION, type: "settings.get", id });
  if (sent.type !== "message") {
    failed(sent.detail);
    return;
  }
  const message = sent.message;
  if (message.type === "settings") {
    showSettings(message.settings);
    ready("Settings loaded.");
    return;
  }
  failed("Could not load settings.");
}

async function saveSettings(): Promise<void> {
  const languages = subtitleLanguages.value.split(",").map((value) => value.trim()).filter(Boolean);

  const settings: Settings = {
    destination: destination.value.trim(),
    defaultPreset: defaultPreset.value as PresetName,
    subtitleLanguages: languages,
    cookiesEnabled: cookiesEnabled.checked,
    notifications: notifications.checked,
  };
  busy("Saving settings…");

  const reply = await saveOptionsSettings(settings);
  if (reply.type === "message" && reply.message.type === "settings.saved") {
    showSettings(reply.message.settings);
    ready("Settings saved.");
    return;
  }
  failed(saveFailureDetail(reply));
  cookiesEnabled.checked = loadedSettings?.cookiesEnabled === true;
}

function showSettings(settings: Settings): void {
  loadedSettings = settings;
  destination.value = settings.destination;
  defaultPreset.value = settings.defaultPreset;
  subtitleLanguages.value = settings.subtitleLanguages.join(", ");
  cookiesEnabled.checked = settings.cookiesEnabled;
  notifications.checked = settings.notifications;
}

function saveFailureDetail(reply: SaveReply): string {
  if (reply.type === "error") {
    return reply.error;
  }
  const message = reply.message;
  return message.type === "error" && message.kind === "INVALID_REQUEST" && message.detail
    ? message.detail
    : "Could not save settings.";
}

async function pickDestination(): Promise<void> {
  const id = requestID();
  const start = destination.value.trim();
  busy("Choose a folder…");
  const sent = await sendNative({
    v: PROTOCOL_VERSION,
    type: "destination.pick",
    id,
    ...(start ? { start } : {}),
  });
  if (sent.type !== "message") {
    failed(sent.detail);
    return;
  }
  const message = sent.message;
  if (message.type === "destination") {
    destination.value = message.path;
    ready("Folder selected.");
    return;
  }
  if (message.type === "error" && message.kind === "CANCELLED") {
    ready("Folder selection cancelled.");
    return;
  }
  failed("Could not choose a folder.");
}

function busy(message: string): void {
  showStatus(message, true, false);
}

function ready(message: string): void {
  showStatus(message, false, false);
}

function failed(message: string): void {
  showStatus(message, false, true);
}

function showStatus(message: string, disabled: boolean, error: boolean): void {
  for (const control of formControls) {
    control.disabled = disabled;
  }
  status.textContent = message;
  status.classList.toggle("error", error);
}

function element<T extends HTMLElement>(id: string): T {
  return document.getElementById(id) as T;
}
