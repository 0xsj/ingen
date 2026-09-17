import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import { buildProviderManifest } from "../src/provider.ts";

function usage() {
  console.error("usage: bun run emit-provider.mjs --plan <path> --output <path> --id <id> --command <path> [--arg <value> ...]");
}

function parseArgs(argv) {
  const values = { args: [] };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (flag === "--arg") {
      if (value === undefined) throw new Error("--arg requires a value");
      values.args.push(value);
      index += 1;
    } else if (flag === "--plan" || flag === "--output" || flag === "--id" || flag === "--command" || flag === "--subject-root" || flag === "--subject-dir") {
      if (value === undefined) throw new Error(`${flag} requires a value`);
      values[flag.slice(2).replaceAll("-", "_")] = value;
      index += 1;
    } else {
      throw new Error(`unknown argument ${flag}`);
    }
  }
  if (!values.plan || !values.output || !values.id || !values.command) {
    usage();
    throw new Error("--plan, --output, --id, and --command are required");
  }
  return values;
}

try {
  const values = parseArgs(process.argv.slice(2));
  const planBytes = await readFile(values.plan);
  const envelope = await buildProviderManifest(planBytes, {
    id: values.id,
    command: values.command,
    args: values.args,
    ...(values.subject_root ? { subjectRoot: values.subject_root } : {}),
    ...(values.subject_dir ? { subjectDir: values.subject_dir } : {}),
  });
  await mkdir(dirname(values.output), { recursive: true });
  await writeFile(values.output, `${JSON.stringify(envelope, null, 2)}\n`, "utf8");
  console.log(`provider: ${values.output}`);
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
