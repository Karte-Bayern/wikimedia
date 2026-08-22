package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BatchStatus describes the outcome for one requested source.
type BatchStatus string

const (
	BatchStarted    BatchStatus = "started"
	BatchDownloaded BatchStatus = "downloaded"
	BatchSkipped    BatchStatus = "skipped"
	BatchDuplicate  BatchStatus = "duplicate"
	BatchFailed     BatchStatus = "failed"
)

// BatchItem is the outcome for one source in a DownloadBatch call.
type BatchItem struct {
	Index       int         `json:"index"`
	Source      Source      `json:"source"`
	Status      BatchStatus `json:"status"`
	File        *File       `json:"file,omitempty"`
	Error       string      `json:"error,omitempty"`
	DuplicateOf int         `json:"duplicate_of,omitempty"`
}

// BatchResult is a resumable download manifest. Item order matches input order.
type BatchResult struct {
	DownloadedAt time.Time   `json:"downloaded_at"`
	Items        []BatchItem `json:"items"`
}

// ProgressEvent reports state changes while DownloadBatch is running.
type ProgressEvent struct {
	Index  int         `json:"index"`
	Source Source      `json:"source"`
	Status BatchStatus `json:"status"`
	Error  string      `json:"error,omitempty"`
}

// BatchOption configures DownloadBatch.
type BatchOption func(*batchConfig) error

type batchConfig struct {
	concurrency int
	manifest    string
	resume      bool
	progress    func(ProgressEvent)
}

// WithConcurrency bounds parallel media downloads. The default is 4 and the
// maximum is 16, helping callers remain considerate of upstream hosts.
func WithConcurrency(value int) BatchOption {
	return func(c *batchConfig) error {
		if value <= 0 || value > 16 {
			return errors.New("download: concurrency must be between 1 and 16")
		}
		c.concurrency = value
		return nil
	}
}

// WithManifest writes the final batch result atomically to path.
func WithManifest(path string) BatchOption {
	return func(c *batchConfig) error {
		if strings.TrimSpace(path) == "" {
			return errors.New("download: manifest path must not be empty")
		}
		c.manifest = path
		return nil
	}
}

// WithResume reuses successful files recorded in an existing manifest when
// their local paths still exist. It requires WithManifest.
func WithResume(value bool) BatchOption {
	return func(c *batchConfig) error { c.resume = value; return nil }
}

// WithProgress receives serialized download state changes. The callback is
// never called concurrently.
func WithProgress(value func(ProgressEvent)) BatchOption {
	return func(c *batchConfig) error { c.progress = value; return nil }
}

// DownloadBatch downloads unique sources with bounded concurrency, detects
// duplicate source URLs/titles, and can persist a resumable manifest.
func (d *Downloader) DownloadBatch(ctx context.Context, sources []Source, directory string, options ...BatchOption) (*BatchResult, error) {
	if d == nil {
		return nil, errors.New("download: nil downloader")
	}
	cfg := batchConfig{concurrency: 4}
	for _, option := range options {
		if option != nil {
			if err := option(&cfg); err != nil {
				return nil, err
			}
		}
	}
	if cfg.resume && cfg.manifest == "" {
		return nil, errors.New("download: resume requires a manifest path")
	}
	result := &BatchResult{Items: make([]BatchItem, len(sources))}
	for index, source := range sources {
		result.Items[index] = BatchItem{Index: index, Source: source}
	}
	previous := map[string]File{}
	if cfg.resume {
		previous = resumableFiles(cfg.manifest)
	}
	jobs := make(chan int, len(sources))
	seen := make(map[string]int)
	for index, source := range sources {
		key := batchSourceKey(source)
		if prior, exists := seen[key]; exists {
			result.Items[index].Status = BatchDuplicate
			result.Items[index].DuplicateOf = prior
			emitProgress(cfg.progress, ProgressEvent{Index: index, Source: source, Status: BatchDuplicate})
			continue
		}
		seen[key] = index
		if file, ok := previous[key]; ok && existingRegularFile(file.Path) {
			copy := file
			result.Items[index].Status = BatchSkipped
			result.Items[index].File = &copy
			emitProgress(cfg.progress, ProgressEvent{Index: index, Source: source, Status: BatchSkipped})
			continue
		}
		jobs <- index
	}
	close(jobs)
	var progressMu sync.Mutex
	emit := func(event ProgressEvent) {
		if cfg.progress == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		cfg.progress(event)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < cfg.concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				source := result.Items[index].Source
				emit(ProgressEvent{Index: index, Source: source, Status: BatchStarted})
				file, err := d.Download(ctx, source, directory)
				if err != nil {
					result.Items[index].Status = BatchFailed
					result.Items[index].Error = err.Error()
					emit(ProgressEvent{Index: index, Source: source, Status: BatchFailed, Error: err.Error()})
					continue
				}
				result.Items[index].Status = BatchDownloaded
				result.Items[index].File = file
				emit(ProgressEvent{Index: index, Source: source, Status: BatchDownloaded})
			}
		}()
	}
	workers.Wait()
	result.DownloadedAt = time.Now().UTC()
	if cfg.manifest != "" {
		if err := writeManifest(cfg.manifest, result); err != nil {
			return result, err
		}
	}
	var failures []error
	for _, item := range result.Items {
		if item.Status == BatchFailed {
			failures = append(failures, fmt.Errorf("%s: %s", item.Source.URL, item.Error))
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, err)
	}
	return result, errors.Join(failures...)
}

func emitProgress(callback func(ProgressEvent), event ProgressEvent) {
	if callback != nil {
		callback(event)
	}
}

func batchSourceKey(source Source) string {
	if value := strings.ToLower(strings.TrimSpace(source.URL)); value != "" {
		return "url:" + value
	}
	if title := strings.ToLower(strings.TrimSpace(source.CommonsTitle)); title != "" {
		return "title:" + title
	}
	return "file:" + strings.ToLower(strings.TrimSpace(source.FileName))
}

func resumableFiles(path string) map[string]File {
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]File{}
	}
	var result BatchResult
	if json.Unmarshal(raw, &result) != nil {
		return map[string]File{}
	}
	files := make(map[string]File)
	for _, item := range result.Items {
		if (item.Status == BatchDownloaded || item.Status == BatchSkipped) && item.File != nil {
			files[batchSourceKey(item.Source)] = *item.File
		}
	}
	return files
}

func existingRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func writeManifest(path string, result *BatchResult) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".wikimedia-batch-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
