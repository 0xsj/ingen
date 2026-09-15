import { readFile } from "node:fs/promises";

const expected = JSON.parse(
  await readFile(new URL("./public-api.json", import.meta.url), "utf8"),
);
const actual = Object.keys(await import("../dist/index.js")).sort();

if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  throw new Error(
    `public API changed unexpectedly:\nactual: ${JSON.stringify(actual)}\nexpected: ${JSON.stringify(expected)}`,
  );
}

console.log("TypeScript public API manifest passed");
