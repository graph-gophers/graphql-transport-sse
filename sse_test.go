package graphqlsse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrameWriter(t *testing.T) {
	tests := []struct {
		name    string
		run     func(*frameWriter) error
		want    string
		wantErr bool
	}{
		{
			name: "event",
			run:  func(w *frameWriter) error { return w.writeEvent("next", map[string]string{"value": "ok"}) },
			want: "event: next\ndata: {\"value\":\"ok\"}\n\n",
		},
		{
			name: "complete",
			run:  func(w *frameWriter) error { return w.writeComplete() },
			want: "event: complete\ndata: \n\n",
		},
		{
			name: "heartbeat",
			run:  func(w *frameWriter) error { return w.heartbeat() },
			want: ": heartbeat\n\n",
		},
		{
			name:    "unmarshalable value",
			run:     func(w *frameWriter) error { return w.writeEvent("next", make(chan int)) },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			w, err := newFrameWriter(rec, 0)
			if err != nil {
				t.Fatal(err)
			}
			err = tt.run(w)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got := rec.Body.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if !strings.HasSuffix(rec.Body.String(), "\n\n") {
				t.Fatal("event was not terminated by a blank line")
			}
		})
	}
}

func TestNewFrameWriter(t *testing.T) {
	tests := []struct {
		name    string
		w       http.ResponseWriter
		wantErr bool
	}{
		{name: "supports flushing", w: httptest.NewRecorder()},
		{name: "does not support flushing", w: struct{ http.ResponseWriter }{httptest.NewRecorder()}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newFrameWriter(tt.w, 0)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// failWriter implements http.ResponseWriter and http.Flusher but always
// fails to write, exercising the frameWriter write-error path.
type failWriter struct {
	header http.Header
}

func (w *failWriter) Header() http.Header       { return w.header }
func (w *failWriter) WriteHeader(int)           {}
func (w *failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (w *failWriter) Flush()                    {}

func TestFrameWriterWriteError(t *testing.T) {
	tests := []struct {
		name string
		run  func(*frameWriter) error
	}{
		{name: "event", run: func(w *frameWriter) error { return w.writeEvent("next", map[string]string{"value": "ok"}) }},
		{name: "complete", run: func(w *frameWriter) error { return w.writeComplete() }},
		{name: "heartbeat", run: func(w *frameWriter) error { return w.heartbeat() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := newFrameWriter(&failWriter{header: http.Header{}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.run(w); err == nil {
				t.Fatal("expected write error")
			}
		})
	}
}
