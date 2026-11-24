// Package files is the disk side of saving attachments: checking a save folder's path, browsing
// and making subfolders inside it, and writing a file under a safe name without ever overwriting
// one. Every name here may come from a stranger (an attachment's file name) or from the page, so
// each path is checked to stay inside the save folder, through junctions and links too.
package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
)

// InvalidError is a path or name that cannot be used, in words for the user.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// CheckRoot checks a save folder's path as the user typed or pasted it: quotes that Windows'
// "Copy as path" adds are taken off; it must be absolute and an existing folder. Nothing is
// created.
func CheckRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	path = strings.TrimSpace(path)
	if path == "" {
		return "", &InvalidError{"Enter the folder's path, for example C:\\Users\\you\\Documents\\Expenses."}
	}
	if !filepath.IsAbs(path) {
		return "", &InvalidError{"Enter the full path, starting with a drive letter such as C:\\."}
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", &InvalidError{fmt.Sprintf("%s does not exist or cannot be opened. Make the folder first.", path)}
	}
	if !info.IsDir() {
		return "", &InvalidError{fmt.Sprintf("%s is a file, not a folder.", path)}
	}
	return path, nil
}

// reserved are names Windows will not use for a file or folder, with any extension.
var reserved = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}

// maxName is the longest file or folder name written, in characters.
const maxName = 120

// SafeName makes a name usable as one file or folder name on Windows: no separators, no
// characters Windows refuses, no control characters, not a reserved device name, no leading or
// trailing dots and spaces, not too long. Empty when nothing usable is left.
func SafeName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, ". ")
	if name == "" || name == ".." {
		return ""
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if reserved[strings.ToLower(strings.TrimRight(base, " "))] || reserved[strings.ToLower(name)] {
		base = "_" + base
	}
	if r := []rune(base); len([]rune(base+ext)) > maxName {
		keep := max(1, maxName-len([]rune(ext)))
		if len(r) > keep {
			base = string(r[:keep])
		}
	}
	return strings.Trim(base, ". ") + ext
}

// Resolve turns a relative path inside a save folder (segments separated by "/", as the page
// sends it) into a full path, and makes sure it stays inside the folder, links and junctions
// followed. The folder must exist.
func Resolve(root, rel string) (string, error) {
	segs, err := segments(rel)
	if err != nil {
		return "", err
	}
	full := root
	for _, seg := range segs {
		full = filepath.Join(full, seg)
		// No link or junction inside a save folder is followed: either could lead anywhere on the
		// disk. (Go does not resolve Windows junctions, so they are checked here, one step at a time.)
		info, err := os.Lstat(full)
		if err != nil {
			return "", &InvalidError{"That folder does not exist."}
		}
		if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || !info.IsDir() {
			return "", &InvalidError{fmt.Sprintf("%q is a link to somewhere else; cockatoo only saves into real folders.", seg)}
		}
	}
	if err := inside(root, full); err != nil {
		return "", err
	}
	return full, nil
}

// segments splits a relative path and checks each part is a plain name.
func segments(rel string) ([]string, error) {
	rel = strings.Trim(strings.ReplaceAll(rel, `\`, "/"), "/")
	if rel == "" {
		return nil, nil
	}
	var out []string
	for _, s := range strings.Split(rel, "/") {
		if s == "" || s == "." || s == ".." || SafeName(s) != s {
			return nil, &InvalidError{fmt.Sprintf("%q is not a folder name cockatoo will use.", s)}
		}
		out = append(out, s)
	}
	return out, nil
}

// inside checks that full, followed through links and junctions, is root or below it.
func inside(root, full string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return &InvalidError{fmt.Sprintf("The save folder %s cannot be opened.", root)}
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return &InvalidError{"That folder does not exist."}
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return &InvalidError{"That folder is outside the save folder."}
	}
	return nil
}

// Subfolders lists the folders directly inside a relative path of a save folder, by name.
// Hidden and system folders are left out.
func Subfolders(root, rel string) ([]string, error) {
	dir, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		out = append(out, e.Name())
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out, nil
}

// MakeSubfolder makes a folder called name inside a relative path of a save folder (or uses it
// when it is already there), and returns the new relative path.
func MakeSubfolder(root, rel, name string) (string, error) {
	parent, err := Resolve(root, rel)
	if err != nil {
		return "", err
	}
	clean := SafeName(name)
	if clean == "" || clean != strings.TrimSpace(name) {
		return "", &InvalidError{fmt.Sprintf("%q cannot be a folder name. Leave out \\ / : * ? \" < > | and names like CON.", strings.TrimSpace(name))}
	}
	full := filepath.Join(parent, clean)
	if err := os.Mkdir(full, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("make %s: %w", full, err)
	}
	if err := inside(root, full); err != nil {
		return "", err
	}
	if rel = strings.Trim(strings.ReplaceAll(rel, `\`, "/"), "/"); rel == "" {
		return clean, nil
	}
	return rel + "/" + clean, nil
}

// Write saves data in dir under name, never over an existing file: when the name is taken it
// becomes "name (2).ext", "name (3).ext" and so on. The data goes to a temporary file first and
// is linked to its final name only when complete, so a stopped save leaves no half-written file
// under a real name. The file is marked as coming from the internet, as mail programs do, so
// Windows warns before opening something that runs. It returns the full path written.
func Write(dir, name string, data []byte) (string, error) {
	tmpName := make([]byte, 6)
	if _, err := rand.Read(tmpName); err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, ".cockatoo-"+hex.EncodeToString(tmpName)+".part")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("write in %s: %w", dir, err)
	}
	defer os.Remove(tmp)
	markFromInternet(tmp)

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		final := filepath.Join(dir, candidate)
		// A hard link is made only when the name is free: no file is ever replaced.
		err := os.Link(tmp, final)
		if err == nil {
			return final, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("save %s: %w", final, err)
		}
	}
	return "", fmt.Errorf("save %s: too many files with this name", filepath.Join(dir, name))
}

// markFromInternet adds the Zone.Identifier stream Windows reads for "this came from the
// internet" (the same mark browsers and mail programs add). Best effort: on a disk that cannot
// hold it, the file is saved without it.
func markFromInternet(path string) {
	if runtime.GOOS != "windows" {
		return
	}
	if err := os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o644); err != nil {
		log.Printf("could not mark a saved attachment as from the internet: %v", err)
	}
}
