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
adapter_config=${PADDOCK_ADAPTER_CONFIG:-}
adapter_profile_sha256=${PADDOCK_ADAPTER_PROFILE_SHA256:-}
adapter_args_file=${PADDOCK_ADAPTER_ARGS_FILE:-}
diff_output=${PADDOCK_DIFF:-paddock-policy-diff.json}
policy_cases=${PADDOCK_CASES:-}
review_output=${PADDOCK_REVIEW:-paddock-policy-review.json}
result_output=${PADDOCK_RESULT:-paddock-ci-result.json}
explanation_output=${PADDOCK_EXPLANATION_OUTPUT:-}
explanation_format=${PADDOCK_EXPLANATION_FORMAT:-text}
explanation_rule=${PADDOCK_EXPLANATION_RULE:-}
explanation_status=${PADDOCK_EXPLANATION_STATUS:-}
adapter_tests=${PADDOCK_ADAPTER_TESTS:-}
adapter_test_result=${PADDOCK_ADAPTER_TEST_RESULT:-paddock-adapter-test-result.json}
adapter_ci_result=${PADDOCK_ADAPTER_CI_RESULT:-}

if [ -n "$graph_input" ] && { [ -n "$adapter" ] || [ -n "$adapter_config" ]; }; then
	echo "PADDOCK_GRAPH cannot be combined with an external adapter or adapter profile" >&2
	exit 2
fi
if [ -n "$adapter_args_file" ] && [ -z "$adapter" ]; then
	echo "PADDOCK_ADAPTER_ARGS_FILE requires PADDOCK_ADAPTER" >&2
	exit 2
fi
if [ -n "$adapter_config" ] && { [ -n "$adapter" ] || [ -n "$adapter_args_file" ]; }; then
	echo "PADDOCK_ADAPTER_CONFIG cannot be combined with PADDOCK_ADAPTER or PADDOCK_ADAPTER_ARGS_FILE" >&2
	exit 2
fi
if [ -n "$adapter_profile_sha256" ] && [ -z "$adapter_config" ]; then
	echo "PADDOCK_ADAPTER_PROFILE_SHA256 requires PADDOCK_ADAPTER_CONFIG" >&2
	exit 2
fi
if [ -n "$adapter_args_file" ] && [ ! -f "$adapter_args_file" ]; then
	echo "adapter args file does not exist: $adapter_args_file" >&2
	exit 2
fi
if [ -n "$adapter_profile_sha256" ]; then
	"$paddock" adapter profile verify \
		--input "$adapter_config" \
		--expected-sha256 "$adapter_profile_sha256"
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
		if [ -n "$adapter_config" ]; then
			set -- "$@" --adapter-config "$adapter_config"
		elif [ -n "$adapter" ]; then
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
adapter-test)
	if [ -z "$adapter_tests" ]; then
		echo "PADDOCK_ADAPTER_TESTS is required for adapter-test" >&2
		exit 2
	fi
	adapter_test_status=0
	set -- "$paddock" adapter test \
		--cases "$adapter_tests" \
		--output "$adapter_test_result"
	if [ -n "$adapter_ci_result" ]; then
		set -- "$@" --ci-result "$adapter_ci_result"
	fi
	"$@" || adapter_test_status=$?
	if [ "$adapter_test_status" -eq 0 ] || [ "$adapter_test_status" -eq 1 ]; then
		"$paddock" adapter test verify \
			--input "$adapter_test_result" \
			--files
		if [ -n "$adapter_ci_result" ]; then
			"$paddock" ci validate --input "$adapter_ci_result"
		fi
	fi
	exit "$adapter_test_status"
;;
gate)
	if [ -n "$graph_input" ]; then
		"$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--graph "$graph_input" \
			--output "$result_output"
	elif [ -n "$adapter" ] || [ -n "$adapter_config" ]; then
		set -- "$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--graph-output "$graph_output" \
			--output "$result_output"
		if [ -n "$adapter_config" ]; then
			set -- "$@" --adapter-config "$adapter_config"
		else
			set -- "$@" --adapter "$adapter"
			if [ -n "$adapter_args_file" ]; then
				while IFS= read -r adapter_arg || [ -n "$adapter_arg" ]; do
					if [ -n "$adapter_arg" ]; then
						set -- "$@" --adapter-arg "$adapter_arg"
					fi
				done < "$adapter_args_file"
			fi
		fi
		"$@"
	else
		"$paddock" ci "$source_root" \
			--policy-lock "$lock" \
			--output "$result_output"
	fi
	;;
handoff)
	# Keep the CI artifact and the explanation as separate machine-readable
	# products. The gate's human output is suppressed so JSON explanation mode
	# remains safe to pipe to another agent or tool.
	handoff_status=0
	sh "$0" gate > /dev/null || handoff_status=$?
	if [ "$handoff_status" -eq 0 ] || [ "$handoff_status" -eq 1 ]; then
		set -- "$paddock" explain "$result_output" --format "$explanation_format"
		if [ -n "$explanation_rule" ]; then
			set -- "$@" --rule "$explanation_rule"
		fi
		if [ -n "$explanation_status" ]; then
			set -- "$@" --status "$explanation_status"
		fi
		explanation_status_code=0
		if [ -n "$explanation_output" ]; then
			"$@" > "$explanation_output" || explanation_status_code=$?
		else
			"$@" || explanation_status_code=$?
		fi
		if [ "$explanation_status_code" -ne 0 ]; then
			exit "$explanation_status_code"
		fi
	fi
	exit "$handoff_status"
	;;
*)
	echo "usage: $0 review|seal|verify|adapter-test|gate|handoff" >&2
	exit 2
	;;
esac
