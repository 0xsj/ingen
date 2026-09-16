//! Malcolm's first executable-specification slice.
//!
//! This crate currently parses the small, declarative core described in the
//! project README. Execution, evidence, and mutation support will build on
//! these typed values in later slices.

mod ir;
mod parser;
mod semantic;

pub mod ast {
    /// A named Malcolm specification.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct Specification {
        pub name: String,
        pub version: Option<String>,
        pub subject: Option<String>,
        pub scenarios: Vec<Scenario>,
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

    /// One top-level JSON body field in the first Malcolm request slice.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub struct BodyField {
        pub name: String,
        pub value: Literal,
    }

    /// Literal values supported by Malcolm's first executable request body.
    #[derive(Debug, Clone, PartialEq, Eq)]
    pub enum Literal {
        String(String),
        Integer(i64),
        Boolean(bool),
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

    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    pub enum RequirementKind {
        Must,
        MustNot,
    }
}

pub use ast::{
    BodyField, CaptureClause, Literal, RequestClause, Requirement, RequirementKind, Scenario,
    SetupClause, Specification, WhenClause,
};
pub use ir::{
    compile, IntermediateRepresentation, RequirementIr, ScenarioIr, SpecificationIr, WhenIr,
    IR_SCHEMA,
};
pub use parser::{parse, ParseError};
pub use semantic::{validate, ValidationError};
