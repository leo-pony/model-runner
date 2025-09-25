package metrics

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func TestTruncateMediaFields(t *testing.T) {
	// Create a mock logger and model manager
	logger := logrus.New()
	modelManager := &models.Manager{}
	recorder := NewOpenAIRecorder(logger, modelManager)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Image URL with base64 data",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "describe the image"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxASERATExIWFhMXGRwVGBgYFRgTExIQFRIXGBUVGxYZICggGh0lHRYVITEiJTArLi4uGB8zODMtNygtLisBCgoKDg0OFQ8QGy0dHR0tNy0tKy0rLS0tLSstLSstKy0rLS0tLSsrLS0rLS03LS0tKy03LS0tLSstLS0rLTctK"
								}
							}
						]
					}
				]
			}`,
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxASERATExIWFhMXGRwVGBgYFRgTExIQFRIXGBUVGxYZICggGh0lHRYVITEiJTAr",
		},
		{
			name: "Audio data",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "transcribe this audio"
							},
							{
								"type": "input_audio",
								"input_audio": {
									"data": "UklGRoqxAgBXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAATElTVBoAAABJTkZPSVNGVA4AAABMYXZmNTguNzYuMTAwAGRhdGFEsQIAsf+Y/2f/Uf83/y//Gf8g/xf/I/8r/0//Vv99/4z/r//H//r/DAAkACwAPAA7AE8ATQBIADMAKgAQAAUA6P/m/93//P8pAE4AXQBuAF8AbQByAKYAwQC/AKcArgCOAJMAoAClAIMAeQBKABgA/f/7/9z/wv+h/33/S/9S/0r/Uv9P/2L/S/9a/"
								}
							}
						]
					}
				]
			}`,
			expected: "UklGRoqxAgBXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAATElTVBoAAABJTkZPSVNGVA4AAABMYXZmNTguNzYuMTAwAGRhdGFE",
		},
		{
			name: "Regular request without media",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				]
			}`,
			expected: "Hello, how are you?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.truncateMediaFields([]byte(tt.input))

			// Parse the result to check if it's valid JSON
			var resultData map[string]interface{}
			if err := json.Unmarshal(result, &resultData); err != nil {
				t.Fatalf("Result is not valid JSON: %v", err)
			}

			// Convert back to string for comparison
			resultStr := string(result)

			// Check if the expected truncation occurred
			if !strings.Contains(resultStr, tt.expected) {
				t.Errorf("Expected result to contain %q, but got %q", tt.expected, resultStr)
			}

			// For media tests, ensure truncation occurred
			if tt.name != "Regular request without media" {
				if !strings.Contains(resultStr, "...[truncated") {
					t.Errorf("Expected truncation marker in result, but got %q", resultStr)
				}
			}
		})
	}
}

func TestTruncateBase64Data(t *testing.T) {
	logger := logrus.New()
	modelManager := &models.Manager{}
	recorder := NewOpenAIRecorder(logger, modelManager)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Short data URL",
			input:    "data:image/jpeg;base64,abc123",
			expected: "data:image/jpeg;base64,abc123",
		},
		{
			name:     "Long data URL",
			input:    "data:image/jpeg;base64," + generateLongString(200),
			expected: "data:image/jpeg;base64," + generateLongString(100) + "...[truncated 100 chars]",
		},
		{
			name:     "Long raw base64",
			input:    generateLongString(200),
			expected: generateLongString(100) + "...[truncated 100 chars]",
		},
		{
			name:     "Short raw base64",
			input:    "abc123",
			expected: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.truncateBase64Data(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function to generate a string of specified length
func generateLongString(length int) string {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = 'A' + byte(i%26)
	}
	return string(result)
}
