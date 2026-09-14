#!/bin/sh
set -eu

# A pass-through adapter fixture for protocol and CI smoke tests. It is not a
# language parser: the first argument is an already prepared graph document.
fixture=${1:-}
if [ -z "$fixture" ]; then
	echo "usage: $0 <paddock-graph.json>" >&2
	exit 2
fi
if [ ! -f "$fixture" ]; then
	echo "graph fixture does not exist: $fixture" >&2
	exit 2
fi

# Consume the request so this behaves like a real stdin/stdout adapter.
request=$(cat)
if [ -z "$request" ]; then
	echo "adapter request is empty" >&2
	exit 2
fi

cat "$fixture"
