package service

import "os/exec"

// The packaged diagnostics runtime is Linux. This keeps Windows builds intact;
// missing POSIX tools are reported by executable detection.
func configureProcess(cmd *exec.Cmd) {}
