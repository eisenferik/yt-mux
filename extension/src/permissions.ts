export type CookieAccess = "granted" | "denied" | "unreadable";

export const COOKIE_PERMISSION: chrome.permissions.Permissions = {
  permissions: ["cookies"],
  origins: [
    "https://youtube.com/*",
    "https://*.youtube.com/*",
    "https://google.com/*",
    "https://*.google.com/*",
  ],
};

export interface CookieGrant {
  granted: boolean;
  newlyGranted: boolean;
}

export async function checkCookiePermission(): Promise<CookieAccess> {
  try {
    return (await chrome.permissions.contains(COOKIE_PERMISSION)) ? "granted" : "denied";
  } catch {
    return "unreadable";
  }
}

async function requestCookiePermission(): Promise<boolean> {
  try {
    return await chrome.permissions.request(COOKIE_PERMISSION);
  } catch {
    return false;
  }
}

// Chrome only honours a permission request from the user-gesture call stack, so
// both checks have to be started before either one is awaited.
export async function grantCookiePermission(): Promise<CookieGrant> {
  const present = checkCookiePermission();
  const requested = requestCookiePermission();
  const [access, granted] = await Promise.all([present, requested]);
  return { granted, newlyGranted: granted && access === "denied" };
}

export async function removeCookiePermission(): Promise<void> {
  await chrome.permissions.remove(COOKIE_PERMISSION);
}
