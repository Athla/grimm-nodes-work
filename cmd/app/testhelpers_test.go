package main

import (
	"bytes"

	"github.com/spf13/cobra"
)

// executeCommand runs root with args, capturing stdout and stderr separately.
// Use for table-driven cobra tests; both buffers are returned even on error so
// assertions can inspect partial output.
func executeCommand(root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}
