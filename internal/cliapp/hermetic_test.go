package cliapp_test

import (
	"os"
	"testing"
)

// TestMain gives every test a disposable HOME so profile creation never
// adopts credentials from the developer's real home or keychain. Tests that
// need a specific home still override it with t.Setenv.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "cliapp-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
