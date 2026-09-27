import { copyFile, mkdir, readFile } from "node:fs/promises";

const dist = new URL("../dist/", import.meta.url);
const manifest = JSON.parse(await readFile(new URL("../manifest.json", import.meta.url), "utf8"));

await mkdir(dist, { recursive: true });
for (const name of ["manifest.json", "options.html", "options.css", ...new Set(Object.values(manifest.icons))]) {
  const target = new URL(name, dist);
  await mkdir(new URL(".", target), { recursive: true });
  await copyFile(new URL(`../${name}`, import.meta.url), target);
}
