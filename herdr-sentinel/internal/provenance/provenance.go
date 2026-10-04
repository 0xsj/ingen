// Package provenance records coordinator command lineage using Amber's public
// SDK. Execution is noninteractive, non-isolated, and not a host attestation.
package provenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	amber "github.com/0xsj/ingen/amber"
)

const ReceiptSchema = "ingen.sentinel-provenance-execution/v1"
const maxInputBytes = 16 << 20

var operationPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
var shaPattern = regexp.MustCompile("^[0-9a-f]{64}$")

type Request struct {
	Root, ParentPath, OutputPath, ReceiptPath string
	ExpectedParentSHA256, Operation           string
	Command                                   []string
}

type Receipt struct {
	Schema           string   `json:"schema"`
	ExecutionID      string   `json:"execution_id"`
	WorkID           string   `json:"work_id"`
	ParentPath       string   `json:"parent_path"`
	ParentSHA256     string   `json:"parent_sha256"`
	ContextPath      string   `json:"context_path"`
	ContextSHA256    string   `json:"context_sha256"`
	ReceiptPath      string   `json:"receipt_path"`
	Operation        string   `json:"operation"`
	Argv             []string `json:"argv"`
	ExecutablePath   string   `json:"executable_path"`
	ExecutableSHA256 string   `json:"executable_sha256"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at"`
	Status           string   `json:"status"`
	ExitCode         *int     `json:"exit_code,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	StdoutPath       string   `json:"stdout_path"`
	StdoutSHA256     string   `json:"stdout_sha256,omitempty"`
	StderrPath       string   `json:"stderr_path"`
	StderrSHA256     string   `json:"stderr_sha256,omitempty"`
	Enforcement      string   `json:"enforcement"`
	Assurance        string   `json:"assurance"`
	Limitations      []string `json:"limitations"`
}

// Start creates an Amber v1 root context with exclusive atomic publication.
func Start(root, outputPath string) (amber.Provenance, string, error) {
	projectRoot, rooted, err := openProjectRoot(root)
	if err != nil {
		return amber.Provenance{}, "", err
	}
	defer rooted.Close()
	output, err := cleanRelative("Amber context output", outputPath)
	if err != nil {
		return amber.Provenance{}, "", err
	}
	value, err := amber.Start()
	if err != nil {
		return amber.Provenance{}, "", fmt.Errorf("start Amber provenance: %w", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return amber.Provenance{}, "", err
	}
	if err := publishNew(rooted, projectRoot, output, data); err != nil {
		return amber.Provenance{}, "", err
	}
	return value, digest(data), nil
}

// Execute creates and publishes the Amber child and reserves every output
// before starting the command. Existing output paths therefore never launch.
func Execute(ctx context.Context, request Request) (Receipt, error) {
	if ctx == nil {
		return Receipt{}, fmt.Errorf("provenance execution context cannot be nil")
	}
	root, rooted, err := openProjectRoot(request.Root)
	if err != nil {
		return Receipt{}, err
	}
	defer rooted.Close()
	parentPath, err := cleanRelative("Amber parent context", request.ParentPath)
	if err != nil {
		return Receipt{}, err
	}
	childPath, err := cleanRelative("Amber child context output", request.OutputPath)
	if err != nil {
		return Receipt{}, err
	}
	receiptPath, err := cleanRelative("provenance execution receipt", request.ReceiptPath)
	if err != nil {
		return Receipt{}, err
	}
	if request.Operation == "" || !operationPattern.MatchString(request.Operation) {
		return Receipt{}, fmt.Errorf("operation must be a nonempty simple name")
	}
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return Receipt{}, fmt.Errorf("command after -- is required")
	}
	for _, arg := range request.Command {
		if strings.ContainsRune(arg, 0) {
			return Receipt{}, fmt.Errorf("command argument contains NUL")
		}
	}
	if request.ExpectedParentSHA256 != "" && !validSHA(request.ExpectedParentSHA256) {
		return Receipt{}, fmt.Errorf("expected parent SHA-256 is malformed")
	}
	stdoutPath, stderrPath := receiptPath+".stdout", receiptPath+".stderr"
	outputs := []string{childPath, receiptPath, stdoutPath, stderrPath}
	for _, path := range append([]string{parentPath}, outputs...) {
		if err := rejectSymlinks(root, path); err != nil {
			return Receipt{}, fmt.Errorf("provenance path %q contains a symbolic link: %w", path, err)
		}
	}
	for i, output := range outputs {
		if output == parentPath {
			return Receipt{}, fmt.Errorf("parent context cannot be an output")
		}
		for _, other := range outputs[i+1:] {
			if pathsOverlap(output, other) {
				return Receipt{}, fmt.Errorf("provenance output paths must be distinct and nonoverlapping")
			}
		}
	}
	for _, output := range outputs {
		if _, err := rooted.Lstat(output); err == nil {
			return Receipt{}, fmt.Errorf("refusing to overwrite existing provenance output %s", output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Receipt{}, fmt.Errorf("inspect provenance output %s: %w", output, err)
		}
	}
	parentBytes, err := readRooted(rooted, parentPath)
	if err != nil {
		return Receipt{}, fmt.Errorf("read Amber parent context: %w", err)
	}
	parentSHA := digest(parentBytes)
	if request.ExpectedParentSHA256 != "" && parentSHA != request.ExpectedParentSHA256 {
		return Receipt{}, fmt.Errorf("Amber parent context digest does not match expected SHA-256")
	}
	if err := rejectDuplicateKeys(parentBytes); err != nil {
		return Receipt{}, fmt.Errorf("strictly decode Amber parent context: %w", err)
	}
	if err := validateAmberShape(parentBytes); err != nil {
		return Receipt{}, err
	}
	parent, err := amber.FromJSON(parentBytes)
	if err != nil {
		return Receipt{}, fmt.Errorf("validate Amber parent context: %w", err)
	}
	child, err := parent.Child(amber.ChildOptions{
		Origin:     amber.OriginLocal,
		References: []amber.Reference{{Type: "operation", ID: request.Operation}},
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("derive Amber child context: %w", err)
	}
	childBytes, err := json.Marshal(child)
	if err != nil {
		return Receipt{}, err
	}
	executable, executableSHA, err := resolveExecutable(request.Command[0], root)
	if err != nil {
		return Receipt{}, err
	}
	if err := rooted.MkdirAll(filepath.ToSlash(filepath.Dir(receiptPath)), 0o700); err != nil {
		return Receipt{}, fmt.Errorf("create provenance output directory: %w", err)
	}
	stdout, err := rooted.OpenFile(stdoutPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Receipt{}, fmt.Errorf("reserve stdout capture: %w", err)
	}
	stderr, err := rooted.OpenFile(stderrPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		stdout.Close()
		_ = rooted.Remove(stdoutPath)
		return Receipt{}, fmt.Errorf("reserve stderr capture: %w", err)
	}
	reservation, err := rooted.OpenFile(receiptPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		stdout.Close()
		stderr.Close()
		_ = rooted.Remove(stdoutPath)
		_ = rooted.Remove(stderrPath)
		return Receipt{}, fmt.Errorf("reserve execution receipt: %w", err)
	}
	if err := reservation.Sync(); err != nil {
		reservation.Close()
		stdout.Close()
		stderr.Close()
		_ = rooted.Remove(receiptPath)
		_ = rooted.Remove(stdoutPath)
		_ = rooted.Remove(stderrPath)
		return Receipt{}, fmt.Errorf("sync receipt reservation: %w", err)
	}
	if err := syncDirectory(rooted, receiptPath); err != nil {
		_ = reservation.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		_ = rooted.Remove(receiptPath)
		_ = rooted.Remove(stdoutPath)
		_ = rooted.Remove(stderrPath)
		return Receipt{}, fmt.Errorf("sync provenance output reservations: %w", err)
	}
	if err := publishNew(rooted, root, childPath, childBytes); err != nil {
		_ = reservation.Close()
		stdout.Close()
		stderr.Close()
		_ = rooted.Remove(receiptPath)
		_ = rooted.Remove(stdoutPath)
		_ = rooted.Remove(stderrPath)
		return Receipt{}, err
	}
	childInfo, childInfoErr := rooted.Stat(childPath)
	if childInfoErr != nil || !childInfo.Mode().IsRegular() {
		_ = reservation.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		if childInfoErr != nil {
			return Receipt{}, fmt.Errorf("inspect published Amber child context: %w", childInfoErr)
		}
		return Receipt{}, fmt.Errorf("published Amber child context is not a regular file")
	}
	stdoutInfo, stdoutInfoErr := stdout.Stat()
	stderrInfo, stderrInfoErr := stderr.Stat()
	receipt := Receipt{
		Schema: ReceiptSchema, ExecutionID: child.ExecutionID().String(), WorkID: child.WorkID().String(),
		ParentPath: parentPath, ParentSHA256: parentSHA, ContextPath: childPath, ContextSHA256: digest(childBytes),
		ReceiptPath: receiptPath, Operation: request.Operation, Argv: append([]string{}, request.Command...),
		ExecutablePath: executable, ExecutableSHA256: executableSHA,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "failed",
		StdoutPath: stdoutPath, StderrPath: stderrPath, Enforcement: "not-isolated", Assurance: "unverified",
		Limitations: []string{"command runs with Sentinel process environment and project filesystem access", "context digests are rechecked after execution, but later project-file edits remain possible", "executable digest is a prelaunch byte check, not running-image identity", "no host attestation", "noninteractive execution only"},
	}
	args := append([]string{}, request.Command...)
	args[0] = executable
	command := exec.Command(args[0], args[1:]...)
	command.Dir, command.Stdout, command.Stderr = root, stdout, stderr
	receipt.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	started, processErr, canceled := runCommand(ctx, command)
	receipt.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	switch {
	case canceled:
		receipt.Status, receipt.Reason = "canceled", "command execution was canceled"
	case !started:
		receipt.Status, receipt.Reason = "failed", "child process could not start: "+processErr.Error()
	case processErr != nil:
		receipt.Status, receipt.Reason = "failed", processErr.Error()
	default:
		receipt.Status = "completed"
	}
	if started {
		code := exitCode(command, processErr)
		receipt.ExitCode = &code
	}
	captureErr := errors.Join(stdoutInfoErr, stderrInfoErr, stdout.Sync(), stderr.Sync())
	if stdoutInfoErr == nil {
		captureErr = errors.Join(captureErr, verifyOwnedOutput(rooted, stdoutPath, stdoutInfo))
	}
	if stderrInfoErr == nil {
		captureErr = errors.Join(captureErr, verifyOwnedOutput(rooted, stderrPath, stderrInfo))
	}
	lineageErr := verifyOwnedOutput(rooted, childPath, childInfo)
	contextContents, contextErr := readRooted(rooted, childPath)
	if contextErr != nil {
		lineageErr = errors.Join(lineageErr, fmt.Errorf("re-read published Amber child context: %w", contextErr))
	} else if digest(contextContents) != receipt.ContextSHA256 {
		lineageErr = errors.Join(lineageErr, fmt.Errorf("published Amber child context contents changed during non-isolated command execution"))
	}
	parentContents, parentErr := readRooted(rooted, parentPath)
	if parentErr != nil {
		lineageErr = errors.Join(lineageErr, fmt.Errorf("re-read Amber parent context: %w", parentErr))
	} else if digest(parentContents) != receipt.ParentSHA256 {
		lineageErr = errors.Join(lineageErr, fmt.Errorf("Amber parent context contents changed during non-isolated command execution"))
	}
	stdoutSHA, stdoutReadErr := hashOpenFile(stdout)
	stderrSHA, stderrReadErr := hashOpenFile(stderr)
	closeErr := errors.Join(stdout.Close(), stderr.Close())
	if err := errors.Join(captureErr, stdoutReadErr, stderrReadErr, closeErr); err != nil {
		receipt.Status, receipt.Reason = "indeterminate", "capture finalization failed: "+err.Error()
	} else {
		receipt.StdoutSHA256, receipt.StderrSHA256 = stdoutSHA, stderrSHA
	}
	if lineageErr != nil {
		receipt.Status = "indeterminate"
		receipt.Reason = "context lineage changed during non-isolated command execution: " + lineageErr.Error()
	}
	receiptBytes, err := json.Marshal(receipt)
	if err == nil {
		err = replaceReservation(rooted, receiptPath, reservation, receiptBytes)
	}
	reservationCloseErr := reservation.Close()
	err = errors.Join(err, reservationCloseErr)
	if err != nil {
		receipt.Status = "indeterminate"
		receipt.Reason = "execution receipt publication failed: " + err.Error()
		return receipt, fmt.Errorf("publish provenance execution receipt: %w", err)
	}
	if receipt.Status == "completed" {
		return receipt, nil
	}
	return receipt, fmt.Errorf("provenance command %s: %s", receipt.Status, receipt.Reason)
}

// SignalContext installs process-level cancellation forwarding for CLI use.
func SignalContext(parent context.Context) (context.Context, func()) {
	return signalContext(parent)
}

func openProjectRoot(root string) (string, *os.Root, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return "", nil, fmt.Errorf("project root must be an explicit absolute path")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, fmt.Errorf("canonicalize project root: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", nil, fmt.Errorf("project root is not a directory")
	}
	rooted, err := os.OpenRoot(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("open project root: %w", err)
	}
	return canonical, rooted, nil
}

func publishNew(rooted *os.Root, projectRoot, relative string, contents []byte) error {
	if err := rooted.MkdirAll(filepath.ToSlash(filepath.Dir(relative)), 0o700); err != nil {
		return fmt.Errorf("create provenance output directory: %w", err)
	}
	if err := rejectSymlinks(projectRoot, relative); err != nil {
		return err
	}
	tempPath := relative + ".tmp-" + randomSuffix()
	file, err := rooted.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary provenance output: %w", err)
	}
	n, err := file.Write(contents)
	if err == nil && n != len(contents) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = rooted.Remove(tempPath)
		return errors.Join(err, closeErr)
	}
	if err := rooted.Link(tempPath, relative); err != nil {
		_ = rooted.Remove(tempPath)
		return fmt.Errorf("publish provenance output without replacement: %w", err)
	}
	if err := rooted.Remove(tempPath); err != nil {
		return err
	}
	dir, err := rooted.Open(filepath.ToSlash(filepath.Dir(relative)))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func replaceReservation(rooted *os.Root, relative string, reservation *os.File, contents []byte) error {
	reservedInfo, err := reservation.Stat()
	if err != nil {
		return fmt.Errorf("inspect execution receipt reservation: %w", err)
	}
	if reservedInfo.Size() != 0 {
		return fmt.Errorf("execution receipt reservation was modified")
	}
	currentInfo, err := rooted.Stat(relative)
	if err != nil {
		return fmt.Errorf("inspect execution receipt path: %w", err)
	}
	if !os.SameFile(reservedInfo, currentInfo) {
		return fmt.Errorf("execution receipt reservation was replaced")
	}
	temp := relative + ".complete-" + randomSuffix()
	file, err := rooted.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := file.Write(contents)
	if err == nil && n != len(contents) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = rooted.Remove(temp)
		return errors.Join(err, closeErr)
	}
	currentInfo, err = rooted.Stat(relative)
	if err != nil {
		_ = rooted.Remove(temp)
		return fmt.Errorf("inspect execution receipt before publication: %w", err)
	}
	if !os.SameFile(reservedInfo, currentInfo) || currentInfo.Size() != 0 {
		_ = rooted.Remove(temp)
		return fmt.Errorf("execution receipt reservation changed before publication")
	}
	if err := rooted.Rename(temp, relative); err != nil {
		_ = rooted.Remove(temp)
		return err
	}
	dir, err := rooted.Open(filepath.ToSlash(filepath.Dir(relative)))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func syncDirectory(rooted *os.Root, path string) error {
	dir, err := rooted.Open(filepath.ToSlash(filepath.Dir(path)))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func readRooted(rooted *os.Root, path string) ([]byte, error) {
	file, err := openProvenanceInput(rooted, path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxInputBytes {
		return nil, fmt.Errorf("input must be a regular file no larger than %d bytes", maxInputBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil || len(data) > maxInputBytes {
		return nil, errors.Join(err, fmt.Errorf("input exceeds size limit"))
	}
	return data, nil
}

func hashOpenFile(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyOwnedOutput(rooted *os.Root, path string, expected os.FileInfo) error {
	actual, err := rooted.Stat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(expected, actual) {
		return fmt.Errorf("captured output path %s was replaced during non-isolated command execution", path)
	}
	return nil
}

func cleanRelative(label, raw string) (string, error) {
	if raw == "" || filepath.IsAbs(raw) || strings.ContainsRune(raw, '\\') {
		return "", fmt.Errorf("%s path must be project-relative", label)
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != raw {
		return "", fmt.Errorf("%s path must be normalized and remain beneath project root", label)
	}
	return filepath.ToSlash(clean), nil
}

func rejectSymlinks(root, relative string) error {
	current := root
	for _, part := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %s is a symbolic link", current)
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func validSHA(value string) bool { return shaPattern.MatchString(value) }

func resolveExecutable(raw, projectRoot string) (string, string, error) {
	path := raw
	if !filepath.IsAbs(path) {
		if strings.ContainsRune(path, filepath.Separator) || strings.HasPrefix(path, ".") {
			path = filepath.Join(projectRoot, path)
		} else {
			found, err := exec.LookPath(path)
			if err != nil {
				return "", "", fmt.Errorf("resolve command executable: %w", err)
			}
			path = found
		}
	}
	path, err := filepath.Abs(path)
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
	}
	if err != nil {
		return "", "", fmt.Errorf("canonicalize command executable: %w", err)
	}
	file, err := openExecutable(path)
	if err != nil {
		return "", "", fmt.Errorf("open command executable for prelaunch digest: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("command executable is not a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", "", fmt.Errorf("hash command executable: %w", err)
	}
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}
