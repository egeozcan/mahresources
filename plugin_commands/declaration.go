// Package plugin_commands defines validated command templates and invocation
// data without depending on the plugin runtime or application context.
package plugin_commands

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultTimeout             = time.Hour
	MaxTimeout                 = 24 * time.Hour
	MaxParameters              = 32
	MaxParameterBytes          = 8 << 10
	MaxAggregateParameterBytes = 64 << 10
	MaxInputFiles              = 4
	MaxInputBytes              = 256 << 10
	MaxAggregateInputBytes     = 512 << 10
)

const redactedValue = "[redacted]"

var (
	commandNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	placeholderPattern   = regexp.MustCompile(`^\{\{([a-z][a-z0-9_]*)\}\}$`)
	parameterNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	shellSafePattern     = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)
)

// Declaration is one executable argv template from a plugin manifest. Timeout
// is normalized by the manifest parser before a declaration is stored. Inputs
// are the file names the plugin may supply contents for; the order is display
// order, and identity compares them as a set.
type Declaration struct {
	Name            string        `json:"name"`
	Argv            []string      `json:"argv"`
	Timeout         time.Duration `json:"timeout"`
	SensitiveParams []string      `json:"sensitive_params,omitempty"`
	Inputs          []string      `json:"inputs,omitempty"`
}

// Invocation is the launch vector and the separately redacted history view.
// It deliberately carries no shell command string: execution consumes Argv.
type Invocation struct {
	Argv         []string
	RedactedArgv []string
	ParamView    map[string]string
}

// InputFile is one supplied input that has passed validation. Content is the
// only place the bytes live outside the exchange folder once the run exists;
// nothing may copy it into a record, a log line or an error.
type InputFile struct {
	Name    string
	Content []byte
}

// ValidateDeclaration refuses templates that could select a path or interpolate
// parameter data into only part of an argument.
func ValidateDeclaration(declaration Declaration) error {
	if !commandNamePattern.MatchString(declaration.Name) {
		return fmt.Errorf("command name %q must match %s", declaration.Name, commandNamePattern)
	}
	if declaration.Timeout <= 0 {
		return fmt.Errorf("command %q timeout must be positive", declaration.Name)
	}
	if declaration.Timeout > MaxTimeout {
		return fmt.Errorf("command %q timeout %s exceeds the maximum %s", declaration.Name, declaration.Timeout, MaxTimeout)
	}
	if len(declaration.Argv) == 0 {
		return fmt.Errorf("command %q argv must not be empty", declaration.Name)
	}
	if err := validateExecutable(declaration.Name, declaration.Argv[0]); err != nil {
		return err
	}

	placeholders := make(map[string]struct{})
	for i, arg := range declaration.Argv {
		if match := placeholderPattern.FindStringSubmatch(arg); match != nil {
			placeholders[match[1]] = struct{}{}
			continue
		}
		if strings.Contains(arg, "{{") || strings.Contains(arg, "}}") {
			return fmt.Errorf("command %q argv[%d] contains a partial or malformed placeholder: placeholders must occupy a whole argv element", declaration.Name, i)
		}
	}

	seenSensitive := make(map[string]struct{}, len(declaration.SensitiveParams))
	for _, name := range declaration.SensitiveParams {
		if !parameterNamePattern.MatchString(name) {
			return fmt.Errorf("command %q sensitive parameter %q must match %s", declaration.Name, name, parameterNamePattern)
		}
		if name == "exchange_dir" {
			return fmt.Errorf("command %q sensitive parameter %q is host-filled and cannot be declared sensitive", declaration.Name, name)
		}
		if _, ok := placeholders[name]; !ok {
			return fmt.Errorf("command %q sensitive parameter %q is not used by argv", declaration.Name, name)
		}
		if _, duplicate := seenSensitive[name]; duplicate {
			return fmt.Errorf("command %q lists sensitive parameter %q more than once", declaration.Name, name)
		}
		seenSensitive[name] = struct{}{}
	}

	if len(declaration.Inputs) > MaxInputFiles {
		return fmt.Errorf("command %q declares %d input files; maximum is %d", declaration.Name, len(declaration.Inputs), MaxInputFiles)
	}
	seenInputs := make(map[string]struct{}, len(declaration.Inputs))
	for _, name := range declaration.Inputs {
		if err := ValidateInputFileName(name); err != nil {
			return fmt.Errorf("command %q: %w", declaration.Name, err)
		}
		if _, duplicate := seenInputs[name]; duplicate {
			return fmt.Errorf("command %q declares input file %q more than once", declaration.Name, name)
		}
		seenInputs[name] = struct{}{}
	}
	return nil
}

// ValidateInputFileName applies the exchange file-name grammar, which is the
// grammar the read path already trusts, plus the one rule the read path does
// not have: a name may not begin with a dot. The files common tools read
// without being asked for them — .netrc, .gitconfig, .env, and this package's
// own .tmp — are therefore out of reach of a supplied input.
func ValidateInputFileName(name string) error {
	if !validExchangeComponent(name) || len(name) > MaxFileNameBytes {
		return fmt.Errorf("input file %q must be a plain file name", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("input file %q must not begin with a dot", name)
	}
	return nil
}

func validateExecutable(commandName, executable string) error {
	if executable == "" {
		return fmt.Errorf("command %q argv[0] must be a nonempty executable basename", commandName)
	}
	if placeholderPattern.MatchString(executable) || strings.Contains(executable, "{{") || strings.Contains(executable, "}}") {
		return fmt.Errorf("command %q argv[0] must be a literal executable basename", commandName)
	}
	if strings.HasPrefix(executable, "-") || strings.Contains(executable, "/") || strings.Contains(executable, `\`) || strings.Contains(executable, "..") {
		return fmt.Errorf("command %q argv[0] %q must be a basename without path separators, '..', or a leading dash", commandName, executable)
	}
	return nil
}

// ValidateInputs checks the contents a plugin supplied against the command's
// declaration and returns them in declaration order. It performs no I/O and
// touches nothing durable, so a caller can refuse a run before the host has
// created a row or a byte. Report order is deterministic: declaration order for
// the supplied names, then sorted for the ones that were never declared.
func ValidateInputs(declaration Declaration, supplied map[string]string) ([]InputFile, error) {
	if len(supplied) == 0 {
		return nil, nil
	}
	if len(supplied) > MaxInputFiles {
		return nil, fmt.Errorf("command %q received %d input files; maximum is %d", declaration.Name, len(supplied), MaxInputFiles)
	}
	declared := make(map[string]struct{}, len(declaration.Inputs))
	for _, name := range declaration.Inputs {
		declared[name] = struct{}{}
	}

	ordered := make([]InputFile, 0, len(supplied))
	total := 0
	for _, name := range declaration.Inputs {
		content, ok := supplied[name]
		if !ok {
			continue
		}
		if len(content) > MaxInputBytes {
			return nil, fmt.Errorf("input file %q is %d bytes; maximum is %d", name, len(content), MaxInputBytes)
		}
		total += len(content)
		ordered = append(ordered, InputFile{Name: name, Content: []byte(content)})
	}

	undeclared := make([]string, 0, len(supplied)-len(ordered))
	for name := range supplied {
		if _, ok := declared[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	sort.Strings(undeclared)
	for _, name := range undeclared {
		// Grammar before declared-ness: a name that is not a plain file name
		// could never have been declared, and its own message says what is
		// wrong with it rather than calling it undeclared.
		if err := ValidateInputFileName(name); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("command %q does not declare input file %q", declaration.Name, name)
	}

	if total > MaxAggregateInputBytes {
		return nil, fmt.Errorf("input file contents total %d bytes; maximum is %d", total, MaxAggregateInputBytes)
	}
	return ordered, nil
}

// BuildInvocation substitutes whole argv elements only. exchangeDir is supplied
// by the host and cannot be overridden by the caller's parameter map.
func BuildInvocation(declaration Declaration, params map[string]string, exchangeDir string) (Invocation, error) {
	if err := ValidateDeclaration(declaration); err != nil {
		return Invocation{}, err
	}
	if len(params) > MaxParameters {
		return Invocation{}, fmt.Errorf("command %q received %d parameters; maximum is %d", declaration.Name, len(params), MaxParameters)
	}
	if _, supplied := params["exchange_dir"]; supplied {
		return Invocation{}, fmt.Errorf("parameter %q is host-filled and cannot be supplied by the caller", "exchange_dir")
	}

	totalBytes := 0
	for name, value := range params {
		if !parameterNamePattern.MatchString(name) {
			return Invocation{}, fmt.Errorf("parameter name %q must match %s", name, parameterNamePattern)
		}
		if value == "" {
			return Invocation{}, fmt.Errorf("parameter %q must not be empty", name)
		}
		if len(value) > MaxParameterBytes {
			return Invocation{}, fmt.Errorf("parameter %q is %d bytes; maximum is %d", name, len(value), MaxParameterBytes)
		}
		totalBytes += len(value)
	}
	if totalBytes > MaxAggregateParameterBytes {
		return Invocation{}, fmt.Errorf("parameter values total %d bytes; maximum is %d", totalBytes, MaxAggregateParameterBytes)
	}

	sensitive := make(map[string]struct{}, len(declaration.SensitiveParams))
	for _, name := range declaration.SensitiveParams {
		sensitive[name] = struct{}{}
	}
	view := make(map[string]string, len(params))
	for name, value := range params {
		if _, hidden := sensitive[name]; hidden {
			view[name] = redactedValue
		} else {
			view[name] = value
		}
	}

	argv := make([]string, len(declaration.Argv))
	redacted := make([]string, len(declaration.Argv))
	for i, template := range declaration.Argv {
		match := placeholderPattern.FindStringSubmatch(template)
		if match == nil {
			argv[i] = template
			redacted[i] = template
			continue
		}
		name := match[1]
		var value string
		if name == "exchange_dir" {
			value = exchangeDir
			if value == "" {
				return Invocation{}, fmt.Errorf("host-filled parameter %q must not be empty", name)
			}
		} else {
			var ok bool
			value, ok = params[name]
			if !ok {
				return Invocation{}, fmt.Errorf("required parameter %q was not supplied", name)
			}
			if value == "" {
				return Invocation{}, fmt.Errorf("required parameter %q must not be empty", name)
			}
		}
		argv[i] = value
		if _, hidden := sensitive[name]; hidden {
			redacted[i] = redactedValue
		} else {
			redacted[i] = value
		}
	}

	return Invocation{Argv: argv, RedactedArgv: redacted, ParamView: view}, nil
}

// SameDeclarations compares command declarations by command name. Argv remains
// positional while sensitive parameter names are a set.
func SameDeclarations(a, b []Declaration) bool {
	if len(a) != len(b) {
		return false
	}
	byName := make(map[string]Declaration, len(a))
	for _, declaration := range a {
		if _, duplicate := byName[declaration.Name]; duplicate {
			return false
		}
		byName[declaration.Name] = declaration
	}
	seen := make(map[string]struct{}, len(b))
	for _, declaration := range b {
		if _, duplicate := seen[declaration.Name]; duplicate {
			return false
		}
		seen[declaration.Name] = struct{}{}
		other, ok := byName[declaration.Name]
		if !ok || declaration.Timeout != other.Timeout || !samePositionalStrings(declaration.Argv, other.Argv) || !sameStringSet(declaration.SensitiveParams, other.SensitiveParams) || !sameStringSet(declaration.Inputs, other.Inputs) {
			return false
		}
	}
	return true
}

func samePositionalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return samePositionalStrings(x, y)
}

// ShellJoin returns a human display string. It is not launch data; execution
// always consumes Invocation.Argv directly.
func ShellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		if arg != "" && shellSafePattern.MatchString(arg) {
			quoted[i] = arg
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
