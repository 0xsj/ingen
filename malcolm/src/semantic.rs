use std::collections::HashSet;

use crate::ast::{BodyField, Literal, RequestClause, Specification};

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

        if let Some(request) = &scenario.request {
            validate_request(request, &format!("{path}.request"), &mut errors);
        }

        if scenario.state.is_some() && scenario.setups.is_empty() {
            errors.push(ValidationError::new(
                format!("{path}.setups"),
                "a state clause requires at least one setup",
            ));
        }

        let mut setup_names = HashSet::new();
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
                validate_request(request, &format!("{setup_path}.request"), &mut errors);
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

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors)
    }
}

fn validate_request(request: &RequestClause, path: &str, errors: &mut Vec<ValidationError>) {
    if request.body.is_empty() {
        errors.push(ValidationError::new(
            format!("{path}.body"),
            "request body must contain at least one field",
        ));
    }

    validate_fields(&request.body, &format!("{path}.body"), errors);
}

fn validate_fields(fields: &[BodyField], path: &str, errors: &mut Vec<ValidationError>) {
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
        validate_literal(&field.value, &format!("{field_path}.value"), errors);
    }
}

fn validate_literal(literal: &Literal, path: &str, errors: &mut Vec<ValidationError>) {
    match literal {
        Literal::Object(fields) => validate_fields(fields, &format!("{path}.object"), errors),
        Literal::Array(values) => {
            for (index, value) in values.iter().enumerate() {
                validate_literal(value, &format!("{path}.array[{index}]"), errors);
            }
        }
        Literal::String(_) | Literal::Integer(_) | Literal::Boolean(_) => {}
    }
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
        };

        let errors = validate(&specification).expect_err("nested duplicate should fail");

        assert!(errors
            .iter()
            .any(|error| error.path == "scenarios[0].request.body[0].value.object[1].name"));
    }
}
