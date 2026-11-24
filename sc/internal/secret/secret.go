// Package secret encrypts small secrets, such as a mail password, before they are stored in
// the DuckDB file. On Windows it uses DPAPI (CryptProtectData): the key belongs to the signed-in
// Windows user and never leaves Windows, so a sealed value opens only for that user on that
// computer. A copy of the DuckDB file on another machine, or read by another user, shows bytes
// that cannot be opened.
package secret

// entropy is mixed into every seal. It is not a key: it only keeps a value sealed by cockatoo
// from opening for a program that calls DPAPI without it. Never change it: every password already
// saved would stop opening. A new format gets a new version beside this one.
var entropy = []byte("cockatoo/secret/v1")
