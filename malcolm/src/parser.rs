use std::fmt;

use crate::ast::{
    BodyField, CaptureClause, Literal, RequestClause, Requirement, RequirementKind, Scenario,
    SetupClause, Specification, WhenClause,
};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum BodyOwner {
    Scenario,
    Setup,
}

/// Parse a Malcolm specification from its declarative source text.
pub fn parse(source: &str) -> Result<Specification, ParseError> {
    let mut specification: Option<Specification> = None;
    let mut scenario: Option<Scenario> = None;
    let mut setup: Option<SetupClause> = None;
    let mut body_owner: Option<BodyOwner> = None;

    for (index, raw_line) in source.lines().enumerate() {
        let line_number = index + 1;
        let line = raw_line.trim();

        if line.is_empty() || line.starts_with('#') {
            continue;
        }

        if let Some(owner) = body_owner {
            if line == "}" {
                body_owner = None;
                continue;
            }

            match owner {
                BodyOwner::Scenario => {
                    let current = scenario.as_mut().expect("scenario owns request body");
                    let request = current
                        .request
                        .as_mut()
                        .expect("scenario request exists while parsing body");
                    parse_body_line(&mut request.body, line, line_number)?;
                }
                BodyOwner::Setup => {
                    let current = setup.as_mut().expect("setup owns request body");
                    let request = current
                        .request
                        .as_mut()
                        .expect("setup request exists while parsing body");
                    parse_body_line(&mut request.body, line, line_number)?;
                }
            }
            continue;
        }

        if let Some(current) = setup.as_mut() {
            if line == "}" {
                let finished = setup.take().expect("setup exists while parsing");
                scenario
                    .as_mut()
                    .expect("setup belongs to a scenario")
                    .setups
                    .push(finished);
                continue;
            }

            if line == "given body {" {
                if current.request.is_some() {
                    return Err(ParseError::new(
                        line_number,
                        "setup has more than one request body",
                    ));
                }
                current.request = Some(RequestClause { body: Vec::new() });
                body_owner = Some(BodyOwner::Setup);
                continue;
            }

            parse_setup_line(current, line, line_number)?;
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

            if let Some(header) = line.strip_prefix("setup ") {
                setup = Some(parse_setup_header(header, line_number)?);
                continue;
            }

            if line == "given body {" {
                if current.request.is_some() {
                    return Err(ParseError::new(
                        line_number,
                        "scenario has more than one request body",
                    ));
                }
                current.request = Some(RequestClause { body: Vec::new() });
                body_owner = Some(BodyOwner::Scenario);
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

    if body_owner.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "request body is missing a closing brace",
        ));
    }

    if setup.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "setup is missing a closing brace",
        ));
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
        state: None,
        request: None,
        when: None,
        setups: Vec::new(),
        requirements: Vec::new(),
    })
}

fn parse_setup_header(line: &str, line_number: usize) -> Result<SetupClause, ParseError> {
    let content = require_open_brace(line, line_number, "setup")?;
    let fields: Vec<&str> = content.split_whitespace().collect();

    if fields.len() != 1 || !is_identifier(fields[0]) {
        return Err(ParseError::new(line_number, "expected setup NAME {"));
    }

    Ok(SetupClause {
        name: fields[0].to_owned(),
        request: None,
        when: None,
        requirements: Vec::new(),
        captures: Vec::new(),
    })
}

fn parse_scenario_line(
    scenario: &mut Scenario,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(state) = line.strip_prefix("state ") {
        if scenario.state.is_some() {
            return Err(ParseError::new(
                line_number,
                "scenario has more than one state clause",
            ));
        }
        scenario.state = Some(parse_identifier(state, line_number, "state")?);
        return Ok(());
    }

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

fn parse_setup_line(
    setup: &mut SetupClause,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(action) = line.strip_prefix("when ") {
        if setup.when.is_some() {
            return Err(ParseError::new(
                line_number,
                "setup has more than one when clause",
            ));
        }
        setup.when = Some(parse_when(action, line_number)?);
        return Ok(());
    }

    if let Some(expression) = line.strip_prefix("must_not ") {
        setup.requirements.push(Requirement {
            kind: RequirementKind::MustNot,
            expression: require_expression(expression, line_number, "must_not")?,
        });
        return Ok(());
    }

    if let Some(expression) = line.strip_prefix("must ") {
        setup.requirements.push(Requirement {
            kind: RequirementKind::Must,
            expression: require_expression(expression, line_number, "must")?,
        });
        return Ok(());
    }

    if let Some(capture) = line.strip_prefix("capture ") {
        setup.captures.push(parse_capture(capture, line_number)?);
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected given body, when, must, must_not, capture, or } inside setup",
    ))
}

fn parse_body_line(
    fields: &mut Vec<BodyField>,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    let (name, raw_value) = line.split_once('=').ok_or_else(|| {
        ParseError::new(line_number, "expected FIELD = VALUE inside request body")
    })?;
    let name = name.trim();
    if !is_identifier(name) {
        return Err(ParseError::new(
            line_number,
            "request body field name must be an identifier",
        ));
    }
    if fields.iter().any(|field| field.name == name) {
        return Err(ParseError::new(
            line_number,
            format!("request body field {name} is duplicated"),
        ));
    }

    fields.push(BodyField {
        name: name.to_owned(),
        value: parse_literal(raw_value.trim(), line_number)?,
    });
    Ok(())
}

fn parse_capture(value: &str, line_number: usize) -> Result<CaptureClause, ParseError> {
    let (name, selector) = value.split_once('=').ok_or_else(|| {
        ParseError::new(line_number, "expected capture NAME = response.body.FIELD")
    })?;
    let name = parse_identifier(name.trim(), line_number, "capture name")?;
    let selector = selector.trim();
    let field = selector.strip_prefix("response.body.").ok_or_else(|| {
        ParseError::new(
            line_number,
            "capture selector must start with response.body.",
        )
    })?;
    let field = parse_identifier(field, line_number, "capture selector field")?;

    Ok(CaptureClause {
        name,
        selector: format!("body.{field}"),
    })
}

fn parse_literal(value: &str, line_number: usize) -> Result<Literal, ParseError> {
    let mut parser = LiteralParser {
        input: value,
        position: 0,
        line_number,
    };
    let literal = parser.parse_value()?;
    parser.skip_whitespace();
    if !parser.at_end() {
        return Err(ParseError::new(
            line_number,
            "request body value contains trailing characters",
        ));
    }
    Ok(literal)
}

struct LiteralParser<'a> {
    input: &'a str,
    position: usize,
    line_number: usize,
}

impl<'a> LiteralParser<'a> {
    fn parse_value(&mut self) -> Result<Literal, ParseError> {
        self.skip_whitespace();
        match self.peek() {
            Some('"') => Ok(Literal::String(self.parse_string()?)),
            Some('[') => self.parse_array(),
            Some('{') => self.parse_object(),
            Some(_) => self.parse_scalar(),
            None => Err(self.error("request body value cannot be empty")),
        }
    }

    fn parse_array(&mut self) -> Result<Literal, ParseError> {
        self.consume('[')?;
        let mut values = Vec::new();
        self.skip_whitespace();
        if self.consume_if(']') {
            return Ok(Literal::Array(values));
        }

        loop {
            values.push(self.parse_value()?);
            self.skip_whitespace();
            if self.consume_if(']') {
                break;
            }
            self.consume(',')?;
        }
        Ok(Literal::Array(values))
    }

    fn parse_object(&mut self) -> Result<Literal, ParseError> {
        self.consume('{')?;
        let mut fields = Vec::new();
        self.skip_whitespace();
        if self.consume_if('}') {
            return Ok(Literal::Object(fields));
        }

        loop {
            let name = self.parse_object_key()?;
            self.skip_whitespace();
            self.consume(':')?;
            let value = self.parse_value()?;
            if fields.iter().any(|field: &BodyField| field.name == name) {
                return Err(self.error(format!("nested request body field {name} is duplicated")));
            }
            fields.push(BodyField { name, value });
            self.skip_whitespace();
            if self.consume_if('}') {
                break;
            }
            self.consume(',')?;
        }
        Ok(Literal::Object(fields))
    }

    fn parse_object_key(&mut self) -> Result<String, ParseError> {
        self.skip_whitespace();
        let key = if self.peek() == Some('"') {
            self.parse_string()?
        } else {
            let start = self.position;
            while matches!(self.peek(), Some(character) if character == '_' || character.is_ascii_alphanumeric())
            {
                self.advance();
            }
            self.input[start..self.position].to_owned()
        };
        if !is_identifier(&key) {
            return Err(self.error("nested request body field name must be an identifier"));
        }
        Ok(key)
    }

    fn parse_scalar(&mut self) -> Result<Literal, ParseError> {
        let start = self.position;
        while matches!(self.peek(), Some(character) if !character.is_whitespace() && !matches!(character, ',' | ']' | '}'))
        {
            self.advance();
        }
        let value = &self.input[start..self.position];
        match value {
            "true" => Ok(Literal::Boolean(true)),
            "false" => Ok(Literal::Boolean(false)),
            _ => value.parse::<i64>().map(Literal::Integer).map_err(|_| {
                self.error("request body value must be true, false, an integer, a quoted string, an object, or an array")
            }),
        }
    }

    fn parse_string(&mut self) -> Result<String, ParseError> {
        self.consume('"')?;
        let start = self.position;
        while let Some(character) = self.peek() {
            if character == '"' {
                let value = self.input[start..self.position].to_owned();
                self.advance();
                return Ok(value);
            }
            if character.is_control() {
                return Err(self.error("request body string contains a control character"));
            }
            self.advance();
        }
        Err(self.error("request body string is missing a closing quote"))
    }

    fn consume(&mut self, expected: char) -> Result<(), ParseError> {
        if self.consume_if(expected) {
            Ok(())
        } else {
            Err(self.error(format!("request body value expected {expected}")))
        }
    }

    fn consume_if(&mut self, expected: char) -> bool {
        if self.peek() == Some(expected) {
            self.advance();
            true
        } else {
            false
        }
    }

    fn skip_whitespace(&mut self) {
        while matches!(self.peek(), Some(character) if character.is_whitespace()) {
            self.advance();
        }
    }

    fn peek(&self) -> Option<char> {
        self.input[self.position..].chars().next()
    }

    fn advance(&mut self) -> Option<char> {
        let character = self.peek()?;
        self.position += character.len_utf8();
        Some(character)
    }

    fn at_end(&self) -> bool {
        self.position == self.input.len()
    }

    fn error(&self, message: impl Into<String>) -> ParseError {
        ParseError::new(self.line_number, message)
    }
}

fn parse_identifier(value: &str, line_number: usize, field: &str) -> Result<String, ParseError> {
    let value = value.trim();
    if !is_identifier(value) {
        return Err(ParseError::new(
            line_number,
            format!("{field} must be an identifier"),
        ));
    }
    Ok(value.to_owned())
}

fn is_identifier(value: &str) -> bool {
    let mut characters = value.chars();
    matches!(characters.next(), Some(character) if character == '_' || character.is_ascii_alphabetic())
        && characters.all(|character| character == '_' || character.is_ascii_alphanumeric())
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
    fn parses_request_body_stateful_setup_and_capture() {
        let source = r#"
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
        "#;

        let specification = parse(source).expect("request source should parse");
        let scenario = &specification.scenarios[0];

        assert_eq!(scenario.state.as_deref(), Some("document_accepted"));
        assert_eq!(
            scenario.request.as_ref().expect("request body").body,
            vec![
                BodyField {
                    name: "name".into(),
                    value: Literal::String("welcome.md".into()),
                },
                BodyField {
                    name: "count".into(),
                    value: Literal::Integer(3),
                },
                BodyField {
                    name: "published".into(),
                    value: Literal::Boolean(true),
                },
            ]
        );
        assert_eq!(scenario.setups.len(), 1);
        assert_eq!(scenario.setups[0].name, "accept_document");
        assert_eq!(
            scenario.setups[0].captures,
            vec![CaptureClause {
                name: "document_id".into(),
                selector: "body.id".into(),
            }]
        );
    }

    #[test]
    fn parses_nested_object_and_array_literals() {
        let source = r#"
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
        "#;

        let specification = parse(source).expect("nested request source should parse");
        let body = &specification.scenarios[0]
            .request
            .as_ref()
            .expect("request body")
            .body;

        assert_eq!(
            body[0],
            BodyField {
                name: "metadata".into(),
                value: Literal::Object(vec![
                    BodyField {
                        name: "source".into(),
                        value: Literal::String("import".into()),
                    },
                    BodyField {
                        name: "priority".into(),
                        value: Literal::Integer(2),
                    },
                    BodyField {
                        name: "reviewed".into(),
                        value: Literal::Boolean(true),
                    },
                ]),
            }
        );
        assert_eq!(
            body[1],
            BodyField {
                name: "tags".into(),
                value: Literal::Array(vec![
                    Literal::String("docs".into()),
                    Literal::String("contract".into()),
                ]),
            }
        );
    }

    #[test]
    fn rejects_duplicate_request_body_fields() {
        let error = parse(
            "spec demo {\n scenario one {\n given body {\n name = \"a\"\n name = \"b\"\n }\n }\n}",
        )
        .unwrap_err();

        assert_eq!(error.line, 5);
        assert!(error.message.contains("duplicated"));
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
