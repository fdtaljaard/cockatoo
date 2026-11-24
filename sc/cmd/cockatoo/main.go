// cockatoo is a portable email client: one exe, one DuckDB file beside it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"cockatoo/internal/api"
	"cockatoo/internal/store"
	"cockatoo/web"
)

// version is stamped by build.ps1 (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	port := flag.Int("port", 7332, "port to listen on (127.0.0.1 only)")
	dataDir := flag.String("data", exeDir, "folder for cockatoo.duckdb")
	noBrowser := flag.Bool("no-browser", false, "do not open a window; just serve")
	flag.Parse()

	if err := run(*port, *dataDir, !*noBrowser); err != nil {
		log.Fatal(err)
	}
}

func run(port int, dataDir string, openWindow bool) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Log to a file beside the data as well as the console, so a double-clicked exe that
	// stops can still be understood afterwards.
	if f, err := os.OpenFile(filepath.Join(dataDir, "cockatoo.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	// Why it stopped, written while the log file is still open.
	defer func() {
		if err != nil {
			log.Printf("stopped: %v", err)
		}
	}()
	log.Printf("cockatoo %s starting (pid %d)", version, os.Getpid())

	dbPath := filepath.Join(dataDir, "cockatoo.duckdb")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	log.Printf("database %s", dbPath)

	presence := &api.Presence{}
	srv := &http.Server{
		Handler:           api.WithEvents(api.Handler(st, web.Dist(), version), presence),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if alreadyRunning(url) {
			// A second double-click should bring up the app, not die quietly.
			log.Printf("cockatoo is already running on %s; opening its window", url)
			if openWindow {
				openAppWindow(url)
			}
			return nil
		}
		return fmt.Errorf("port %d is in use by another program: %w (start with -port <other>)", port, err)
	}
	log.Printf("listening on %s", url)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	// The app lives as long as a window is connected to /api/events. When the last one has
	// been gone for the grace period, stop. Without a window (-no-browser) it runs until Ctrl+C.
	windowGone := make(chan struct{})
	if openWindow {
		openAppWindow(url)
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for range t.C {
				if presence.Gone(10 * time.Second) {
					close(windowGone)
					return
				}
			}
		}()
	} else {
		fmt.Println("Open", url, "in a browser. Press Ctrl+C to stop.")
	}

	select {
	case <-ctx.Done():
		log.Print("stopping")
	case <-windowGone:
		log.Print("window closed, stopping")
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// alreadyRunning says whether the thing on our port is another cockatoo.
func alreadyRunning(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
