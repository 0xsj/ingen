//! Malcolm's first executable-specification slice.
//!
//! This crate currently parses the small, declarative core described in the
//! project README, including typed request bodies and stateful setup data.
//! Mutation declarations are carried as typed intent for the Sorna boundary;
//! provenance declarations are carried as typed intent for the Amber/Sorna
//! boundary; execution and provenance storage remain owned by the surrounding
//! tools.

mod ir;
mod parser;
mod semantic;

/// Maximum repeat count accepted by the Malcolm-to-Sorna boundary.
pub const MAX_GENERATED_REPEAT_COUNT: i64 = 1_000_000;

pub mod ast {
    /// A named Malcolm specification.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct Specification {
        pub name: String,
        pub version: Option<String>,
        pub subject: Option<String>,
        pub scenarios: Vec<Scenario>,
        pub fixtures: Vec<FixtureClause>,
        pub mutations: Vec<MutationClause>,
        pub provenance: Option<ProvenanceClause>,
        pub isolation: Option<IsolationClause>,
    }

    /// A scenario containing setup, an action, and behavioral requirements.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct Scenario {
        pub name: String,
        pub given: Vec<String>,
        pub state: Option<String>,
        pub request: Option<RequestClause>,
        pub when: Option<WhenClause>,
        pub setups: Vec<SetupClause>,
        pub requirements: Vec<Requirement>,
    }

    /// Executable data sent as the JSON body of a request.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct RequestClause {
        pub body: Vec<BodyField>,
    }

    /// One JSON object field in a request body.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct BodyField {
        pub name: String,
        pub value: Literal,
    }

    /// Typed JSON values supported by Malcolm request bodies.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub enum Literal {
        String(String),
        Integer(i64),
        Boolean(bool),
        Object(Vec<BodyField>),
        Array(Vec<Literal>),
        /// A deterministic string generator materialized by Sorna.
        Repeat {
            value: String,
            count: i64,
        },
    }

    /// A request/assertion sequence that establishes state before the target.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct SetupClause {
        pub name: String,
        pub request: Option<RequestClause>,
        pub when: Option<WhenClause>,
        pub requirements: Vec<Requirement>,
        pub captures: Vec<CaptureClause>,
    }

    /// A named value captured from a setup response body.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct CaptureClause {
        pub name: String,
        pub selector: String,
    }

    /// The public operation exercised by a scenario.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct WhenClause {
        pub method: String,
        pub path: String,
    }

    /// A behavioral assertion attached to a scenario.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct Requirement {
        pub kind: RequirementKind,
        pub expression: String,
    }

    /// A reviewed mutation declaration lowered into Sorna's mutation catalogue.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct MutationClause {
        pub id: String,
        pub scenario: Option<String>,
        pub change: Option<MutationChange>,
        pub expected_rule: Option<String>,
    }

    /// The first supported mutation operator changes an HTTP status value.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct MutationChange {
        pub field: String,
        pub from: i64,
        pub to: i64,
    }

    /// A digest-pinned logical input fixture owned by the oracle boundary.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct FixtureClause {
        pub id: String,
        pub owner: Option<FixtureOwner>,
        pub purpose: Option<String>,
        pub sha256: Option<String>,
    }

    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
    pub enum FixtureOwner {
        Oracle,
    }

    /// Provenance requirements that a consumer must observe at the public
    /// subject boundary.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct ProvenanceClause {
        pub requirements: Vec<ProvenanceRequirement>,
    }

    /// State reset semantics for independently executable scenario cases.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct IsolationClause {
        pub scope: IsolationScope,
        pub reset: Option<ResetClause>,
    }

    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
    pub enum IsolationScope {
        Scenario,
    }

    /// A subject-owned public operation that clears state before a case.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct ResetClause {
        pub method: String,
        pub path: String,
    }

    /// One typed provenance requirement.
    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
    pub struct ProvenanceRequirement {
        pub kind: ProvenanceRequirementKind,
        pub field: ProvenanceField,
    }

    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
    pub enum ProvenanceRequirementKind {
        Create,
    }

    #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
    pub enum ProvenanceField {
        ExecutionId,
    }

    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum RequirementKind {
        Must,
        MustNot,
    }
}

pub use ast::{
    BodyField, CaptureClause, FixtureClause, FixtureOwner, IsolationClause, IsolationScope,
    Literal, MutationChange, MutationClause, ProvenanceClause, ProvenanceField,
    ProvenanceRequirement, ProvenanceRequirementKind, RequestClause, Requirement, RequirementKind,
    ResetClause, Scenario, SetupClause, Specification, WhenClause,
};
pub use ir::{
    compile, BodyFieldIr, CaptureIr, FixtureIr, IntermediateRepresentation, IsolationIr,
    MutationChangeIr, MutationIr, ProvenanceIr, ProvenanceRequirementIr, RequestIr, RequirementIr,
    ResetIr, ScenarioIr, SetupIr, SpecificationIr, WhenIr, IR_SCHEMA,
};
pub use parser::{parse, ParseError};
pub use semantic::{validate, ValidationError};
