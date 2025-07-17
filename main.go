package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// A collection of ScopeLogs from a Resource.
// Refer to 'type ResourceLogs struct' in
// opentelemetry-collector/pdata/internal/data/protogen/logs/v1/logs.pb.go
type ResourceLogs struct {
	// The resource for the logs in this message.
	// If this field is not set then resource info is unknown.
	Resource Resource `json:"resource"`
	// A list of ScopeLogs that originate from a resource.
	ScopeLogs []*ScopeLogs `json:"scope_logs,omitempty"`
}

// Resource information, partially from 'type Resource struct' in
// opentelemetry-collector/pdata/internal/data/protogen/resource/v1/resource.pb.go
type Resource struct {
	Attributes []KeyValue `json:"attributes"`
}

type KeyValue struct {
	Key   string         `json:"key"`
	Value AttributeValue `json:"value"`
}

// AttributeValue represents the OTLP attribute value format.
// Refer to 'type AnyValue struct' in
// opentelemetry-collector/pdata/internal/data/protogen/common/v1/common.pb.go
type AttributeValue struct {
	StringValue *string      `json:"stringValue,omitempty"`
	BoolValue   *bool        `json:"boolValue,omitempty"`
	IntValue    *int64       `json:"intValue,omitempty"`
	DoubleValue *float64     `json:"doubleValue,omitempty"`
	ArrayValue  *ArrayValue  `json:"arrayValue,omitempty"`
	KvlistValue *KvlistValue `json:"kvlistValue,omitempty"`
	BytesValue  []byte       `json:"bytesValue,omitempty"`
}

type ArrayValue struct {
	Values []AttributeValue `json:"values"`
}

type KvlistValue struct {
	Values []KeyValue `json:"values"`
}

// A collection of Logs produced by a Scope.
// Refer to 'type ScopeLogs struct' in
// opentelemetry-collector/pdata/internal/data/protogen/logs/v1/logs.pb.go
type ScopeLogs struct {
	// The instrumentation scope information for the logs in this message.
	// Semantically when InstrumentationScope isn't set, it is equivalent with
	// an empty instrumentation scope name (unknown).
	Scope Scope `json:"scope"`
	// A list of log records.
	LogRecords []*LogRecord `json:"log_records,omitempty"`
}

// Scope is a message representing the instrumentation scope information
// such as the fully qualified name and version.
// Refer to `type InstrumentationScope struct` in
// opentelemetry-collector/pdata/internal/data/protogen/common/v1/common.pb.go
type Scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// A log record according to OpenTelemetry Log Data Model:
// https://github.com/open-telemetry/oteps/blob/main/text/logs/0097-log-data-model.md
// Refer to `type LogRecord struct` in
// opentelemetry-collector/pdata/internal/data/protogen/logs/v1/logs.pb.go
type LogRecord struct {
	// time_unix_nano is the time when the event occurred.
	// Value is UNIX Epoch time in nanoseconds since 00:00:00 UTC on 1 January 1970.
	// Value of 0 indicates unknown or missing timestamp.
	TimeUnixNano uint64 `json:"timeUnixNano"`
	// The severity text (also known as log level). The original string representation as
	// it is known at the source. [Optional].
	SeverityText string `json:"severityText"`
	// Numerical value of the severity, normalized to values described in Log Data Model.
	// [Optional].
	SeverityNumber int `json:"severityNumber"`
	// A value containing the body of the log record. Can be for example a human-readable
	// string message (including multi-line) describing the event in a free form or it can
	// be a structured data composed of arrays and maps of other values. [Optional].
	Body AttributeValue `json:"body"`
	// Additional attributes that describe the specific event occurrence. [Optional].
	// Attribute keys MUST be unique (it is not allowed to have more than one
	// attribute with the same key).
	Attributes []KeyValue `json:"attributes,omitempty"`
}

type LogSender struct {
	client    *http.Client
	endpoint  string
	batchSize int
	logQueue  chan LogRecord
}

func NewLogSender(endpoint string, batchSize int) *LogSender {
	return &LogSender{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		endpoint:  endpoint,
		batchSize: batchSize,
		logQueue:  make(chan LogRecord, 1000),
	}
}

func (ls *LogSender) Start() {
	go ls.processQueue()
}

func (ls *LogSender) Log(severityText string, severityNumber int, message string, attrs map[string]interface{}) {
	body := AttributeValue{StringValue: &message}

	entry := LogRecord{
		TimeUnixNano:   uint64(time.Now().UnixNano()),
		SeverityText:   severityText,
		SeverityNumber: severityNumber,
		Body:           body,
	}

	if attrs != nil {
		entry.Attributes = make([]KeyValue, 0, len(attrs))
		for k, v := range attrs {
			entry.Attributes = append(entry.Attributes, KeyValue{
				Key:   k,
				Value: convertToAttributeValue(v),
			})
		}
	}

	select {
	case ls.logQueue <- entry:
	default:
		log.Println("Log queue full, dropping log entry")
	}
}

func convertToAttributeValue(v interface{}) AttributeValue {
	switch val := v.(type) {
	case string:
		return AttributeValue{StringValue: &val}
	case int:
		intVal := int64(val)
		return AttributeValue{IntValue: &intVal}
	case int64:
		return AttributeValue{IntValue: &val}
	case float64:
		return AttributeValue{DoubleValue: &val}
	case bool:
		return AttributeValue{BoolValue: &val}
	default:
		strVal := fmt.Sprintf("%v", val)
		return AttributeValue{StringValue: &strVal}
	}
}

func (ls *LogSender) processQueue() {
	var batch []*LogRecord
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case entry := <-ls.logQueue:
			batch = append(batch, &entry)
			if len(batch) >= ls.batchSize {
				ls.sendBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				ls.sendBatch(batch)
				batch = nil
			}
		}
	}
}

func (ls *LogSender) sendBatch(batch []*LogRecord) {
	resource := Resource{
		Attributes: []KeyValue{
			{
				Key: "service.name",
				Value: AttributeValue{
					StringValue: stringPtr("my-go-app"),
				},
			},
			{
				Key: "service.version",
				Value: AttributeValue{
					StringValue: stringPtr("1.0.0"),
				},
			},
			{
				Key: "telemetry.sdk.language",
				Value: AttributeValue{
					StringValue: stringPtr("go"),
				},
			},
		},
	}
	scope := Scope{
		Name:    "custom-logger",
		Version: "1.0",
	}
	scopeLogs := []*ScopeLogs{
		{
			Scope:      scope,
			LogRecords: batch,
		},
	}
	payload := map[string]interface{}{
		"resourceLogs": []ResourceLogs{
			{
				Resource:  resource,
				ScopeLogs: scopeLogs,
			},
		},
	}

	// payload := map[string]interface{}{
	// 	"resourceLogs": []map[string]interface{}{
	// 		{
	// 			"resource": resource,
	// 			"scopeLogs": []map[string]interface{}{
	// 				{
	// 					"scope": Scope{
	// 						Name:    "custom-logger",
	// 						Version: "1.0",
	// 					},
	// 					"logRecords": batch,
	// 				},
	// 			},
	// 		},
	// 	},
	// }

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling log batch: %v", err)
		return
	}

	req, err := http.NewRequest("POST", ls.endpoint+"/v1/logs", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ls.client.Do(req)
	if err != nil {
		log.Printf("Error sending logs: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var responseBody map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&responseBody); err == nil {
			log.Printf("Error response: %+v", responseBody)
		} else {
			log.Printf("Failed to decode error response: %v", err)
		}
		log.Printf("Received status code: %d", resp.StatusCode)
	} else {
		log.Println("Logs successfully sent")
	}
}

func stringPtr(s string) *string {
	return &s
}

func main() {
	logSender := NewLogSender("http://localhost:4318", 10)
	logSender.Start()

	// Example logs with different attribute types
	logSender.Log("INFO", 9, "Application started", nil)
	logSender.Log("WARN", 13, "Retry attempt", map[string]interface{}{
		"retry.count": 3,          // int
		"timeout":     30.5,       // float
		"enabled":     true,       // bool
		"user":        "john_doe", // string
	})

	// Keep the app running
	select {}
}
