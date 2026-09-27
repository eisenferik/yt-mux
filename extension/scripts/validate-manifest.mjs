import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { COOKIE_PERMISSION } from "../dist/permissions.js";

const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));
assert.equal(manifest.manifest_version, 3);
assert.equal(manifest.background.type, "module");
assert.equal(manifest.content_scripts, undefined, "content scripts are forbidden");
assert.equal(manifest.host_permissions, undefined, "ordinary host permissions are forbidden");
assert.deepEqual(manifest.optional_permissions, ["cookies"]);
assert.deepEqual(manifest.optional_permissions, COOKIE_PERMISSION.permissions);
assert.deepEqual(manifest.optional_host_permissions, COOKIE_PERMISSION.origins);
assert.ok(!manifest.permissions.includes("cookies"), "cookie access must stay removable");
assert.ok(manifest.permissions.includes("nativeMessaging"));
assert.ok(manifest.permissions.includes("activeTab"));
assert.ok(!JSON.stringify(manifest).includes("<all_urls>"));

process.stdout.write("Manifest security policy validated.\n");
