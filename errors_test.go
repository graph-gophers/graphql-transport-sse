package graphqlsse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWriteHTTPError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTTPError(rec, http.StatusBadRequest, errors.New("bad"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got, want := rec.Body.String(), "bad\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestWriteStreamError(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := newFrameWriter(rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeStreamError(w, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	want := "event: next\ndata: {\"errors\":[{\"message\":\"boom\"}]}\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestSplitHeader(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "single", in: "text/event-stream", want: []string{"text/event-stream"}},
		{name: "quality", in: "text/event-stream;q=0.9", want: []string{"text/event-stream"}},
		{name: "spaces", in: "text/event-stream, application/json", want: []string{"text/event-stream", "application/json"}},
		{name: "tabs", in: "\ttext/event-stream\t,\tapplication/json\t", want: []string{"text/event-stream", "application/json"}},
		{name: "empty", in: "", want: []string{""}},
		{name: "space before semicolon", in: "0 ;0", want: []string{"0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitHeader(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitHeader(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
