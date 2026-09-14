#!/bin/sh
set -eu

command_name=${1:-gate}
paddock=${PADDOCK:-paddock}
policy=${PADDOCK_POLICY:-paddock.yaml}
proposed_policy=${PADDOCK_PROPOSED_POLICY:-}
lock=${PADDOCK_LOCK:-paddock.lock.json}
source_root=${PADDOCK_SOURCE_ROOT:-.}
graph_input=${PADDOCK_GRAPH:-}
diff_output=${PADDOCK_DIFF:-paddock-policy-diff.json}
result_output=${PADDOCK_RESULT:-paddock-ci-result.json}

case "$command_name" in
review)
	if [ -z "$proposed_policy" ]; then
		echo "PADDOCK_PROPOSED_POLICY is required for review" >&2
		exit 2
	fi
	"$paddock" policy diff \
		--before "$policy" \
		--after "$proposed_policy" \
		--format json >"$diff_output"
	;;
seal)
	"$paddock" policy seal \
		--input "$policy" \
		--output "$lock"
	;;
verify)
	"$paddock" policy verify \
		--policy "$policy" \
		--lock "$lock"
	;;
gate)
	if [ -n "$graph_input" ]; then
		"$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--graph "$graph_input" \
			--output "$result_output"
	else
		"$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--output "$result_output"
	fi
	;;
*)
	echo "usage: $0 review|seal|verify|gate" >&2
	exit 2
	;;
esac
