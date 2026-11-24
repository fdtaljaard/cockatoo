//go:build windows

package secret

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Seal encrypts plain for the signed-in Windows user.
func Seal(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, errors.New("seal: nothing to encrypt")
	}
	in, ent := blob(plain), blob(entropy)
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	return take(out), nil
}

// Open decrypts a value sealed by Seal. It fails for another Windows user, on another computer,
// and for bytes that were changed.
func Open(sealed []byte) ([]byte, error) {
	if len(sealed) == 0 {
		return nil, errors.New("open: nothing to decrypt")
	}
	in, ent := blob(sealed), blob(entropy)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("open: this value was not saved by this Windows user on this computer: %w", err)
	}
	return take(out), nil
}

func blob(b []byte) windows.DataBlob {
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// take copies what Windows returned into Go memory and frees Windows' copy.
func take(out windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size))
}
