package sandbox

import (
	"testing"
)

// TestSandbox performs basic sandbox testing.
func TestSandbox(t *testing.T) {
	sandbox, err := New(t.Context(), ConfigurationLlamaCpp, nil, "date")
	if err != nil {
		t.Fatal("unable to create sandboxed process:", err)
	}
	err = sandbox.Command().Wait()
	if err != nil {
		t.Error("unable to wait for process completion:", err)
	}
	err = sandbox.Close()
	if err != nil {
		t.Error("sandbox closure failed:", err)
	}
}
