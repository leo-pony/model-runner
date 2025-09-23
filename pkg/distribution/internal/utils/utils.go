package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode"
)

// FormatBytes converts bytes to a human-readable string with appropriate unit
func FormatBytes(bytes int) string {
	size := float64(bytes)
	var unit string
	switch {
	case size >= 1<<30:
		size /= 1 << 30
		unit = "GB"
	case size >= 1<<20:
		size /= 1 << 20
		unit = "MB"
	case size >= 1<<10:
		size /= 1 << 10
		unit = "KB"
	default:
		unit = "bytes"
	}
	return fmt.Sprintf("%.2f %s", size, unit)
}

// ShowProgress displays a progress bar for data transfer operations
func ShowProgress(operation string, progressChan chan int64, totalSize int64) {
	for bytesComplete := range progressChan {
		if totalSize > 0 {
			mbComplete := float64(bytesComplete) / (1024 * 1024)
			mbTotal := float64(totalSize) / (1024 * 1024)
			fmt.Printf("\r%s: %.2f MB / %.2f MB", operation, mbComplete, mbTotal)
		} else {
			mb := float64(bytesComplete) / (1024 * 1024)
			fmt.Printf("\r%s: %.2f MB", operation, mb)
		}
	}
	fmt.Println() // Move to new line after progress
}

// ReadContent reads content from a local file or URL
func ReadContent(source string) ([]byte, error) {
	// Check if the source is a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// Parse the URL
		_, err := url.Parse(source)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %v", err)
		}

		// Make HTTP request
		resp, err := http.Get(source)
		if err != nil {
			return nil, fmt.Errorf("failed to download file: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download file: HTTP status %d", resp.StatusCode)
		}

		// Create progress reader
		contentLength := resp.ContentLength
		progressChan := make(chan int64, 100)

		// Start progress reporting goroutine
		go ShowProgress("Downloading", progressChan, contentLength)

		// Create a wrapper reader to track progress
		progressReader := &ProgressReader{
			Reader:       resp.Body,
			ProgressChan: progressChan,
		}

		// Read the content
		content, err := io.ReadAll(progressReader)
		close(progressChan)
		return content, err
	}

	// If not a URL, treat as local file path
	return os.ReadFile(source)
}

// ProgressReader wraps an io.Reader to track reading progress
type ProgressReader struct {
	Reader       io.Reader
	ProgressChan chan int64
	Total        int64
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.Total += int64(n)
		pr.ProgressChan <- pr.Total
	}
	return n, err
}

// SanitizeForLog sanitizes a string for safe logging by removing or escaping
// control characters that could cause log injection attacks.
// TODO: Consider migrating to structured logging which
// handles sanitization automatically through field encoding.
func SanitizeForLog(s string) string {
	if s == "" {
		return ""
	}

	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		switch {
		// Replace newlines and carriage returns with escaped versions.
		case r == '\n':
			result.WriteString("\\n")
		case r == '\r':
			result.WriteString("\\r")
		case r == '\t':
			result.WriteString("\\t")
		// Remove other control characters (0x00-0x1F, 0x7F).
		case unicode.IsControl(r):
			// Skip control characters or replace with placeholder.
			result.WriteString("?")
		// Escape backslashes to prevent escape sequence injection.
		case r == '\\':
			result.WriteString("\\\\")
		// Keep printable characters.
		case unicode.IsPrint(r):
			result.WriteRune(r)
		default:
			// Replace non-printable characters with placeholder.
			result.WriteString("?")
		}
	}

	const maxLength = 100
	if result.Len() > maxLength {
		return result.String()[:maxLength] + "...[truncated]"
	}

	return result.String()
}
