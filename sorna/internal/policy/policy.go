// Package policy loads, validates, canonicalizes, and seals Sorna capability
// policies.
package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var allowedStatuses = map[string]bool{
	"draft":      true,
	"sealed":     true,
	"superseded": true,
}

var allowedEnforcement = map[string]bool{
	"declared-only":       true,
	"host-enforced":       true,
	"externally-attested": true,
}

var allowedNetworkModes = map[string]bool{
	"disabled":     true,
	"allowlist":    true,
	"unrestricted": true,
}

const Schema = "ingen.policy/v1"

// Document is the machine-readable capability policy. The body remains
// map-backed so policy versions can add controls without changing Go types.
type Document struct {
	Policy map[string]any
}

// Sealed is the immutable canonical policy artifact used by evidence.
type Sealed struct {
	Document      Document
	CanonicalJSON []byte
	SHA256        string
}

// Reference identifies a sealed policy without exposing its full contents.
type Reference struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
}

// Reference returns the stable identity of a sealed policy.
func (sealed Sealed) Reference() Reference {
	id, _ := sealed.Document.Policy["id"].(string)
	version, _ := integer(sealed.Document.Policy["version"])
	return Reference{ID: id, Version: version, SHA256: sealed.SHA256}
}

// SubjectID returns the stable logical identity of the system under test that
// this role's process policy refers to.
func SubjectID(document Document) (string, error) {
	process, ok := document.Policy["process"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("policy.process must be an object")
	}
	identity, ok := process["subject_id"].(string)
	if !ok || strings.TrimSpace(identity) == "" {
		return "", fmt.Errorf("policy.process.subject_id must be a non-empty string")
	}
	return identity, nil
}

// ValidationError contains every structural policy problem found.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	var builder strings.Builder
	builder.WriteString("invalid policy:")
	for _, problem := range e.Problems {
		builder.WriteString("\n- ")
		builder.WriteString(problem)
	}
	return builder.String()
}

// LoadFile loads a JSON-compatible YAML policy and validates its shape.
func LoadFile(path string) (Document, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return Document{}, fmt.Errorf("policy file must use .json, .yaml, or .yml")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	value, err := decode(contents)
	if err != nil {
		return Document{}, fmt.Errorf("parse %s: %w", path, err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return Document{}, &ValidationError{Problems: []string{"document must be an object containing a policy"}}
	}
	body, ok := root["policy"].(map[string]any)
	if !ok {
		return Document{}, &ValidationError{Problems: []string{"document.policy must be an object"}}
	}
	document := Document{Policy: body}
	if problems := Validate(document); len(problems) > 0 {
		return Document{}, &ValidationError{Problems: problems}
	}
	return document, nil
}

// Validate returns all structural errors in the v1 policy shape.
func Validate(document Document) []string {
	problems := make([]string, 0)
	if document.Policy == nil {
		return []string{"document.policy must be an object"}
	}
	body := document.Policy
	for _, field := range []string{"schema", "id", "version", "status", "purpose", "enforcement", "filesystem", "network", "process"} {
		if _, ok := body[field]; !ok {
			problems = append(problems, "policy."+field+" is required")
		}
	}
	if value, ok := body["schema"]; ok {
		schema, valid := value.(string)
		if !valid || schema != Schema {
			problems = append(problems, "policy.schema must be ingen.policy/v1")
		}
	}
	if value, ok := body["id"]; ok && !nonEmptyString(value) {
		problems = append(problems, "policy.id must be a non-empty string")
	}
	if value, ok := body["version"]; ok {
		version, valid := integer(value)
		if !valid || version < 1 {
			problems = append(problems, "policy.version must be a positive integer")
		}
	}
	if value, ok := body["status"]; ok {
		status, valid := value.(string)
		if !valid || !allowedStatuses[status] {
			problems = append(problems, "policy.status must be draft, sealed, or superseded")
		}
	}
	if value, ok := body["purpose"]; ok && !nonEmptyString(value) {
		problems = append(problems, "policy.purpose must be a non-empty string")
	}
	if value, ok := body["enforcement"]; ok {
		enforcement, valid := value.(string)
		if !valid || !allowedEnforcement[enforcement] {
			problems = append(problems, "policy.enforcement must be declared-only, host-enforced, or externally-attested")
		}
	}
	validateFilesystem(body["filesystem"], &problems)
	validateNetwork(body["network"], &problems)
	validateProcess(body["process"], &problems)
	return problems
}

// CanonicalJSON returns deterministic JSON bytes for a valid policy.
func CanonicalJSON(document Document) ([]byte, error) {
	if problems := Validate(document); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	root := map[string]any{"policy": document.Policy}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("canonicalize policy: %w", err)
	}
	return buffer.Bytes(), nil
}

// Seal canonicalizes a draft policy and returns its content hash without
// mutating the input document.
func Seal(document Document) (Sealed, error) {
	if problems := Validate(document); len(problems) > 0 {
		return Sealed{}, &ValidationError{Problems: problems}
	}
	status, _ := document.Policy["status"].(string)
	if status != "draft" {
		return Sealed{}, &ValidationError{Problems: []string{fmt.Sprintf("only draft policies can be sealed, got %q", status)}}
	}
	copyDocument, err := clone(document)
	if err != nil {
		return Sealed{}, err
	}
	copyDocument.Policy["status"] = "sealed"
	canonical, err := CanonicalJSON(copyDocument)
	if err != nil {
		return Sealed{}, err
	}
	digest := sha256.Sum256(canonical)
	return Sealed{
		Document:      copyDocument,
		CanonicalJSON: canonical,
		SHA256:        hex.EncodeToString(digest[:]),
	}, nil
}

// SealFile writes canonical.json and hash.txt for a draft policy.
func SealFile(path, outputDir string) (Sealed, error) {
	document, err := LoadFile(path)
	if err != nil {
		return Sealed{}, err
	}
	sealed, err := Seal(document)
	if err != nil {
		return Sealed{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Sealed{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "canonical.json"), sealed.CanonicalJSON, 0o644); err != nil {
		return Sealed{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "hash.txt"), []byte(sealed.SHA256+"\n"), 0o644); err != nil {
		return Sealed{}, err
	}
	return sealed, nil
}

func validateFilesystem(value any, problems *[]string) {
	body, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, "policy.filesystem must be an object")
		return
	}
	seen := make(map[string]string)
	for _, category := range []string{"read", "write", "deny"} {
		rawEntries, present := body[category]
		if !present {
			*problems = append(*problems, "policy.filesystem."+category+" is required")
			continue
		}
		entries, valid := rawEntries.([]any)
		if !valid {
			*problems = append(*problems, "policy.filesystem."+category+" must be a list")
			continue
		}
		for index, rawEntry := range entries {
			path := fmt.Sprintf("policy.filesystem.%s[%d]", category, index)
			entry, valid := rawEntry.(map[string]any)
			if !valid {
				*problems = append(*problems, path+" must be an object")
				continue
			}
			entryPath, validPath := entry["path"].(string)
			if !validPath || strings.TrimSpace(entryPath) == "" {
				*problems = append(*problems, path+".path must be a non-empty string")
				continue
			}
			if !nonEmptyString(entry["reason"]) {
				*problems = append(*problems, path+".reason must be a non-empty string")
			}
			if previous, exists := seen[entryPath]; exists {
				*problems = append(*problems, path+".path overlaps "+previous)
			} else {
				seen[entryPath] = path + ".path"
			}
		}
	}
}

func validateNetwork(value any, problems *[]string) {
	body, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, "policy.network must be an object")
		return
	}
	mode, valid := body["mode"].(string)
	if !valid || !allowedNetworkModes[mode] {
		*problems = append(*problems, "policy.network.mode must be disabled, allowlist, or unrestricted")
	}
	rawAllow, present := body["allow"]
	if mode == "allowlist" && !present {
		*problems = append(*problems, "policy.network.allow is required for allowlist mode")
		return
	}
	if !present {
		return
	}
	if mode != "allowlist" {
		*problems = append(*problems, "policy.network.allow is only valid in allowlist mode")
	}
	allow, valid := rawAllow.([]any)
	if !valid {
		*problems = append(*problems, "policy.network.allow must be a list")
		return
	}
	if mode == "allowlist" && len(allow) == 0 {
		*problems = append(*problems, "policy.network.allow must not be empty in allowlist mode")
	}
	for index, rawEntry := range allow {
		path := fmt.Sprintf("policy.network.allow[%d]", index)
		entry, valid := rawEntry.(map[string]any)
		if !valid {
			*problems = append(*problems, path+" must be an object")
			continue
		}
		if !nonEmptyString(entry["host"]) {
			*problems = append(*problems, path+".host must be a non-empty string")
		}
		if !nonEmptyString(entry["purpose"]) {
			*problems = append(*problems, path+".purpose must be a non-empty string")
		}
		ports, valid := entry["ports"].([]any)
		if !valid || len(ports) == 0 {
			*problems = append(*problems, path+".ports must be a non-empty list")
			continue
		}
		for portIndex, rawPort := range ports {
			port, valid := integer(rawPort)
			if !valid || port < 1 || port > 65535 {
				*problems = append(*problems, fmt.Sprintf("%s.ports[%d] must be between 1 and 65535", path, portIndex))
			}
		}
		if direction, present := entry["direction"]; present {
			value, valid := direction.(string)
			if !valid || (value != "inbound" && value != "outbound" && value != "both") {
				*problems = append(*problems, path+".direction must be inbound, outbound, or both")
			}
		}
	}
}

func validateProcess(value any, problems *[]string) {
	body, ok := value.(map[string]any)
	if !ok {
		*problems = append(*problems, "policy.process must be an object")
		return
	}
	if _, ok := body["can_invoke_subject"].(bool); !ok {
		*problems = append(*problems, "policy.process.can_invoke_subject must be a boolean")
	}
	if !nonEmptyString(body["subject_id"]) {
		*problems = append(*problems, "policy.process.subject_id must be a non-empty string")
	}
	if rawTools, present := body["allowed_tools"]; present {
		tools, valid := rawTools.([]any)
		if !valid {
			*problems = append(*problems, "policy.process.allowed_tools must be a list")
			return
		}
		seen := make(map[string]bool)
		for index, rawTool := range tools {
			path := fmt.Sprintf("policy.process.allowed_tools[%d]", index)
			tool, valid := rawTool.(map[string]any)
			if !valid {
				*problems = append(*problems, path+" must be an object")
				continue
			}
			name, valid := tool["name"].(string)
			if !valid || strings.TrimSpace(name) == "" {
				*problems = append(*problems, path+".name must be a non-empty string")
			} else if seen[name] {
				*problems = append(*problems, path+".name duplicates "+name)
			} else {
				seen[name] = true
			}
			if !nonEmptyString(tool["purpose"]) {
				*problems = append(*problems, path+".purpose must be a non-empty string")
			}
		}
	}
}

func clone(document Document) (Document, error) {
	contents, err := json.Marshal(document.Policy)
	if err != nil {
		return Document{}, fmt.Errorf("clone policy: %w", err)
	}
	var body map[string]any
	if err := json.Unmarshal(contents, &body); err != nil {
		return Document{}, fmt.Errorf("clone policy: %w", err)
	}
	return Document{Policy: body}, nil
}

func decode(contents []byte) (any, error) {
	var value any
	if err := yaml.Unmarshal(contents, &value); err != nil {
		return nil, err
	}
	return normalize(value), nil
}

func normalize(value any) any {
	switch value := value.(type) {
	case map[string]any:
		object := make(map[string]any, len(value))
		for key, child := range value {
			object[key] = normalize(child)
		}
		return object
	case map[any]any:
		object := make(map[string]any, len(value))
		for key, child := range value {
			object[fmt.Sprint(key)] = normalize(child)
		}
		return object
	case []any:
		sequence := make([]any, len(value))
		for index, child := range value {
			sequence[index] = normalize(child)
		}
		return sequence
	default:
		return value
	}
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func integer(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return int64(value), uint64(value) <= uint64(^uint64(0)>>1)
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(value), true
	case float64:
		return int64(value), float64(int64(value)) == value
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
