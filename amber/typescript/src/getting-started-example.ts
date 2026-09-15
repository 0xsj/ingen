import { Provenance, ProvenanceContext } from "./index.js";

const root = Provenance.start();
const context = ProvenanceContext.empty().withProvenance(root);
const incoming = context.provenance;

if (!incoming) {
  throw new Error("missing provenance");
}

const child = incoming.child({ origin: "incoming" });
console.log(`root execution: ${root.execution_id}`);
console.log(`child execution: ${child.execution_id}`);
