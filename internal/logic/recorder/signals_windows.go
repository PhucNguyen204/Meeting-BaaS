// +build windows

package recorder

import "os"

// Windows does not support SIGSTOP/SIGCONT. These are stubs for compilation.
func signalInterrupt() os.Signal { return os.Interrupt }
func signalStop() os.Signal      { return os.Interrupt }
func signalContinue() os.Signal  { return os.Interrupt }
