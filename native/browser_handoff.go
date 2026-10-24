package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nocturney/slicebridge/native/makerworld"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const makerWorldMaxBytes int64 = 250 << 20

// A malicious or accidental burst of distinct external app URLs must not
// launch unbounded 250 MB downloads. This limits active imports per desktop
// process, not the amount of work a user can perform over time.
const maxConcurrentMakerWorldImports = 2

// BrowserBridge opens only the private slicebridge:// protocol, never
// overrides bambustudio:// or any system-wide .3mf default file handler.
// It never buffers unbounded live 3MF download requests after startup.
type BrowserBridge struct {
	mu            sync.Mutex
	waiting       []string
	active        map[string]bool
	ctx           context.Context
	frontendReady bool
	// Test injection never exists in the production singleton. Tests use it to
	// exercise startup queue ordering without downloading real models.
	launchForTest func(context.Context, string)
}

var externalBridge = &BrowserBridge{active: map[string]bool{}}

// reserveActiveImport is called only after Developer Mode and native
// slicebridge:// validation. It is a permit counter, with no I/O or events,
// and allows the import to start only when a desktop context exists.
// Duplicates are silently ignored; overload has an explicit busy result.
func (b *BrowserBridge) reserveActiveImport(raw string) (ctx context.Context, ok, busy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ctx == nil || b.active[raw] {
		return nil, false, false
	}
	if len(b.active) >= maxConcurrentMakerWorldImports {
		return b.ctx, false, true
	}
	if b.active == nil {
		b.active = make(map[string]bool, maxConcurrentMakerWorldImports)
	}
	b.active[raw] = true
	return b.ctx, true, false
}

// The Wails runtime exists before the frontend subscribes to its event
// stream. Do not drop or launch any startup import until the JS frontend
// explicitly acknowledges that its imported/failed listeners are registered.
func (b *BrowserBridge) setContext(ctx context.Context) {
	b.mu.Lock()
	b.ctx = ctx
	b.mu.Unlock()
	b.drainWaiting()
}

func (b *BrowserBridge) markFrontendReady() {
	b.mu.Lock()
	b.frontendReady = true
	b.mu.Unlock()
	b.drainWaiting()
}

// reserveWaitingImport pops the oldest distinct startup URL only when a
// permit is available. The pop and active-set insert are under ONE mutex,
// so concurrent Wails events cannot exceed the two-download limit.
func (b *BrowserBridge) reserveWaitingImport() (context.Context, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ctx == nil || !b.frontendReady || len(b.active) >= maxConcurrentMakerWorldImports {
		return nil, "", false
	}
	for len(b.waiting) > 0 {
		raw := b.waiting[0]
		b.waiting[0] = ""
		b.waiting = b.waiting[1:]
		if b.active[raw] {
			continue
		}
		if b.active == nil {
			b.active = make(map[string]bool, maxConcurrentMakerWorldImports)
		}
		b.active[raw] = true
		return b.ctx, raw, true
	}
	return nil, "", false
}

// Buffered external-app URLs must survive startup and be resumed as active
// downloads finish. These URLs were already validated by acceptArg, but
// Developer Mode is checked again to respect an in-flight disable.
func (b *BrowserBridge) drainWaiting() {
	for {
		ctx, raw, ok := b.reserveWaitingImport()
		if !ok {
			return
		}
		if checkDeveloperMode() != nil {
			b.mu.Lock()
			delete(b.active, raw)
			b.mu.Unlock()
			continue
		}
		b.beginActiveImport(ctx, raw)
	}
}

func (b *BrowserBridge) releaseActiveImport(raw string) {
	b.mu.Lock()
	delete(b.active, raw)
	b.mu.Unlock()
	b.drainWaiting()
}
func (b *BrowserBridge) acceptArg(arg string) {
	// A browser extension installed earlier can still send MakerWorld URIs.
	// Ignore them unless Developer Mode is explicitly enabled.
	if checkDeveloperMode() != nil {
		return
	}
	if !strings.HasPrefix(strings.ToLower(arg), "slicebridge://") {
		return
	}
	link, err := makerworld.ParseLink(arg)
	if err != nil || link.Scheme != "slicebridge" {
		return
	}
	b.mu.Lock()
	if b.ctx == nil || !b.frontendReady {
		if len(b.waiting) < 12 {
			duplicate := b.active[arg]
			for _, queued := range b.waiting {
				if queued == arg {
					duplicate = true
					break
				}
			}
			if !duplicate {
				b.waiting = append(b.waiting, arg)
			}
		}
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	b.handle(arg)
}
func (b *BrowserBridge) handle(raw string) {
	// Re-check here too: URIs queued before startup must not download after a disable.
	if checkDeveloperMode() != nil {
		return
	}
	ctx, reserved, busy := b.reserveActiveImport(raw)
	if !reserved {
		if busy {
			// A non-sensitive, bounded failure message is preferable to silently
			// queueing hundreds of high-volume imports. Never expose the URL.
			wailsruntime.EventsEmit(ctx, "bridge:makerworld-import-failed",
				"MakerWorld already has two active imports. Retry after they finish.")
		}
		return
	}
	b.beginActiveImport(ctx, raw)
}

// beginActiveImport must only be invoked after a successful permit
// reservation. A test hook keeps queue and permit tests fully offline.
func (b *BrowserBridge) beginActiveImport(ctx context.Context, raw string) {
	if b.launchForTest != nil {
		b.launchForTest(ctx, raw)
		return
	}
	go func() {
		defer b.releaseActiveImport(raw)
		link, err := makerworld.ParseLink(raw)
		if err != nil {
			return
		}
		folder, err := downloadFolder()
		if err == nil {
			client := newMakerWorldHTTPClient()
			var file *FileInfo
			file, err = download3MF(ctx, client, link, folder)
			if err == nil {
				// Send only the *local* file info, and never emit signed URL tokens.
				wailsruntime.EventsEmit(ctx, "bridge:makerworld-imported", map[string]any{"file": file, "source": link.Source})
				return
			}
		}
		// Never log or display raw remote URL (short-lived signed link).
		wailsruntime.EventsEmit(ctx, "bridge:makerworld-import-failed", classifyImportError(err))
	}()
}
func classifyImportError(err error) string {
	if err == nil {
		return "unknown"
	}
	var status *httpStatusError
	if errors.As(err, &status) {
		if status.code == 401 || status.code == 403 || status.code == 418 {
			return "MakerWorld requires browser authentication or verification. Use the site's normal 3MF download."
		}
		return fmt.Sprintf("MakerWorld download was rejected (HTTP %d). Use the site's download button.", status.code)
	}
	return "Could not import this MakerWorld project. Try downloading its 3MF normally."
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

func downloadFolder() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads", "SliceBridge")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
func trustedHTTPS(raw string) bool {
	link, err := makerworld.ParseLink("slicebridge://import?url=" + urlQueryEscape(raw))
	return err == nil && link.Scheme == "slicebridge"
}

// Only used to construct a URL for the already shared strict ParseLink
// validator. We avoid net/url import to keep this helper small.
func urlQueryEscape(s string) string {
	const table = "0123456789ABCDEF"
	var b strings.Builder
	for _, v := range []byte(s) {
		if (v >= 'A' && v <= 'Z') || (v >= 'a' && v <= 'z') ||
			(v >= '0' && v <= '9') || v == '-' || v == '_' || v == '.' || v == '~' {
			b.WriteByte(v)
		} else {
			b.WriteByte('%')
			b.WriteByte(table[v>>4])
			b.WriteByte(table[v&15])
		}
	}
	return b.String()
}

func newMakerWorldHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}
	transport := &http.Transport{
		MaxIdleConns: 4, IdleConnTimeout: 25 * time.Second, TLSHandshakeTimeout: 8 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Redirect and hostname checks also apply to resolved addresses; do not
			// connect to loopback/private IPs even if DNS is manipulated.
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if ip.IP.IsLoopback() || ip.IP.IsPrivate() || ip.IP.IsLinkLocalUnicast() ||
					ip.IP.IsMulticast() || ip.IP.IsUnspecified() {
					continue
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			}
			return nil, errors.New("no public source address")
		},
	}
	return &http.Client{Timeout: 90 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("too many redirects")
		}
		if !trustedHTTPS(req.URL.String()) {
			return errors.New("untrusted download redirect")
		}
		return nil
	}}
}

func verify3MFArchive(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(file, st.Size())
	if err != nil {
		return errors.New("download is not a 3MF archive")
	}
	if len(zr.File) > 10000 {
		return errors.New("too many entries")
	}
	model, content := false, false
	for _, f := range zr.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if name == "[content_types].xml" {
			content = true
		}
		if strings.HasPrefix(name, "3d/") && strings.HasSuffix(name, ".model") {
			model = true
		}
	}
	if !model || !content {
		return errors.New("no 3MF model manifest")
	}
	return nil
}

// Download is bounded, redirected only to MakerWorld/Bambu CDN and published
// atomically with no overwrite; credentials are never copied out of browser.
func download3MF(ctx context.Context, client *http.Client, link makerworld.Link, folder string) (*FileInfo, error) {
	if link.Scheme != "slicebridge" || !trustedHTTPS(link.RemoteURL) {
		return nil, errors.New("untrusted model URL")
	}
	if err := os.MkdirAll(folder, 0700); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", link.RemoteURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream, application/zip, application/vnd.ms-package.3dmanufacturing-3dmodel+xml, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{resp.StatusCode}
	}
	if resp.ContentLength > makerWorldMaxBytes {
		return nil, errors.New("3MF download too large")
	}
	// Any fake/custom http.Client still must enforce trusted redirects.
	if !trustedHTTPS(resp.Request.URL.String()) {
		return nil, errors.New("untrusted response origin")
	}
	temp, err := os.CreateTemp(folder, ".slicebridge-download-*.partial")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	n, copyErr := io.Copy(temp, io.LimitReader(resp.Body, makerWorldMaxBytes+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if n == 0 || n > makerWorldMaxBytes {
		return nil, errors.New("3MF download size invalid")
	}
	if err := verify3MFArchive(tempPath); err != nil {
		return nil, err
	}
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("MakerWorld-%s.3mf", time.Now().UTC().Format("20060102-150405"))
		if i > 0 {
			name = fmt.Sprintf("MakerWorld-%s-%d.3mf", time.Now().UTC().Format("20060102-150405"), i)
		}
		dest := filepath.Join(folder, name)
		in, err := os.Open(tempPath)
		if err != nil {
			return nil, err
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			in.Close()
			continue
		}
		if err != nil {
			in.Close()
			return nil, err
		}
		_, err = io.Copy(out, in)
		inErr := in.Close()
		outErr := out.Close()
		if err == nil {
			err = inErr
		}
		if err == nil {
			err = outErr
		}
		if err != nil {
			os.Remove(dest)
			return nil, err
		}
		return fileInfo(dest)
	}
	return nil, errors.New("output filename collisions")
}

// Explicit per-user registration is available only on Windows, never
// triggered automatically when the app is installed or started.
// Legacy entrypoint for older frontends; use local setup, never GitHub source.
func (a *App) OpenBrowserSetup() error {
	if err := a.requireDeveloperMode(); err != nil {
		return err
	}
	_, err := a.PrepareBrowserExtension("edge")
	return err
}

// Keep the registration command identical to what the Windows status check
// expects. Go's %q escapes path separators, which reg.exe stores literally.
func windowsProtocolCommand(executable string) string {
	return "\"" + executable + "\" \"%1\""
}
func windowsProtocolRegistered(regOutput []byte, executable string) bool {
	return strings.Contains(strings.ToLower(string(regOutput)), strings.ToLower(windowsProtocolCommand(executable)))
}

func (a *App) BrowserLinkStatus() (map[string]any, error) {
	if runtime.GOOS != "windows" {
		return map[string]any{"platform": runtime.GOOS, "registered": runtime.GOOS == "darwin"}, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path := "HKCU\\Software\\Classes\\slicebridge\\shell\\open\\command"
	out, err := exec.Command("reg.exe", "query", path, "/ve").CombinedOutput()
	registered := err == nil && windowsProtocolRegistered(out, executable)
	return map[string]any{"platform": "windows", "registered": registered}, nil
}
func (a *App) EnableBrowserLink() (map[string]any, error) {
	if err := a.requireDeveloperMode(); err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		return a.BrowserLinkStatus()
	}
	if a.ctx == nil {
		return nil, errors.New("desktop not ready")
	}
	// OS state changes only after an explicit consent from this button.
	answer, err := wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type: wailsruntime.QuestionDialog, Title: "Enable SliceBridge browser links?",
		Message: "Register slicebridge:// for this Windows account? This does not change Bambu Studio or .3mf defaults.",
	})
	if err != nil {
		return nil, err
	}
	if answer != "Yes" {
		return a.BrowserLinkStatus()
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := windowsProtocolCommand(exe)
	root := "HKCU\\Software\\Classes\\slicebridge"
	entries := [][]string{
		{"add", root, "/ve", "/d", "URL:SliceBridge model link", "/f"},
		{"add", root, "/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f"},
		{"add", root + "\\shell\\open\\command", "/ve", "/d", command, "/f"},
	}
	for _, args := range entries {
		out, err := exec.Command("reg.exe", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Windows failed to register SliceBridge protocol: %w (%s)", err, string(out))
		}
	}
	return a.BrowserLinkStatus()
}
