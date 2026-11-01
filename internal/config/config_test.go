package config

import "testing"

func TestRejectsInsecureProductionURL(t *testing.T) {
	t.Setenv("KB_PUBLIC_URL", "http://example.com")
	t.Setenv("KB_DEV", "false")
	if _, e := Load(); e == nil {
		t.Fatal("insecure production accepted")
	}
}
func TestLocalDevURLAllowed(t *testing.T) {
	t.Setenv("KB_PUBLIC_URL", "http://localhost:8080")
	t.Setenv("KB_DEV", "true")
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if c.ListenAddr != ":8080" {
		t.Fatal(c.ListenAddr)
	}
}
