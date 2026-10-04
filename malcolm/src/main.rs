use std::env;
use std::fmt;
use std::fs;
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};

use malcolm::{compile, parse, ParseError, ValidationError};

const HELP: &str = "Usage: malcolm <INPUT> [-o <OUTPUT>]\n       malcolm [--version | version [--format text|json]]\n\nParse and validate a Malcolm specification, then emit malcolm.ir/v1 JSON.\n\nINPUT may be a file path or - to read from stdin. Without -o, JSON is written to stdout.\n\nOptions:\n  -o, --output PATH  Write JSON to PATH instead of stdout\n  -h, --help         Show this help\n";

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
    let arguments: Vec<String> = args.into_iter().collect();
    if arguments.first().map(String::as_str) == Some("--version") {
        if arguments.len() != 1 {
            return Err(CliError::usage("--version does not accept arguments"));
        }
        return write_version("text", stdout);
    }
    if arguments.first().map(String::as_str) == Some("version") {
        let format = match arguments.as_slice() {
            [_] => "text",
            [_, flag, format] if flag == "--format" || flag == "-f" => format.as_str(),
            _ => return Err(CliError::usage("version accepts only --format text|json")),
        };
        return write_version(format, stdout);
    }

    let options = match parse_args(arguments)? {
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

fn write_version<W: Write>(format: &str, stdout: &mut W) -> Result<(), CliError> {
    let version = option_env!("INGEN_VERSION").unwrap_or("dev");
    let revision = option_env!("INGEN_REVISION").unwrap_or("unknown");
    let build_date = option_env!("INGEN_BUILD_DATE").unwrap_or("unknown");
    let source_inputs_sha256 = option_env!("INGEN_SOURCE_INPUTS_SHA256").unwrap_or("unknown");
    let dirty = option_env!("INGEN_DIRTY").unwrap_or("unknown");
    let toolchain = option_env!("INGEN_TOOLCHAIN").unwrap_or("unknown");
    let goos = goos_name(std::env::consts::OS);
    let goarch = goarch_name(std::env::consts::ARCH);
    let name = env!("CARGO_PKG_NAME");
    match format {
        "text" => {
            let text = format!(
                "{name} {version}\ncommit {revision}\nbuilt {build_date}\nrevision {revision}\nsource_inputs_sha256 {source_inputs_sha256}\ndirty {dirty}\nplatform {goos}/{goarch}\ntoolchain {toolchain}\n"
            );
            stdout.write_all(text.as_bytes()).map_err(CliError::output)
        }
        "json" => {
            let json = format!(
                "{{\n  \"schema\": \"ingen.tool-version/v1\",\n  \"name\": {},\n  \"version\": {},\n  \"revision\": {},\n  \"commit\": {},\n  \"build_date\": {},\n  \"source_inputs_sha256\": {},\n  \"dirty\": {},\n  \"goos\": {},\n  \"goarch\": {},\n  \"toolchain\": {}\n}}\n",
                json_string(name), json_string(version), json_string(revision), json_string(revision),
                json_string(build_date), json_string(source_inputs_sha256), json_string(dirty),
                json_string(goos), json_string(goarch), json_string(toolchain),
            );
            stdout.write_all(json.as_bytes()).map_err(CliError::output)
        }
        _ => Err(CliError::usage(format!(
            "unsupported version format {format:?}; use text or json"
        ))),
    }
}

fn json_string(value: &str) -> String {
    let mut out = String::from("\"");
    for character in value.chars() {
        match character {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if c < ' ' => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

fn goos_name(os: &str) -> &'static str {
    match os {
        "macos" => "darwin",
        "windows" => "windows",
        "linux" => "linux",
        "freebsd" => "freebsd",
        "android" => "android",
        "ios" => "ios",
        "illumos" => "illumos",
        "solaris" => "solaris",
        _ => "unknown",
    }
}

fn goarch_name(arch: &str) -> &'static str {
    match arch {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        "x86" => "386",
        "arm" => "arm",
        "powerpc64" => "ppc64",
        "powerpc64le" => "ppc64le",
        "s390x" => "s390x",
        "riscv64" => "riscv64",
        _ => "unknown",
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
            "contract.malc".into(),
            "--output".into(),
            "contract.json".into(),
        ])
        .expect("arguments should parse")
        .expect("help was not requested");

        assert_eq!(
            options,
            Options {
                input: PathBuf::from("contract.malc"),
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
    fn version_json_uses_the_shared_tool_schema() {
        let mut output = Vec::new();
        run(
            ["version".into(), "--format".into(), "json".into()],
            &mut output,
        )
        .expect("version JSON should succeed");
        let output = String::from_utf8(output).expect("version JSON should be UTF-8");
        assert!(output.contains("\"schema\": \"ingen.tool-version/v1\""));
        assert!(output.contains("\"name\": \"malcolm\""));
        assert!(output.contains("\"revision\": \"unknown\""));
        assert!(output.contains("\"commit\": \"unknown\""));
        assert!(output.contains("\"source_inputs_sha256\": \"unknown\""));
        assert!(output.contains("\"dirty\": \"unknown\""));
        assert!(output.contains(&format!(
            "\"goos\": \"{}\"",
            goos_name(std::env::consts::OS)
        )));
        assert!(output.contains(&format!(
            "\"goarch\": \"{}\"",
            goarch_name(std::env::consts::ARCH)
        )));
        assert!(output.contains("\"toolchain\": \"unknown\""));
    }

    #[test]
    fn version_flags_are_strict_and_text_is_supported() {
        let mut output = Vec::new();
        run(["--version".into()], &mut output).expect("text version should succeed");
        assert!(String::from_utf8(output)
            .expect("text version should be UTF-8")
            .starts_with("malcolm dev\n"));

        for arguments in [
            vec!["version".into(), "--format".into()],
            vec!["version".into(), "--format".into(), "yaml".into()],
            vec!["version".into(), "extra".into()],
            vec!["--version".into(), "--format".into(), "json".into()],
        ] {
            let error =
                run(arguments, &mut Vec::new()).expect_err("invalid version args must fail");
            assert!(matches!(error, CliError::Usage(_)));
        }
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
            "malcolm-cli-{label}-{}-{nonce}.malc",
            std::process::id()
        ))
    }
}
