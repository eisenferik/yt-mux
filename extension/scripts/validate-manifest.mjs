import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { COOKIE_PERMISSION } from "../dist/permissions.js";
import { EXTENSION_VERSION } from "../dist/protocol.js";

const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));
assert.deepEqual(manifest.optional_permissions, COOKIE_PERMISSION.permissions);
assert.deepEqual(manifest.optional_host_permissions, COOKIE_PERMISSION.origins);
assert.equal(EXTENSION_VERSION, manifest.version);

process.stdout.write("Cookie permission and version match the manifest.\n");
