package deploy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/opentable/sous/tools/cmd"
)

type Contracts map[string]Contract

func (cs Contracts) Validate() error {
	for _, c := range cs {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("Contract invalid: %s", err)
		}
	}
	return nil
}

type Contract struct {
	Name, Filename        string
	StartServers          []string
	Values                map[string]string
	Servers               map[string]TestServer
	Preconditions, Checks Checks
}

func (c Contract) Errorf(format string, a ...interface{}) error {
	f := c.Filename + ": " + format
	return fmt.Errorf(f, a...)
}

func (c Contract) Validate() error {
	if c.Name == "" {
		return c.Errorf("%s: Contract Name must not be empty")
	}
	if err := c.Preconditions.Validate(); err != nil {
		return c.Errorf("Precondition invalid: %s", err)
	}
	if err := c.Checks.Validate(); err != nil {
		return c.Errorf("Check invalid: %s", err)
	}
	return nil
}

type Checks []Check

func (cs Checks) Validate() error {
	for _, c := range cs {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type TestServer struct {
	Name          string
	DefaultValues map[string]string
	Startup       *StartupInfo
	Docker        DockerServer
}

type StartupInfo struct {
	CompleteWhen *Check
}

type DockerServer struct {
	Image         string
	Env           map[string]string
	Options, Args []string
}

type GetHTTPAssertion struct {
	URL, ResponseBodyContains, ResponseJSONContains string
	ResponseStatusCode                              int
	AnyResponse                                     bool
}

// Check MUST specify exactly one of GET, Shell, or Contract. If
// more than one of those are specified the check is invalid. This
// slightly ugly switching makes the YAML contract definitions
// much more readable, and is easily verifiable.
type Check struct {
	Name       string
	Timeout    time.Duration
	Setup      Action
	HTTPCheck  `yaml:",inline"`
	ShellCheck `yaml:",inline"`
}

type Action struct {
	Shell string
}

// Validate checks that we have a well-formed check.
func (c *Check) Validate() error {
	httpError := c.HTTPCheck.Validate()
	shellError := c.ShellCheck.Validate()
	if httpError != nil && shellError != nil {
		if c.HTTPCheck.GET != "" {
			return fmt.Errorf("%s", httpError)
		}
		if c.ShellCheck.Shell != "" {
			return fmt.Errorf("%s", shellError)
		}
		return fmt.Errorf("multiple errors: (%s) and (%s)", httpError, shellError)
	}
	if httpError == nil && shellError == nil {
		return fmt.Errorf("You have specified both Shell and GET, pick one or the other")
	}
	return nil
}

func (c *Check) Execute() error {
	if c.HTTPCheck.Validate() == nil {
		return c.HTTPCheck.Execute()
	}
	if c.ShellCheck.Validate() == nil {
		return c.ShellCheck.Execute()
	}
	return c.Validate()
}

type HTTPCheck struct {
	// GET must be a URL, or empty if Shell is not empty.
	// The following 4 fields are assertions about
	// the response after getting that URL via HTTP.
	GET             string
	StatusCode      int
	StatusCodeRange []int
	// TODO: Either implement this somehow or remove it
	//BodyContainsJSON   interface{}
	BodyContainsString       string
	BodyDoesNotContainString string
}

// Validate HTTPCheck, return an error if it is not valid.
func (c HTTPCheck) Validate() error {
	if c.GET == "" {
		return fmt.Errorf("GET not specified")
	}
	if c.StatusCode == 0 && len(c.StatusCodeRange) == 0 && c.BodyContainsString == "" && c.BodyDoesNotContainString == "" {
		return fmt.Errorf("you must supply at least one of: StatusCode, StatusCodeRange, BodyContainsString, BodyDoesNotContainString")
	}
	if c.StatusCode < 0 || c.StatusCode > 999 {
		return fmt.Errorf("StatusCode was %d; want 0 ≤ StatusCode ≤ 999")
	}
	if len(c.StatusCodeRange) == 1 || len(c.StatusCodeRange) > 2 {
		return fmt.Errorf("StatusCodeRange was %v; want it to be empty or contain exactly 2 elements")
	}
	return nil
}

// Execute an HTTPCheck, you must first check it is valid with Validate, or the behaviour is undefined.
func (c HTTPCheck) Execute() error {
	response, err := http.Get(c.GET)
	if err != nil {
		return err
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if c.StatusCode != 0 && response.StatusCode != c.StatusCode {
		return fmt.Errorf("got status code %d; want %d", response.StatusCode, c.StatusCode)
	}
	if len(c.StatusCodeRange) != 0 {
		if response.StatusCode < c.StatusCodeRange[0] || response.StatusCode > c.StatusCodeRange[1] {
			return fmt.Errorf("got status code %s; want something in the range %d..%d", response.StatusCode, c.StatusCodeRange[0], c.StatusCodeRange[1])
		}
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("unable to read response body: %s", err)
	}
	if c.BodyContainsString != "" && !strings.Contains(string(body), c.BodyContainsString) {
		return fmt.Errorf("expected to find string %q in body but did not", c.BodyContainsString)
	}
	if c.BodyDoesNotContainString != "" && strings.Contains(string(body), c.BodyDoesNotContainString) {
		return fmt.Errorf("found string %q in body, expected not to", c.BodyDoesNotContainString)
	}
	return nil
}

type ShellCheck struct {
	// Shell must be a valid POSIX shell command, or empty if GET is not
	// empty. The command will be executed and the exit code checked
	// against the expected code (note that ints default to zero, so the
	// default case is that we expect a success (0) exit code.
	Shell    string
	ExitCode int
}

func (c ShellCheck) Validate() error {
	if c.Shell == "" {
		return fmt.Errorf("Shell command not specified")
	}
	return nil
}

func (c ShellCheck) Execute() error {
	// Wrap the command in a subshell so the command can contain pipelines.
	// Note that the spaces between the parentheses are mandatory for compatibility
	// with further subshells defined in the contract, so don't remove them.
	command := fmt.Sprintf("( %s )", c.Shell)
	code := cmd.ExitCode("/bin/sh", "-c", command)
	if code != c.ExitCode {
		return fmt.Errorf("got exit code %d; want %d", code, c.ExitCode)
	}
	return nil
}

func (c Check) String() string {
	if c.Name != "" {
		return c.Name
	}
	if c.Shell != "" {
		return c.Shell
	}
	if c.GET != "" {
		return fmt.Sprintf("GET %s", c.GET)
	}
	return "INVALID CHECK"
}
