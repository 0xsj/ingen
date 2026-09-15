import { readFile } from "node:fs/promises";

const readJson = async (url) => JSON.parse(await readFile(url, "utf8"));
const expected = (await readFile(new URL("../../VERSION", import.meta.url), "utf8")).trim();
const packageJson = await readJson(new URL("../package.json", import.meta.url));
const lockfile = await readJson(new URL("../package-lock.json", import.meta.url));
const lockRoot = lockfile.packages?.[""];

if (!expected || packageJson.version !== expected || lockfile.lockfileVersion !== 3 || lockRoot?.version !== expected) {
  throw new Error(
    `release version mismatch: VERSION=${expected}, package=${packageJson.version}, lock=${lockRoot?.version}`,
  );
}

console.log(`Release version metadata passed: ${expected}`);
