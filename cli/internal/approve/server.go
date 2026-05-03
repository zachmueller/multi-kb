package approve

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/route"
)

const defaultIdleTimeout = 5 * time.Minute

// StartServer launches the approval web UI server.
// It binds to an available port on localhost, opens the browser, and blocks
// until shut down by idle timeout, all notes resolved, or Ctrl+C.
func StartServer(pendingDir string, cfg *config.Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return runServer(ctx, pendingDir, cfg, defaultIdleTimeout, os.Stdout, true)
}

// runServer is the testable core of StartServer.
// openBrowserFlag controls whether the browser is opened (false in tests).
func runServer(ctx context.Context, pendingDir string, cfg *config.Config, idleTimeout time.Duration, out io.Writer, openBrowserFlag bool) error {
	mux := http.NewServeMux()

	// Idle tracking
	idleTimer := time.NewTimer(idleTimeout)
	var idleMu sync.Mutex

	resetIdle := func() {
		idleMu.Lock()
		defer idleMu.Unlock()
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleTimeout)
	}

	// Register API and asset routes
	innerMux := http.NewServeMux()
	registerRoutes(innerMux, pendingDir, cfg)

	// Wrap with idle-reset and all-resolved check
	allResolvedCh := make(chan struct{}, 1)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resetIdle()
		innerMux.ServeHTTP(w, r)

		// After POST actions, check if all notes are resolved
		if r.Method == http.MethodPost {
			go func() {
				count, err := route.PendingCount(pendingDir)
				if err == nil && count == 0 {
					select {
					case allResolvedCh <- struct{}{}:
					default:
					}
				}
			}()
		}
	})

	// Bind to auto-selected port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("approve: cannot bind to port: %w", err)
	}

	addr := listener.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://127.0.0.1:%d", addr.Port)

	fmt.Fprintf(out, "Approval UI: %s\n", url)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if openBrowserFlag {
		go openBrowser(url)
	}

	// Shutdown coordination
	shutdownCh := make(chan string, 1)

	go func() {
		select {
		case <-ctx.Done():
			shutdownCh <- "interrupt"
		case <-idleTimer.C:
			shutdownCh <- "idle timeout"
		case <-allResolvedCh:
			shutdownCh <- "all notes resolved"
		}
	}()

	// Start serving in background
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or server error
	select {
	case reason := <-shutdownCh:
		fmt.Fprintf(out, "\nShutting down (%s)...\n", reason)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("approve: server error: %w", err)
		}
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("approve: shutdown error: %w", err)
	}

	// Drain any server error
	if err := <-errCh; err != nil {
		return fmt.Errorf("approve: server error: %w", err)
	}

	return nil
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	// Ignore errors — browser open is best-effort
	_ = cmd.Start()
}
