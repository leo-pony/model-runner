package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/logging"
)

type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

type RequestResponsePair struct {
	ID         string    `json:"id"`
	Model      string    `json:"model"`
	Method     string    `json:"method"`
	URL        string    `json:"url"`
	Request    string    `json:"request"`
	Response   string    `json:"response"`
	Timestamp  time.Time `json:"timestamp"`
	StatusCode int       `json:"status_code"`
}

type OpenAIRecorder struct {
	log     logging.Logger
	records map[string][]*RequestResponsePair
	m       sync.RWMutex
}

func NewOpenAIRecorder(log logging.Logger) *OpenAIRecorder {
	return &OpenAIRecorder{
		log:     log,
		records: make(map[string][]*RequestResponsePair),
	}
}

func (r *OpenAIRecorder) RecordRequest(model string, req *http.Request, body []byte) string {
	r.m.Lock()
	defer r.m.Unlock()

	recordID := fmt.Sprintf("%s_%d", model, time.Now().UnixNano())

	record := &RequestResponsePair{
		ID:        recordID,
		Model:     model,
		Method:    req.Method,
		URL:       req.URL.Path,
		Request:   string(body),
		Timestamp: time.Now(),
	}

	if r.records[model] == nil {
		r.records[model] = make([]*RequestResponsePair, 0, 10)
	}

	r.records[model] = append(r.records[model], record)

	if len(r.records[model]) > 10 {
		r.records[model] = r.records[model][1:]
	}

	return recordID
}

func (r *OpenAIRecorder) NewResponseRecorder(w http.ResponseWriter) http.ResponseWriter {
	rc := &responseRecorder{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
	return rc
}

func (r *OpenAIRecorder) RecordResponse(id, model string, rw http.ResponseWriter) {
	rr := rw.(*responseRecorder)

	responseBody := rr.body.String()
	statusCode := rr.statusCode

	var response string
	if strings.Contains(responseBody, "data: ") {
		response = r.convertStreamingResponse(responseBody)
	} else {
		response = responseBody
	}

	r.m.Lock()
	defer r.m.Unlock()

	if modelRecords, exists := r.records[model]; exists {
		for _, record := range modelRecords {
			if record.ID == id {
				record.Response = response
				record.StatusCode = statusCode
				return
			}
		}
		r.log.Errorf("Matching request (id=%s) not found for model %s - %d\n%s", id, model, statusCode, response)
	} else {
		r.log.Errorf("Model %s not found in records - %d\n%s", model, statusCode, response)
	}
}

func (r *OpenAIRecorder) convertStreamingResponse(streamingBody string) string {
	lines := strings.Split(streamingBody, "\n")
	var contentBuilder strings.Builder
	var lastChunk map[string]interface{}

	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			lastChunk = chunk

			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							contentBuilder.WriteString(content)
						}
					}
				}
			}
		}
	}

	if lastChunk == nil {
		return streamingBody
	}

	finalResponse := make(map[string]interface{})

	for key, value := range lastChunk {
		finalResponse[key] = value
	}

	if choices, ok := finalResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			choice["message"] = map[string]interface{}{
				"role":    "assistant",
				"content": contentBuilder.String(),
			}
			delete(choice, "delta")

			if _, ok := choice["finish_reason"]; !ok {
				choice["finish_reason"] = "stop"
			}
		}
	}

	finalResponse["object"] = "chat.completion"

	jsonResult, err := json.Marshal(finalResponse)
	if err != nil {
		return streamingBody
	}

	return string(jsonResult)
}

func (r *OpenAIRecorder) GetRecordsByModelHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		model := req.URL.Query().Get("model")

		if model == "" {
			http.Error(w, "A 'model' query parameter is required", http.StatusBadRequest)
		} else {
			// Retrieve records for the specified model.
			records := r.GetRecordsByModel(model)
			if records == nil {
				// No records found for the specified model.
				http.Error(w, fmt.Sprintf("No records found for model '%s'", model), http.StatusNotFound)
				return
			}

			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"model":   model,
				"records": records,
				"count":   len(records),
			}); err != nil {
				http.Error(w, fmt.Sprintf("Failed to encode records for model '%s': %v", model, err),
					http.StatusInternalServerError)
				return
			}
		}
	}
}

func (r *OpenAIRecorder) GetRecordsByModel(model string) []*RequestResponsePair {
	r.m.RLock()
	defer r.m.RUnlock()

	if modelRecords, exists := r.records[model]; exists {
		result := make([]*RequestResponsePair, len(modelRecords))
		copy(result, modelRecords)
		return result
	}

	return nil
}

func (r *OpenAIRecorder) RemoveModel(model string) {
	r.m.Lock()
	defer r.m.Unlock()

	if _, exists := r.records[model]; exists {
		delete(r.records, model)
		r.log.Infof("Removed records for model: %s", model)
	} else {
		r.log.Warnf("No records found for model: %s", model)
	}
}
