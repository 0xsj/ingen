//go:build !unix

package codexbroker_test

import "os/exec"

func prepareInstalledProcess(cmd *exec.Cmd) {}
func cleanupInstalledProcess(cmd *exec.Cmd) {}
