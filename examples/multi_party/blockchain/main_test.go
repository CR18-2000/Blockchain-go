package main

import (
	"os"
	"testing"
)

func TestMain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short mode")
	}
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = os.Args[:1]
	maincrypto()
}
