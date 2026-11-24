package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// A browser that can show the UI as an app window (--app=: no tabs, no address bar). Edge and
// Chrome are both Chromium and take the same flags, so the UI is not built for either one: it
// is a plain web page, and these are only the two windows it can be shown in without a browser
// frame. Edge is first because every Windows 10, 11 and Server 2022 machine has it.
type appBrowser struct {
	name string // for the log and the profile folder
	exe  string // file name under App Paths
	rel  string // path under %LOCALAPPDATA% and the Program Files folders
}

var appBrowsers = []appBrowser{
	{"edge", "msedge.exe", `Microsoft\Edge\Application\msedge.exe`},
	{"chrome", "chrome.exe", `Google\Chrome\Application\chrome.exe`},
}

// openAppWindow opens the UI as an app window in Edge, or Chrome when Edge is not there, each
// with the app's own profile folder; the last resort is a tab in the default browser. It does
// not wait on the browser's process: Chromium hands windows between processes, so that is no
// measure of whether the window is open. The page's /api/events connection is.
func openAppWindow(url string) {
	if runtime.GOOS == "windows" {
		for _, b := range appBrowsers {
			for _, exe := range browserCandidates(appPaths(b.exe), b.rel, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")) {
				if _, err := os.Stat(exe); err != nil {
					continue
				}
				// One profile folder, kept between runs: a fresh profile costs seconds every
				// start, and a kept one remembers the window's size and place.
				base, err := os.UserCacheDir()
				if err != nil {
					base = os.TempDir()
				}
				profile := filepath.Join(base, "cockatoo", b.name+"-profile")
				if err := os.MkdirAll(profile, 0o755); err != nil {
					log.Printf("%s profile folder: %v", b.name, err)
					break
				}
				cmd := exec.Command(exe, "--app="+url, "--user-data-dir="+profile, "--no-first-run",
					"--no-default-browser-check", "--window-size=1200,800")
				if err := cmd.Start(); err != nil {
					log.Printf("%s: %v", b.name, err)
					break
				}
				go func() { _ = cmd.Wait() }()
				log.Printf("opened the window in %s", b.name)
				return
			}
		}
		// The last resort: the default browser, as an ordinary tab.
		if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err == nil {
			log.Printf("opened %s in the default browser", url)
			return
		}
	}
	fmt.Println("Open", url, "in a browser.")
}

// browserCandidates are the places a browser's exe may be, in the order they are tried: where
// the App Paths registry key says it is (machine, then user), the per-user install under
// %LOCALAPPDATA%, then the two Program Files folders. Quotes around a registry value are
// removed, blanks and repeats left out; the caller tries each until one exists.
func browserCandidates(appPaths []string, rel, localAppData, programFilesX86, programFiles string) []string {
	out, seen := []string{}, map[string]bool{}
	add := func(p string) {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p == "" || seen[strings.ToLower(p)] {
			return
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	for _, p := range appPaths {
		add(p)
	}
	for _, base := range []string{localAppData, programFilesX86, programFiles} {
		if strings.TrimSpace(base) != "" {
			add(filepath.Join(base, rel))
		}
	}
	return out
}
