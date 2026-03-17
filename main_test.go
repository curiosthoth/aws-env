package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/curiosthoth/aws-env/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeManager struct {
	values map[string]string
	errs   map[string]string
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (f fakeManager) Get(scheme string, secretName string, jmesPath *string) (string, bool, string) {
	k := scheme + "://" + secretName
	if jmesPath != nil {
		k += "#" + *jmesPath
	}
	if errMsg, ok := f.errs[k]; ok {
		return "", false, errMsg
	}
	if value, ok := f.values[k]; ok {
		return value, true, ""
	}
	return "", false, ""
}

func TestRunPipeMode(t *testing.T) {
	manager := fakeManager{
		values: map[string]string{
			"secretsmanager://app/secret#foo.bar": "resolved",
			"ssm:///config/value":                 "plain",
		},
		errs: map[string]string{
			"secretsmanager://missing": "not found upstream",
		},
	}

	t.Run("default output and errors", func(t *testing.T) {
		in := bytes.NewBufferString("A=plain\nB=secretsmanager://app/secret##foo##bar\nC=secretsmanager://missing\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, internal.DefaultDelimiter, "", false, false)

		assert.Equal(t, "A=plain\nB=resolved\n", stdout.String())
		assert.Contains(t, stderr.String(), "Warning:")
		assert.Contains(t, stderr.String(), "Failed to retrieve secretsmanager missing")
	})

	t.Run("export and associative array conflict", func(t *testing.T) {
		in := bytes.NewBufferString("A=plain\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, internal.DefaultDelimiter, "ENV", true, false)

		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "only one of -a or -e")
	})

	t.Run("custom delimiter and silent mode", func(t *testing.T) {
		in := bytes.NewBufferString("A=ssm:///config/value\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, "::", "", false, true)

		assert.Equal(t, "A=plain\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("scanner error and fallback not found message", func(t *testing.T) {
		in := errReader{}
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, fakeManager{}, internal.DefaultDelimiter, "", false, false)

		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "Error reading line")
	})

	t.Run("export formatting", func(t *testing.T) {
		in := bytes.NewBufferString("A=plain\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, internal.DefaultDelimiter, "", true, true)

		assert.Equal(t, "export A=plain\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("invalid env input writes parse error", func(t *testing.T) {
		in := bytes.NewBufferString("INVALID\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, internal.DefaultDelimiter, "", false, false)

		assert.Empty(t, stdout.String())
		assert.Contains(t, stderr.String(), "invalid env string")
	})

	t.Run("associative array formatting", func(t *testing.T) {
		in := bytes.NewBufferString("A=plain\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		runPipeMode(in, &stdout, &stderr, manager, internal.DefaultDelimiter, "ENV", false, true)

		assert.Equal(t, "ENV[A]=plain\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestRunInitMode(t *testing.T) {
	manager := fakeManager{
		values: map[string]string{
			"secretsmanager://app/secret#foo.bar": "resolved",
		},
		errs: map[string]string{
			"ssm:///missing": "missing parameter",
		},
	}
	setValues := map[string]string{}

	var stderr bytes.Buffer
	err := runInitMode(manager, internal.DefaultDelimiter, false, &stderr, []string{"echo", "hello"}, cliDeps{
		getEnviron: func() []string {
			return []string{
				"PLAIN=value",
				"TARGET=secretsmanager://app/secret##foo##bar",
				"OTHER=ssm:///missing",
				"INVALID",
			}
		},
		setEnv: func(key string, value string) error {
			setValues[key] = value
			return nil
		},
		lookPath: func(file string) (string, error) {
			assert.Equal(t, "echo", file)
			return "/bin/echo", nil
		},
		exec: func(path string, args []string, env []string) error {
			assert.Equal(t, "/bin/echo", path)
			assert.Equal(t, []string{"echo", "hello"}, args)
			assert.NotEmpty(t, env)
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "resolved", setValues["TARGET"])
	assert.Contains(t, stderr.String(), "missing parameter")
	assert.Contains(t, stderr.String(), "invalid env string")

	t.Run("silent lookup failure", func(t *testing.T) {
		var silentStderr bytes.Buffer
		err := runInitMode(fakeManager{}, internal.DefaultDelimiter, true, &silentStderr, []string{"echo"}, cliDeps{
			getEnviron: func() []string { return []string{"A=secretsmanager://missing"} },
			setEnv:     func(string, string) error { return nil },
			lookPath:   func(string) (string, error) { return "/bin/echo", nil },
			exec:       func(string, []string, []string) error { return nil },
		})
		require.NoError(t, err)
		assert.Empty(t, silentStderr.String())
	})
}

func TestRunCLI(t *testing.T) {
	manager := fakeManager{values: map[string]string{"secretsmanager://name": "value"}}

	t.Run("pipe mode", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := runCLI([]string{"-p"}, bytes.NewBufferString("A=secretsmanager://name\n"), &stdout, &stderr, manager, cliDeps{})
		assert.Equal(t, 0, code)
		assert.Equal(t, "A=value\n", stdout.String())
	})

	t.Run("empty delimiter", func(t *testing.T) {
		code := runCLI([]string{"-p", "-d", ""}, bytes.NewBufferString(""), &bytes.Buffer{}, &bytes.Buffer{}, manager, cliDeps{})
		assert.Equal(t, 2, code)
	})

	t.Run("missing command", func(t *testing.T) {
		code := runCLI([]string{}, bytes.NewBufferString(""), &bytes.Buffer{}, &bytes.Buffer{}, manager, cliDeps{})
		assert.Equal(t, 2, code)
	})

	t.Run("flag parse error", func(t *testing.T) {
		code := runCLI([]string{"-unknown"}, bytes.NewBufferString(""), &bytes.Buffer{}, &bytes.Buffer{}, manager, cliDeps{})
		assert.Equal(t, 2, code)
	})

	t.Run("init mode errors", func(t *testing.T) {
		var stderr bytes.Buffer
		code := runCLI([]string{"echo"}, bytes.NewBufferString(""), &bytes.Buffer{}, &stderr, manager, cliDeps{
			getEnviron: func() []string { return []string{} },
			setEnv:     func(string, string) error { return nil },
			lookPath: func(string) (string, error) {
				return "", errors.New("missing executable")
			},
			exec: func(string, []string, []string) error { return nil },
		})
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr.String(), "missing executable")
	})

	t.Run("init mode warnings", func(t *testing.T) {
		var stderr bytes.Buffer
		code := runCLI([]string{"-a", "ENV", "-e", "echo"}, bytes.NewBufferString(""), &bytes.Buffer{}, &stderr, manager, cliDeps{
			getEnviron: func() []string { return []string{} },
			setEnv:     func(string, string) error { return nil },
			lookPath:   func(string) (string, error) { return "/bin/echo", nil },
			exec:       func(string, []string, []string) error { return nil },
		})
		assert.Equal(t, 0, code)
		assert.Contains(t, stderr.String(), "-a flag is only valid")
		assert.Contains(t, stderr.String(), "-e flag is only valid")
	})
}

func TestRunMainAndMain(t *testing.T) {
	originalArgs := cliArgs
	originalStdin := cliStdin
	originalStdout := cliStdout
	originalStderr := cliStderr
	originalNewManager := newManager
	originalEnviron := osEnviron
	originalSetenv := osSetenv
	originalLookPath := lookPathFn
	originalExec := syscallExecFn
	originalExit := osExit
	defer func() {
		cliArgs = originalArgs
		cliStdin = originalStdin
		cliStdout = originalStdout
		cliStderr = originalStderr
		newManager = originalNewManager
		osEnviron = originalEnviron
		osSetenv = originalSetenv
		lookPathFn = originalLookPath
		syscallExecFn = originalExec
		osExit = originalExit
	}()

	cliArgs = func() []string { return []string{"-p"} }
	cliStdin = bytes.NewBufferString("A=secretsmanager://name\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cliStdout = &stdout
	cliStderr = &stderr
	newManager = func() valueGetter {
		return fakeManager{values: map[string]string{"secretsmanager://name": "value"}}
	}

	assert.Equal(t, 0, runMain())
	assert.Equal(t, "A=value\n", stdout.String())

	osEnviron = func() []string { return []string{} }
	osSetenv = func(string, string) error { return nil }
	lookPathFn = func(string) (string, error) { return "/bin/echo", nil }
	syscallExecFn = func(string, []string, []string) error { return nil }

	deps := defaultCliDeps()
	require.NotNil(t, deps.getEnviron)
	require.NotNil(t, deps.setEnv)
	require.NotNil(t, deps.lookPath)
	require.NotNil(t, deps.exec)
	_, err := deps.lookPath("echo")
	require.NoError(t, err)
	require.NoError(t, deps.setEnv("A", "B"))
	require.NoError(t, deps.exec("/bin/echo", []string{"echo"}, os.Environ()))

	main()

	exitCode := 0
	osExit = func(code int) {
		exitCode = code
	}
	cliArgs = func() []string { return []string{"-unknown"} }
	main()
	assert.Equal(t, 2, exitCode)
}

func TestLookupWriters(t *testing.T) {
	scheme := internal.SecretsManagerScheme
	secretName := "name"
	path := "foo.bar"
	envVar := internal.EnvVar{
		Name:       "A",
		Scheme:     &scheme,
		SecretName: &secretName,
		JMESPath:   &path,
	}

	t.Run("write success", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		writeLookupResult(&stdout, &stderr, "%s=%s\n", envVar, "value", true, "", false)
		assert.Equal(t, "A=value\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("silent miss", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		writeLookupResult(&stdout, &stderr, "%s=%s\n", envVar, "", false, "", true)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("not found formatting", func(t *testing.T) {
		var stderr bytes.Buffer
		writeLookupError(&stderr, envVar, "")
		assert.Contains(t, stderr.String(), "not found")
	})

	t.Run("missing path formatting", func(t *testing.T) {
		var stderr bytes.Buffer
		noPathVar := internal.EnvVar{
			Name:       "B",
			Scheme:     &scheme,
			SecretName: &secretName,
		}
		writeLookupError(&stderr, noPathVar, "boom")
		assert.Contains(t, stderr.String(), "path=")
		assert.Contains(t, stderr.String(), "boom")
	})
}
