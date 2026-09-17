#!/bin/sh
set -eu

: "${HERDR_PLUGIN_STATE_DIR:?HERDR_PLUGIN_STATE_DIR is required}"

state_dir="$HERDR_PLUGIN_STATE_DIR/events"
mkdir -p "$state_dir"
capture_dir="$(mktemp -d "$state_dir/capture.XXXXXX")"

printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$capture_dir/captured_at"
printf '%s\n' "${HERDR_PLUGIN_EVENT-}" > "$capture_dir/event"
printf '%s\n' "${HERDR_WORKSPACE_ID-}" > "$capture_dir/workspace_id"
printf '%s\n' "${HERDR_TAB_ID-}" > "$capture_dir/tab_id"
printf '%s\n' "${HERDR_PANE_ID-}" > "$capture_dir/pane_id"
printf '%s\n' "${HERDR_PLUGIN_CONTEXT_JSON-}" > "$capture_dir/context.json"
printf '%s\n' "${HERDR_PLUGIN_EVENT_JSON-}" > "$capture_dir/event.json"
printf 'capture_dir=%s\n' "$capture_dir"
