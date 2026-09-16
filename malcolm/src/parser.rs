use std::fmt;

use crate::ast::{Requirement, RequirementKind, Scenario, Specification, WhenClause};

/// Parse a Malcolm specification from its declarative source text.
pub fn parse(source: &str) -> Result<Specification, ParseError> {
    let mut specification: Option<Specification> = None;
    let mut scenario: Option<Scenario> = None;

    for (index, raw_line) in source.lines().enumerate() {
        let line_number = index + 1;
        let line = raw_line.trim();

        if line.is_empty() || line.starts_with('#') {
            continue;
        }

        if let Some(current) = scenario.as_mut() {
            if line == "}" {
                let finished = scenario.take().expect("scenario exists while parsing");
                specification_mut(&mut specification, line_number)?
                    .scenarios
                    .push(finished);
                continue;
            }

            parse_scenario_line(current, line, line_number)?;
            continue;
        }

        if let Some(current) = specification.as_mut() {
            if line == "}" {
                return Ok(specification
                    .take()
                    .expect("specification exists while parsing"));
            }

            if let Some(subject) = line.strip_prefix("subject ") {
                if current.subject.is_some() {
                    return Err(ParseError::new(
                        line_number,
                        "specification has more than one subject",
                    ));
                }
                current.subject = Some(parse_quoted(subject, line_number, "subject")?);
                continue;
            }

            if let Some(header) = line.strip_prefix("scenario ") {
                scenario = Some(parse_scenario_header(header, line_number)?);
                continue;
            }

            return Err(ParseError::new(
                line_number,
                "expected `subject`, `scenario`, or `}` inside specification",
            ));
        }

        if specification.is_none() {
            specification = Some(parse_spec_header(line, line_number)?);
        }
    }

    if scenario.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "scenario is missing a closing `}`",
        ));
    }

    Err(ParseError::new(
        source.lines().count().max(1),
        "specification is missing a closing `}`",
    ))
}

fn parse_spec_header(line: &str, line_number: usize) -> Result<Specification, ParseError> {
    let content = require_open_brace(line, line_number, "specification")?;
    let fields: Vec<&str> = content.split_whitespace().collect();

    if fields.first() != Some(&"spec") || !(2..=3).contains(&fields.len()) {
        return Err(ParseError::new(
            line_number,
            "expected `spec NAME [VERSION] {`",
        ));
    }

    Ok(Specification {
        name: fields[1].to_owned(),
        version: fields.get(2).map(|value| (*value).to_owned()),
        subject: None,
        scenarios: Vec::new(),
    })
}

fn parse_scenario_header(line: &str, line_number: usize) -> Result<Scenario, ParseError> {
    let content = require_open_brace(line, line_number, "scenario")?;
    let fields: Vec<&str> = content.split_whitespace().collect();

    if fields.len() != 1 || fields[0].is_empty() {
        return Err(ParseError::new(line_number, "expected `scenario NAME {`"));
    }

    Ok(Scenario {
        name: fields[0].to_owned(),
        given: Vec::new(),
        when: None,
        requirements: Vec::new(),
    })
}

fn parse_scenario_line(
    scenario: &mut Scenario,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(expression) = line.strip_prefix("given ") {
        scenario
            .given
            .push(require_expression(expression, line_number, "given")?);
        return Ok(());
    }

    if let Some(action) = line.strip_prefix("when ") {
        if scenario.when.is_some() {
            return Err(ParseError::new(
                line_number,
                "scenario has more than one `when` clause",
            ));
        }
        scenario.when = Some(parse_when(action, line_number)?);
        return Ok(());
    }

    if let Some(expression) = line.strip_prefix("must_not ") {
        scenario.requirements.push(Requirement {
            kind: RequirementKind::MustNot,
            expression: require_expression(expression, line_number, "must_not")?,
        });
        return Ok(());
    }

    if let Some(expression) = line.strip_prefix("must ") {
        scenario.requirements.push(Requirement {
            kind: RequirementKind::Must,
            expression: require_expression(expression, line_number, "must")?,
        });
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected `given`, `when`, `must`, `must_not`, or `}` inside scenario",
    ))
}

fn parse_when(value: &str, line_number: usize) -> Result<WhenClause, ParseError> {
    let (method, path) = value
        .split_once(' ')
        .ok_or_else(|| ParseError::new(line_number, "expected `when METHOD \"/path\"`"))?;

    if method.is_empty() {
        return Err(ParseError::new(
            line_number,
            "`when` requires an HTTP method",
        ));
    }

    Ok(WhenClause {
        method: method.to_owned(),
        path: parse_quoted(path.trim(), line_number, "when path")?,
    })
}

fn require_open_brace<'a>(
    line: &'a str,
    line_number: usize,
    construct: &str,
) -> Result<&'a str, ParseError> {
    line.strip_suffix('{')
        .map(str::trim)
        .ok_or_else(|| ParseError::new(line_number, format!("{construct} must end with `{{`")))
}

fn parse_quoted(value: &str, line_number: usize, field: &str) -> Result<String, ParseError> {
    let value = value.trim();
    if value.len() < 2 || !value.starts_with('"') || !value.ends_with('"') {
        return Err(ParseError::new(
            line_number,
            format!("{field} must be a quoted string"),
        ));
    }

    let inner = &value[1..value.len() - 1];
    if inner.contains('"') {
        return Err(ParseError::new(
            line_number,
            format!("{field} contains an unsupported unescaped quote"),
        ));
    }
    Ok(inner.to_owned())
}

fn require_expression<'a>(
    expression: &'a str,
    line_number: usize,
    clause: &str,
) -> Result<String, ParseError> {
    let expression = expression.trim();
    if expression.is_empty() {
        return Err(ParseError::new(
            line_number,
            format!("{clause} requires an expression"),
        ));
    }
    Ok(expression.to_owned())
}

fn specification_mut(
    specification: &mut Option<Specification>,
    line_number: usize,
) -> Result<&mut Specification, ParseError> {
    specification.as_mut().ok_or_else(|| {
        ParseError::new(
            line_number,
            "scenario appeared before a specification header",
        )
    })
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParseError {
    pub line: usize,
    pub message: String,
}

impl ParseError {
    fn new(line: usize, message: impl Into<String>) -> Self {
        Self {
            line,
            message: message.into(),
        }
    }
}

impl fmt::Display for ParseError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(formatter, "line {}: {}", self.line, self.message)
    }
}

impl std::error::Error for ParseError {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ast::RequirementKind;

    #[test]
    fn parses_the_core_specification_shape() {
        let source = r#"
            # Comments and blank lines are allowed.
            spec document_api v1 {
              subject "documents"

              scenario submit_document {
                given document is valid
                when POST "/documents"
                must response.status == 202
                must response.body.document_id exists
                must_not response.body.error exists
              }
            }
        "#;

        let specification = parse(source).expect("valid Malcolm source");

        assert_eq!(specification.name, "document_api");
        assert_eq!(specification.version.as_deref(), Some("v1"));
        assert_eq!(specification.subject.as_deref(), Some("documents"));
        assert_eq!(specification.scenarios.len(), 1);

        let scenario = &specification.scenarios[0];
        assert_eq!(scenario.name, "submit_document");
        assert_eq!(scenario.given, ["document is valid"]);
        assert_eq!(
            scenario.when,
            Some(WhenClause {
                method: "POST".into(),
                path: "/documents".into(),
            })
        );
        assert_eq!(scenario.requirements.len(), 3);
        assert_eq!(scenario.requirements[2].kind, RequirementKind::MustNot);
    }

    #[test]
    fn reports_the_line_for_invalid_when_clause() {
        let error = parse("spec demo {\n scenario one {\n when GET\n }\n}").unwrap_err();

        assert_eq!(error.line, 3);
        assert!(error.message.contains("when METHOD"));
    }

    #[test]
    fn rejects_unknown_scenario_clauses() {
        let error = parse("spec demo {\n scenario one {\n then pass\n }\n}").unwrap_err();

        assert_eq!(error.line, 3);
        assert!(error.message.contains("given"));
    }

    #[test]
    fn rejects_unclosed_scenarios() {
        let error = parse("spec demo {\n scenario one {\n must true\n}").unwrap_err();

        assert_eq!(error.line, 4);
        assert!(error.message.contains("specification is missing"));
    }
}
