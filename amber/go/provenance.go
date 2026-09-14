package amber

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	Version                = 1
	MaxReferences          = 32
	MaxStringLength        = 256
	maxUUIDLength          = 36
	maxDepth        uint64 = ^uint64(0)
)

var (
	ErrInvalidProvenance = errors.New("invalid amber provenance")
	ErrInvalidTransition = errors.New("invalid amber transition")
)

// ID is a canonical lowercase UUID used for work, execution, correlation,
// causation, and replay identities.
type ID string

func (id ID) String() string { return string(id) }

// IDFactory creates an Amber ID. Production code uses the cryptographically
// random default; callers can inject a deterministic factory for conformance
// tests or controlled replay tooling.
type IDFactory func() (ID, error)

// Origin describes how an execution entered the application.
type Origin string

const (
	OriginLocal    Origin = "local"
	OriginIncoming Origin = "incoming"
	OriginRetry    Origin = "retry"
	OriginReplay   Origin = "replay"
)

// Mode describes whether an execution is normal, a retry, or a replay.
type ModeKind string

const (
	ModeNormal ModeKind = "normal"
	ModeRetry  ModeKind = "retry"
	ModeReplay ModeKind = "replay"
)

type Mode struct {
	Kind          ModeKind `json:"kind"`
	OfExecutionID ID       `json:"of_execution_id,omitempty"`
	ReplayID      ID       `json:"replay_id,omitempty"`
}

// Causation identifies the immediate known cause of an execution.
type Causation struct {
	Kind string `json:"kind"`
	ID   ID     `json:"id"`
	Type string `json:"type,omitempty"`
}

// Actor identifies an application-defined principal.
type Actor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type Attribution struct {
	InitiatedBy *Actor `json:"initiated_by,omitempty"`
	ExecutedBy  *Actor `json:"executed_by,omitempty"`
	OnBehalfOf  *Actor `json:"on_behalf_of,omitempty"`
	TenantID    string `json:"tenant_id,omitempty"`
}

type Reference struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Relation string `json:"relation,omitempty"`
}

// StartOptions configures a new logical unit of work.
type StartOptions struct {
	Attribution *Attribution
	References  []Reference
	IDFactory   IDFactory
}

// ChildOptions configures a child logical unit of work.
type ChildOptions struct {
	Cause       *Causation
	Attribution *Attribution
	Origin      Origin
	References  []Reference
}

// Provenance is an immutable application-level provenance value. Its fields
// are private so callers can only create changed values through transitions.
type Provenance struct {
	version       int
	workID        ID
	executionID   ID
	correlationID ID
	causation     *Causation
	origin        Origin
	depth         uint64
	attempt       uint64
	attribution   *Attribution
	mode          Mode
	references    []Reference
	idFactory     IDFactory
}

// Start creates a new logical unit of work. At most one options value may be
// supplied; omitting it uses empty attribution and references.
func Start(options ...StartOptions) (Provenance, error) {
	if len(options) > 1 {
		return Provenance{}, fmt.Errorf("%w: Start accepts at most one options value", ErrInvalidTransition)
	}

	var opts StartOptions
	if len(options) == 1 {
		opts = options[0]
	}

	idFactory := opts.IDFactory
	if idFactory == nil {
		idFactory = newID
	}

	workID, err := idFactory()
	if err != nil {
		return Provenance{}, err
	}
	executionID, err := idFactory()
	if err != nil {
		return Provenance{}, err
	}
	correlationID, err := idFactory()
	if err != nil {
		return Provenance{}, err
	}

	p := Provenance{
		version:       Version,
		workID:        workID,
		executionID:   executionID,
		correlationID: correlationID,
		origin:        OriginLocal,
		depth:         0,
		attempt:       1,
		mode:          Mode{Kind: ModeNormal},
		idFactory:     idFactory,
	}
	p.attribution = cloneAttribution(opts.Attribution)
	p.references = cloneReferences(opts.References)
	if err := p.Validate(); err != nil {
		return Provenance{}, err
	}
	return p, nil
}

// Child creates a new logical unit of work derived from the receiver.
func (p Provenance) Child(options ...ChildOptions) (Provenance, error) {
	if len(options) > 1 {
		return Provenance{}, fmt.Errorf("%w: Child accepts at most one options value", ErrInvalidTransition)
	}
	if err := p.Validate(); err != nil {
		return Provenance{}, err
	}
	if p.depth == maxDepth {
		return Provenance{}, fmt.Errorf("%w: depth overflow", ErrInvalidTransition)
	}

	var opts ChildOptions
	if len(options) == 1 {
		opts = options[0]
	}
	origin := opts.Origin
	if origin == "" {
		origin = OriginLocal
	}
	if origin != OriginLocal && origin != OriginIncoming {
		return Provenance{}, fmt.Errorf("%w: child origin must be local or incoming", ErrInvalidTransition)
	}

	workID, err := p.idFactory()
	if err != nil {
		return Provenance{}, err
	}
	executionID, err := p.idFactory()
	if err != nil {
		return Provenance{}, err
	}

	cause := opts.Cause
	if cause == nil {
		cause = &Causation{Kind: "execution", ID: p.executionID}
	} else {
		cause = cloneCausation(cause)
	}

	child := Provenance{
		version:       Version,
		workID:        workID,
		executionID:   executionID,
		correlationID: p.correlationID,
		causation:     cause,
		origin:        origin,
		depth:         p.depth + 1,
		attempt:       1,
		attribution:   cloneAttribution(opts.Attribution),
		mode:          Mode{Kind: ModeNormal},
		references:    cloneReferences(opts.References),
		idFactory:     p.idFactory,
	}
	if opts.Attribution == nil {
		child.attribution = cloneAttribution(p.attribution)
	}
	if err := child.Validate(); err != nil {
		return Provenance{}, err
	}
	return child, nil
}

// Retry creates a new execution of the same logical work.
func (p Provenance) Retry() (Provenance, error) {
	if err := p.Validate(); err != nil {
		return Provenance{}, err
	}
	if p.attempt == maxDepth {
		return Provenance{}, fmt.Errorf("%w: attempt overflow", ErrInvalidTransition)
	}
	executionID, err := p.idFactory()
	if err != nil {
		return Provenance{}, err
	}

	retry := Provenance{
		version:       Version,
		workID:        p.workID,
		executionID:   executionID,
		correlationID: p.correlationID,
		causation:     &Causation{Kind: "execution", ID: p.executionID},
		origin:        OriginRetry,
		depth:         p.depth,
		attempt:       p.attempt + 1,
		attribution:   cloneAttribution(p.attribution),
		mode:          Mode{Kind: ModeRetry, OfExecutionID: p.executionID},
		references:    cloneReferences(p.references),
		idFactory:     p.idFactory,
	}
	if err := retry.Validate(); err != nil {
		return Provenance{}, err
	}
	return retry, nil
}

// Replay creates an intentional re-execution of the same logical work.
func (p Provenance) Replay() (Provenance, error) {
	if err := p.Validate(); err != nil {
		return Provenance{}, err
	}
	executionID, err := p.idFactory()
	if err != nil {
		return Provenance{}, err
	}
	replayID, err := p.idFactory()
	if err != nil {
		return Provenance{}, err
	}

	replay := Provenance{
		version:       Version,
		workID:        p.workID,
		executionID:   executionID,
		correlationID: p.correlationID,
		causation:     &Causation{Kind: "execution", ID: p.executionID},
		origin:        OriginReplay,
		depth:         p.depth,
		attempt:       p.attempt,
		attribution:   cloneAttribution(p.attribution),
		mode:          Mode{Kind: ModeReplay, OfExecutionID: p.executionID, ReplayID: replayID},
		references:    cloneReferences(p.references),
		idFactory:     p.idFactory,
	}
	if err := replay.Validate(); err != nil {
		return Provenance{}, err
	}
	return replay, nil
}

func (p Provenance) Version() int            { return p.version }
func (p Provenance) WorkID() ID              { return p.workID }
func (p Provenance) ExecutionID() ID         { return p.executionID }
func (p Provenance) CorrelationID() ID       { return p.correlationID }
func (p Provenance) Origin() Origin          { return p.origin }
func (p Provenance) Depth() uint64           { return p.depth }
func (p Provenance) Attempt() uint64         { return p.attempt }
func (p Provenance) Mode() Mode              { return p.mode }
func (p Provenance) References() []Reference { return cloneReferences(p.references) }

func (p Provenance) Causation() (Causation, bool) {
	if p.causation == nil {
		return Causation{}, false
	}
	return *p.causation, true
}

func (p Provenance) Attribution() (Attribution, bool) {
	if p.attribution == nil {
		return Attribution{}, false
	}
	return *cloneAttribution(p.attribution), true
}

// Validate checks a value against the Amber v1 invariants.
func (p Provenance) Validate() error {
	if p.version != Version {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidProvenance, p.version)
	}
	for name, id := range map[string]ID{
		"work_id":        p.workID,
		"execution_id":   p.executionID,
		"correlation_id": p.correlationID,
	} {
		if err := validateID(id); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidProvenance, name, err)
		}
	}
	if p.origin != OriginLocal && p.origin != OriginIncoming && p.origin != OriginRetry && p.origin != OriginReplay {
		return fmt.Errorf("%w: unsupported origin %q", ErrInvalidProvenance, p.origin)
	}
	if p.attempt == 0 {
		return fmt.Errorf("%w: attempt must be positive", ErrInvalidProvenance)
	}
	if err := validateMode(p.mode, p.executionID); err != nil {
		return fmt.Errorf("%w: mode: %v", ErrInvalidProvenance, err)
	}
	if (p.origin == OriginRetry) != (p.mode.Kind == ModeRetry) {
		return fmt.Errorf("%w: retry origin and mode must agree", ErrInvalidProvenance)
	}
	if (p.origin == OriginReplay) != (p.mode.Kind == ModeReplay) {
		return fmt.Errorf("%w: replay origin and mode must agree", ErrInvalidProvenance)
	}
	if p.mode.Kind == ModeNormal && p.origin != OriginLocal && p.origin != OriginIncoming {
		return fmt.Errorf("%w: normal mode requires local or incoming origin", ErrInvalidProvenance)
	}
	if p.causation != nil {
		if err := validateCausation(*p.causation); err != nil {
			return fmt.Errorf("%w: causation: %v", ErrInvalidProvenance, err)
		}
	}
	if p.attribution != nil {
		if err := validateAttribution(*p.attribution); err != nil {
			return fmt.Errorf("%w: attribution: %v", ErrInvalidProvenance, err)
		}
	}
	if len(p.references) > MaxReferences {
		return fmt.Errorf("%w: references exceed %d", ErrInvalidProvenance, MaxReferences)
	}
	for i, reference := range p.references {
		if err := validateReference(reference); err != nil {
			return fmt.Errorf("%w: reference %d: %v", ErrInvalidProvenance, i, err)
		}
	}
	return nil
}

type wireProvenance struct {
	Version       int          `json:"version"`
	WorkID        ID           `json:"work_id"`
	ExecutionID   ID           `json:"execution_id"`
	CorrelationID ID           `json:"correlation_id"`
	Causation     *Causation   `json:"causation,omitempty"`
	Origin        Origin       `json:"origin"`
	Depth         uint64       `json:"depth"`
	Attempt       uint64       `json:"attempt"`
	Attribution   *Attribution `json:"attribution,omitempty"`
	Mode          Mode         `json:"mode"`
	References    []Reference  `json:"references,omitempty"`
}

// MarshalJSON emits the canonical Amber JSON representation.
func (p Provenance) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(wireProvenance{
		Version:       p.version,
		WorkID:        p.workID,
		ExecutionID:   p.executionID,
		CorrelationID: p.correlationID,
		Causation:     cloneCausation(p.causation),
		Origin:        p.origin,
		Depth:         p.depth,
		Attempt:       p.attempt,
		Attribution:   cloneAttribution(p.attribution),
		Mode:          p.mode,
		References:    cloneReferences(p.references),
	})
}

// UnmarshalJSON validates before replacing the receiver, so a failed decode
// never partially updates an existing value.
func (p *Provenance) UnmarshalJSON(data []byte) error {
	var wire wireProvenance
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", ErrInvalidProvenance, err)
	}
	candidate := Provenance{
		version:       wire.Version,
		workID:        wire.WorkID,
		executionID:   wire.ExecutionID,
		correlationID: wire.CorrelationID,
		causation:     cloneCausation(wire.Causation),
		origin:        wire.Origin,
		depth:         wire.Depth,
		attempt:       wire.Attempt,
		attribution:   cloneAttribution(wire.Attribution),
		mode:          wire.Mode,
		references:    cloneReferences(wire.References),
	}
	if p.idFactory != nil {
		candidate.idFactory = p.idFactory
	} else {
		candidate.idFactory = newID
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	*p = candidate
	return nil
}

// FromJSON decodes and validates a provenance value.
func FromJSON(data []byte) (Provenance, error) {
	return FromJSONWithIDFactory(data, newID)
}

// FromJSONWithIDFactory decodes a value and carries the supplied ID factory
// into subsequent transitions. It is primarily useful for deterministic
// conformance tests and controlled replay tooling.
func FromJSONWithIDFactory(data []byte, idFactory IDFactory) (Provenance, error) {
	if idFactory == nil {
		idFactory = newID
	}
	p := Provenance{idFactory: idFactory}
	if err := json.Unmarshal(data, &p); err != nil {
		return Provenance{}, err
	}
	return p, nil
}

func newID() (ID, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate amber ID: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	var text [maxUUIDLength]byte
	hex.Encode(text[0:8], bytes[0:4])
	text[8] = '-'
	hex.Encode(text[9:13], bytes[4:6])
	text[13] = '-'
	hex.Encode(text[14:18], bytes[6:8])
	text[18] = '-'
	hex.Encode(text[19:23], bytes[8:10])
	text[23] = '-'
	hex.Encode(text[24:36], bytes[10:16])
	return ID(text[:]), nil
}

func validateID(id ID) error {
	value := string(id)
	if len(value) != maxUUIDLength {
		return fmt.Errorf("must be a canonical UUID")
	}
	for i, char := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if char != '-' {
				return fmt.Errorf("must be a canonical UUID")
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return fmt.Errorf("must be a lowercase hexadecimal UUID")
		}
	}
	if value[14] != '4' {
		return fmt.Errorf("must be a UUIDv4")
	}
	if !strings.ContainsRune("89ab", rune(value[19])) {
		return fmt.Errorf("must use the RFC 4122 variant")
	}
	return nil
}

func validateMode(mode Mode, executionID ID) error {
	switch mode.Kind {
	case ModeNormal:
		if mode.OfExecutionID != "" || mode.ReplayID != "" {
			return fmt.Errorf("normal mode cannot contain source IDs")
		}
	case ModeRetry:
		if err := validateSourceID(mode.OfExecutionID, executionID); err != nil {
			return err
		}
		if mode.ReplayID != "" {
			return fmt.Errorf("retry mode cannot contain replay_id")
		}
	case ModeReplay:
		if err := validateSourceID(mode.OfExecutionID, executionID); err != nil {
			return err
		}
		if err := validateSourceID(mode.ReplayID, executionID); err != nil {
			return fmt.Errorf("replay_id: %v", err)
		}
	default:
		return fmt.Errorf("unsupported kind %q", mode.Kind)
	}
	return nil
}

func validateSourceID(source, executionID ID) error {
	if err := validateID(source); err != nil {
		return fmt.Errorf("source execution ID: %v", err)
	}
	if source == executionID {
		return fmt.Errorf("source execution ID must differ from execution_id")
	}
	return nil
}

func validateCausation(cause Causation) error {
	switch cause.Kind {
	case "execution", "event", "message", "request", "artifact":
	default:
		return fmt.Errorf("unsupported kind %q", cause.Kind)
	}
	if err := validateID(cause.ID); err != nil {
		return fmt.Errorf("id: %v", err)
	}
	if err := validateOptionalString(cause.Type); err != nil {
		return fmt.Errorf("type: %v", err)
	}
	return nil
}

func validateAttribution(attribution Attribution) error {
	if attribution.InitiatedBy != nil {
		if err := validateActor(*attribution.InitiatedBy); err != nil {
			return fmt.Errorf("initiated_by: %v", err)
		}
	}
	if attribution.ExecutedBy != nil {
		if err := validateActor(*attribution.ExecutedBy); err != nil {
			return fmt.Errorf("executed_by: %v", err)
		}
	}
	if attribution.OnBehalfOf != nil {
		if err := validateActor(*attribution.OnBehalfOf); err != nil {
			return fmt.Errorf("on_behalf_of: %v", err)
		}
	}
	if attribution.TenantID != "" {
		return validateString(attribution.TenantID)
	}
	return nil
}

func validateActor(actor Actor) error {
	if actor.ID == "" || actor.Type == "" {
		return fmt.Errorf("id and type are required")
	}
	if err := validateString(actor.ID); err != nil {
		return fmt.Errorf("id: %v", err)
	}
	return validateString(actor.Type)
}

func validateReference(reference Reference) error {
	if reference.Type == "" || reference.ID == "" {
		return fmt.Errorf("type and id are required")
	}
	if err := validateString(reference.Type); err != nil {
		return fmt.Errorf("type: %v", err)
	}
	if err := validateString(reference.ID); err != nil {
		return fmt.Errorf("id: %v", err)
	}
	return validateOptionalString(reference.Relation)
}

func validateOptionalString(value string) error {
	if value == "" {
		return nil
	}
	return validateString(value)
}

func validateString(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("must contain valid UTF-8")
	}
	if utf8.RuneCountInString(value) > MaxStringLength {
		return fmt.Errorf("must be at most %d Unicode scalar values", MaxStringLength)
	}
	return nil
}

func cloneCausation(value *Causation) *Causation {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneActor(value *Actor) *Actor {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneAttribution(value *Attribution) *Attribution {
	if value == nil {
		return nil
	}
	copy := *value
	copy.OnBehalfOf = cloneActor(value.OnBehalfOf)
	return &copy
}

func cloneReferences(values []Reference) []Reference {
	if values == nil {
		return nil
	}
	return append([]Reference(nil), values...)
}
