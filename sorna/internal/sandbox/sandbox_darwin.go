//go:build darwin

package sandbox

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func preparePlatform(command []string, commandPath, _ string, readPaths, writePaths, denyPaths []string, networkRules []NetworkRule, allowedTools []string) (Prepared, error) {
	profile := seatbeltProfile(commandPath, readPaths, writePaths, denyPaths, networkRules, allowedTools)
	wrapped := make([]string, 0, len(command)+3)
	wrapped = append(wrapped, "/usr/bin/sandbox-exec", "-p", profile)
	wrapped = append(wrapped, command...)
	return Prepared{
		Command:     wrapped,
		Backend:     "macos-seatbelt",
		Enforcement: "host-enforced",
	}, nil
}

func seatbeltProfile(commandPath string, readPaths, writePaths, denyPaths []string, networkRules []NetworkRule, allowedTools []string) string {
	var builder strings.Builder
	builder.WriteString("(version 1)\n")
	builder.WriteString("(deny default)\n")
	builder.WriteString("(allow process-fork)\n")
	builder.WriteString("(allow signal (target self))\n")
	// Basic Darwin binaries inspect a small amount of host state and touch
	// Apple's tracing helper while starting. These are bootstrap capabilities,
	// not project filesystem or network access.
	builder.WriteString("(allow sysctl-read)\n")
	writeRule(&builder, "file-write-data", "literal", "/dev/dtracehelper")
	writeRule(&builder, "file-ioctl", "literal", "/dev/dtracehelper")
	// Path traversal on Darwin reads the root directory itself before walking
	// into the explicitly allowed ancestors. This grants directory data for
	// `/` only; it does not grant file contents below the root.
	writeRule(&builder, "file-read-data", "literal", "/")
	writeRule(&builder, "file-read-metadata", "literal", commandPath)
	writeRule(&builder, "file-read*", "literal", commandPath)
	commandDir := filepath.Dir(commandPath)
	writeAncestorTraversalRules(&builder, commandDir)
	writeRule(&builder, "file-read*", "subpath", commandDir)
	// The wrapper may start the requested command, and forked descendants may
	// only exec the command or an explicitly declared tool. This is the host
	// enforcement of the process/tool portion of the policy.
	writeRule(&builder, "process-exec", "literal", commandPath)
	for _, toolPath := range allowedTools {
		writeRule(&builder, "process-exec", "literal", toolPath)
	}
	for _, path := range []string{
		"/usr/lib",
		"/usr/share/locale",
		"/System/Library",
		"/System/Volumes/Preboot/Cryptexes/OS",
		"/dev",
	} {
		writeAncestorTraversalRules(&builder, path)
		writeRule(&builder, "file-read-metadata", "subpath", path)
		writeRule(&builder, "file-read*", "subpath", path)
	}
	for _, path := range readPaths {
		writeAncestorTraversalRules(&builder, path)
		writeRule(&builder, "file-read*", "subpath", path)
	}
	for _, path := range writePaths {
		writeAncestorTraversalRules(&builder, path)
		writeRule(&builder, "file-write*", "subpath", path)
	}
	for _, rule := range networkRules {
		for _, port := range rule.Ports {
			endpoint := fmt.Sprintf("%s:%d", rule.Host, port)
			switch rule.Direction {
			case "inbound":
				writeNetworkRule(&builder, "network-bind", "local", "tcp", endpoint)
				writeNetworkRule(&builder, "network-inbound", "local", "tcp", endpoint)
			case "outbound":
				writeNetworkRule(&builder, "network-outbound", "remote", "tcp", endpoint)
			case "both":
				writeNetworkRule(&builder, "network-bind", "local", "tcp", endpoint)
				writeNetworkRule(&builder, "network-inbound", "local", "tcp", endpoint)
				writeNetworkRule(&builder, "network-outbound", "remote", "tcp", endpoint)
			}
		}
	}
	for _, path := range denyPaths {
		writeDenyRule(&builder, "file-read*", "subpath", path)
		writeDenyRule(&builder, "file-write*", "subpath", path)
	}
	return builder.String()
}

func writeNetworkRule(builder *strings.Builder, operation, relation, protocol, endpoint string) {
	builder.WriteString("(allow ")
	builder.WriteString(operation)
	builder.WriteString(" (")
	builder.WriteString(relation)
	builder.WriteByte(' ')
	builder.WriteString(protocol)
	builder.WriteByte(' ')
	builder.WriteString(seatbeltString(endpoint))
	builder.WriteString("))\n")
}

func writeAncestorTraversalRules(builder *strings.Builder, path string) {
	for current := filepath.Clean(path); current != "."; current = filepath.Dir(current) {
		writeRule(builder, "file-read-metadata", "subpath", current)
		writeRule(builder, "file-read-data", "literal", current)
		if current == string(filepath.Separator) {
			break
		}
	}
}

func writeRule(builder *strings.Builder, operation, filter, path string) {
	writeSeatbeltRule(builder, "allow", operation, filter, path)
}

func writeDenyRule(builder *strings.Builder, operation, filter, path string) {
	writeSeatbeltRule(builder, "deny", operation, filter, path)
}

func writeSeatbeltRule(builder *strings.Builder, decision, operation, filter, path string) {
	builder.WriteByte('(')
	builder.WriteString(decision)
	builder.WriteByte(' ')
	builder.WriteString(operation)
	builder.WriteString(" (")
	builder.WriteString(filter)
	builder.WriteByte(' ')
	builder.WriteString(seatbeltString(path))
	builder.WriteString("))\n")
}

func seatbeltString(value string) string {
	return strconv.Quote(value)
}
