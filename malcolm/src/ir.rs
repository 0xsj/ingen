use crate::ast::{RequirementKind, Specification};
use crate::semantic::{validate, ValidationError};

pub const IR_SCHEMA: &str = "malcolm.ir/v1";

/// Malcolm's language-neutral representation of a validated specification.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IntermediateRepresentation {
    pub schema: String,
    pub specification: SpecificationIr,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SpecificationIr {
    pub name: String,
    pub version: Option<String>,
    pub subject: Option<String>,
    pub scenarios: Vec<ScenarioIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ScenarioIr {
    pub name: String,
    pub given: Vec<String>,
    pub when: WhenIr,
    pub requirements: Vec<RequirementIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WhenIr {
    pub method: String,
    pub path: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RequirementIr {
    pub kind: String,
    pub expression: String,
}

/// Validate a specification and compile it into Malcolm's first IR.
pub fn compile(
    specification: &Specification,
) -> Result<IntermediateRepresentation, Vec<ValidationError>> {
    validate(specification)?;

    Ok(IntermediateRepresentation::from_valid(specification))
}

impl IntermediateRepresentation {
    fn from_valid(specification: &Specification) -> Self {
        let scenarios = specification
            .scenarios
            .iter()
            .map(|scenario| {
                let when = scenario
                    .when
                    .as_ref()
                    .expect("validated scenario has a when clause");

                ScenarioIr {
                    name: scenario.name.clone(),
                    given: scenario.given.clone(),
                    when: WhenIr {
                        method: when.method.clone(),
                        path: when.path.clone(),
                    },
                    requirements: scenario
                        .requirements
                        .iter()
                        .map(|requirement| RequirementIr {
                            kind: match requirement.kind {
                                RequirementKind::Must => "must".into(),
                                RequirementKind::MustNot => "must_not".into(),
                            },
                            expression: requirement.expression.clone(),
                        })
                        .collect(),
                }
            })
            .collect();

        Self {
            schema: IR_SCHEMA.into(),
            specification: SpecificationIr {
                name: specification.name.clone(),
                version: specification.version.clone(),
                subject: specification.subject.clone(),
                scenarios,
            },
        }
    }

    /// Serialize the IR with stable field and array ordering.
    pub fn to_json(&self) -> String {
        let mut json = String::new();
        json.push('{');
        write_field_string(&mut json, "schema", &self.schema, false);
        json.push_str(",\"specification\":{");

        write_field_string(&mut json, "name", &self.specification.name, true);
        write_field_optional_string(
            &mut json,
            "version",
            self.specification.version.as_deref(),
            true,
        );
        write_field_optional_string(
            &mut json,
            "subject",
            self.specification.subject.as_deref(),
            false,
        );

        json.push_str(",\"scenarios\":[");
        for (index, scenario) in self.specification.scenarios.iter().enumerate() {
            if index > 0 {
                json.push(',');
            }
            write_scenario(&mut json, scenario);
        }
        json.push_str("]}}");
        json
    }
}

fn write_scenario(json: &mut String, scenario: &ScenarioIr) {
    json.push('{');
    write_field_string(json, "name", &scenario.name, true);
    write_field_string_array(json, "given", &scenario.given, false);

    json.push_str(",\"when\":{");
    write_field_string(json, "method", &scenario.when.method, true);
    write_field_string(json, "path", &scenario.when.path, false);
    json.push('}');

    json.push_str(",\"requirements\":[");
    for (index, requirement) in scenario.requirements.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        json.push('{');
        write_field_string(json, "kind", &requirement.kind, true);
        write_field_string(json, "expression", &requirement.expression, false);
        json.push('}');
    }
    json.push_str("]}");
}

fn write_field_string(json: &mut String, name: &str, value: &str, comma_after: bool) {
    write_json_string(json, name);
    json.push(':');
    write_json_string(json, value);
    if comma_after {
        json.push(',');
    }
}

fn write_field_optional_string(
    json: &mut String,
    name: &str,
    value: Option<&str>,
    comma_after: bool,
) {
    write_json_string(json, name);
    json.push(':');
    match value {
        Some(value) => write_json_string(json, value),
        None => json.push_str("null"),
    }
    if comma_after {
        json.push(',');
    }
}

fn write_field_string_array(json: &mut String, name: &str, values: &[String], comma_after: bool) {
    write_json_string(json, name);
    json.push_str(":[");
    for (index, value) in values.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        write_json_string(json, value);
    }
    json.push(']');
    if comma_after {
        json.push(',');
    }
}

fn write_json_string(json: &mut String, value: &str) {
    json.push('"');
    for character in value.chars() {
        match character {
            '"' => json.push_str("\\\""),
            '\\' => json.push_str("\\\\"),
            '\u{08}' => json.push_str("\\b"),
            '\u{0C}' => json.push_str("\\f"),
            '\n' => json.push_str("\\n"),
            '\r' => json.push_str("\\r"),
            '\t' => json.push_str("\\t"),
            character if character.is_control() => {
                json.push_str("\\u");
                let code = character as u32;
                json.push(hex_digit((code >> 12) & 0xF));
                json.push(hex_digit((code >> 8) & 0xF));
                json.push(hex_digit((code >> 4) & 0xF));
                json.push(hex_digit(code & 0xF));
            }
            character => json.push(character),
        }
    }
    json.push('"');
}

fn hex_digit(value: u32) -> char {
    match value {
        0..=9 => char::from(b'0' + value as u8),
        10..=15 => char::from(b'a' + (value as u8 - 10)),
        _ => unreachable!("hex digit is four bits"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{parse, Requirement, RequirementKind, Scenario, Specification, WhenClause};

    #[test]
    fn emits_stable_language_neutral_json() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  subject "documents"
                  scenario submit_document {
                    given document is valid
                    when POST "/documents"
                    must response.status == 202
                    must_not response.body.error exists
                  }
                }
            "#,
        )
        .expect("source should parse");

        let ir = compile(&specification).expect("source should validate");

        assert_eq!(
            ir.to_json(),
            r#"{"schema":"malcolm.ir/v1","specification":{"name":"document_api","version":"v1","subject":"documents","scenarios":[{"name":"submit_document","given":["document is valid"],"when":{"method":"POST","path":"/documents"},"requirements":[{"kind":"must","expression":"response.status == 202"},{"kind":"must_not","expression":"response.body.error exists"}]}]}}"#
        );
        assert_eq!(ir.to_json(), ir.to_json());
    }

    #[test]
    fn compilation_stops_before_ir_for_invalid_models() {
        let specification = Specification {
            name: "invalid".into(),
            version: None,
            subject: None,
            scenarios: Vec::new(),
        };

        let errors = compile(&specification).expect_err("invalid model should not compile");

        assert_eq!(errors.len(), 1);
        assert_eq!(errors[0].path, "scenarios");
    }

    #[test]
    fn escapes_json_string_content() {
        let specification = Specification {
            name: "quo\"te".into(),
            version: None,
            subject: Some("line\nnext".into()),
            scenarios: vec![Scenario {
                name: "one".into(),
                given: vec!["slash \\".into()],
                when: Some(WhenClause {
                    method: "GET".into(),
                    path: "/items".into(),
                }),
                requirements: vec![Requirement {
                    kind: RequirementKind::Must,
                    expression: "response.ok == true\t".into(),
                }],
            }],
        };

        let json = compile(&specification)
            .expect("model should validate")
            .to_json();

        assert!(json.contains(r#""name":"quo\"te""#));
        assert!(json.contains(r#""subject":"line\nnext""#));
        assert!(json.contains(r#""given":["slash \\"]"#));
        assert!(json.contains(r#""expression":"response.ok == true\t""#));
    }
}
