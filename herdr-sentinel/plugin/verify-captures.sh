#!/bin/sh
set -eu

state_dir=${1:?usage: verify-captures.sh <plugin-state-directory>}
events_dir="$state_dir/events"

if [ ! -d "$events_dir" ]; then

    for capture_dir in "$state_dir"/capture.*; do

        if [ -d "$capture_dir" ]; then

            events_dir="$state_dir"
            break
        fi
    done
fi

if [ ! -d "$events_dir" ]; then

    printf '%s\n' "missing events directory: $events_dir" >&2
    exit 1
fi

capture_count=0
identity_state=absent
timestamp_state=absent

for capture_dir in "$events_dir"/capture.*; do

    [ -d "$capture_dir" ] || continue
    capture_count=$((capture_count + 1))

    for required_file in captured_at event event.json; do

        if [ ! -s "$capture_dir/$required_file" ]; then

            printf '%s\n' "missing capture file: $capture_dir/$required_file" >&2
            exit 1
        fi
    done

    if ! jq -e 'type == "object" and (.event | type) == "string" and (.data | type) == "object"' \
        "$capture_dir/event.json" >/dev/null; then

        printf '%s\n' "invalid Herdr event envelope: $capture_dir/event.json" >&2
        exit 1
    fi

    capture_identity=$(jq -r '
        if has("event_id") or (.data | has("event_id"))
        then "present"
        else "absent"
        end
    ' "$capture_dir/event.json")
    capture_timestamp=$(jq -r '
        if has("timestamp") or has("at") or
           (.data | (has("timestamp") or has("at")))
        then "present"
        else "absent"
        end
    ' "$capture_dir/event.json")

    if [ "$capture_identity" = present ]; then

        identity_state=present
    fi
    if [ "$capture_timestamp" = present ]; then

        timestamp_state=present
    fi
done

if [ "$capture_count" -eq 0 ]; then

    printf '%s\n' "no capture directories found under: $events_dir" >&2
    exit 1
fi

printf 'capture_count=%s\n' "$capture_count"
printf 'raw_event_envelope=valid\n'
printf 'probe_observed_at=present\n'
printf 'host_event_identity=%s\n' "$identity_state"
printf 'host_event_timestamp=%s\n' "$timestamp_state"

if [ "$identity_state" = absent ] || [ "$timestamp_state" = absent ]; then

    printf 'sentinel_binding=blocked_missing_host_contract\n'
else

    printf 'sentinel_binding=eligible_for_contract_review\n'
fi
