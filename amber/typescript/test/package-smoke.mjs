import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const tarball = resolve(process.argv[2] ?? "");
if (!tarball) {
  throw new Error("package smoke test requires a tarball path");
}

const directory = mkdtempSync(join(tmpdir(), "amber-package-smoke-"));
const smokeFile = join(directory, "smoke.mjs");
writeFileSync(
  smokeFile,
  `import {
  AmberValidationError,
  Provenance,
  decodeValue,
  encodeValue,
} from "@0xsj/amber";

const root = Provenance.start();
const encoded = encodeValue(root);
const decoded = decodeValue(encoded, "reject");
if (!decoded.present || decoded.provenance.execution_id !== root.execution_id) {
  throw new Error("installed package failed the public transport round trip");
}
if (!(AmberValidationError.prototype instanceof Error)) {
  throw new Error("installed package did not expose its public error type");
}
console.log("TypeScript package smoke test passed");
`,
);

try {
  execFileSync("npm", ["init", "--yes"], { cwd: directory, stdio: "ignore" });
  execFileSync("npm", ["install", "--ignore-scripts", "--no-save", tarball], {
    cwd: directory,
    stdio: "inherit",
  });
  execFileSync(process.execPath, [smokeFile], {
    cwd: directory,
    stdio: "inherit",
  });
} finally {
  rmSync(directory, { recursive: true, force: true });
}
