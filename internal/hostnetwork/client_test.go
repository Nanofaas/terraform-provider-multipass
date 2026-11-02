package hostnetwork

import (
	"os/exec"
	"testing"
)

func TestBridgeValidationAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, address string
		valid         bool
	}{
		{"nf-test", "10.60.0.1/24", true}, {"nf-test", "192.168.20.1/24", true},
		{"this-name-is-too-long", "10.60.0.1/24", false}, {"bad;name", "10.60.0.1/24", false},
		{"nf-test", "8.8.8.8/24", false}, {"nf-test", "10.60.0.0/24", false}, {"nf-test", "10.60.0.255/24", false},
		{"nf-test", "10.60.0.1/32", false}, {"nf-test", "10.0.0.1/7", false}, {"nf-test", "fd00::1/64", false},
	} {
		if err := Validate(tc.name, tc.address); (err == nil) != tc.valid {
			t.Errorf("%v: %v", tc, err)
		}
	}
}
func TestBridgeHostHelper(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 required for helper tests")
	}
	cmd := exec.Command(python, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py", "-v")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
