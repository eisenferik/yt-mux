import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { youTubeVideoID } from "../dist/youtube-url.js";

const fixture = JSON.parse(
  readFileSync(new URL("../../testdata/youtube-urls.json", import.meta.url), "utf8"),
);

test("youTubeVideoID reads the video from every accepted YouTube HTTPS URL", () => {
  for (const raw of fixture.allowed) {
    assert.equal(youTubeVideoID(raw), "abcdefghijk", raw);
  }
});

test("youTubeVideoID rejects everything else", () => {
  for (const { url, why } of fixture.rejected) {
    assert.equal(youTubeVideoID(url), undefined, `${url}: ${why}`);
  }
});
