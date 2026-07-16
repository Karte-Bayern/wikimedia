// Package download provides bounded, atomic media downloads separate from API metadata fetching.
package download

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Wikimedia publishes SHA-1 values used here only for compatibility integrity checks.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

// Downloader performs validated, bounded, atomic downloads.
type Downloader struct {
	httpClient     mediawiki.HTTPDoer
	userAgent      string
	maximumBytes   int64
	allowedHosts   map[string]struct{}
	allowedSchemes map[string]struct{}
	overwrite      bool
	strictMIME     bool
	validator      URLValidator
	now            func() time.Time
}

// New creates a Downloader. A descriptive User-Agent is mandatory.
func New(options ...Option) (*Downloader, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			if err := option(&cfg); err != nil {
				return nil, err
			}
		}
	}
	if err := mediawiki.ValidateUserAgent(cfg.userAgent); err != nil {
		return nil, err
	}
	d := &Downloader{userAgent: cfg.userAgent, maximumBytes: cfg.maximumBytes, allowedHosts: cloneSet(cfg.allowedHosts), allowedSchemes: cloneSet(cfg.allowedSchemes), overwrite: cfg.overwrite, strictMIME: cfg.strictMIME, validator: cfg.validator, now: time.Now}
	if cfg.httpClient != nil {
		d.httpClient = cfg.httpClient
	} else {
		d.httpClient = &http.Client{Timeout: cfg.timeout, CheckRedirect: func(req *http.Request, _ []*http.Request) error { return d.validateURL(req.URL) }}
	}
	return d, nil
}

// Download writes source into directory and returns the committed local file.
func (d *Downloader) Download(ctx context.Context, source Source, directory string) (*File, error) {
	if d == nil || d.httpClient == nil {
		return nil, errors.New("download: nil downloader")
	}
	parsed, err := url.Parse(strings.TrimSpace(source.URL))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if err := d.validateURL(parsed); err != nil {
		return nil, err
	}
	if source.Size > d.maximumBytes && source.Size > 0 {
		return nil, fmt.Errorf("%w: advertised size %d exceeds limit %d", ErrTooLarge, source.Size, d.maximumBytes)
	}
	fileName := SanitizeFileName(source.FileName)
	if fileName == "" {
		fileName = FileNameFromURL(parsed)
	}
	if fileName == "" {
		fileName = "download"
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("download: empty destination directory")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("download: create directory: %w", err)
	}
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(absoluteDirectory, fileName)
	if err := d.checkDestination(destination); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", d.userAgent)
	response, err := d.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("download: request: %w", err)
	}
	defer response.Body.Close()
	if response.Request != nil && response.Request.URL != nil {
		if err := d.validateURL(response.Request.URL); err != nil {
			return nil, err
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download: HTTP %d", response.StatusCode)
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return nil, fmt.Errorf("%w: %s", ErrContentEncoding, encoding)
	}
	if response.ContentLength > d.maximumBytes {
		return nil, fmt.Errorf("%w: content length %d exceeds limit %d", ErrTooLarge, response.ContentLength, d.maximumBytes)
	}
	responseMIME, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	sourceMIME, _, _ := mime.ParseMediaType(source.MIMEType)
	if d.strictMIME && sourceMIME != "" && responseMIME != "" && !strings.EqualFold(sourceMIME, responseMIME) {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrMIMEMismatch, sourceMIME, responseMIME)
	}

	temporary, err := os.CreateTemp(absoluteDirectory, ".wikimedia-*.part")
	if err != nil {
		return nil, fmt.Errorf("download: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	sha1Hash, sha256Hash := sha1.New(), sha256.New() // #nosec G401 -- see package comment above.
	writer := io.MultiWriter(temporary, sha1Hash, sha256Hash)
	written, copyErr := io.Copy(writer, io.LimitReader(response.Body, d.maximumBytes+1))
	if copyErr != nil {
		return nil, fmt.Errorf("download: copy: %w", copyErr)
	}
	if written > d.maximumBytes {
		return nil, fmt.Errorf("%w: observed %d bytes exceeds limit %d", ErrTooLarge, written, d.maximumBytes)
	}
	if source.Size > 0 && written != source.Size {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrSizeMismatch, source.Size, written)
	}
	sha1Base36 := digestBase36(sha1Hash.Sum(nil))
	if source.SHA1Base36 != "" && !strings.EqualFold(strings.TrimSpace(source.SHA1Base36), sha1Base36) {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, source.SHA1Base36, sha1Base36)
	}
	if err := temporary.Sync(); err != nil {
		return nil, fmt.Errorf("download: sync: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := d.commit(temporaryPath, destination); err != nil {
		return nil, err
	}
	committed = true
	return &File{Path: destination, FileName: fileName, SourceURL: parsed.String(), CommonsTitle: source.CommonsTitle, MIMEType: firstNonEmpty(responseMIME, sourceMIME), Size: written, SHA256: hex.EncodeToString(sha256Hash.Sum(nil)), SHA1Base36: sha1Base36, DownloadedAt: d.now().UTC()}, nil
}

func (d *Downloader) validateURL(value *url.URL) error {
	if value == nil || value.Host == "" || value.User != nil {
		return ErrInvalidURL
	}
	if _, ok := d.allowedSchemes[strings.ToLower(value.Scheme)]; !ok {
		return fmt.Errorf("%w: scheme %q", ErrInvalidURL, value.Scheme)
	}
	host := strings.ToLower(value.Hostname())
	if _, ok := d.allowedHosts[host]; !ok {
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}
	if d.validator != nil {
		return d.validator(value)
	}
	return nil
}
func (d *Downloader) checkDestination(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafeDestination
	}
	if !d.overwrite {
		return ErrAlreadyExists
	}
	return nil
}
func (d *Downloader) commit(temporary, destination string) error {
	if d.overwrite {
		if runtime.GOOS == "windows" {
			_ = os.Remove(destination)
		}
		if err := os.Rename(temporary, destination); err != nil {
			return fmt.Errorf("download: commit: %w", err)
		}
		return nil
	}
	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("download: commit without overwrite: %w", err)
	}
	if err := os.Remove(temporary); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("download: remove temporary link: %w", err)
	}
	return nil
}

// SanitizeFileName removes path components, controls, and platform-hostile punctuation.
func SanitizeFileName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\\", "/"), "\x00", ""))
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, value)
	value = strings.Trim(value, " .")
	if value == "." || value == ".." {
		return ""
	}
	extension := filepath.Ext(value)
	base := strings.TrimSuffix(value, extension)
	if windowsReservedBase(base) {
		value = "_" + value
		extension = filepath.Ext(value)
		base = strings.TrimSuffix(value, extension)
	}
	if len(value) > 240 {
		maximumBaseBytes := 240 - len(extension)
		if maximumBaseBytes < 1 {
			return truncateUTF8(value, 240)
		}
		value = truncateUTF8(base, maximumBaseBytes) + extension
	}
	return value
}

func windowsReservedBase(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(value) == 4 && (strings.HasPrefix(value, "COM") || strings.HasPrefix(value, "LPT")) {
		return value[3] >= '1' && value[3] <= '9'
	}
	return false
}

func truncateUTF8(value string, maximumBytes int) string {
	if maximumBytes <= 0 {
		return ""
	}
	if len(value) <= maximumBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > maximumBytes {
			break
		}
		end = index
	}
	if end == 0 {
		return ""
	}
	return value[:end]
}

// FileNameFromURL derives and sanitizes the final path segment of a URL.
func FileNameFromURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	decoded, err := url.PathUnescape(filepath.Base(value.Path))
	if err != nil {
		decoded = filepath.Base(value.Path)
	}
	return SanitizeFileName(decoded)
}
func digestBase36(value []byte) string { return new(big.Int).SetBytes(value).Text(36) }
func stringSet(values []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}
func cloneSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for key := range source {
		result[key] = struct{}{}
	}
	return result
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
