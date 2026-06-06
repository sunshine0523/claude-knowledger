package cli

import (
	"bytes"
	"net/http"
	"testing"
)

func TestServeCommandUsesConfiguredAddress(t *testing.T) {
	oldListenAndServe := listenAndServe
	defer func() { listenAndServe = oldListenAndServe }()

	var gotAddress string
	listenAndServe = func(address string, handler http.Handler) error {
		gotAddress = address
		if handler == nil {
			t.Fatalf("expected non-nil web handler")
		}
		return nil
	}

	buf := new(bytes.Buffer)
	cmd := NewRootCommandWithAddress(nil, ":34125")
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"serve"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if gotAddress != ":34125" {
		t.Fatalf("expected address :34125, got %q", gotAddress)
	}
	if !bytes.Contains(buf.Bytes(), []byte("http://127.0.0.1:34125/")) {
		t.Fatalf("expected serve output to include URL, got %q", buf.String())
	}
}
