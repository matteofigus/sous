// +build linux darwin

package shell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"

	"github.com/opentable/sous/util/whitespace"
)

type (
	// Command is a wrapper around an exec.Cmd
	Command struct {
		// Sh is a copy of the shell this command is executing in.
		*Sh
		// Name is the name of the command itself.
		Name string
		// Args is a list of args to be passed to the command.
		Args []string
	}
	// Result is the result of running a command to completion.
	Result struct {
		Command                  *Command
		Stdout, Stderr, Combined *Output
		Err                      error
		ExitCode                 int
	}
	Error struct {
		// Err is the original error that was returned.
		Err error
		// Result is the complete result of the command execution that caused
		// this error.
		Result *Result
		// Command is the command which caused this error.
		Command *Command
	}
)

func (e Error) Error() string {
	return fmt.Sprintf("shell> %s\n%s\ncommand failed: %s",
		e.Result.Command.String(), e.Result.Combined.String(), e.Err)
}

func newError(err error, r *Result) Error {
	return Error{
		Err:     err,
		Result:  r,
		Command: r.Command,
	}
}

// Stdout returns the stdout stream as a string. It returns an error for the
// same reasons as .Succeed
func (c *Command) Stdout() (string, error) {
	r, err := c.SucceedResult()
	if err != nil {
		return "", err
	}
	return r.Stdout.String(), nil
}

// Stderr is returns the stderr stream as a string. It returns an error for the
// same reasons as .Result
func (c *Command) Stderr() (string, error) {
	r, err := c.SucceedResult()
	if err != nil {
		return "", err
	}
	return r.Stderr.String(), nil
}

// Lines returns Stdout split by newline. Leading and trailing empty lines are
// removed, and each line is trimmed of whitespace.
func (c *Command) Lines() ([]string, error) {
	stdout, err := c.Stdout()
	if err != nil {
		return nil, err
	}
	rawLines := strings.Split(stdout, "\n")
	lines := []string{}
	for _, l := range rawLines {
		trimmed := whitespace.Trim(l)
		if len(trimmed) == 0 {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines, nil
}

// JSON tries to parse the stdout from the command as JSON, populating the
// value you pass. (This value should be a pointer.)
func (c *Command) JSON(v interface{}) error {
	r, err := c.Result()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(r.Stdout.Reader())
	return decoder.Decode(v)
}

// ExitCode only returns an error if there were io issues starting the command,
// it does not return an error for a command which fails and returns an error
// code, which is unlike most of Sh's methods. If it returns an error, then it
// also returns -1 for the exit code.
func (c *Command) ExitCode() (int, error) {
	r, err := c.Result()
	if err != nil {
		return -1, err
	}
	return r.ExitCode, nil
}

// Result only returns an error if it's a startup error, not if the command
// itself exits with an error code. If you need an error to be returned on
// non-zero exit codes, use SucceedResult instead.
func (s *Command) Result() (*Result, error) {
	c := exec.Command(s.Name, s.Args...)
	outbuf := &bytes.Buffer{}
	errbuf := &bytes.Buffer{}
	combinedbuf := &bytes.Buffer{}
	outWriters := []io.Writer{outbuf, combinedbuf}
	errWriters := []io.Writer{errbuf, combinedbuf}
	if s.TeeOut != nil {
		outWriters = append(outWriters, s.TeeOut)
	}
	if s.TeeErr != nil {
		errWriters = append(errWriters, s.TeeErr)
	}

	c.Stdout = io.MultiWriter(outWriters...)
	c.Stderr = io.MultiWriter(errWriters...)

	if err := c.Start(); err != nil {
		return nil, err
	}
	code := 0
	err := c.Wait()
	if err != nil {
		code = -1 // in case the following fails
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				code = status.ExitStatus()
			}
		}
		// TODO: Consider handling ErrNotFound as a special case here.
	}
	return &Result{
		Command:  s,
		Stdout:   &Output{outbuf},
		Stderr:   &Output{errbuf},
		Combined: &Output{combinedbuf},
		Err:      err,
		ExitCode: code,
	}, nil
}

// SucceedResult is similar to Result, except that it also returns an error if
// the command itself fails (returns a non-zero exit code).
func (c *Command) SucceedResult() (*Result, error) {
	r, err := c.Result()
	if err != nil {
		return r, err
	}
	if r.Err != nil {
		return r, newError(r.Err, r)
	}
	return r, nil
}

// Succeed returns an error if the command fails for any reason (fails to start
// or finishes with a non-zero exist code).
func (c *Command) Succeed() error {
	_, err := c.SucceedResult()
	return err
}

// Fail returns an error if the command succeeds to execute, or if it fails to
// start. It returns nil only if the command starts successfully and then exits
// with a non-zero exit code.
func (c *Command) Fail() error {
	_, err := c.FailResult()
	return err
}

// FailResult returns an error when the command fails to be invoked at all, or
// when the command is successfully invoked, and then runs successfully. It
// does not return an error when the command is invoked successfully and then
// fails.
func (c *Command) FailResult() (*Result, error) {
	r, err := c.Result()
	if err != nil {
		return r, err
	}
	if r.Err == nil {
		return r, fmt.Errorf("command %s succeeded, expected failure")
	}
	return r, nil
}

func (c *Command) String() string {
	args := strings.Join(c.Args, " ")
	return fmt.Sprintf("%s %s", c.Name, args)
}
