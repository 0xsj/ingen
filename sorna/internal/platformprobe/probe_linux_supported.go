//go:build linux && (amd64 || arm64)

package platformprobe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const landlockCreateRulesetNumber = 444
const landlockCreateRulesetVersion = 1

const (
	landlockFSExecute     = uint64(1 << 0)
	landlockFSWriteFile   = uint64(1 << 1)
	landlockFSReadFile    = uint64(1 << 2)
	landlockFSReadDir     = uint64(1 << 3)
	landlockFSRemoveDir   = uint64(1 << 4)
	landlockFSRemoveFile  = uint64(1 << 5)
	landlockFSMakeChar    = uint64(1 << 6)
	landlockFSMakeDir     = uint64(1 << 7)
	landlockFSMakeReg     = uint64(1 << 8)
	landlockFSMakeSock    = uint64(1 << 9)
	landlockFSMakeFIFO    = uint64(1 << 10)
	landlockFSMakeBlock   = uint64(1 << 11)
	landlockFSMakeSym     = uint64(1 << 12)
	landlockFSRefer       = uint64(1 << 13)
	landlockFSTruncate    = uint64(1 << 14)
	landlockFSIoctlDev    = uint64(1 << 15)
	landlockNetBindTCP    = uint64(1 << 0)
	landlockNetConnectTCP = uint64(1 << 1)
)

const (
	prGetSeccomp      = 21
	prSetSeccomp      = 22
	prSetNoNewPrivs   = 38
	prGetNoNewPrivs   = 39
	seccompModeFilter = 2
	bpfRetK           = 0x06
	seccompRetAllow   = 0x7fff0000
)

type landlockRulesetAttr struct {
	HandledFS  uint64
	HandledNet uint64
}

type seccompBPFInstruction struct {
	Code uint16
	JT   uint8
	JF   uint8
	K    uint32
}

type seccompBPFProgram struct {
	Length uint16
	Filter *seccompBPFInstruction
}

type accessBitSpec struct {
	name     string
	domain   string
	mask     uint64
	abi      int
	required bool
}

var requiredLandlockBits = []accessBitSpec{
	{name: "fs_execute", domain: "filesystem", mask: landlockFSExecute, abi: 1, required: true},
	{name: "fs_write_file", domain: "filesystem", mask: landlockFSWriteFile, abi: 1, required: true},
	{name: "fs_read_file", domain: "filesystem", mask: landlockFSReadFile, abi: 1, required: true},
	{name: "fs_read_dir", domain: "filesystem", mask: landlockFSReadDir, abi: 1, required: true},
	{name: "fs_remove_dir", domain: "filesystem", mask: landlockFSRemoveDir, abi: 1, required: true},
	{name: "fs_remove_file", domain: "filesystem", mask: landlockFSRemoveFile, abi: 1, required: true},
	{name: "fs_make_char", domain: "filesystem", mask: landlockFSMakeChar, abi: 1},
	{name: "fs_make_dir", domain: "filesystem", mask: landlockFSMakeDir, abi: 1, required: true},
	{name: "fs_make_reg", domain: "filesystem", mask: landlockFSMakeReg, abi: 1, required: true},
	{name: "fs_make_sock", domain: "filesystem", mask: landlockFSMakeSock, abi: 1, required: true},
	{name: "fs_make_fifo", domain: "filesystem", mask: landlockFSMakeFIFO, abi: 1, required: true},
	{name: "fs_make_block", domain: "filesystem", mask: landlockFSMakeBlock, abi: 1},
	{name: "fs_make_sym", domain: "filesystem", mask: landlockFSMakeSym, abi: 1, required: true},
	{name: "fs_refer", domain: "filesystem", mask: landlockFSRefer, abi: 2, required: true},
	{name: "fs_truncate", domain: "filesystem", mask: landlockFSTruncate, abi: 3, required: true},
	{name: "fs_ioctl_dev", domain: "filesystem", mask: landlockFSIoctlDev, abi: 5, required: true},
	{name: "net_bind_tcp", domain: "network_port_only", mask: landlockNetBindTCP, abi: 4},
	{name: "net_connect_tcp", domain: "network_port_only", mask: landlockNetConnectTCP, abi: 4},
}

func collectPlatform(report *Report) {
	if kernel, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		report.KernelRelease = strings.TrimSpace(string(kernel))
		if report.KernelRelease == "" {
			report.KernelReleaseStatus = Check{Status: Indeterminate, Detail: "kernel release file was empty"}
		} else {
			report.KernelReleaseStatus = Check{Status: Available}
		}
	} else {
		report.KernelReleaseStatus = Check{Status: Indeterminate, Detail: fmt.Sprintf("read kernel release: %v", err)}
	}
	report.Landlock = probeLandlock()
	report.ProcExecutableObservation = probeProcExecutable()
}

func probeLandlock() LandlockReport {
	result, _, errno := syscall.Syscall(landlockCreateRulesetNumber, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		status := Indeterminate
		if errno == syscall.ENOSYS || errno == syscall.EOPNOTSUPP {
			status = Unavailable
		}
		return LandlockReport{Status: status, RequiredAccessBits: []AccessBit{}, Detail: fmt.Sprintf("query Landlock ABI: %v", errno)}
	}
	if result < 1 || result > 255 {
		return LandlockReport{Status: Indeterminate, RequiredAccessBits: []AccessBit{}, Detail: fmt.Sprintf("kernel returned unexpected Landlock ABI version %d", result)}
	}
	abi := int(result)
	accessBits := make([]AccessBit, 0, len(requiredLandlockBits))
	for _, spec := range requiredLandlockBits {
		bit := AccessBit{
			Name: spec.name, Domain: spec.domain,
			Mask: fmt.Sprintf("0x%x", spec.mask), IntroducedABI: spec.abi,
			RequiredForBaseline: spec.required,
			Status:              Unavailable,
		}
		if abi < spec.abi {
			bit.Detail = fmt.Sprintf("requires Landlock ABI %d; detected ABI %d", spec.abi, abi)
			accessBits = append(accessBits, bit)
			continue
		}
		status, detail := probeLandlockAccessBit(spec)
		bit.Status, bit.Detail = status, detail
		accessBits = append(accessBits, bit)
	}
	return LandlockReport{Status: Available, ABI: &abi, RequiredAccessBits: accessBits}
}

func probeLandlockAccessBit(spec accessBitSpec) (Status, string) {
	attr := landlockRulesetAttr{}
	size := unsafe.Sizeof(uint64(0))
	if spec.domain == "network_port_only" {
		attr.HandledFS = landlockFSReadFile
		attr.HandledNet = spec.mask
		size = unsafe.Sizeof(attr)
	} else {
		attr.HandledFS = spec.mask
	}
	fd, _, errno := syscall.Syscall(landlockCreateRulesetNumber, uintptr(unsafe.Pointer(&attr)), uintptr(size), 0)
	runtime.KeepAlive(attr)
	if errno == 0 {
		if closeErr := syscall.Close(int(fd)); closeErr != nil {
			return Indeterminate, fmt.Sprintf("ruleset probe succeeded but close failed: %v", closeErr)
		}
		return Available, ""
	}
	if errno == syscall.EINVAL {
		return Unavailable, "kernel rejected this access bit"
	}
	if errno == syscall.ENOSYS || errno == syscall.EOPNOTSUPP {
		return Unavailable, fmt.Sprintf("access-bit probe unavailable: %v", errno)
	}
	return Indeterminate, fmt.Sprintf("access-bit probe failed: %v", errno)
}

func probeProcExecutable() Check {
	const path = "/proc/self/exe"
	target, err := os.Readlink(path)
	if err != nil {
		if errorsIsAbsent(err) {
			return Check{Status: Unavailable, Detail: "/proc/self/exe is unavailable"}
		}
		return Check{Status: Indeterminate, Detail: fmt.Sprintf("read /proc/self/exe: %v", err)}
	}
	if !filepath.IsAbs(target) {
		return Check{Status: Indeterminate, Detail: "/proc/self/exe returned a non-absolute target"}
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Check{Status: Indeterminate, Detail: "/proc/self/exe target could not be confirmed as a regular executable"}
	}
	return Check{Status: Available, Detail: "self executable path is observable through procfs; arbitrary process visibility is not tested"}
}

func errorsIsAbsent(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR)
}

// RunSeccompProbeChild is intended for a short-lived child created by Probe.
// It sets no_new_privs and installs an allow-all filter on that child only; it
// must not be called in a process that should remain unrestricted.
func RunSeccompProbeChild() ChildReport {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result := ChildReport{
		NoNewPrivileges: Check{Status: Indeterminate, Detail: "PR_GET_NO_NEW_PRIVS was not queried"},
		SeccompFilter:   Check{Status: Indeterminate, Detail: "seccomp filter was not attempted"},
	}
	mode, _, errno := callPrctl(prGetSeccomp, 0, 0, 0, 0)
	if errno != 0 {
		result.SeccompFilter = Check{Status: Indeterminate, Detail: fmt.Sprintf("PR_GET_SECCOMP failed: %v", errno)}
	} else {
		result.InitialSeccompMode = int(mode)
		if mode > 2 {
			result.SeccompFilter = Check{Status: Indeterminate, Detail: "PR_GET_SECCOMP returned an unknown mode"}
		}
	}
	noNewPrivs, _, errno := callPrctl(prGetNoNewPrivs, 0, 0, 0, 0)
	if errno != 0 || noNewPrivs > 1 {
		result.NoNewPrivileges = Check{Status: Indeterminate, Detail: "PR_GET_NO_NEW_PRIVS could not be read"}
		return result
	}
	result.InitialNoNewPrivs = int(noNewPrivs)
	_, _, errno = callPrctl(prSetNoNewPrivs, 1, 0, 0, 0)
	if errno != 0 {
		result.NoNewPrivileges = Check{Status: Indeterminate, Detail: fmt.Sprintf("PR_SET_NO_NEW_PRIVS failed: %v", errno)}
		return result
	}
	confirmed, _, errno := callPrctl(prGetNoNewPrivs, 0, 0, 0, 0)
	if errno != 0 || confirmed != 1 {
		result.NoNewPrivileges = Check{Status: Indeterminate, Detail: "PR_SET_NO_NEW_PRIVS did not read back as enabled"}
		return result
	}
	result.NoNewPrivileges = Check{Status: Available, Detail: "no_new_privs was set and confirmed on the probe child thread"}
	if result.SeccompFilter.Status == Indeterminate && strings.Contains(result.SeccompFilter.Detail, "unknown mode") {
		return result
	}
	instruction := seccompBPFInstruction{Code: bpfRetK, K: seccompRetAllow}
	program := seccompBPFProgram{Length: 1, Filter: &instruction}
	_, _, errno = callPrctl(prSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(&program)), 0, 0)
	runtime.KeepAlive(program)
	runtime.KeepAlive(instruction)
	if errno != 0 {
		result.SeccompFilter = Check{Status: Indeterminate, Detail: fmt.Sprintf("SECCOMP_MODE_FILTER installation failed in the probe child: %v", errno)}
	} else {
		result.SeccompFilter = Check{Status: Available, Detail: "an allow-all seccomp filter installed on the probe child thread; TSYNC and filter policy correctness are not tested"}
	}
	return result
}

func callPrctl(option int, arg2, arg3, arg4, arg5 uintptr) (uintptr, uintptr, syscall.Errno) {
	return syscall.Syscall6(syscall.SYS_PRCTL, uintptr(option), arg2, arg3, arg4, arg5, 0)
}
