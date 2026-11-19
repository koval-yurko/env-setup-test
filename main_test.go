package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvKeepsProcessEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# comment\nPROBE_A=from-file\nexport PROBE_B=\"quoted\"\nPROBE_C=from-file\n"), 0o600)
	t.Setenv("PROBE_C", "from-env")
	os.Unsetenv("PROBE_A")
	os.Unsetenv("PROBE_B")
	t.Cleanup(func() { os.Unsetenv("PROBE_A"); os.Unsetenv("PROBE_B") })

	fromFile, err := loadDotEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	if r := secretRow("PROBE_A", fromFile); r.Value != "from-file" || r.Detail != "source: .env" {
		t.Errorf("PROBE_A: %+v", r)
	}
	if r := secretRow("PROBE_B", fromFile); r.Value != "quoted" {
		t.Errorf("PROBE_B: %+v", r)
	}
	if r := secretRow("PROBE_C", fromFile); r.Value != "from-env" || r.Detail != "source: env" {
		t.Errorf("PROBE_C: %+v", r)
	}
	if r := secretRow("PROBE_MISSING", fromFile); r.OK {
		t.Errorf("PROBE_MISSING: %+v", r)
	}
}

func TestAWSCredSource(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_TEST")
	t.Setenv("AWS_SESSION_TOKEN", "")
	if got := awsCredSource(map[string]bool{"AWS_ACCESS_KEY_ID": true}); got != "access keys from .env" {
		t.Errorf("got %q", got)
	}
	t.Setenv("AWS_SESSION_TOKEN", "tok")
	if got := awsCredSource(nil); got != "temporary keys from env" {
		t.Errorf("got %q", got)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_PROFILE", "p")
	if got := awsCredSource(nil); got != "profile p" {
		t.Errorf("got %q", got)
	}
}
