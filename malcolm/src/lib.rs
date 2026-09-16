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
        pub when: Option<WhenClause>,
        pub requirements: Vec<Requirement>,
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

pub use ast::{Requirement, RequirementKind, Scenario, Specification, WhenClause};
pub use ir::{
    compile, IntermediateRepresentation, RequirementIr, ScenarioIr, SpecificationIr, WhenIr,
    IR_SCHEMA,
};
pub use parser::{parse, ParseError};
pub use semantic::{validate, ValidationError};
