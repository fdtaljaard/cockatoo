//go:build !windows

package secret

import "errors"

var errWindowsOnly = errors.New("saving a password needs Windows (DPAPI)")

// Seal is only available on Windows.
func Seal(plain []byte) ([]byte, error) { return nil, errWindowsOnly }

// Open is only available on Windows.
func Open(sealed []byte) ([]byte, error) { return nil, errWindowsOnly }
