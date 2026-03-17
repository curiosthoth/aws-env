package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/curiosthoth/aws-env/internal"
)

type valueGetter interface {
	Get(scheme string, secretName string, jmesPath *string) (string, bool, string)
}

// cliDeps isolates process and environment side effects so the CLI can be tested without execing.
type cliDeps struct {
	getEnviron func() []string
	setEnv     func(string, string) error
	lookPath   func(string) (string, error)
	exec       func(string, []string, []string) error
}

var (
	newManager              = func() valueGetter { return internal.NewCachedSecretsManager() }
	cliArgs                 = func() []string { return os.Args[1:] }
	cliStdin      io.Reader = os.Stdin
	cliStdout     io.Writer = os.Stdout
	cliStderr     io.Writer = os.Stderr
	osEnviron               = os.Environ
	osSetenv                = os.Setenv
	lookPathFn              = exec.LookPath
	syscallExecFn           = syscall.Exec
	osExit                  = os.Exit
)

func runPipeMode(
	reader io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	manager valueGetter,
	delimiter string,
	associativeArrayName string,
	export bool,
	silent bool,
) {
	if !silent {
		_, _ = fmt.Fprintf(stderr, "Warning: make sure to pipe the result into some other command instead of printing the vars to stdout\n")
	}
	if export && associativeArrayName != "" {
		_, _ = fmt.Fprintf(stderr, "Error: only one of -a or -e can be used at a time. Stopped.")
		return
	}

	outputLineFormatter := "%s=%s\n"
	if associativeArrayName != "" {
		outputLineFormatter = associativeArrayName + "[%s]=%s\n"
	}
	if export {
		outputLineFormatter = "export " + outputLineFormatter
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		envVar, err := internal.SplitEnvString(scanner.Text(), delimiter)
		if err != nil {
			if !silent {
				_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			}
			continue
		}

		if envVar.SecretName == nil || envVar.Scheme == nil {
			_, _ = fmt.Fprintf(stdout, outputLineFormatter, envVar.Name, envVar.RawValue)
			continue
		}

		actualValue, found, errMsg := manager.Get(*envVar.Scheme, *envVar.SecretName, envVar.JMESPath)
		writeLookupResult(stdout, stderr, outputLineFormatter, envVar, actualValue, found, errMsg, silent)
	}

	if err := scanner.Err(); err != nil && !silent {
		_, _ = fmt.Fprintf(stderr, "Error reading line: %v", err)
	}
}

// runInitMode resolves remote env vars in the current environment before replacing the process.
func runInitMode(
	manager valueGetter,
	delimiter string,
	silent bool,
	stderr io.Writer,
	args []string,
	deps cliDeps,
) error {
	for _, envVarLine := range deps.getEnviron() {
		envVar, err := internal.SplitEnvString(envVarLine, delimiter)
		if err != nil {
			if !silent {
				_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			}
			continue
		}
		if envVar.SecretName == nil || envVar.Scheme == nil {
			continue
		}

		actualValue, found, errMsg := manager.Get(*envVar.Scheme, *envVar.SecretName, envVar.JMESPath)
		if found {
			_ = deps.setEnv(envVar.Name, actualValue)
			continue
		}
		if silent {
			continue
		}
		writeLookupError(stderr, envVar, errMsg)
	}

	path, err := deps.lookPath(args[0])
	if err != nil {
		return err
	}
	return deps.exec(path, args, deps.getEnviron())
}

// writeLookupResult writes the resolved value or emits the corresponding lookup error.
func writeLookupResult(
	stdout io.Writer,
	stderr io.Writer,
	outputLineFormatter string,
	envVar internal.EnvVar,
	actualValue string,
	found bool,
	errMsg string,
	silent bool,
) {
	if found {
		_, _ = fmt.Fprintf(stdout, outputLineFormatter, envVar.Name, actualValue)
		return
	}
	if silent {
		return
	}
	writeLookupError(stderr, envVar, errMsg)
}

// writeLookupError formats a backend lookup failure consistently for both CLI modes.
func writeLookupError(stderr io.Writer, envVar internal.EnvVar, errMsg string) {
	jmesPathStr := ""
	if envVar.JMESPath != nil {
		jmesPathStr = *envVar.JMESPath
	}
	if errMsg != "" {
		_, _ = fmt.Fprintf(stderr, "Error: Failed to retrieve %s %s (path=%s): %s\n", *envVar.Scheme, *envVar.SecretName, jmesPathStr, errMsg)
		return
	}
	_, _ = fmt.Fprintf(stderr, "Error: %s %s (path=%s) not found\n", *envVar.Scheme, *envVar.SecretName, jmesPathStr)
}

// runCLI parses flags and executes the requested mode.
func runCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, manager valueGetter, deps cliDeps) int {
	fs := flag.NewFlagSet("aws-env", flag.ContinueOnError)
	fs.SetOutput(stderr)

	pipeMode := fs.Bool("p", false, "Run in Pipe Mode.")
	associativeArrayName := fs.String("a", "", "Name of the associative array (Bash) to output, instead of the env=value format. Only valid when in Pipe Mode.")
	export := fs.Bool("e", false, "Output export commands for the variables. Only valid when in Pipe Mode.")
	silent := fs.Bool("s", false, "Silent mode. Do not print warnings or errors to stderr.")
	delimiter := fs.String("d", internal.DefaultDelimiter, "Delimiter used to separate the remote name from nested JSON keys.")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *delimiter == "" {
		_, _ = fmt.Fprintf(stderr, "Error: -d delimiter must not be empty\n")
		return 2
	}

	if *pipeMode {
		runPipeMode(stdin, stdout, stderr, manager, *delimiter, *associativeArrayName, *export, *silent)
		return 0
	}

	if *associativeArrayName != "" {
		_, _ = fmt.Fprintf(stderr, "Warning: -a flag is only valid in Pipe Mode. Ignoring the flag.\n")
	}
	if *export {
		_, _ = fmt.Fprintf(stderr, "Warning: -e flag is only valid in Pipe Mode. Ignoring the flag.\n")
	}
	if len(fs.Args()) == 0 {
		_, _ = fmt.Fprintln(stderr, "Error: missing command to exec")
		return 2
	}
	if err := runInitMode(manager, *delimiter, *silent, stderr, fs.Args(), deps); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func defaultCliDeps() cliDeps {
	return cliDeps{
		getEnviron: osEnviron,
		setEnv:     osSetenv,
		lookPath:   lookPathFn,
		exec: func(path string, args []string, env []string) error {
			return syscallExecFn(path, args, env)
		},
	}
}

// runMain wires the real process dependencies into the testable CLI implementation.
func runMain() int {
	return runCLI(cliArgs(), cliStdin, cliStdout, cliStderr, newManager(), defaultCliDeps())
}

func main() {
	if code := runMain(); code != 0 {
		osExit(code)
	}
}
