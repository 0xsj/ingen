// Package governance defines Hammond's contract governance records and rules.
package governance

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const Schema = "ingen.hammond-governance/v1"
const PolicySchema = "ingen.hammond-review-policy/v1"
const AuthoritySchema = "ingen.hammond-authority/v1"
const AuthorityTrustSchema = "ingen.hammond-authority-trust/v1"
const AuthorityRootSchema = "ingen.hammond-authority-root/v1"
const MembershipSchema = "ingen.hammond-membership/v1"

type State string

const (
	StateRegistered State = "registered"
	StateInReview   State = "in_review"
	StateApproved   State = "approved"
	StateRejected   State = "rejected"
	StateSuperseded State = "superseded"
)

type EventType string

const (
	EventRegistered        EventType = "registered"
	EventReviewOpened      EventType = "review-opened"
	EventApprovalRecorded  EventType = "approval-recorded"
	EventRejectionRecorded EventType = "rejection-recorded"
	EventAmendmentCreated  EventType = "amendment-created"
	EventSuperseded        EventType = "superseded"
)

type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
)

type AmendmentKind string

const (
	AmendmentClarifying  AmendmentKind = "clarifying"
	AmendmentAdditive    AmendmentKind = "additive"
	AmendmentRestrictive AmendmentKind = "restrictive"
	AmendmentCorrective  AmendmentKind = "corrective"
	AmendmentBreaking    AmendmentKind = "breaking"
)

type Artifact struct {
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
}

type ContractReference struct {
	ProjectID string   `json:"project_id"`
	ID        string   `json:"id"`
	Version   int      `json:"version"`
	Schema    string   `json:"schema"`
	Artifact  Artifact `json:"artifact"`
}

type PolicyReference struct {
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Schema   string   `json:"schema"`
	Artifact Artifact `json:"artifact"`
}

type AuthorityReference struct {
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Schema   string   `json:"schema"`
	Artifact Artifact `json:"artifact"`
}

type AuthorityTrustReference struct {
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Schema   string   `json:"schema"`
	Artifact Artifact `json:"artifact"`
}

type AuthorityRootReference struct {
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Schema   string   `json:"schema"`
	Artifact Artifact `json:"artifact"`
}

type MembershipReference struct {
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Schema   string   `json:"schema"`
	Artifact Artifact `json:"artifact"`
}

func (reference AuthorityReference) Key() string {
	return fmt.Sprintf("%s:%d:%s:%s", reference.ID, reference.Version, reference.Schema, reference.Artifact.SHA256)
}

func (reference AuthorityReference) Equal(other AuthorityReference) bool {
	return reference.ID == other.ID &&
		reference.Version == other.Version &&
		reference.Schema == other.Schema &&
		reference.Artifact.SHA256 == other.Artifact.SHA256
}

func (reference AuthorityTrustReference) Key() string {
	return fmt.Sprintf("%s:%d:%s:%s", reference.ID, reference.Version, reference.Schema, reference.Artifact.SHA256)
}

func (reference AuthorityTrustReference) Equal(other AuthorityTrustReference) bool {
	return reference.ID == other.ID &&
		reference.Version == other.Version &&
		reference.Schema == other.Schema &&
		reference.Artifact.SHA256 == other.Artifact.SHA256
}

func (reference AuthorityRootReference) Key() string {
	return fmt.Sprintf("%s:%d:%s:%s", reference.ID, reference.Version, reference.Schema, reference.Artifact.SHA256)
}

func (reference AuthorityRootReference) Equal(other AuthorityRootReference) bool {
	return reference.ID == other.ID &&
		reference.Version == other.Version &&
		reference.Schema == other.Schema &&
		reference.Artifact.SHA256 == other.Artifact.SHA256
}

func (reference MembershipReference) Key() string {
	return fmt.Sprintf("%s:%d:%s:%s", reference.ID, reference.Version, reference.Schema, reference.Artifact.SHA256)
}

func (reference MembershipReference) Equal(other MembershipReference) bool {
	return reference.ID == other.ID &&
		reference.Version == other.Version &&
		reference.Schema == other.Schema &&
		reference.Artifact.SHA256 == other.Artifact.SHA256
}

func (reference PolicyReference) Key() string {
	return fmt.Sprintf("%s:%d:%s:%s", reference.ID, reference.Version, reference.Schema, reference.Artifact.SHA256)
}

func (reference PolicyReference) Equal(other PolicyReference) bool {
	return reference.ID == other.ID &&
		reference.Version == other.Version &&
		reference.Schema == other.Schema &&
		reference.Artifact.SHA256 == other.Artifact.SHA256
}

func (reference ContractReference) Identity() ContractIdentity {
	return ContractIdentity{
		ProjectID:      reference.ProjectID,
		ID:             reference.ID,
		Version:        reference.Version,
		Schema:         reference.Schema,
		ArtifactSHA256: reference.Artifact.SHA256,
	}
}

type ContractIdentity struct {
	ProjectID      string `json:"project_id"`
	ID             string `json:"id"`
	Version        int    `json:"version"`
	Schema         string `json:"schema"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

func (identity ContractIdentity) Key() string {
	return fmt.Sprintf("%s:%s:%d:%s:%s", identity.ProjectID, identity.ID, identity.Version, identity.Schema, identity.ArtifactSHA256)
}

func (identity ContractIdentity) Equal(other ContractIdentity) bool {
	return identity == other
}

type Event struct {
	ID             string               `json:"id"`
	Type           EventType            `json:"type"`
	Actor          string               `json:"actor"`
	Role           string               `json:"role,omitempty"`
	At             string               `json:"at"`
	ReviewCycleID  string               `json:"review_cycle_id,omitempty"`
	Decision       Decision             `json:"decision,omitempty"`
	ArtifactSHA256 string               `json:"artifact_sha256,omitempty"`
	Membership     *MembershipReference `json:"membership,omitempty"`
	Reason         string               `json:"reason,omitempty"`
	Predecessor    *ContractIdentity    `json:"predecessor,omitempty"`
	Successor      *ContractIdentity    `json:"successor,omitempty"`
	AmendmentKind  AmendmentKind        `json:"amendment_kind,omitempty"`
}

type Record struct {
	Schema   string            `json:"schema"`
	RecordID string            `json:"record_id"`
	Contract ContractReference `json:"contract"`
	Policy   PolicyReference   `json:"policy"`
	State    State             `json:"state"`
	Events   []Event           `json:"events"`
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
