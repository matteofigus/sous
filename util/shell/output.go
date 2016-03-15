package shell

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	wsRegexpChars = ` \t\r\n`
	wsChars       = " \t\r\n"
)

var whitespace = regexp.MustCompile(fmt.Sprintf("[%s]+", wsRegexpChars))

func trimWS(s string) string {
	return strings.Trim(s, wsChars)
}

// Output represents a read-only output stream of a command.
type Output struct{ buffer *bytes.Buffer }

// String is gives the entire output as a string, with whitespace trimmed.
func (o *Output) String() string {
	return trimWS(o.buffer.String())
}

// Lines gives the entire output as a string slice, with each item in the slice
// representing one line of the output. Lines are determined by splitting the
// string on newline characters. Preceeding and trailing empty lines  are
// removed from the output, and each line is trimmed of whitespace.
func (o *Output) Lines() []string {
	lines := strings.Split(o.String(), "\n")
	for i, s := range lines {
		lines[i] = trimWS(s)
	}
	return lines
}

// Table treats the entire output like a table. First it splits the output into
// lines in the same way as Lines(). Then, each line is further split by
// all regions of contiguous whitespace. Empty lines are removed.
func (o *Output) Table() [][]string {
	lines := o.Lines()
	rows := make([][]string, len(lines))
	for _, line := range lines {
		cells := whitespace.Split(line, -1)
		rows = append(rows, cells)
	}
	return rows
}

// Bytes returns the entire output as a byte slice.
func (o *Output) Bytes() []byte {
	return o.buffer.Bytes()
}

// Reader returns a reader, allowing you to read the entire output.
func (o *Output) Reader() io.Reader {
	return o.buffer
}
