const videoPathNames = new Set(["shorts", "live", "embed", "v", "clip"]);

export function youTubeVideoID(raw: string | undefined): string | undefined {
  if (!raw || raw.length > 8192) {
    return undefined;
  }
  try {
    const url = new URL(raw);
    if (
      url.protocol !== "https:" ||
      (url.port !== "" && url.port !== "443") ||
      url.username !== "" ||
      url.password !== ""
    ) {
      return undefined;
    }
    const host = url.hostname.toLowerCase().replace(/\.$/, "");
    if (host !== "youtube.com" && !host.endsWith(".youtube.com") && host !== "youtu.be") {
      return undefined;
    }
    return videoIDFromPath(url, host);
  } catch {
    return undefined;
  }
}

function videoIDFromPath(url: URL, host: string): string | undefined {
  const segments = url.pathname.replace(/^\/+|\/+$/g, "").split("/");
  const first = segments[0] ?? "";
  if (host === "youtu.be") {
    return segments.length === 1 && first !== "" ? first : undefined;
  }
  if (first === "watch") {
    const id = url.searchParams.get("v") ?? "";
    return segments.length === 1 && id !== "" ? id : undefined;
  }
  const second = segments[1] ?? "";
  return segments.length === 2 && videoPathNames.has(first) && second !== "" ? second : undefined;
}
