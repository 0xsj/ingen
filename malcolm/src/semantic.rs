use std::collections::HashSet;

use crate::ast::{
    BodyField, FixtureOwner, IsolationScope, Literal, ProvenanceField, ProvenanceRequirementKind,
    RequestClause, Specification,
};
use crate::MAX_GENERATED_REPEAT_COUNT;

/// Validate the meaning and completeness of a parsed specification.
///
/// Parsing produces a structured value. Validation checks whether that value
/// is complete enough for a later compilation stage and returns all findings
/// so a contract author can fix them in one pass.
pub fn validate(specification: &Specification) -> Result<(), Vec<ValidationError>> {
    let mut errors = Vec::new();

    if specification.name.trim().is_empty() {
        errors.push(ValidationError::new(
            "name",
            "specification name cannot be empty",
        ));
    }

    if matches!(&specification.version, Some(version) if version.trim().is_empty()) {
        errors.push(ValidationError::new(
            "version",
            "specification version cannot be empty",
        ));
    }

    if matches!(&specification.subject, Some(subject) if subject.trim().is_empty()) {
        errors.push(ValidationError::new(
            "subject",
            "subject cannot be empty when provided",
        ));
    }

    if specification.scenarios.is_empty() {
        errors.push(ValidationError::new(
            "scenarios",
            "specification must contain at least one scenario",
        ));
    }

    let mut scenario_names = HashSet::new();
    for (index, scenario) in specification.scenarios.iter().enumerate() {
        let path = format!("scenarios[{index}]");

        if scenario.name.trim().is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.name"),
                "scenario name cannot be empty",
            ));
        } else if !scenario_names.insert(&scenario.name) {
            errors.push(ValidationError::new(
                format!("{path}.name"),
                format!("scenario name `{}` is duplicated", scenario.name),
            ));
        }

        if scenario.given.iter().any(|given| given.trim().is_empty()) {
            errors.push(ValidationError::new(
                format!("{path}.given"),
                "given clauses cannot be empty",
            ));
        }

        if matches!(&scenario.state, Some(state) if state.trim().is_empty()) {
            errors.push(ValidationError::new(
                format!("{path}.state"),
                "state cannot be empty when provided",
            ));
        }

        let scenario_capture_names = scenario
            .setups
            .iter()
            .flat_map(|setup| setup.captures.iter().map(|capture| capture.name.clone()))
            .collect::<HashSet<_>>();

        if let Some(request) = &scenario.request {
            validate_request(
                request,
                &format!("{path}.request"),
                &scenario_capture_names,
                &mut errors,
            );
        }

        if scenario.state.is_some() && scenario.setups.is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.setups"),
                "a state clause requires at least one setup",
            ));
        }

        let mut setup_names = HashSet::new();
        let mut available_captures = HashSet::new();
        for (setup_index, setup) in scenario.setups.iter().enumerate() {
            let setup_path = format!("{path}.setups[{setup_index}]");
            if setup.name.trim().is_empty() {
                errors.push(ValidationError::new(
                    format!("{setup_path}.name"),
                    "setup name cannot be empty",
                ));
            } else if !setup_names.insert(&setup.name) {
                errors.push(ValidationError::new(
                    format!("{setup_path}.name"),
                    format!("setup name {} is duplicated", setup.name),
                ));
            }

            if let Some(request) = &setup.request {
                validate_request(
                    request,
                    &format!("{setup_path}.request"),
                    &available_captures,
                    &mut errors,
                );
            }

            match &setup.when {
                None => errors.push(ValidationError::new(
                    format!("{setup_path}.when"),
                    "setup must contain a when clause",
                )),
                Some(action) => {
                    if action.method.trim().is_empty() {
                        errors.push(ValidationError::new(
                            format!("{setup_path}.when.method"),
                            "when method cannot be empty",
                        ));
                    }
                    if action.path.trim().is_empty() {
                        errors.push(ValidationError::new(
                            format!("{setup_path}.when.path"),
                            "when path cannot be empty",
                        ));
                    }
                }
            }

            if setup.requirements.is_empty() {
                errors.push(ValidationError::new(
                    format!("{setup_path}.requirements"),
                    "setup must contain at least one requirement",
                ));
            }
            for (requirement_index, requirement) in setup.requirements.iter().enumerate() {
                if requirement.expression.trim().is_empty() {
                    errors.push(ValidationError::new(
                        format!("{setup_path}.requirements[{requirement_index}].expression"),
                        "requirement expression cannot be empty",
                    ));
                }
            }

            let mut capture_names = HashSet::new();
            for (capture_index, capture) in setup.captures.iter().enumerate() {
                let capture_path = format!("{setup_path}.captures[{capture_index}]");
                if capture.name.trim().is_empty() {
                    errors.push(ValidationError::new(
                        format!("{capture_path}.name"),
                        "capture name cannot be empty",
                    ));
                } else if !capture_names.insert(&capture.name) {
                    errors.push(ValidationError::new(
                        format!("{capture_path}.name"),
                        format!("capture name {} is duplicated", capture.name),
                    ));
                }
                if capture.selector.trim().is_empty() {
                    errors.push(ValidationError::new(
                        format!("{capture_path}.selector"),
                        "capture selector cannot be empty",
                    ));
                } else if !is_body_selector(&capture.selector) {
                    errors.push(ValidationError::new(
                        format!("{capture_path}.selector"),
                        "capture selector must be a body.FIELD[.FIELD...] path",
                    ));
                }
            }

            for capture in &setup.captures {
                if is_identifier(&capture.name) {
                    available_captures.insert(capture.name.clone());
                }
            }
        }

        match &scenario.when {
            None => errors.push(ValidationError::new(
                format!("{path}.when"),
                "scenario must contain a when clause",
            )),
            Some(action) => {
                if action.method.trim().is_empty() {
                    errors.push(ValidationError::new(
                        format!("{path}.when.method"),
                        "when method cannot be empty",
                    ));
                }
                if action.path.trim().is_empty() {
                    errors.push(ValidationError::new(
                        format!("{path}.when.path"),
                        "when path cannot be empty",
                    ));
                }
            }
        }

        if scenario.requirements.is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.requirements"),
                "scenario must contain at least one requirement",
            ));
        }

        for (requirement_index, requirement) in scenario.requirements.iter().enumerate() {
            if requirement.expression.trim().is_empty() {
                errors.push(ValidationError::new(
                    format!("{path}.requirements[{requirement_index}].expression"),
                    "requirement expression cannot be empty",
                ));
            }
        }
    }

    let mut fixture_names = HashSet::new();
    for (index, fixture) in specification.fixtures.iter().enumerate() {
        let path = format!("fixtures[{index}]");
        if fixture.id.trim().is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.id"),
                "fixture ID cannot be empty",
            ));
        } else if !fixture_names.insert(&fixture.id) {
            errors.push(ValidationError::new(
                format!("{path}.id"),
                format!("fixture ID `{}` is duplicated", fixture.id),
            ));
        }
        match fixture.owner {
            None => errors.push(ValidationError::new(
                format!("{path}.owner"),
                "fixture must declare owner oracle",
            )),
            Some(FixtureOwner::Oracle) => {}
        }
        match &fixture.purpose {
            None => errors.push(ValidationError::new(
                format!("{path}.purpose"),
                "fixture must declare a purpose",
            )),
            Some(purpose) if purpose.trim().is_empty() => errors.push(ValidationError::new(
                format!("{path}.purpose"),
                "fixture purpose cannot be empty",
            )),
            Some(_) => {}
        }
        match &fixture.sha256 {
            None => errors.push(ValidationError::new(
                format!("{path}.sha256"),
                "fixture must declare a SHA-256 digest",
            )),
            Some(digest) if !is_sha256_digest(digest) => errors.push(ValidationError::new(
                format!("{path}.sha256"),
                "fixture SHA-256 must be 64 lowercase hexadecimal characters",
            )),
            Some(_) => {}
        }
    }

    let mut mutation_names = HashSet::new();
    for (index, mutation) in specification.mutations.iter().enumerate() {
        let path = format!("mutations[{index}]");

        if mutation.id.trim().is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.id"),
                "mutation id cannot be empty",
            ));
        } else if !mutation_names.insert(&mutation.id) {
            errors.push(ValidationError::new(
                format!("{path}.id"),
                format!("mutation id `{}` is duplicated", mutation.id),
            ));
        }

        let target_scenario = mutation.scenario.as_deref().and_then(|name| {
            specification
                .scenarios
                .iter()
                .find(|scenario| scenario.name == name)
        });
        if mutation.scenario.is_none() {
            errors.push(ValidationError::new(
                format!("{path}.scenario"),
                "mutation must target a scenario",
            ));
        } else if target_scenario.is_none() {
            errors.push(ValidationError::new(
                format!("{path}.scenario"),
                "mutation target scenario does not exist",
            ));
        }

        match &mutation.change {
            None => errors.push(ValidationError::new(
                format!("{path}.change"),
                "mutation must contain a status change",
            )),
            Some(change) => {
                if change.field != "response.status" {
                    errors.push(ValidationError::new(
                        format!("{path}.change.field"),
                        "only response.status mutations are supported",
                    ));
                }
                for (name, value) in [("from", change.from), ("to", change.to)] {
                    if !(100..=599).contains(&value) {
                        errors.push(ValidationError::new(
                            format!("{path}.change.{name}"),
                            "status must be between 100 and 599",
                        ));
                    }
                }
                if change.from == change.to {
                    errors.push(ValidationError::new(
                        format!("{path}.change"),
                        "mutation status change must change the value",
                    ));
                }
            }
        }

        match (
            &mutation.expected_rule,
            mutation.scenario.as_deref(),
            target_scenario,
        ) {
            (None, _, _) => errors.push(ValidationError::new(
                format!("{path}.expected_rule"),
                "mutation must identify an expected failing rule",
            )),
            (Some(rule), Some(scenario_name), Some(scenario)) => {
                let prefix = format!("{scenario_name}.requirement.");
                let valid_index = rule
                    .strip_prefix(&prefix)
                    .and_then(|value| value.parse::<usize>().ok())
                    .filter(|index| *index > 0 && *index <= scenario.requirements.len());
                if valid_index.is_none() {
                    errors.push(ValidationError::new(
                        format!("{path}.expected_rule"),
                        format!(
                            "expected rule must be {scenario_name}.requirement.N for an existing scenario requirement"
                        ),
                    ));
                }
            }
            (Some(_), _, _) => {}
        }
    }

    if let Some(provenance) = &specification.provenance {
        if provenance.requirements.is_empty() {
            errors.push(ValidationError::new(
                "provenance.requirements",
                "provenance must contain at least one requirement",
            ));
        }

        let mut seen = HashSet::new();
        for (index, requirement) in provenance.requirements.iter().enumerate() {
            let path = format!("provenance.requirements[{index}]");
            if !seen.insert((requirement.kind, requirement.field)) {
                errors.push(ValidationError::new(
                    path,
                    "provenance requirement is duplicated",
                ));
            }
            match (requirement.kind, requirement.field) {
                (ProvenanceRequirementKind::Create, ProvenanceField::ExecutionId) => {}
            }
        }
    }

    if let Some(isolation) = &specification.isolation {
        match isolation.scope {
            IsolationScope::Scenario => {}
        }
        match &isolation.reset {
            None => errors.push(ValidationError::new(
                "isolation.reset",
                "isolation must contain a reset request",
            )),
            Some(reset) => {
                if reset.method != "POST" {
                    errors.push(ValidationError::new(
                        "isolation.reset.method",
                        "isolation reset must use POST",
                    ));
                }
                if reset.path.trim().is_empty() || !reset.path.starts_with('/') {
                    errors.push(ValidationError::new(
                        "isolation.reset.path",
                        "isolation reset path must start with /",
                    ));
                }
            }
        }
    }

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors)
    }
}

fn validate_request(
    request: &RequestClause,
    path: &str,
    available_captures: &HashSet<String>,
    errors: &mut Vec<ValidationError>,
) {
    if request.body.is_empty() {
        errors.push(ValidationError::new(
            format!("{path}.body"),
            "request body must contain at least one field",
        ));
    }

    validate_fields(
        &request.body,
        &format!("{path}.body"),
        available_captures,
        errors,
    );
}

fn is_sha256_digest(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|character| character.is_ascii_digit() || (b'a'..=b'f').contains(&character))
}

fn validate_fields(
    fields: &[BodyField],
    path: &str,
    available_captures: &HashSet<String>,
    errors: &mut Vec<ValidationError>,
) {
    let mut field_names = HashSet::new();
    for (field_index, field) in fields.iter().enumerate() {
        let field_path = format!("{path}[{field_index}]");
        if field.name.trim().is_empty() {
            errors.push(ValidationError::new(
                format!("{field_path}.name"),
                "request body field name cannot be empty",
            ));
        } else if !field_names.insert(&field.name) {
            errors.push(ValidationError::new(
                format!("{field_path}.name"),
                format!("request body field {} is duplicated", field.name),
            ));
        }
        if field.name == "generated" {
            errors.push(ValidationError::new(
                format!("{field_path}.name"),
                "request body field name 'generated' is reserved for generator values",
            ));
        }
        validate_literal(
            &field.value,
            &format!("{field_path}.value"),
            available_captures,
            errors,
        );
    }
}

fn validate_literal(
    literal: &Literal,
    path: &str,
    available_captures: &HashSet<String>,
    errors: &mut Vec<ValidationError>,
) {
    match literal {
        Literal::Object(fields) => validate_fields(
            fields,
            &format!("{path}.object"),
            available_captures,
            errors,
        ),
        Literal::Array(values) => {
            for (index, value) in values.iter().enumerate() {
                validate_literal(
                    value,
                    &format!("{path}.array[{index}]"),
                    available_captures,
                    errors,
                );
            }
        }
        Literal::Repeat { count, .. } => {
            if !(1..=MAX_GENERATED_REPEAT_COUNT).contains(count) {
                errors.push(ValidationError::new(
                    format!("{path}.repeat.count"),
                    format!("repeat count must be between 1 and {MAX_GENERATED_REPEAT_COUNT}"),
                ));
            }
        }
        Literal::String(value) => validate_template_references(
            value,
            &format!("{path}.string"),
            available_captures,
            errors,
        ),
        Literal::Integer(_) | Literal::Boolean(_) => {}
    }
}

fn validate_template_references(
    value: &str,
    path: &str,
    available_captures: &HashSet<String>,
    errors: &mut Vec<ValidationError>,
) {
    let mut remaining = value;
    while let Some(start) = remaining.find('{') {
        let after_open = &remaining[start + 1..];
        let Some(end) = after_open.find('}') else {
            errors.push(ValidationError::new(
                path,
                "capture placeholder is missing a closing brace",
            ));
            return;
        };

        let name = after_open[..end].trim();
        if !is_identifier(name) {
            errors.push(ValidationError::new(
                path,
                "capture placeholder name must be an identifier",
            ));
        } else if !available_captures.contains(name) {
            errors.push(ValidationError::new(
                path,
                format!("capture '{name}' is not available from an earlier setup"),
            ));
        }
        remaining = &after_open[end + 1..];
    }
}

fn is_identifier(value: &str) -> bool {
    let mut characters = value.chars();
    matches!(characters.next(), Some(character) if character == '_' || character.is_ascii_alphabetic())
        && characters.all(|character| character == '_' || character.is_ascii_alphanumeric())
}

fn is_body_selector(value: &str) -> bool {
    let Some(path) = value.strip_prefix("body.") else {
        return false;
    };
    !path.is_empty() && path.split('.').all(is_identifier)
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ValidationError {
    pub path: String,
    pub message: String,
}

impl ValidationError {
    fn new(path: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            path: path.into(),
            message: message.into(),
        }
    }
}

impl std::fmt::Display for ValidationError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(formatter, "{}: {}", self.path, self.message)
    }
}

impl std::error::Error for ValidationError {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        parse, BodyField, Literal, RequestClause, Requirement, RequirementKind, Scenario,
        Specification, WhenClause,
    };

    #[test]
    fn accepts_a_complete_parsed_specification() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  subject "documents"
                  scenario submit_document {
                    given document is valid
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("source should parse");

        assert_eq!(validate(&specification), Ok(()));
    }

    #[test]
    fn accepts_a_mutation_that_targets_an_existing_rule() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                  mutation "return-200" {
                    target submit_document
                    change response.status from 202 to 200
                    expect rule submit_document.requirement.1 to fail
                  }
                }
            "#,
        )
        .expect("source should parse");

        assert_eq!(validate(&specification), Ok(()));
    }

    #[test]
    fn accepts_a_digest_pinned_oracle_fixture() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  fixture "welcome-document" {
                    owner oracle
                    purpose "canonical document input"
                    sha256 "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
                  }
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("source should parse");

        assert_eq!(validate(&specification), Ok(()));
    }

    #[test]
    fn rejects_fixture_without_owner_or_digest() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  fixture "unbound" {
                    purpose "missing ownership and digest"
                  }
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("source should parse");

        let errors = validate(&specification).expect_err("incomplete fixture should fail");
        assert!(errors.iter().any(|error| error.path == "fixtures[0].owner"));
        assert!(errors
            .iter()
            .any(|error| error.path == "fixtures[0].sha256"));
    }

    #[test]
    fn rejects_mutations_with_unbound_targets_and_noop_changes() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                  mutation "return-200" {
                    target missing_scenario
                    change response.status from 202 to 202
                    expect rule submit_document.requirement.2 to fail
                  }
                }
            "#,
        )
        .expect("source should parse");

        let errors = validate(&specification).expect_err("invalid mutation should fail");

        assert!(errors
            .iter()
            .any(|error| error.path == "mutations[0].scenario"));
        assert!(errors
            .iter()
            .any(|error| error.path == "mutations[0].change"));
    }

    #[test]
    fn rejects_mutations_with_unknown_expected_rules() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                  mutation "return-200" {
                    target submit_document
                    change response.status from 202 to 200
                    expect rule submit_document.requirement.2 to fail
                  }
                }
            "#,
        )
        .expect("source should parse");

        let errors = validate(&specification).expect_err("invalid rule reference should fail");

        assert!(errors
            .iter()
            .any(|error| error.path == "mutations[0].expected_rule"));
    }

    #[test]
    fn reports_all_missing_parts_in_one_pass() {
        let specification = Specification {
            name: "demo".into(),
            version: None,
            subject: None,
            scenarios: vec![Scenario {
                name: "one".into(),
                given: vec![" ".into()],
                state: None,
                request: None,
                when: None,
                setups: Vec::new(),
                requirements: Vec::new(),
            }],
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
        };

        let errors = validate(&specification).expect_err("incomplete spec should fail");
        let paths: Vec<&str> = errors.iter().map(|error| error.path.as_str()).collect();

        assert!(paths.contains(&"scenarios[0].given"));
        assert!(paths.contains(&"scenarios[0].when"));
        assert!(paths.contains(&"scenarios[0].requirements"));
    }

    #[test]
    fn rejects_duplicate_scenario_names_and_empty_values() {
        let scenario = Scenario {
            name: "same".into(),
            given: Vec::new(),
            state: None,
            request: None,
            when: Some(WhenClause {
                method: "GET".into(),
                path: "/items".into(),
            }),
            setups: Vec::new(),
            requirements: vec![Requirement {
                kind: RequirementKind::Must,
                expression: " ".into(),
            }],
        };
        let specification = Specification {
            name: "demo".into(),
            version: Some("v1".into()),
            subject: Some("items".into()),
            scenarios: vec![scenario.clone(), scenario],
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
        };

        let errors = validate(&specification).expect_err("duplicate names should fail");

        assert!(errors.iter().any(|error| error.path == "scenarios[1].name"));
        assert!(errors
            .iter()
            .any(|error| error.path == "scenarios[0].requirements[0].expression"));
        assert!(errors
            .iter()
            .any(|error| error.path == "scenarios[1].requirements[0].expression"));
    }

    #[test]
    fn rejects_a_specification_without_scenarios() {
        let specification = Specification {
            name: "demo".into(),
            version: None,
            subject: None,
            scenarios: Vec::new(),
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
        };

        let errors = validate(&specification).expect_err("empty spec should fail");

        assert_eq!(errors.len(), 1);
        assert_eq!(errors[0].path, "scenarios");
    }

    #[test]
    fn rejects_duplicate_nested_request_fields() {
        let specification = Specification {
            name: "demo".into(),
            version: Some("v1".into()),
            subject: None,
            scenarios: vec![Scenario {
                name: "create".into(),
                given: Vec::new(),
                state: None,
                request: Some(RequestClause {
                    body: vec![BodyField {
                        name: "metadata".into(),
                        value: Literal::Object(vec![
                            BodyField {
                                name: "source".into(),
                                value: Literal::String("import".into()),
                            },
                            BodyField {
                                name: "source".into(),
                                value: Literal::String("duplicate".into()),
                            },
                        ]),
                    }],
                }),
                when: Some(WhenClause {
                    method: "POST".into(),
                    path: "/documents".into(),
                }),
                setups: Vec::new(),
                requirements: vec![Requirement {
                    kind: RequirementKind::Must,
                    expression: "response.status == 202".into(),
                }],
            }],
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
        };

        let errors = validate(&specification).expect_err("nested duplicate should fail");

        assert!(errors
            .iter()
            .any(|error| error.path == "scenarios[0].request.body[0].value.object[1].name"));
    }

    #[test]
    fn rejects_repeat_counts_outside_the_sorna_limit() {
        let specification = parse(
            r#"
                spec document_boundary v1 {
                  scenario reject_oversized_document {
                    given body {
                      content = repeat("a", 1000001)
                    }
                    when POST "/documents"
                    must response.status == 413
                  }
                }
            "#,
        )
        .expect("repeat generator source should parse");

        let errors = validate(&specification).expect_err("unbounded repeat should fail");
        assert!(errors.iter().any(|error| {
            error.path == "scenarios[0].request.body[0].value.repeat.count"
                && error.message.contains("between 1 and 1000000")
        }));
    }

    #[test]
    fn reserves_the_sorna_generator_marker_as_a_body_field_name() {
        let specification = parse(
            r#"
                spec document_boundary v1 {
                  scenario create_document {
                    given body {
                      metadata = {"generated": "ordinary data"}
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("body source should parse");

        let errors = validate(&specification).expect_err("reserved field should fail");
        assert!(errors.iter().any(|error| {
            error.path == "scenarios[0].request.body[0].value.object[0].name"
                && error.message.contains("reserved")
        }));
    }

    #[test]
    fn accepts_capture_interpolation_in_a_later_request_body() {
        let specification = parse(
            r#"
                spec document_interpolation v1 {
                  scenario reuse_captured_name {
                    setup create_seed {
                      given body {
                        name = "seed copy.txt"
                      }
                      when POST "/documents"
                      must response.status == 202
                      capture document_name = response.body.name
                    }
                    given body {
                      name = "{document_name}"
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("interpolation source should parse");

        assert_eq!(validate(&specification), Ok(()));
    }

    #[test]
    fn rejects_capture_interpolation_without_an_available_capture() {
        let specification = parse(
            r#"
                spec document_interpolation v1 {
                  scenario missing_capture {
                    given body {
                      name = "{document_name}"
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("interpolation source should parse");

        let errors = validate(&specification).expect_err("missing capture should fail");
        assert!(errors.iter().any(|error| {
            error.path == "scenarios[0].request.body[0].value.string"
                && error.message.contains("not available")
        }));
    }

    #[test]
    fn rejects_setup_body_interpolation_before_the_capture_exists() {
        let specification = parse(
            r#"
                spec document_interpolation v1 {
                  scenario setup_order {
                    setup create_seed {
                      given body {
                        name = "{document_name}"
                      }
                      when POST "/documents"
                      must response.status == 202
                      capture document_name = response.body.name
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("interpolation source should parse");

        let errors = validate(&specification).expect_err("forward capture should fail");
        assert!(errors.iter().any(|error| {
            error.path == "scenarios[0].setups[0].request.body[0].value.string"
                && error.message.contains("not available")
        }));
    }
}
