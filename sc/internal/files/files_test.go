package files

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"invoice.pdf":               "invoice.pdf",
		"../../evil.exe":            "_.._evil.exe",
		`C:\Windows\x.dll`:          "C__Windows_x.dll",
		"a:b*c?d\"e<f>g|h.txt":      "a_b_c_d_e_f_g_h.txt",
		"con.txt":                   "_con.txt",
		"NUL":                       "_NUL",
		"report.  ":                 "report",
		" .hidden":                  "hidden",
		"line\r\nbreak\x00.pdf":     "line__break_.pdf",
		"..":                        "",
		"":                          "",
		"Rechnung Müller – Nov.pdf": "Rechnung Müller – Nov.pdf",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 300) + ".pdf"
	if got := SafeName(long); len([]rune(got)) > maxName || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("long name %q", got)
	}
}

func TestCheckRoot(t *testing.T) {
	dir := t.TempDir()
	if got, err := CheckRoot(` "` + dir + `" `); err != nil || got != filepath.Clean(dir) {
		t.Fatalf("quoted path: %q %v", got, err)
	}
	if _, err := CheckRoot("relative\\path"); !errors.As(err, new(*InvalidError)) {
		t.Errorf("relative: %v", err)
	}
	if _, err := CheckRoot(filepath.Join(dir, "missing")); !errors.As(err, new(*InvalidError)) {
		t.Errorf("missing: %v", err)
	}
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("x"), 0o644)
	if _, err := CheckRoot(f); !errors.As(err, new(*InvalidError)) {
		t.Errorf("a file: %v", err)
	}
}

func TestBrowseAndMake(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "2026-10"), 0o755)
	os.Mkdir(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644)

	got, err := Subfolders(root, "")
	if err != nil || strings.Join(got, ",") != "2026-10" {
		t.Fatalf("subfolders %v %v", got, err)
	}
	rel, err := MakeSubfolder(root, "", "2026-11")
	if err != nil || rel != "2026-11" {
		t.Fatalf("make %q %v", rel, err)
	}
	rel, err = MakeSubfolder(root, "2026-11", "2026-11-24")
	if err != nil || rel != "2026-11/2026-11-24" {
		t.Fatalf("make nested %q %v", rel, err)
	}
	if _, err := MakeSubfolder(root, "2026-11", "2026-11-24"); err != nil {
		t.Errorf("making one that exists: %v", err)
	}
	for _, bad := range []string{"..", "a/b", `a\b`, "con", "x:y", "", " "} {
		if _, err := MakeSubfolder(root, "", bad); err == nil {
			t.Errorf("made a folder called %q", bad)
		}
	}
	for _, bad := range []string{"..", "../x", "2026-11/../..", "C:/Windows", "a:b"} {
		if _, err := Resolve(root, bad); err == nil {
			t.Errorf("resolved %q", bad)
		}
	}
	if _, err := Resolve(root, "missing"); err == nil {
		t.Error("resolved a folder that does not exist")
	}
}

// A junction inside the save folder that points outside it is refused.
func TestResolveRefusesJunctionOut(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows thing")
	}
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "escape")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Skipf("cannot make a junction here: %v %s", err, out)
	}
	if _, err := Resolve(root, "escape"); err == nil {
		t.Fatal("followed a junction out of the save folder")
	}
}

func TestWriteNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, body := range []string{"one", "two", "three"} {
		p, err := Write(dir, "invoice.pdf", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Base(p))
	}
	if strings.Join(paths, ",") != "invoice.pdf,invoice (2).pdf,invoice (3).pdf" {
		t.Fatalf("names %v", paths)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "invoice.pdf")); string(b) != "one" {
		t.Errorf("the first file was overwritten: %q", b)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("a temporary file was left: %s", e.Name())
		}
	}
	if runtime.GOOS == "windows" {
		if mark, err := os.ReadFile(filepath.Join(dir, "invoice.pdf") + ":Zone.Identifier"); err != nil || !strings.Contains(string(mark), "ZoneId=3") {
			t.Errorf("not marked as from the internet: %q %v", mark, err)
		}
	}
}
