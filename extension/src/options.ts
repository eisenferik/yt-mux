import { sendNative } from "./native.js";
import { SaveReply, saveOptionsSettings } from "./settings-save.js";
import {
  isPreset,
  PROTOCOL_VERSION,
  requestID,
  Settings,
} from "./protocol.js";

const form = requiredElement("settings-form", HTMLFormElement);
const destination = requiredElement("destination", HTMLInputElement);
const defaultPreset = requiredElement("default-preset", HTMLSelectElement);
const subtitleLanguages = requiredElement("subtitle-languages", HTMLInputElement);
const cookiesEnabled = requiredElement("cookies-enabled", HTMLInputElement);
const notifications = requiredElement("notifications", HTMLInputElement);
const saveButton = requiredElement("save-button", HTMLButtonElement);
const browseButton = requiredElement("browse-button", HTMLButtonElement);
const status = requiredElement("status", HTMLElement);
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
  if (message.type === "error") {
    failed(message.detail ?? "Could not load settings.");
    return;
  }
  failed("Native host returned an unexpected response.");
}

async function saveSettings(): Promise<void> {
  const preset = defaultPreset.value;
  if (!isPreset(preset)) {
    failed("Choose a supported preset.");
    return;
  }
  const languages = subtitleLanguages.value.split(",").map((value) => value.trim()).filter(Boolean);
  if (languages.length === 0) {
    failed("Enter at least one subtitle language.");
    return;
  }

  const settings: Settings = {
    destination: destination.value.trim(),
    defaultPreset: preset,
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
  if (reply.message.type === "error") {
    return reply.message.detail ?? "Could not save settings.";
  }
  return "Native host returned an unexpected response.";
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
  if (message.type === "error") {
    failed(message.detail ?? "Could not choose a folder.");
    return;
  }
  failed("Native host returned an unexpected response.");
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

function requiredElement<T extends HTMLElement>(id: string, expected: new () => T): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`Missing element: ${id}`);
  }
  if (!(element instanceof expected)) {
    throw new Error(`Unexpected element type: ${id}`);
  }
  return element;
}
