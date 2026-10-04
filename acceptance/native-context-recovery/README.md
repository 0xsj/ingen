# Native context and recovery fixtures

Run `../native-context-recovery.sh` on macOS for the offline contained-profile
check. It creates a fresh project under `/private/tmp/ingen-native-context.*`,
seeds prior context files, starts a live loopback control listener, and runs the
typed profile with an offline Codex-compatible executable. The fixture verifies
the fixed argument vector, exact prompt bytes, fresh private context paths,
credential filtering, denied prior-context reads, and denied loopback access.
It then verifies and imports the role report, retrieves the prompt snapshot from
Lockwood, and compares the recovered bytes exactly. The script also runs the
native-session package tests for cancellation and execution-lease recovery.

The default run does not connect to Herdr or invoke Codex. For an explicitly
requested live host check, run it inside an existing managed terminal with
`HERDR_ENV=1` and pass `--live --socket /canonical/path/to/herdr.sock`. It
creates fresh, script-owned background workspaces, exercises active recovery,
duplicate execution refusal, pane process observation, graceful and ignored
signal cancellation, evidence collection, and exact owned-workspace cleanup.
It does not start, stop, or upgrade Herdr. Leave any live host result labeled
as a local unverified observation.

`codex-fixture/` builds an offline executable fixture for Sentinel's typed
Codex CLI profile. It is not Codex and does not contact a model provider. It
checks the pinned argv and stdin prompt, verifies fresh private context paths,
and attempts to read deliberately denied prior-context files. The separate
`loopback_receiver.py` supplies a positive connectivity control before the
contained fixture attempts the same endpoint.
