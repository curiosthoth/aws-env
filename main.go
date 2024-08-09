package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/curiosthoth/aws-env/internal"
	"os"
)

func runPipeMode(manager *internal.CachedSecretsManager, associativeArrayName string, silent bool) {
	if !silent {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: make sure to pipe the result into some other command instead of printing the vars to stdout\n")
	}

	outputLineFormatter := "%s=%s\n"
	if associativeArrayName != "" {
		outputLineFormatter = associativeArrayName + "[%s]=%s\n"
	}

	// Iterate the lines from os.Stdin and SplitEnvString for each of them and print the result
	reader := bufio.NewReader(os.Stdin)
	for {
		envStr, err := reader.ReadString('\n')

		// Check for EOF (end of input)
		if err != nil {
			if err.Error() == "EOF" {
				break // Exit the loop on EOF
			}
			if !silent {
				_, _ = fmt.Fprintf(os.Stderr, "Error reading line: %v", err)
			}
			continue // Continue to the next iteration on error
		}

		envVar, err := internal.SplitEnvString(envStr)
		if err != nil {
			if !silent {
				_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}
		// Regular Env Var, output and continue
		if envVar.SecretName == nil {
			_, _ = fmt.Fprintf(os.Stdout, outputLineFormatter, envVar.Name, envVar.RawValue)
			_ = os.Stdout.Sync()
			continue
		}
		// Otherwise fetch from SecretsManager
		actualValue, found := manager.Get(*envVar.SecretName, envVar.JMESPath)
		if found {
			_, _ = fmt.Fprintf(os.Stdout, outputLineFormatter, envVar.Name, actualValue)
			_ = os.Stdout.Sync()
		} else {
			jmesPathStr := ""
			if envVar.JMESPath != nil {
				jmesPathStr = *envVar.JMESPath
			}
			if !silent {
				_, _ = fmt.Fprintf(os.Stderr, "Error: Secret %s (path=%s) not found\n", *envVar.SecretName, jmesPathStr)
			}
		}
	}
}

func runInitMode(manager *internal.CachedSecretsManager, silent bool) {

}

func main() {
	// Define a boolean flag with the name "p" (for Pipe Mode)
	pipeMode := flag.Bool("p", false, "Run in Pipe Mode.")
	associativeArrayName := flag.String("a", "", "Name of the associative array (Bash) to output, instead of the env=value format. Only valid when in Pipe Mode.")
	silent := flag.Bool("s", false, "Silent mode. Do not print warnings or errors to stderr.")
	// Parse the command-line flags
	flag.Parse()

	// Create a new CachedSecretsManager
	manager := internal.NewCachedSecretsManager()

	// Check if the -p flag was provided
	if *pipeMode {
		runPipeMode(manager, *associativeArrayName, *silent) // Call the Pipe Mode function
	} else {
		if *associativeArrayName != "" {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: -a flag is only valid in Pipe Mode. Ignoring the flag.\n")
		}
		runInitMode(manager, *silent) // Call the Init Mode function
	}
}
