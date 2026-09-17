use crate::ast::{
    FixtureClause, FixtureOwner, IsolationClause, IsolationScope, Literal, MutationClause,
    ProvenanceClause, ProvenanceField, ProvenanceRequirementKind, RequirementKind, Specification,
};
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
    pub fixtures: Vec<FixtureIr>,
    pub mutations: Vec<MutationIr>,
    pub provenance: Option<ProvenanceIr>,
    pub isolation: Option<IsolationIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ScenarioIr {
    pub name: String,
    pub given: Vec<String>,
    pub state: Option<String>,
    pub request: Option<RequestIr>,
    pub when: WhenIr,
    pub setups: Vec<SetupIr>,
    pub requirements: Vec<RequirementIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RequestIr {
    pub body: Vec<BodyFieldIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BodyFieldIr {
    pub name: String,
    pub value: Literal,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SetupIr {
    pub name: String,
    pub request: Option<RequestIr>,
    pub when: WhenIr,
    pub requirements: Vec<RequirementIr>,
    pub captures: Vec<CaptureIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CaptureIr {
    pub name: String,
    pub selector: String,
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

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MutationIr {
    pub id: String,
    pub scenario: String,
    pub change: MutationChangeIr,
    pub expected_rule: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MutationChangeIr {
    pub field: String,
    pub from: i64,
    pub to: i64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProvenanceIr {
    pub requirements: Vec<ProvenanceRequirementIr>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProvenanceRequirementIr {
    pub kind: String,
    pub field: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IsolationIr {
    pub scope: String,
    pub reset: ResetIr,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ResetIr {
    pub method: String,
    pub path: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FixtureIr {
    pub id: String,
    pub owner: String,
    pub purpose: String,
    pub sha256: String,
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
                    state: scenario.state.clone(),
                    request: scenario.request.as_ref().map(request_from_ast),
                    when: WhenIr {
                        method: when.method.clone(),
                        path: when.path.clone(),
                    },
                    setups: scenario
                        .setups
                        .iter()
                        .map(|setup| {
                            let setup_when = setup
                                .when
                                .as_ref()
                                .expect("validated setup has a when clause");
                            SetupIr {
                                name: setup.name.clone(),
                                request: setup.request.as_ref().map(request_from_ast),
                                when: WhenIr {
                                    method: setup_when.method.clone(),
                                    path: setup_when.path.clone(),
                                },
                                requirements: setup
                                    .requirements
                                    .iter()
                                    .map(requirement_from_ast)
                                    .collect(),
                                captures: setup
                                    .captures
                                    .iter()
                                    .map(|capture| CaptureIr {
                                        name: capture.name.clone(),
                                        selector: capture.selector.clone(),
                                    })
                                    .collect(),
                            }
                        })
                        .collect(),
                    requirements: scenario
                        .requirements
                        .iter()
                        .map(requirement_from_ast)
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
                fixtures: specification
                    .fixtures
                    .iter()
                    .map(fixture_from_ast)
                    .collect(),
                mutations: specification
                    .mutations
                    .iter()
                    .map(mutation_from_ast)
                    .collect(),
                provenance: specification.provenance.as_ref().map(provenance_from_ast),
                isolation: specification.isolation.as_ref().map(isolation_from_ast),
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
        json.push(']');
        if !self.specification.fixtures.is_empty() {
            json.push_str(",\"fixtures\":[");
            for (index, fixture) in self.specification.fixtures.iter().enumerate() {
                if index > 0 {
                    json.push(',');
                }
                write_fixture(&mut json, fixture);
            }
            json.push(']');
        }
        if !self.specification.mutations.is_empty() {
            json.push_str(",\"mutations\":[");
            for (index, mutation) in self.specification.mutations.iter().enumerate() {
                if index > 0 {
                    json.push(',');
                }
                write_mutation(&mut json, mutation);
            }
            json.push(']');
        }
        if let Some(provenance) = &self.specification.provenance {
            json.push_str(",\"provenance\":");
            write_provenance(&mut json, provenance);
        }
        if let Some(isolation) = &self.specification.isolation {
            json.push_str(",\"isolation\":");
            write_isolation(&mut json, isolation);
        }
        json.push_str("}}");
        json
    }
}

fn mutation_from_ast(mutation: &MutationClause) -> MutationIr {
    let change = mutation
        .change
        .as_ref()
        .expect("validated mutation has a change");
    MutationIr {
        id: mutation.id.clone(),
        scenario: mutation
            .scenario
            .clone()
            .expect("validated mutation has a target scenario"),
        change: MutationChangeIr {
            field: change.field.clone(),
            from: change.from,
            to: change.to,
        },
        expected_rule: mutation
            .expected_rule
            .clone()
            .expect("validated mutation has an expected rule"),
    }
}

fn fixture_from_ast(fixture: &FixtureClause) -> FixtureIr {
    FixtureIr {
        id: fixture.id.clone(),
        owner: match fixture.owner {
            Some(FixtureOwner::Oracle) => "oracle".into(),
            None => panic!("validated fixture has an owner"),
        },
        purpose: fixture
            .purpose
            .clone()
            .expect("validated fixture has a purpose"),
        sha256: fixture
            .sha256
            .clone()
            .expect("validated fixture has a SHA-256 digest"),
    }
}

fn write_mutation(json: &mut String, mutation: &MutationIr) {
    json.push('{');
    write_field_string(json, "id", &mutation.id, true);
    write_field_string(json, "scenario", &mutation.scenario, true);
    json.push_str("\"change\":{");
    write_field_string(json, "field", &mutation.change.field, true);
    json.push_str("\"from\":");
    json.push_str(&mutation.change.from.to_string());
    json.push_str(",\"to\":");
    json.push_str(&mutation.change.to.to_string());
    json.push('}');
    json.push(',');
    write_field_string(json, "expected_rule", &mutation.expected_rule, false);
    json.push('}');
}

fn write_fixture(json: &mut String, fixture: &FixtureIr) {
    json.push('{');
    write_field_string(json, "id", &fixture.id, true);
    write_field_string(json, "owner", &fixture.owner, true);
    write_field_string(json, "purpose", &fixture.purpose, true);
    write_field_string(json, "sha256", &fixture.sha256, false);
    json.push('}');
}

fn provenance_from_ast(provenance: &ProvenanceClause) -> ProvenanceIr {
    ProvenanceIr {
        requirements: provenance
            .requirements
            .iter()
            .map(|requirement| ProvenanceRequirementIr {
                kind: match requirement.kind {
                    ProvenanceRequirementKind::Create => "create".into(),
                },
                field: match requirement.field {
                    ProvenanceField::ExecutionId => "execution_id".into(),
                },
            })
            .collect(),
    }
}

fn isolation_from_ast(isolation: &IsolationClause) -> IsolationIr {
    let reset = isolation
        .reset
        .as_ref()
        .expect("validated isolation has a reset request");
    IsolationIr {
        scope: match isolation.scope {
            IsolationScope::Scenario => "scenario".into(),
        },
        reset: ResetIr {
            method: reset.method.clone(),
            path: reset.path.clone(),
        },
    }
}

fn write_provenance(json: &mut String, provenance: &ProvenanceIr) {
    json.push_str("{\"requirements\":[");
    for (index, requirement) in provenance.requirements.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        json.push('{');
        write_field_string(json, "kind", &requirement.kind, true);
        write_field_string(json, "field", &requirement.field, false);
        json.push('}');
    }
    json.push_str("]}");
}

fn write_isolation(json: &mut String, isolation: &IsolationIr) {
    json.push('{');
    write_field_string(json, "scope", &isolation.scope, true);
    json.push_str("\"reset\":{");
    write_field_string(json, "method", &isolation.reset.method, true);
    write_field_string(json, "path", &isolation.reset.path, false);
    json.push_str("}}");
}

fn request_from_ast(request: &crate::ast::RequestClause) -> RequestIr {
    RequestIr {
        body: request
            .body
            .iter()
            .map(|field| BodyFieldIr {
                name: field.name.clone(),
                value: field.value.clone(),
            })
            .collect(),
    }
}

fn requirement_from_ast(requirement: &crate::ast::Requirement) -> RequirementIr {
    RequirementIr {
        kind: match requirement.kind {
            RequirementKind::Must => "must".into(),
            RequirementKind::MustNot => "must_not".into(),
        },
        expression: requirement.expression.clone(),
    }
}

fn write_scenario(json: &mut String, scenario: &ScenarioIr) {
    json.push('{');
    write_field_string(json, "name", &scenario.name, true);
    write_field_string_array(json, "given", &scenario.given, false);
    if let Some(state) = &scenario.state {
        json.push_str(",\"state\":");
        write_json_string(json, state);
    }
    if let Some(request) = &scenario.request {
        json.push_str(",\"request\":");
        write_request(json, request);
    }

    json.push_str(",\"when\":{");
    write_field_string(json, "method", &scenario.when.method, true);
    write_field_string(json, "path", &scenario.when.path, false);
    json.push('}');

    if !scenario.setups.is_empty() {
        json.push_str(",\"setups\":[");
        for (index, setup) in scenario.setups.iter().enumerate() {
            if index > 0 {
                json.push(',');
            }
            write_setup(json, setup);
        }
        json.push(']');
    }

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

fn write_setup(json: &mut String, setup: &SetupIr) {
    json.push('{');
    write_field_string(json, "name", &setup.name, false);
    if let Some(request) = &setup.request {
        json.push_str(",\"request\":");
        write_request(json, request);
    }
    json.push_str(",\"when\":{");
    write_field_string(json, "method", &setup.when.method, true);
    write_field_string(json, "path", &setup.when.path, false);
    json.push('}');
    json.push_str(",\"requirements\":[");
    for (index, requirement) in setup.requirements.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        write_requirement(json, requirement);
    }
    json.push(']');
    json.push_str(",\"captures\":[");
    for (index, capture) in setup.captures.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        json.push('{');
        write_field_string(json, "name", &capture.name, true);
        write_field_string(json, "selector", &capture.selector, false);
        json.push('}');
    }
    json.push_str("]}");
}

fn write_request(json: &mut String, request: &RequestIr) {
    json.push_str("{\"body\":");
    write_ir_object(json, &request.body);
    json.push('}');
}

fn write_ir_object(json: &mut String, fields: &[BodyFieldIr]) {
    json.push('{');
    for (index, field) in fields.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        write_json_string(json, &field.name);
        json.push(':');
        write_literal(json, &field.value);
    }
    json.push('}');
}

fn write_ast_object(json: &mut String, fields: &[crate::ast::BodyField]) {
    json.push('{');
    for (index, field) in fields.iter().enumerate() {
        if index > 0 {
            json.push(',');
        }
        write_json_string(json, &field.name);
        json.push(':');
        write_literal(json, &field.value);
    }
    json.push('}');
}

fn write_literal(json: &mut String, value: &Literal) {
    match value {
        Literal::String(value) => write_json_string(json, value),
        Literal::Integer(value) => json.push_str(&value.to_string()),
        Literal::Boolean(value) => json.push_str(if *value { "true" } else { "false" }),
        Literal::Object(fields) => write_ast_object(json, fields),
        Literal::Array(values) => {
            json.push('[');
            for (index, value) in values.iter().enumerate() {
                if index > 0 {
                    json.push(',');
                }
                write_literal(json, value);
            }
            json.push(']');
        }
        Literal::Repeat { value, count } => {
            json.push_str("{\"generated\":{\"kind\":\"repeat\",\"value\":");
            write_json_string(json, value);
            json.push_str(",\"count\":");
            json.push_str(&count.to_string());
            json.push_str("}}");
        }
    }
}

fn write_requirement(json: &mut String, requirement: &RequirementIr) {
    json.push('{');
    write_field_string(json, "kind", &requirement.kind, true);
    write_field_string(json, "expression", &requirement.expression, false);
    json.push('}');
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
    fn emits_executable_request_and_setup_data() {
        let specification = parse(
            r#"
                spec document_flow v2 {
                  scenario read_document {
                    state document_accepted
                    given body {
                      name = "welcome.md"
                      count = 3
                      published = true
                    }
                    setup accept_document {
                      given body {
                        name = "welcome.md"
                      }
                      when POST "/documents"
                      must response.status == 202
                      capture document_id = response.body.id
                    }
                    when GET "/documents/{document_id}"
                    must response.status == 200
                  }
                }
            "#,
        )
        .expect("source should parse");

        let json = compile(&specification)
            .expect("source should validate")
            .to_json();

        assert_eq!(
            json,
            r#"{"schema":"malcolm.ir/v1","specification":{"name":"document_flow","version":"v2","subject":null,"scenarios":[{"name":"read_document","given":[],"state":"document_accepted","request":{"body":{"name":"welcome.md","count":3,"published":true}},"when":{"method":"GET","path":"/documents/{document_id}"},"setups":[{"name":"accept_document","request":{"body":{"name":"welcome.md"}},"when":{"method":"POST","path":"/documents"},"requirements":[{"kind":"must","expression":"response.status == 202"}],"captures":[{"name":"document_id","selector":"body.id"}]}],"requirements":[{"kind":"must","expression":"response.status == 200"}]}]}}"#
        );
    }

    #[test]
    fn emits_nested_request_values_as_json() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario create_document {
                    given body {
                      metadata = {"source": "import", "priority": 2, "reviewed": true}
                      tags = ["docs", "contract"]
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("nested request source should parse");

        let json = compile(&specification)
            .expect("nested request source should validate")
            .to_json();

        assert!(json.contains(
            r#""metadata":{"source":"import","priority":2,"reviewed":true},"tags":["docs","contract"]"#
        ));
    }

    #[test]
    fn emits_multiline_literals_and_decoded_string_escapes() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario create_document {
                    given body {
                      metadata = {
                        "source": "import",
                        "labels": [
                          "docs",
                          "contract"
                        ]
                      }
                      message = "say \"hello\"\nnext"
                    }
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("multiline request source should parse");

        let json = compile(&specification)
            .expect("multiline request source should validate")
            .to_json();

        assert!(json.contains(
            r#""metadata":{"source":"import","labels":["docs","contract"]},"message":"say \"hello\"\nnext""#
        ));
    }

    #[test]
    fn emits_mutation_declarations_for_the_sorna_boundary() {
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
        .expect("mutation source should parse");

        let json = compile(&specification)
            .expect("mutation source should validate")
            .to_json();

        assert!(json.contains(
            r#""mutations":[{"id":"return-200","scenario":"submit_document","change":{"field":"response.status","from":202,"to":200},"expected_rule":"submit_document.requirement.1"}]"#
        ));
    }

    #[test]
    fn emits_execution_id_provenance_requirements_for_the_sorna_boundary() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  provenance {
                    must create execution_id
                  }
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("provenance source should parse");

        let json = compile(&specification)
            .expect("provenance source should validate")
            .to_json();

        assert!(json.contains(
            r#""provenance":{"requirements":[{"kind":"create","field":"execution_id"}]}"#
        ));
    }

    #[test]
    fn emits_per_scenario_isolation_reset_for_the_sorna_boundary() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  isolation per scenario {
                    reset POST "/__malcolm/reset"
                  }
                  scenario submit_document {
                    when POST "/documents"
                    must response.status == 202
                  }
                }
            "#,
        )
        .expect("isolation source should parse");

        let json = compile(&specification)
            .expect("isolation source should validate")
            .to_json();

        assert!(json.contains(
            r#""isolation":{"scope":"scenario","reset":{"method":"POST","path":"/__malcolm/reset"}}"#
        ));
    }

    #[test]
    fn emits_digest_pinned_fixture_for_the_sorna_boundary() {
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
        .expect("fixture source should parse");

        let json = compile(&specification)
            .expect("fixture source should validate")
            .to_json();

        assert!(json.contains(
            r#""fixtures":[{"id":"welcome-document","owner":"oracle","purpose":"canonical document input","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]"#
        ));
    }

    #[test]
    fn emits_nested_response_selectors_without_rewriting_the_path() {
        let specification = parse(
            r#"
                spec document_api v1 {
                  scenario inspect_document {
                    setup create_document {
                      given body {
                        name = "welcome.md"
                      }
                      when POST "/documents"
                      must response.body.metadata.owner.id exists
                      capture owner_id = response.body.metadata.owner.id
                    }
                    when GET "/documents"
                    must response.status == 200
                  }
                }
            "#,
        )
        .expect("nested selector source should parse");

        let json = compile(&specification)
            .expect("nested selector source should validate")
            .to_json();

        assert!(json.contains(r#""expression":"response.body.metadata.owner.id exists""#));
        assert!(json.contains(r#""selector":"body.metadata.owner.id""#));
    }

    #[test]
    fn emits_repeat_generator_as_sorna_ir() {
        let specification = parse(
            r#"
                spec document_boundary v1 {
                  scenario reject_oversized_document {
                    given body {
                      content = repeat("a", 4097)
                    }
                    when POST "/documents"
                    must response.status == 413
                  }
                }
            "#,
        )
        .expect("repeat generator source should parse");

        let json = compile(&specification)
            .expect("repeat generator source should validate")
            .to_json();

        assert!(
            json.contains(r#""content":{"generated":{"kind":"repeat","value":"a","count":4097}}"#)
        );
    }

    #[test]
    fn compilation_stops_before_ir_for_invalid_models() {
        let specification = Specification {
            name: "invalid".into(),
            version: None,
            subject: None,
            scenarios: Vec::new(),
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
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
                state: None,
                request: None,
                when: Some(WhenClause {
                    method: "GET".into(),
                    path: "/items".into(),
                }),
                setups: Vec::new(),
                requirements: vec![Requirement {
                    kind: RequirementKind::Must,
                    expression: "response.ok == true\t".into(),
                }],
            }],
            fixtures: Vec::new(),
            mutations: Vec::new(),
            provenance: None,
            isolation: None,
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
