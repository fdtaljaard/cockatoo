//go:build windows

package secret

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	plain := []byte("abcd efgh ijkl mnop")
	sealed, err := Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("the sealed value contains the plain text")
	}
	got, err := Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("opened %q, want %q", got, plain)
	}

	// A changed value does not open.
	sealed[len(sealed)-1] ^= 0xff
	if _, err := Open(sealed); err == nil {
		t.Fatal("a changed value opened")
	}
}

func TestSealRefusesEmpty(t *testing.T) {
	if _, err := Seal(nil); err == nil {
		t.Fatal("sealed nothing")
	}
	if _, err := Open(nil); err == nil {
		t.Fatal("opened nothing")
	}
}
