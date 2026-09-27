import { checkCookiePermission } from "./permissions.js";
import type { CookieAccess } from "./permissions.js";
import { NativeCookie } from "./protocol.js";

export interface CookieCollection {
  access: CookieAccess;
  cookies: NativeCookie[];
}

export async function collectCookies(): Promise<CookieCollection> {
  const access = await checkCookiePermission();
  if (access !== "granted") {
    return { access, cookies: [] };
  }
  let stored: chrome.cookies.Cookie[][];
  try {
    stored = await Promise.all([
      chrome.cookies.getAll({ domain: "youtube.com" }),
      chrome.cookies.getAll({ domain: "google.com" }),
    ]);
  } catch {
    return { access: "unreadable", cookies: [] };
  }
  const unique = new Map<string, NativeCookie>();
  for (const cookie of stored.flat()) {
    const converted: NativeCookie = {
      domain: cookie.domain,
      name: cookie.name,
      value: cookie.value,
      path: cookie.path,
      secure: cookie.secure,
    };
    if (cookie.httpOnly) {
      converted.httpOnly = true;
    }
    if (cookie.expirationDate !== undefined) {
      converted.expirationDate = cookie.expirationDate;
    }
    unique.set(`${cookie.domain}\0${cookie.path}\0${cookie.name}`, converted);
  }
  return { access: "granted", cookies: [...unique.values()] };
}
