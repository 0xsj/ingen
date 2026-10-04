// Package execution exposes Sorna's existing host-enforced command preparation
// boundary to other InGen verticals.
//
// On macOS, Prepare wraps the requested command in Seatbelt and applies the
// sealed policy's filesystem roots, declared tools, and TCP network rules. The
// small Darwin runtime paths needed by ordinary binaries remain available.
// Unsupported operating systems fail closed because no enforcement backend is
// available. Prepared describes the policy and executable identity used to
// create the wrapper; it does not authenticate the caller, attest the host, or
// prove that the process and its descendants obeyed every policy rule.
package execution

import (
	"ingen/sorna/internal/sandbox"
	"ingen/sorna/policy"
)

// Prepared is the command and normalized enforcement metadata returned by
// Prepare. Its fields are aliases of Sorna's internal sandbox representation.
type Prepared = sandbox.Prepared

// Prepare resolves the command and policy paths and returns a command wrapped
// by the platform's host enforcement backend. Callers run the returned
// Prepared.Command with their chosen working directory, environment, and
// standard streams.
func Prepare(command []string, root string, sealed policy.Sealed) (Prepared, error) {
	return sandbox.Prepare(command, root, sealed)
}

// PathCovered reports whether target falls within one of the supplied policy
// capability roots using the same canonicalization behavior as Prepare.
func PathCovered(roots []string, target string) bool {
	return sandbox.PathCovered(roots, target)
}
