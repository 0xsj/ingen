import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const tarball = resolve(process.argv[2] ?? "");
if (!tarball) {
  throw new Error("package smoke test requires a tarball path");
}

const directory = mkdtempSync(join(tmpdir(), "amber-package-smoke-"));
const smokeFile = join(directory, "smoke.mjs");
const expectedPublicApi = JSON.parse(
  readFileSync(new URL("./public-api.json", import.meta.url), "utf8"),
);
writeFileSync(
  smokeFile,
  `const expectedPublicApi = ${JSON.stringify(expectedPublicApi)};
const installedApi = await import("@0xsj/amber");
const installedTestingApi = await import("@0xsj/amber/testing");
if (JSON.stringify(Object.keys(installedApi).sort()) !== JSON.stringify(expectedPublicApi)) {
  throw new Error("installed package public API does not match the manifest");
}

const {
  AmberValidationError,
  Provenance,
  decodeValue,
  encodeValue,
  setProvenanceAttributes,
} = installedApi;

const root = Provenance.start();
const encoded = encodeValue(root);
const decoded = decodeValue(encoded, "reject");
if (!decoded.present || decoded.provenance.execution_id !== root.execution_id) {
  throw new Error("installed package failed the public transport round trip");
}
if (!(AmberValidationError.prototype instanceof Error)) {
  throw new Error("installed package did not expose its public error type");
}
const attributes = {};
setProvenanceAttributes({ setAttributes: (value) => Object.assign(attributes, value) }, root);
if (attributes["amber.execution_id"] !== root.execution_id) {
  throw new Error("installed package did not expose its OpenTelemetry adapter");
}
if (typeof installedTestingApi.runProvenanceStoreContract !== "function") {
  throw new Error("installed package did not expose its testing subpath");
}
await installedTestingApi.runProvenanceStoreContract(new installedApi.MemoryStore());
console.log("TypeScript package smoke test passed");
`,
);

try {
  execFileSync("npm", ["init", "--yes"], { cwd: directory, stdio: "ignore" });
  execFileSync("npm", ["install", "--ignore-scripts", "--no-save", tarball], {
    cwd: directory,
    stdio: "inherit",
  });
  const installedRoot = join(directory, "node_modules", "@0xsj", "amber");
  const installedPackage = JSON.parse(readFileSync(join(installedRoot, "package.json"), "utf8"));
  if (installedPackage.license !== "MIT") {
    throw new Error("installed package did not preserve MIT license metadata");
  }
  if (!existsSync(join(installedRoot, "LICENSE"))) {
    throw new Error("installed package did not include LICENSE");
  }
  if (!existsSync(join(installedRoot, "README.md"))) {
    throw new Error("installed package did not include README.md");
  }
  execFileSync(process.execPath, [smokeFile], {
    cwd: directory,
    stdio: "inherit",
  });
} finally {
  rmSync(directory, { recursive: true, force: true });
}
