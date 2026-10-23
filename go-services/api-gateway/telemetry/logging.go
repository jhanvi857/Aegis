package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLogLevel(l string) LogLevel {
	switch l {
	case "debug":
		return LevelDebug
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

type StructuredLogger struct {
	mu     sync.Mutex
	out    io.Writer
	level  LogLevel
	isJSON bool
}

type LogEntry struct {
	Timestamp  string                 `json:"timestamp"`
	Level      string                 `json:"level"`
	Component  string                 `json:"component"`
	Message    string                 `json:"message"`
	TraceID    string                 `json:"traceId,omitempty"`
	CallerID   string                 `json:"callerId,omitempty"`
	DurationMS float64                `json:"durationMs,omitempty"`
	HTTPStatus int                    `json:"httpStatus,omitempty"`
	Fields     map[string]interface{} `json:"fields,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

var Logger = NewStructuredLogger(os.Stdout, LevelInfo, true)

func NewStructuredLogger(out io.Writer, level LogLevel, isJSON bool) *StructuredLogger {
	return &StructuredLogger{
		out:    out,
		level:  level,
		isJSON: isJSON,
	}
}

func (l *StructuredLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *StructuredLogger) log(level LogLevel, levelStr, component, msg, traceID, callerID string, durationMS float64, status int, err error, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	entry := LogEntry{
		Timestamp:  now,
		Level:      levelStr,
		Component:  component,
		Message:    msg,
		TraceID:    traceID,
		CallerID:   callerID,
		DurationMS: durationMS,
		HTTPStatus: status,
		Fields:     fields,
		Error:      errStr,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isJSON {
		data, _ := json.Marshal(entry)
		_, _ = fmt.Fprintln(l.out, string(data))
	} else {
		_, _ = fmt.Fprintf(l.out, "%s [%s] (%s) %s\n", now, levelStr, component, msg)
	}
}

func (l *StructuredLogger) Debug(component, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, "DEBUG", component, msg, "", "", 0, 0, nil, f)
}

func (l *StructuredLogger) Info(component, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, "INFO", component, msg, "", "", 0, 0, nil, f)
}

func (l *StructuredLogger) Warn(component, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, "WARN", component, msg, "", "", 0, 0, nil, f)
}

func (l *StructuredLogger) Error(component, msg string, err error, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, "ERROR", component, msg, "", "", 0, 0, err, f)
}

// responseWriterInterceptor captures HTTP status code for logging
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterInterceptor) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// RequestLoggerMiddleware structured HTTP access logging
func RequestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		interceptor := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = r.Header.Get("X-Request-ID")
		}

		next.ServeHTTP(interceptor, r)

		duration := float64(time.Since(start).Microseconds()) / 1000.0

		// Don't clutter with frequent internal polling logs if nominal
		if (r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics") && interceptor.statusCode == http.StatusOK {
			return
		}

		fields := map[string]interface{}{
			"method":    r.Method,
			"path":      r.URL.Path,
			"remoteIP":  r.RemoteAddr,
			"userAgent": r.UserAgent(),
		}

		lvl := LevelInfo
		lvlStr := "INFO"
		if interceptor.statusCode >= 500 {
			lvl = LevelError
			lvlStr = "ERROR"
		} else if interceptor.statusCode >= 400 {
			lvl = LevelWarn
			lvlStr = "WARN"
		}

		Logger.log(lvl, lvlStr, "http", fmt.Sprintf("%s %s -> %d (%.2fms)", r.Method, r.URL.Path, interceptor.statusCode, duration), traceID, "", duration, interceptor.statusCode, nil, fields)
	})
}
