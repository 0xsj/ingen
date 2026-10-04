# Local toolchain bundles

Build and install a local bundle containing the eight module commands, the
`sorna-malcolm` bridge, Amber Go SDK source, and an Amber TypeScript npm tarball.
The builder requires Go, Rust/Cargo, Node/npm, populated Go/Cargo dependency
caches, and existing `amber/typescript/node_modules`. It builds the native
Darwin or Linux amd64/arm64 target only and installs no dependencies.
The selected Rust toolchain must include its `rust-docs` component, which
provides the standard-library copyright and license texts copied into bundles.

```sh
python3 packaging/ingen_package.py build --output-dir /absolute/fresh/bundle \
  --release-version 0.0.0-local-review
python3 packaging/ingen_package.py verify --bundle /absolute/fresh/bundle \
  --manifest-sha256 <digest-printed-by-build> --target darwin/arm64
python3 packaging/ingen_package.py install --bundle /absolute/fresh/bundle \
  --manifest-sha256 <digest-printed-by-build> --target darwin/arm64 \
  --prefix /absolute/empty/user-prefix
```

Keep the selected manifest digest separately. Verification checks its exact
bytes, the target, inventory, file hashes, modes, and SDK package shape.
Installation copies to a fresh or empty prefix and verifies the copy before
returning. Existing files, escaping paths, symlinks, unknown artifacts, and
tampering are rejected. The installed v2 manifest retains toolchain versions,
source revision, and a dirty marker. Its `source_inputs` inventory hashes
selected Go/Rust/TypeScript sources, embedded assets, dependency locks, and
distribution/license files. The builder copies those exact bytes into a
verified private snapshot and builds from it, rejecting source drift. This
fingerprint identifies those selected inputs, including uncommitted changes;
it does not attest the entire checkout, dependency caches, compiler binaries,
or running images. Build settings are recorded; independent build
reproducibility has not been demonstrated.

All nine commands support `--version` and `version --format json`. The JSON
schema is [`ingen.tool-version/v1`](../core/cliversion/spec/tool-version-v1.schema.json).
Reports bind the selected version, revision, source-input digest, dirty state,
compiled platform, and toolchain. Ordinary development builds report unknown
source identity when it was not injected.

The ecosystem uses MIT. Root and Amber licenses accompany the bundle, alongside
selected Go, YAML, and Rust notices. Dependencies retain their own licenses;
these copies do not establish a complete platform license review. The manifest
labels the bundle `local-review-only`. Signing, independent reproducibility,
and clean-machine/platform installation remain release work. Linux binaries do
not provide Linux containment: Sorna still fails closed there.

## Archive distribution

Export an already verified bundle as a tar.gz archive. Two exports of the same
bundle use stable ordering, modes, ownership, and timestamps. This archive
stability does not establish reproducible compilation.

```sh
python3 packaging/ingen_package.py archive --bundle /absolute/fresh/bundle \
  --manifest-sha256 <trusted-manifest-digest> --output /absolute/ingen.tar.gz
python3 /absolute/trusted/ingen-package.py verify-archive \
  --archive /absolute/ingen.tar.gz --manifest-sha256 <trusted-manifest-digest>
python3 /absolute/trusted/ingen-package.py install-archive \
  --archive /absolute/ingen.tar.gz --manifest-sha256 <trusted-manifest-digest> \
  --prefix /absolute/empty/user-prefix
```

The standalone `share/ingen-package.py` requires Python 3.10 or newer and its
standard library. Archive verification and installation execute no bundled
commands and require no Go, Cargo, Node, or checkout. Acquire the helper and
manifest digest through a separately trusted channel, and authenticate the
helper before executing it. A helper copied from an untrusted archive cannot
authenticate itself; a sidecar digest delivered beside that archive is not a
trust anchor. Signature distribution is still pending.

Verification bounds compressed/decompressed sizes and rejects duplicate paths,
links, special files, unsafe modes, and inventory differences. Installation
verifies a private bounded archive snapshot and verifies the final prefix.

The optional clean-room archive check uses an already-present immutable
container image and caller-supplied SHA-256 digests for the image, standalone
helper, archive, and bundle manifest:

```sh
acceptance/archive-cleanroom.sh \
  --image-id sha256:<64-lowercase-hex> \
  --helper /absolute/trusted/ingen-package.py --helper-sha256 <64-hex> \
  --archive /absolute/ingen.tar.gz --archive-sha256 <64-hex> \
  --manifest-sha256 <64-hex> --target darwin/arm64
```

The image must already exist locally; the command does not pull it. The
container disables networking, drops capabilities, uses a read-only root and
non-root UID, and runs the Python standard-library verifier with a PATH
containing only its private `python3` link. It checks archive verification,
installation, exact archive/install bytes, and tamper rejection. It never runs
the bundled native programs, so it is not evidence of Linux program execution
or Linux containment. Trust the image, helper, archive, and manifest digests
through a channel separate from the files being checked.
The target is the archive's declared target, even when the verifier runs on
Linux. The hosted Linux job exercises this Python-only archive path in a
filtered user environment; it does not apply the local Docker restrictions.

## Release evidence inventory audit

`release_evidence.py` reads a caller-selected inventory matching
[`release-evidence-inventory-v1.schema.json`](release-evidence-inventory-v1.schema.json).
The inventory names a canonical absolute evidence root and every gate in the
closed catalog: `bundle_install`, `ecosystem_workflow`,
`signed_file_ingress`, `github_protocol`, `native_governed_workflow`,
`runtime_containment`, `linux_enforcement`,
`host_signed_callbacks_restart`, `live_github_delivery`,
`native_install_matrix`, `hosted_ci`, `distribution_signature_rebuild`,
and `provider_test`. Evidence references are root-relative regular files with
exact SHA-256 digests. Symlinks, duplicate JSON keys, unknown gates, oversized
files, and path traversal are rejected. Bundle evidence must include its
manifest and archive; the trusted local package verifier checks their declared
inventory and target from private snapshots.

```sh
python3 packaging/release_evidence.py audit \
  --inventory /absolute/release-evidence-inventory.json \
  --output /absolute/fresh/release-evidence-audit.json
```

Exit `0` is reserved for an inventory where every required gate contract has
been machine-checked; exit `1` means evidence or required review is incomplete;
exit `2` means malformed input, unsafe paths, or digest/identity failure. In
the current implementation the external gates below always remain review
blockers, so a clean local audit returns `1`. The
four local-review gates check selected fields in recognized local reports:
bundle/archive identity, the synthetic governance fixture and workflow
outcomes, local signer scope, and synthetic GitHub protocol scope. The report
lists exactly which fields it checked; it does not imply that it traversed all
child references in those reports.

Future host, containment, Linux, hosted-CI, and signed-distribution gates are
integrity-checked declarations that remain review-required blockers. Their
`passed` strings cannot establish semantic completion or attestation. The
provider test is recorded as explicitly deferred under `local-review`; it is
not a local-review blocker and is not reported as provider evidence. A ready
local-review summary is only a prompt for human review. The auditor never
grants release approval, signs or publishes a release, executes bundled
programs, or contacts a provider or hosted service.

## Run Sentinel with installed tools

Pass `--tool-dir /absolute/user-prefix/bin` to `sentinel contract create`,
`oracle freeze`, `verify`, `evidence verify`, and `evidence gate`. This selects
those binaries directly; `--ingen-root` and `--malcolm-manifest` are incompatible
with installed mode. Review the oracle policy's declared tools before sealing
it: replace the scaffold's development `go` grant with the selected installed
Sorna executable, or remove unused tool grants. Sentinel does not change the
policy when selecting installed tools.

```sh
/absolute/user-prefix/bin/sentinel contract create \
  --root /absolute/project --tool-dir /absolute/user-prefix/bin
```

For Amber Go consumers, point a local Go module replacement at
`<prefix>/sdk/amber-go`. Install the TypeScript tarball under
`<prefix>/sdk/amber-typescript` with the consumer's package manager.

`make ingen-package-check` exercises verification/installation failures.
`make ingen-install-check` builds a fresh bundle, installs it under an owned
temporary prefix through the standalone archive helper, checks all installed
version reports against the manifest and public schema, then runs synthetic
HTTP verification from an unrelated directory with Go and Cargo absent from
PATH. The latter requires macOS
containment and records its proof directories. It establishes those installed
handoffs without providing operator approval or native Herdr acceptance.
