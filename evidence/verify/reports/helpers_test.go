package reports

import (
	"bytes"
	"io"
	"os"

	"github.com/gookit/color"
)

// captureOutput captures stdout while executing f and returns the combined buffered color output and raw output.
func captureOutput(f func()) string {
	var buf bytes.Buffer

	oldStdout := os.Stdout

	r, w, _ := os.Pipe()
	os.Stdout = w

	color.SetOutput(&buf)

	defer func() {
		os.Stdout = oldStdout
		color.ResetOutput()
	}()

	done := make(chan bool)
	go func() {
		f()
		_ = w.Close()
		done <- true
	}()

	var capturedOutput bytes.Buffer
	_, _ = io.Copy(&capturedOutput, r)
	<-done

	return buf.String() + capturedOutput.String()
}
