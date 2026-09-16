use std::env;
use std::fmt;
use std::fs;
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};

use malcolm::{compile, parse, ParseError, ValidationError};

const HELP: &str = "Usage: malcolm <INPUT> [-o <OUTPUT>]\n\nParse and validate a Malcolm specification, then emit malcolm.ir/v1 JSON.\n\nINPUT may be a file path or - to read from stdin. Without -o, JSON is written to stdout.\n\nOptions:\n  -o, --output PATH  Write JSON to PATH instead of stdout\n  -h, --help         Show this help\n";

fn main() {
    if let Err(error) = run(env::args().skip(1), &mut io::stdout()) {
        eprintln!("error: {error}");
        std::process::exit(1);
    }
}

fn run<I, W>(args: I, stdout: &mut W) -> Result<(), CliError>
where
    I: IntoIterator<Item = String>,
    W: Write,
{
    let options = match parse_args(args)? {
        Some(options) => options,
        None => {
            stdout
                .write_all(HELP.as_bytes())
                .map_err(CliError::output)?;
            return Ok(());
        }
    };

    let source = read_source(&options.input)?;
    let specification = parse(&source).map_err(CliError::Parse)?;
    let ir = compile(&specification).map_err(CliError::Validation)?;

    let mut json = ir.to_json();
    json.push('\n');

    match options.output {
        Some(path) => fs::write(&path, json).map_err(|source| CliError::Io {
            operation: "write",
            path,
            source,
        }),
        None => stdout.write_all(json.as_bytes()).map_err(CliError::output),
    }
}

#[derive(Debug, PartialEq, Eq)]
struct Options {
    input: PathBuf,
    output: Option<PathBuf>,
}

fn parse_args<I>(args: I) -> Result<Option<Options>, CliError>
where
    I: IntoIterator<Item = String>,
{
    let mut arguments = args.into_iter();
    let mut input = None;
    let mut output = None;

    while let Some(argument) = arguments.next() {
        match argument.as_str() {
            "-h" | "--help" => return Ok(None),
            "-o" | "--output" => {
                let value = arguments
                    .next()
                    .ok_or_else(|| CliError::usage("an output path is required after --output"))?;
                output = Some(PathBuf::from(value));
            }
            "-" => set_input(&mut input, PathBuf::from(argument))?,
            _ if argument.starts_with('-') => {
                return Err(CliError::usage(format!("unknown option {argument}")))
            }
            _ => set_input(&mut input, PathBuf::from(argument))?,
        }
    }

    let input = input.ok_or_else(|| CliError::usage("an input path is required"))?;
    Ok(Some(Options { input, output }))
}

fn set_input(input: &mut Option<PathBuf>, value: PathBuf) -> Result<(), CliError> {
    if input.is_some() {
        return Err(CliError::usage("only one input path is allowed"));
    }
    *input = Some(value);
    Ok(())
}

fn read_source(path: &Path) -> Result<String, CliError> {
    if path == Path::new("-") {
        let mut source = String::new();
        io::stdin()
            .read_to_string(&mut source)
            .map_err(|source| CliError::Io {
                operation: "read",
                path: PathBuf::from("<stdin>"),
                source,
            })?;
        return Ok(source);
    }

    fs::read_to_string(path).map_err(|source| CliError::Io {
        operation: "read",
        path: path.to_owned(),
        source,
    })
}

#[derive(Debug)]
enum CliError {
    Usage(String),
    Io {
        operation: &'static str,
        path: PathBuf,
        source: io::Error,
    },
    Parse(ParseError),
    Validation(Vec<ValidationError>),
    Output(io::Error),
}

impl CliError {
    fn usage(message: impl Into<String>) -> Self {
        Self::Usage(message.into())
    }

    fn output(source: io::Error) -> Self {
        Self::Output(source)
    }
}

impl fmt::Display for CliError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Usage(message) => write!(formatter, "{message}\n\n{HELP}"),
            Self::Io {
                operation,
                path,
                source,
            } => write!(
                formatter,
                "could not {operation} {}: {source}",
                path.display()
            ),
            Self::Parse(error) => write!(formatter, "parse failed: {error}"),
            Self::Validation(errors) => {
                write!(formatter, "validation failed:")?;
                for error in errors {
                    write!(formatter, "\n  {error}")?;
                }
                Ok(())
            }
            Self::Output(error) => write!(formatter, "could not write stdout: {error}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_input_and_output_options() {
        let options = parse_args([
            "contract.malcolm".into(),
            "--output".into(),
            "contract.json".into(),
        ])
        .expect("arguments should parse")
        .expect("help was not requested");

        assert_eq!(
            options,
            Options {
                input: PathBuf::from("contract.malcolm"),
                output: Some(PathBuf::from("contract.json")),
            }
        );
    }

    #[test]
    fn accepts_stdin_as_the_input_path() {
        let options = parse_args(["-".into()])
            .expect("arguments should parse")
            .expect("help was not requested");

        assert_eq!(options.input, PathBuf::from("-"));
    }

    #[test]
    fn reports_missing_input() {
        let error = parse_args(std::iter::empty()).expect_err("input should be required");

        assert!(matches!(error, CliError::Usage(message) if message.contains("input path")));
    }

    #[test]
    fn help_is_requested_without_parsing_a_contract() {
        assert_eq!(parse_args(["--help".into()]).expect("help is valid"), None);
    }

    #[test]
    fn runs_a_contract_to_stdout() {
        let path = test_path("valid");
        fs::write(
            &path,
            "spec demo v1 {\n subject \"items\"\n scenario list {\n when GET \"/items\"\n must response.status == 200\n }\n}\n",
        )
        .expect("test input should be writable");

        let mut output = Vec::new();
        run([path.to_string_lossy().into_owned()], &mut output).expect("CLI should succeed");
        let output = String::from_utf8(output).expect("JSON output should be UTF-8");

        assert!(output.starts_with(r#"{"schema":"malcolm.ir/v1""#));
        assert!(output.ends_with('\n'));
        fs::remove_file(path).expect("test input should be removable");
    }

    fn test_path(label: &str) -> PathBuf {
        let nonce = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system clock should be after the Unix epoch")
            .as_nanos();
        env::temp_dir().join(format!(
            "malcolm-cli-{label}-{}-{nonce}.malcolm",
            std::process::id()
        ))
    }
}
