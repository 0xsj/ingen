//go:build !linux || (!amd64 && !arm64)

package platformprobe

import "fmt"

func collectPlatform(report *Report) {
	reason := fmt.Sprintf("Linux kernel probes are unavailable on %s/%s; supported targets are linux/amd64 and linux/arm64", report.OperatingSystem, report.Architecture)
	report.KernelReleaseStatus = Check{Status: Unavailable, Detail: reason}
	report.Landlock = LandlockReport{Status: Unavailable, RequiredAccessBits: []AccessBit{}, Detail: reason}
	report.ProcExecutableObservation = Check{Status: Unavailable, Detail: reason}
}

// RunSeccompProbeChild is a no-op on targets whose syscall ABI is not probed.
func RunSeccompProbeChild() ChildReport {
	reason := "seccomp child probing is only implemented on Linux amd64 and arm64"
	return ChildReport{
		NoNewPrivileges: Check{Status: Unavailable, Detail: reason},
		SeccompFilter:   Check{Status: Unavailable, Detail: reason},
	}
}
