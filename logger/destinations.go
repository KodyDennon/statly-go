package logger

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ==================== Console Destination ====================

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBgRed  = "\033[41m"
	colorWhite  = "\033[37m"
)

var levelColors = map[Level]string{
	LevelTrace: colorGray,
	LevelDebug: colorCyan,
	LevelInfo:  colorGreen,
	LevelWarn:  colorYellow,
	LevelError: colorRed,
	LevelFatal: colorBgRed + colorWhite,
	LevelAudit: colorBlue,
}

var levelLabels = map[Level]string{
	LevelTrace: "TRACE",
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO ",
	LevelWarn:  "WARN ",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
	LevelAudit: "AUDIT",
}

// ConsoleDestination writes logs to the console.
type ConsoleDestination struct {
	config *ConsoleConfig
	output io.Writer
	mu     sync.Mutex
}

// NewConsoleDestination creates a new console destination.
func NewConsoleDestination(config *ConsoleConfig) *ConsoleDestination {
	output := config.Output
	if output == nil {
		output = os.Stderr
	}

	return &ConsoleDestination{
		config: config,
		output: output,
	}
}

// Name returns the destination name.
func (c *ConsoleDestination) Name() string {
	return "console"
}

// Write writes a log entry.
func (c *ConsoleDestination) Write(entry *Entry) {
	if !c.config.Enabled {
		return
	}

	var output string
	if c.config.Format == "json" {
		data, _ := json.Marshal(entry.ToMap())
		output = string(data)
	} else {
		output = c.formatPretty(entry)
	}

	c.mu.Lock()
	fmt.Fprintln(c.output, output)
	c.mu.Unlock()
}

func (c *ConsoleDestination) formatPretty(entry *Entry) string {
	var buf bytes.Buffer
	useColors := c.config.Colors && c.supportsColors()

	// Timestamp
	if c.config.Timestamps {
		ts := entry.Timestamp.Format("2006-01-02 15:04:05.000")
		if useColors {
			buf.WriteString(colorDim + ts + colorReset + " ")
		} else {
			buf.WriteString(ts + " ")
		}
	}

	// Level
	label := levelLabels[entry.Level]
	if useColors {
		color := levelColors[entry.Level]
		buf.WriteString(color + label + colorReset + " ")
	} else {
		buf.WriteString(label + " ")
	}

	// Logger name
	if entry.LoggerName != "" {
		if useColors {
			buf.WriteString(colorBlue + "[" + entry.LoggerName + "]" + colorReset + " ")
		} else {
			buf.WriteString("[" + entry.LoggerName + "] ")
		}
	}

	// Message
	buf.WriteString(entry.Message)

	// Context
	if len(entry.Context) > 0 {
		data, _ := json.MarshalIndent(entry.Context, "", "  ")
		if useColors {
			buf.WriteString("\n" + colorDim + string(data) + colorReset)
		} else {
			buf.WriteString("\n" + string(data))
		}
	}

	return buf.String()
}

func (c *ConsoleDestination) supportsColors() bool {
	if !c.config.Colors {
		return false
	}

	// Check if output is a file (TTY check)
	if f, ok := c.output.(*os.File); ok {
		stat, _ := f.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			return false
		}
	}

	// Check environment
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}

// Flush flushes the output.
func (c *ConsoleDestination) Flush() {
	// Nothing to flush for console
}

// Close closes the destination.
func (c *ConsoleDestination) Close() {
	// Nothing to close for console
}

// ==================== File Destination ====================

// FileDestination writes logs to a file with rotation.
type FileDestination struct {
	config      *FileConfig
	file        *os.File
	currentSize int64
	lastRotate  time.Time
	maxSize     int64
	mu          sync.Mutex
}

// NewFileDestination creates a new file destination.
func NewFileDestination(config *FileConfig) *FileDestination {
	// Parse max size
	maxSize := parseSize(config.MaxSize)
	if maxSize == 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}

	d := &FileDestination{
		config:     config,
		maxSize:    maxSize,
		lastRotate: time.Now(),
	}

	// Ensure directory exists
	dir := filepath.Dir(config.Path)
	os.MkdirAll(dir, 0755)

	// Open file
	d.openFile()

	return d
}

func parseSize(size string) int64 {
	if size == "" {
		return 0
	}

	var value float64
	var unit string
	fmt.Sscanf(size, "%f%s", &value, &unit)

	switch unit {
	case "KB", "kb":
		return int64(value * 1024)
	case "MB", "mb":
		return int64(value * 1024 * 1024)
	case "GB", "gb":
		return int64(value * 1024 * 1024 * 1024)
	default:
		return int64(value)
	}
}

func (f *FileDestination) openFile() {
	file, err := os.OpenFile(f.config.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Statly Logger] Failed to open log file: %v\n", err)
		return
	}

	stat, _ := file.Stat()
	f.file = file
	f.currentSize = stat.Size()
}

// Name returns the destination name.
func (f *FileDestination) Name() string {
	return "file"
}

// Write writes a log entry.
func (f *FileDestination) Write(entry *Entry) {
	if !f.config.Enabled || f.file == nil {
		return
	}

	var line string
	if f.config.Format == "json" {
		data, _ := json.Marshal(entry.ToMap())
		line = string(data) + "\n"
	} else {
		ts := entry.Timestamp.Format("2006-01-02 15:04:05.000")
		level := levelLabels[entry.Level]
		logger := ""
		if entry.LoggerName != "" {
			logger = "[" + entry.LoggerName + "] "
		}
		line = fmt.Sprintf("%s [%s] %s%s", ts, level, logger, entry.Message)
		if len(entry.Context) > 0 {
			data, _ := json.Marshal(entry.Context)
			line += " " + string(data)
		}
		line += "\n"
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	n, _ := f.file.WriteString(line)
	f.currentSize += int64(n)

	f.checkRotation()
}

func (f *FileDestination) checkRotation() {
	shouldRotate := false

	if f.config.RotationType == "size" {
		shouldRotate = f.currentSize >= f.maxSize
	} else if f.config.RotationType == "time" {
		interval := f.getRotationInterval()
		shouldRotate = time.Since(f.lastRotate) >= interval
	}

	if shouldRotate {
		f.rotate()
	}
}

func (f *FileDestination) getRotationInterval() time.Duration {
	switch f.config.RotationInterval {
	case "hourly":
		return time.Hour
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (f *FileDestination) rotate() {
	if f.file == nil {
		return
	}

	f.file.Close()

	// Generate rotated filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	rotatedPath := fmt.Sprintf("%s.%s", f.config.Path, timestamp)

	// Rename current file
	if err := os.Rename(f.config.Path, rotatedPath); err != nil {
		fmt.Fprintf(os.Stderr, "[Statly Logger] Failed to rotate log file: %v\n", err)
	}

	// Compress if configured
	if f.config.Compress {
		go f.compressFile(rotatedPath)
	}

	// Cleanup old files
	f.cleanupOldFiles()

	// Reopen file
	f.openFile()
	f.lastRotate = time.Now()
}

func (f *FileDestination) compressFile(path string) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(path + ".gz")
	if err != nil {
		return
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	io.Copy(gz, in)
	os.Remove(path)
}

func (f *FileDestination) cleanupOldFiles() {
	dir := filepath.Dir(f.config.Path)
	base := filepath.Base(f.config.Path)

	files, _ := os.ReadDir(dir)
	var rotatedFiles []os.DirEntry

	for _, file := range files {
		if file.Name() != base && len(file.Name()) > len(base) && file.Name()[:len(base)+1] == base+"." {
			rotatedFiles = append(rotatedFiles, file)
		}
	}

	// Remove files exceeding max_files
	if f.config.MaxFiles > 0 && len(rotatedFiles) > f.config.MaxFiles {
		for i := 0; i < len(rotatedFiles)-f.config.MaxFiles; i++ {
			os.Remove(filepath.Join(dir, rotatedFiles[i].Name()))
		}
	}
}

// Flush flushes the file.
func (f *FileDestination) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file != nil {
		f.file.Sync()
	}
}

// Close closes the destination.
func (f *FileDestination) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}
}

// ==================== Observe Destination ====================

var defaultSampling = map[Level]float64{
	LevelTrace: 0.01,  // 1%
	LevelDebug: 0.1,   // 10%
	LevelInfo:  0.5,   // 50%
	LevelWarn:  1.0,   // 100%
	LevelError: 1.0,   // 100%
	LevelFatal: 1.0,   // 100%
	LevelAudit: 1.0,   // 100%
}

// ObserveDestination sends logs to Statly Observe.
type ObserveDestination struct {
	dsn       string
	endpoint  string
	config    *ObserveConfig
	sampling  map[Level]float64
	queue     chan *Entry
	done      chan struct{}
	wg        sync.WaitGroup
}

// NewObserveDestination creates a new Observe destination.
func NewObserveDestination(dsn string, config *ObserveConfig) *ObserveDestination {
	if config == nil {
		config = &ObserveConfig{
			Enabled:       true,
			BatchSize:     50,
			FlushInterval: 5 * time.Second,
		}
	}

	sampling := make(map[Level]float64)
	for k, v := range defaultSampling {
		sampling[k] = v
	}
	for k, v := range config.Sampling {
		sampling[k] = v
	}

	d := &ObserveDestination{
		dsn:      dsn,
		endpoint: parseEndpoint(dsn),
		config:   config,
		sampling: sampling,
		queue:    make(chan *Entry, 1000),
		done:     make(chan struct{}),
	}

	d.wg.Add(1)
	go d.worker()

	return d
}

func parseEndpoint(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "https://statly.live/api/v1/logs/ingest"
	}
	return fmt.Sprintf("%s://%s/api/v1/logs/ingest", u.Scheme, u.Host)
}

func (o *ObserveDestination) worker() {
	defer o.wg.Done()

	batch := make([]*Entry, 0, o.config.BatchSize)
	ticker := time.NewTicker(o.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-o.queue:
			batch = append(batch, entry)
			if len(batch) >= o.config.BatchSize {
				o.sendBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				o.sendBatch(batch)
				batch = batch[:0]
			}

		case <-o.done:
			// Drain queue
			for {
				select {
				case entry := <-o.queue:
					batch = append(batch, entry)
				default:
					if len(batch) > 0 {
						o.sendBatch(batch)
					}
					return
				}
			}
		}
	}
}

func (o *ObserveDestination) sendBatch(batch []*Entry) {
	if len(batch) == 0 {
		return
	}

	logs := make([]map[string]interface{}, len(batch))
	for i, entry := range batch {
		logs[i] = entry.ToMap()
	}

	payload := map[string]interface{}{"logs": logs}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", o.endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Statly-DSN", o.dsn)
	req.Header.Set("User-Agent", "statly-observe-go/0.2.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Statly Logger] Failed to send logs: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		fmt.Fprintf(os.Stderr, "[Statly Logger] API error: %d\n", resp.StatusCode)
	}
}

// Name returns the destination name.
func (o *ObserveDestination) Name() string {
	return "observe"
}

// Write writes a log entry.
func (o *ObserveDestination) Write(entry *Entry) {
	if !o.config.Enabled {
		return
	}

	// Apply sampling (audit logs never sampled)
	if entry.Level != LevelAudit {
		rate := o.sampling[entry.Level]
		if rand.Float64() > rate {
			return
		}
	}

	select {
	case o.queue <- entry:
	default:
		// Queue full, drop entry
	}
}

// Flush flushes queued entries.
func (o *ObserveDestination) Flush() {
	// Wait for queue to drain
	for len(o.queue) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
}

// Close closes the destination.
func (o *ObserveDestination) Close() {
	close(o.done)
	o.wg.Wait()
}
