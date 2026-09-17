use std::fmt;

use crate::ast::{
    BodyField, CaptureClause, FixtureClause, FixtureOwner, IsolationClause, IsolationScope,
    Literal, MutationChange, MutationClause, ProvenanceClause, ProvenanceField,
    ProvenanceRequirement, ProvenanceRequirementKind, RequestClause, Requirement, RequirementKind,
    ResetClause, Scenario, SetupClause, Specification, WhenClause,
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
    let mut mutation: Option<MutationClause> = None;
    let mut fixture: Option<FixtureClause> = None;
    let mut provenance_clause: Option<ProvenanceClause> = None;
    let mut isolation_clause: Option<IsolationClause> = None;
    let mut body_owner: Option<BodyOwner> = None;
    let mut pending_body_value: Option<(String, usize)> = None;

    for (index, raw_line) in source.lines().enumerate() {
        let line_number = index + 1;
        let line = raw_line.trim();

        if line.is_empty() || line.starts_with('#') {
            continue;
        }

        if let Some(owner) = body_owner {
            if let Some((mut value, start_line)) = pending_body_value.take() {
                value.push(' ');
                value.push_str(line);
                if body_value_is_complete(&value) {
                    match owner {
                        BodyOwner::Scenario => {
                            let current = scenario.as_mut().expect("scenario owns request body");
                            let request = current
                                .request
                                .as_mut()
                                .expect("scenario request exists while parsing body");
                            parse_body_line(&mut request.body, &value, start_line)?;
                        }
                        BodyOwner::Setup => {
                            let current = setup.as_mut().expect("setup owns request body");
                            let request = current
                                .request
                                .as_mut()
                                .expect("setup request exists while parsing body");
                            parse_body_line(&mut request.body, &value, start_line)?;
                        }
                    }
                } else {
                    pending_body_value = Some((value, start_line));
                }
                continue;
            }

            if line == "}" {
                body_owner = None;
                continue;
            }

            if body_value_is_complete(line) {
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
            } else {
                pending_body_value = Some((line.to_owned(), line_number));
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

        if let Some(current) = mutation.as_mut() {
            if line == "}" {
                let finished = mutation.take().expect("mutation exists while parsing");
                specification_mut(&mut specification, line_number)?
                    .mutations
                    .push(finished);
                continue;
            }

            parse_mutation_line(current, line, line_number)?;
            continue;
        }

        if let Some(current) = fixture.as_mut() {
            if line == "}" {
                let finished = fixture.take().expect("fixture exists while parsing");
                specification_mut(&mut specification, line_number)?
                    .fixtures
                    .push(finished);
                continue;
            }

            parse_fixture_line(current, line, line_number)?;
            continue;
        }

        if let Some(current) = provenance_clause.as_mut() {
            if line == "}" {
                let finished = provenance_clause
                    .take()
                    .expect("provenance clause exists while parsing");
                specification_mut(&mut specification, line_number)?.provenance = Some(finished);
                continue;
            }

            parse_provenance_line(current, line, line_number)?;
            continue;
        }

        if let Some(current) = isolation_clause.as_mut() {
            if line == "}" {
                let finished = isolation_clause
                    .take()
                    .expect("isolation clause exists while parsing");
                specification_mut(&mut specification, line_number)?.isolation = Some(finished);
                continue;
            }

            parse_isolation_line(current, line, line_number)?;
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

            if let Some(header) = line.strip_prefix("mutation ") {
                mutation = Some(parse_mutation_header(header, line_number)?);
                continue;
            }

            if let Some(header) = line.strip_prefix("fixture ") {
                fixture = Some(parse_fixture_header(header, line_number)?);
                continue;
            }

            if line == "provenance {" {
                if current.provenance.is_some() {
                    return Err(ParseError::new(
                        line_number,
                        "specification has more than one provenance clause",
                    ));
                }
                provenance_clause = Some(ProvenanceClause {
                    requirements: Vec::new(),
                });
                continue;
            }

            if line == "isolation per scenario {" {
                if current.isolation.is_some() {
                    return Err(ParseError::new(
                        line_number,
                        "specification has more than one isolation clause",
                    ));
                }
                isolation_clause = Some(IsolationClause {
                    scope: IsolationScope::Scenario,
                    reset: None,
                });
                continue;
            }

            return Err(ParseError::new(
                line_number,
                "expected `subject`, `scenario`, `fixture`, `mutation`, `provenance`, `isolation per scenario`, or `}` inside specification",
            ));
        }

        if specification.is_none() {
            specification = Some(parse_spec_header(line, line_number)?);
        }
    }

    if let Some((_, line_number)) = pending_body_value {
        return Err(ParseError::new(
            line_number,
            "request body literal is missing a closing delimiter",
        ));
    }

    if mutation.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "mutation is missing a closing `}`",
        ));
    }

    if fixture.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "fixture is missing a closing brace",
        ));
    }

    if provenance_clause.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "provenance is missing a closing brace",
        ));
    }

    if isolation_clause.is_some() {
        return Err(ParseError::new(
            source.lines().count().max(1),
            "isolation is missing a closing brace",
        ));
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
        fixtures: Vec::new(),
        mutations: Vec::new(),
        provenance: None,
        isolation: None,
    })
}

fn parse_mutation_header(line: &str, line_number: usize) -> Result<MutationClause, ParseError> {
    let content = require_open_brace(line, line_number, "mutation")?;
    Ok(MutationClause {
        id: parse_quoted(content, line_number, "mutation id")?,
        scenario: None,
        change: None,
        expected_rule: None,
    })
}

fn parse_fixture_header(line: &str, line_number: usize) -> Result<FixtureClause, ParseError> {
    let content = require_open_brace(line, line_number, "fixture")?;
    Ok(FixtureClause {
        id: parse_quoted(content, line_number, "fixture id")?,
        owner: None,
        purpose: None,
        sha256: None,
    })
}

fn parse_fixture_line(
    fixture: &mut FixtureClause,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(value) = line.strip_prefix("owner ") {
        if fixture.owner.is_some() {
            return Err(ParseError::new(
                line_number,
                "fixture has more than one owner clause",
            ));
        }
        let owner = parse_identifier(value, line_number, "fixture owner")?;
        if owner != "oracle" {
            return Err(ParseError::new(line_number, "fixture owner must be oracle"));
        }
        fixture.owner = Some(FixtureOwner::Oracle);
        return Ok(());
    }

    if let Some(value) = line.strip_prefix("purpose ") {
        if fixture.purpose.is_some() {
            return Err(ParseError::new(
                line_number,
                "fixture has more than one purpose clause",
            ));
        }
        fixture.purpose = Some(parse_quoted(value, line_number, "fixture purpose")?);
        return Ok(());
    }

    if let Some(value) = line.strip_prefix("sha256 ") {
        if fixture.sha256.is_some() {
            return Err(ParseError::new(
                line_number,
                "fixture has more than one sha256 clause",
            ));
        }
        fixture.sha256 = Some(parse_quoted(value, line_number, "fixture sha256")?);
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected owner oracle, purpose \"...\", sha256 \"...\", or } inside fixture",
    ))
}

fn parse_mutation_line(
    mutation: &mut MutationClause,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(value) = line.strip_prefix("target ") {
        if mutation.scenario.is_some() {
            return Err(ParseError::new(
                line_number,
                "mutation has more than one target clause",
            ));
        }
        mutation.scenario = Some(parse_identifier(
            value,
            line_number,
            "mutation target scenario",
        )?);
        return Ok(());
    }

    if let Some(value) = line.strip_prefix("change ") {
        if mutation.change.is_some() {
            return Err(ParseError::new(
                line_number,
                "mutation has more than one change clause",
            ));
        }
        let fields: Vec<&str> = value.split_whitespace().collect();
        if fields.len() != 5
            || fields[0] != "response.status"
            || fields[1] != "from"
            || fields[3] != "to"
        {
            return Err(ParseError::new(
                line_number,
                "expected `change response.status from STATUS to STATUS`",
            ));
        }
        mutation.change = Some(MutationChange {
            field: fields[0].to_owned(),
            from: parse_integer(fields[2], line_number, "mutation source status")?,
            to: parse_integer(fields[4], line_number, "mutation target status")?,
        });
        return Ok(());
    }

    if let Some(value) = line.strip_prefix("expect rule ") {
        if mutation.expected_rule.is_some() {
            return Err(ParseError::new(
                line_number,
                "mutation has more than one expected-rule clause",
            ));
        }
        let rule = value.strip_suffix(" to fail").unwrap_or_default().trim();
        if rule.is_empty() {
            return Err(ParseError::new(
                line_number,
                "expected `expect rule SCENARIO.requirement.N to fail`",
            ));
        }
        mutation.expected_rule = Some(rule.to_owned());
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected target, change, expect rule, or } inside mutation",
    ))
}

fn parse_provenance_line(
    provenance: &mut ProvenanceClause,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    let fields: Vec<&str> = line.split_whitespace().collect();
    if fields.len() == 3
        && fields[0] == "must"
        && fields[1] == "create"
        && fields[2] == "execution_id"
    {
        let requirement = ProvenanceRequirement {
            kind: ProvenanceRequirementKind::Create,
            field: ProvenanceField::ExecutionId,
        };
        if provenance.requirements.contains(&requirement) {
            return Err(ParseError::new(
                line_number,
                "provenance has more than one must create execution_id clause",
            ));
        }
        provenance.requirements.push(requirement);
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected must create execution_id or closing brace inside provenance",
    ))
}

fn parse_isolation_line(
    isolation: &mut IsolationClause,
    line: &str,
    line_number: usize,
) -> Result<(), ParseError> {
    if let Some(value) = line.strip_prefix("reset ") {
        if isolation.reset.is_some() {
            return Err(ParseError::new(
                line_number,
                "isolation has more than one reset clause",
            ));
        }
        let (method, path) = value
            .split_once(' ')
            .ok_or_else(|| ParseError::new(line_number, "expected `reset METHOD \"/path\"`"))?;
        if method != "POST" {
            return Err(ParseError::new(
                line_number,
                "isolation reset must use POST",
            ));
        }
        isolation.reset = Some(ResetClause {
            method: method.to_owned(),
            path: parse_quoted(path.trim(), line_number, "isolation reset path")?,
        });
        return Ok(());
    }

    Err(ParseError::new(
        line_number,
        "expected reset POST \"/path\" or closing brace inside isolation",
    ))
}

fn parse_integer(value: &str, line_number: usize, field: &str) -> Result<i64, ParseError> {
    value
        .parse::<i64>()
        .map_err(|_| ParseError::new(line_number, format!("{field} must be an integer")))
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

fn body_value_is_complete(value: &str) -> bool {
    let mut expected_closers = Vec::new();
    let mut quoted = false;
    let mut escaped = false;

    for character in value.chars() {
        if quoted {
            if escaped {
                escaped = false;
            } else if character == '\\' {
                escaped = true;
            } else if character == '"' {
                quoted = false;
            }
            continue;
        }

        if character == '"' {
            quoted = true;
            continue;
        }

        match character {
            '{' => expected_closers.push('}'),
            '[' => expected_closers.push(']'),
            '}' | ']' => {
                if expected_closers.last() == Some(&character) {
                    expected_closers.pop();
                } else {
                    return true;
                }
            }
            _ => {}
        }
    }

    quoted || expected_closers.is_empty()
}

fn parse_capture(value: &str, line_number: usize) -> Result<CaptureClause, ParseError> {
    let (name, selector) = value.split_once('=').ok_or_else(|| {
        ParseError::new(line_number, "expected capture NAME = response.body.FIELD")
    })?;
    let name = parse_identifier(name.trim(), line_number, "capture name")?;
    let selector = selector.trim();
    let path = selector.strip_prefix("response.body.").ok_or_else(|| {
        ParseError::new(
            line_number,
            "capture selector must start with response.body.",
        )
    })?;
    let path = parse_selector_path(path, line_number)?;

    Ok(CaptureClause {
        name,
        selector: format!("body.{path}"),
    })
}

fn parse_selector_path(value: &str, line_number: usize) -> Result<String, ParseError> {
    if value.split('.').all(is_identifier) {
        Ok(value.to_owned())
    } else {
        Err(ParseError::new(
            line_number,
            "capture selector path must contain identifier-like fields separated by dots",
        ))
    }
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
            Some(character) if character.is_ascii_alphabetic() => {
                if self.input[self.position..].starts_with("repeat") {
                    self.parse_repeat()
                } else {
                    self.parse_scalar()
                }
            }
            Some(_) => self.parse_scalar(),
            None => Err(self.error("request body value cannot be empty")),
        }
    }

    fn parse_repeat(&mut self) -> Result<Literal, ParseError> {
        for expected in "repeat".chars() {
            self.consume(expected)?;
        }
        self.skip_whitespace();
        self.consume('(')?;
        self.skip_whitespace();
        if self.peek() != Some('"') {
            return Err(self.error("repeat generator value must be a quoted string"));
        }
        let value = self.parse_string()?;
        self.skip_whitespace();
        self.consume(',')?;
        self.skip_whitespace();
        let count = self.parse_repeat_count()?;
        self.skip_whitespace();
        self.consume(')')?;
        Ok(Literal::Repeat { value, count })
    }

    fn parse_repeat_count(&mut self) -> Result<i64, ParseError> {
        let start = self.position;
        while matches!(
            self.peek(),
            Some(character)
                if !character.is_whitespace() && !matches!(character, ',' | ']' | '}' | ')')
        ) {
            self.advance();
        }
        let value = &self.input[start..self.position];
        value
            .parse::<i64>()
            .map_err(|_| self.error("repeat generator count must be an integer"))
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
                self.error("request body value must be true, false, an integer, a quoted string, an object, an array, or repeat(\"text\", count)")
            }),
        }
    }

    fn parse_string(&mut self) -> Result<String, ParseError> {
        self.consume('"')?;
        let mut raw_value = String::new();
        while let Some(character) = self.peek() {
            if character == '"' {
                self.advance();
                return decode_quoted_content(&raw_value, self.line_number, "request body string");
            }
            if character == '\\' {
                raw_value.push(character);
                self.advance();
                if let Some(escaped) = self.peek() {
                    raw_value.push(escaped);
                    self.advance();
                    continue;
                }
                return Err(
                    self.error("request body string contains an incomplete escape sequence")
                );
            }
            if character.is_control() {
                return Err(self.error("request body string contains a control character"));
            }
            raw_value.push(character);
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
    decode_quoted_content(inner, line_number, field)
}

fn decode_quoted_content(
    value: &str,
    line_number: usize,
    field: &str,
) -> Result<String, ParseError> {
    let mut decoded = String::new();
    let mut characters = value.chars();

    while let Some(character) = characters.next() {
        if character == '"' {
            return Err(ParseError::new(
                line_number,
                format!("{field} contains an unsupported unescaped quote"),
            ));
        }

        if character == '\\' {
            let escaped = characters.next().ok_or_else(|| {
                ParseError::new(
                    line_number,
                    format!("{field} contains an incomplete escape sequence"),
                )
            })?;
            let decoded_character = match escaped {
                '"' => '"',
                '\\' => '\\',
                'b' => '\u{08}',
                'f' => '\u{0C}',
                'n' => '\n',
                'r' => '\r',
                't' => '\t',
                _ => {
                    return Err(ParseError::new(
                        line_number,
                        format!("{field} contains an unsupported escape sequence \\\\{escaped}"),
                    ))
                }
            };
            decoded.push(decoded_character);
            continue;
        }

        if character.is_control() {
            return Err(ParseError::new(
                line_number,
                format!("{field} contains a control character"),
            ));
        }
        decoded.push(character);
    }

    Ok(decoded)
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
    fn parses_escaped_quotes_and_backslashes() {
        let source = r#"
            spec document_api v1 {
              subject "document \"api\""
              scenario create_document {
                given body {
                  message = "say \"hello\"\\world\nnext"
                }
                when GET "/documents/a\\b"
                must response.status == 200
              }
            }
        "#;

        let specification = parse(source).expect("escaped quoted strings should parse");
        let scenario = &specification.scenarios[0];

        assert_eq!(specification.subject.as_deref(), Some("document \"api\""));
        assert_eq!(
            scenario.when.as_ref().expect("when clause").path,
            "/documents/a\\b"
        );
        assert_eq!(
            scenario.request.as_ref().expect("request body").body[0].value,
            Literal::String("say \"hello\"\\world\nnext".into())
        );
    }

    #[test]
    fn parses_multiline_nested_body_literals() {
        let source = r#"
            spec document_api v1 {
              scenario create_document {
                given body {
                  metadata = {
                    "source": "import }",
                    "labels": [
                      "docs",
                      "contract"
                    ]
                  }
                  tags = [
                    "nested",
                    "body"
                  ]
                }
                when POST "/documents"
                must response.status == 202
              }
            }
        "#;

        let specification = parse(source).expect("multiline body literals should parse");
        let body = &specification.scenarios[0]
            .request
            .as_ref()
            .expect("request body")
            .body;

        assert_eq!(
            body,
            &vec![
                BodyField {
                    name: "metadata".into(),
                    value: Literal::Object(vec![
                        BodyField {
                            name: "source".into(),
                            value: Literal::String("import }".into()),
                        },
                        BodyField {
                            name: "labels".into(),
                            value: Literal::Array(vec![
                                Literal::String("docs".into()),
                                Literal::String("contract".into()),
                            ]),
                        },
                    ]),
                },
                BodyField {
                    name: "tags".into(),
                    value: Literal::Array(vec![
                        Literal::String("nested".into()),
                        Literal::String("body".into()),
                    ]),
                },
            ]
        );
    }

    #[test]
    fn rejects_unsupported_string_escape() {
        let error = parse(
            "spec demo {\n scenario one {\n given body {\n name = \"bad\\q\"\n }\n when POST \"/documents\"\n must response.status == 400\n }\n}",
        )
        .unwrap_err();

        assert_eq!(error.line, 4);
        assert!(error.message.contains("unsupported escape sequence"));
    }

    #[test]
    fn reports_the_start_line_for_an_unclosed_multiline_literal() {
        let error = parse(
            "spec demo {\n scenario one {\n given body {\n metadata = {\n \"source\": \"import\"\n",
        )
        .unwrap_err();

        assert_eq!(error.line, 4);
        assert!(error.message.contains("missing a closing delimiter"));
    }

    #[test]
    fn parses_a_status_mutation_declaration() {
        let source = r#"
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
        "#;

        let specification = parse(source).expect("mutation declaration should parse");

        assert_eq!(
            specification.mutations,
            vec![MutationClause {
                id: "return-200".into(),
                scenario: Some("submit_document".into()),
                change: Some(MutationChange {
                    field: "response.status".into(),
                    from: 202,
                    to: 200,
                }),
                expected_rule: Some("submit_document.requirement.1".into()),
            }]
        );
    }

    #[test]
    fn parses_an_execution_id_provenance_declaration() {
        let source = r#"
            spec document_api v1 {
              provenance {
                must create execution_id
              }
              scenario submit_document {
                when POST "/documents"
                must response.status == 202
              }
            }
        "#;

        let specification = parse(source).expect("provenance declaration should parse");

        assert_eq!(
            specification.provenance,
            Some(ProvenanceClause {
                requirements: vec![ProvenanceRequirement {
                    kind: ProvenanceRequirementKind::Create,
                    field: ProvenanceField::ExecutionId,
                }],
            })
        );
    }

    #[test]
    fn parses_per_scenario_isolation_with_a_reset_request() {
        let source = r#"
            spec document_api v1 {
              isolation per scenario {
                reset POST "/__malcolm/reset"
              }
              scenario submit_document {
                when POST "/documents"
                must response.status == 202
              }
            }
        "#;

        let specification = parse(source).expect("isolation declaration should parse");

        assert_eq!(
            specification.isolation,
            Some(IsolationClause {
                scope: IsolationScope::Scenario,
                reset: Some(ResetClause {
                    method: "POST".into(),
                    path: "/__malcolm/reset".into(),
                }),
            })
        );
    }

    #[test]
    fn parses_a_digest_pinned_oracle_fixture() {
        let source = r#"
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
        "#;

        let specification = parse(source).expect("fixture declaration should parse");

        assert_eq!(specification.fixtures.len(), 1);
        assert_eq!(specification.fixtures[0].id, "welcome-document");
        assert_eq!(specification.fixtures[0].owner, Some(FixtureOwner::Oracle));
        assert_eq!(
            specification.fixtures[0].purpose.as_deref(),
            Some("canonical document input")
        );
        assert_eq!(
            specification.fixtures[0].sha256.as_deref(),
            Some("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
        );
    }

    #[test]
    fn parses_repeat_generator() {
        let source = r#"
            spec document_boundary v1 {
              scenario reject_oversized_document {
                given body {
                  content = repeat("a", 4097)
                }
                when POST "/documents"
                must response.status == 413
              }
            }
        "#;

        let specification = parse(source).expect("repeat generator source should parse");
        assert_eq!(
            specification.scenarios[0]
                .request
                .as_ref()
                .expect("request body")
                .body[0]
                .value,
            Literal::Repeat {
                value: "a".into(),
                count: 4097,
            }
        );
    }

    #[test]
    fn parses_nested_capture_selector_paths() {
        let source = r#"
            spec document_api v1 {
              scenario inspect_document {
                setup create_document {
                  given body {
                    name = "welcome.md"
                  }
                  when POST "/documents"
                  must response.status == 202
                  capture owner_id = response.body.metadata.owner.id
                }
                when GET "/documents"
                must response.status == 200
              }
            }
        "#;

        let specification = parse(source).expect("nested selector source should parse");

        assert_eq!(
            specification.scenarios[0].setups[0].captures[0].selector,
            "body.metadata.owner.id"
        );
    }

    #[test]
    fn rejects_invalid_nested_capture_selector_paths() {
        let error = parse(
            "spec demo v1 {\n scenario inspect {\n setup create {\n given body {\n name = \"a\"\n }\n when POST \"/documents\"\n must response.status == 202\n capture owner = response.body.metadata..id\n }\n when GET \"/documents\"\n must response.status == 200\n }\n}",
        )
        .unwrap_err();

        assert_eq!(error.line, 9);
        assert!(error.message.contains("identifier-like fields"));
    }

    #[test]
    fn rejects_repeat_generator_with_non_string_value() {
        let error = parse(
            "spec demo v1 {\n scenario one {\n given body {\n content = repeat(true, 2)\n }\n when POST \"/documents\"\n must response.status == 413\n }\n}",
        )
        .unwrap_err();

        assert_eq!(error.line, 4);
        assert!(error.message.contains("quoted string"));
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
