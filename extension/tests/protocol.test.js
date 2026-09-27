import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { PRESET_NAMES } from "../dist/protocol.js";

test("the preset names are the ones the options page offers", () => {
  const page = readFileSync(new URL("../options.html", import.meta.url), "utf8");
  const offered = [...page.matchAll(/<option value="([^"]*)"/g)].map((match) => match[1]);
  assert.deepEqual([...PRESET_NAMES].sort(), [...offered].sort());
});
