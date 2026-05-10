// +build !windows

package recorder

import (
	"os"
	"syscall"
)

func signalInterrupt() os.Signal { return syscall.SIGINT }
func signalStop() os.Signal      { return syscall.SIGSTOP }
func signalContinue() os.Signal  { return syscall.SIGCONT }
