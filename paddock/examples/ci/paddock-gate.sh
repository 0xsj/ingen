#!/bin/sh
set -eu

command_name=${1:-gate}
paddock=${PADDOCK:-paddock}
policy=${PADDOCK_POLICY:-paddock.yaml}
proposed_policy=${PADDOCK_PROPOSED_POLICY:-}
lock=${PADDOCK_LOCK:-paddock.lock.json}
source_root=${PADDOCK_SOURCE_ROOT:-.}
graph_input=${PADDOCK_GRAPH:-}
graph_output=${PADDOCK_GRAPH_OUTPUT:-paddock-graph.json}
adapter=${PADDOCK_ADAPTER:-}
adapter_args_file=${PADDOCK_ADAPTER_ARGS_FILE:-}
diff_output=${PADDOCK_DIFF:-paddock-policy-diff.json}
policy_cases=${PADDOCK_CASES:-}
review_output=${PADDOCK_REVIEW:-paddock-policy-review.json}
result_output=${PADDOCK_RESULT:-paddock-ci-result.json}

if [ -n "$graph_input" ] && [ -n "$adapter" ]; then
	echo "PADDOCK_GRAPH and PADDOCK_ADAPTER cannot both be set" >&2
	exit 2
fi
if [ -n "$adapter_args_file" ] && [ -z "$adapter" ]; then
	echo "PADDOCK_ADAPTER_ARGS_FILE requires PADDOCK_ADAPTER" >&2
	exit 2
fi
if [ -n "$adapter_args_file" ] && [ ! -f "$adapter_args_file" ]; then
	echo "adapter args file does not exist: $adapter_args_file" >&2
	exit 2
fi

case "$command_name" in
review)
	if [ -z "$proposed_policy" ]; then
		echo "PADDOCK_PROPOSED_POLICY is required for review" >&2
		exit 2
	fi
	if [ -n "$policy_cases" ]; then
		review_status=0
		set -- "$paddock" policy review \
			--before "$policy" \
			--after "$proposed_policy" \
			--cases "$policy_cases" \
			--output "$review_output"
		if [ -n "$adapter" ]; then
			set -- "$@" --adapter "$adapter"
			if [ -n "$adapter_args_file" ]; then
				while IFS= read -r adapter_arg || [ -n "$adapter_arg" ]; do
					if [ -n "$adapter_arg" ]; then
						set -- "$@" --adapter-arg "$adapter_arg"
					fi
				done < "$adapter_args_file"
			fi
		fi
		"$@" || review_status=$?
		if [ "$review_status" -eq 0 ] || [ "$review_status" -eq 1 ]; then
			"$paddock" policy review verify --input "$review_output"
		fi
		exit "$review_status"
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
	elif [ -n "$adapter" ]; then
		set -- "$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--adapter "$adapter"
		if [ -n "$adapter_args_file" ]; then
			while IFS= read -r adapter_arg || [ -n "$adapter_arg" ]; do
				if [ -n "$adapter_arg" ]; then
					set -- "$@" --adapter-arg "$adapter_arg"
				fi
			done < "$adapter_args_file"
		fi
		set -- "$@" --graph-output "$graph_output" --output "$result_output"
		"$@"
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
