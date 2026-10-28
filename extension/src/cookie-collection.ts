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
  const stored = await Promise.all([
    chrome.cookies.getAll({ domain: "youtube.com" }),
    chrome.cookies.getAll({ domain: "google.com" }),
  ]);
  const cookies = stored.flat().map((cookie) => {
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
    return converted;
  });
  return { access: "granted", cookies };
}
