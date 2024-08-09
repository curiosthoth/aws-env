package internal

import (
	"fmt"
	"testing"
)

func Test_SplitEnvString(t *testing.T) {
	e1, err := SplitEnvString("env1=val1")
	fmt.Printf("%v %v\n", e1, err)

	e2, err := SplitEnvString("env2=val2=qq")
	fmt.Printf("%v %v\n", e2, err)

	e3, err := SplitEnvString("env3=secretsmanager://val3/sub/qq")
	fmt.Printf("%v %v\n", e3, err)

	e4, err := SplitEnvString("env4=secretsmanager://val4/sub/qq##jmesPath-1")
	fmt.Printf("%v %v\n", e4, err)

	e5, err := SplitEnvString("env4=secretsmanager://val4/sub/qq##jmesPath-1##jmesPath-2")
	fmt.Printf("%v %v\n", e5, err)
}
