package graphqlsse

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// errReader always fails to read, exercising the io.ReadAll error path in
// parseOperation.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errReader) Close() error             { return nil }

func TestParseOperation(t *testing.T) {
	tests := []struct {
		name    string
		req     *http.Request
		maxRead int64
		want    operationRequest
		wantErr bool
	}{
		{
			name: "get",
			req: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{RawQuery: "query=query%20Test%7Bfield%7D&operationName=Test&variables=%7B%22id%22%3A1%7D"},
			},
			want: operationRequest{Query: "query Test{field}", OperationName: "Test", Variables: map[string]any{"id": float64(1)}},
		},
		{
			name: "post",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
				Body:   io.NopCloser(strings.NewReader(`{"query":"{ field }"}`)),
			},
			want: operationRequest{Query: "{ field }"},
		},
		{
			name: "wrong content type",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"text/plain"}},
				Body:   io.NopCloser(strings.NewReader(`{"query":"{ field }"}`)),
			},
			wantErr: true,
		},
		{
			name: "missing content type",
			req: &http.Request{
				Method: http.MethodPost,
				Body:   io.NopCloser(strings.NewReader(`{"query":"{ field }"}`)),
			},
			wantErr: true,
		},
		{
			name: "nil body",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
			},
			wantErr: true,
		},
		{
			name: "read error",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   errReader{},
			},
			wantErr: true,
		},
		{
			name: "body too large",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   io.NopCloser(strings.NewReader(`{"query":"{ field }"}`)),
			},
			maxRead: 5,
			wantErr: true,
		},
		{
			name: "invalid json",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   io.NopCloser(strings.NewReader(`not json`)),
			},
			wantErr: true,
		},
		{
			name: "extra data after body",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   io.NopCloser(strings.NewReader(`{"query":"{ field }"}{"query":"{ other }"}`)),
			},
			wantErr: true,
		},
		{
			name: "missing query in post body",
			req: &http.Request{
				Method: http.MethodPost,
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   io.NopCloser(strings.NewReader(`{"query":""}`)),
			},
			wantErr: true,
		},
		{
			name: "missing query in get",
			req: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{RawQuery: ""},
			},
			wantErr: true,
		},
		{
			name: "query too large",
			req: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{RawQuery: "query=%7Bfield%7D"},
			},
			maxRead: 5,
			wantErr: true,
		},
		{
			name: "invalid variables",
			req: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{RawQuery: "query=%7Bfield%7D&variables=invalid"},
			},
			wantErr: true,
		},
		{
			name: "unsupported method",
			req: &http.Request{
				Method: http.MethodPut,
				URL:    &url.URL{RawQuery: "query=%7Bfield%7D"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxRead := tt.maxRead
			if maxRead == 0 {
				maxRead = 1024
			}
			got, err := parseOperation(tt.req, maxRead)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if err == nil && got.Query != tt.want.Query {
				t.Fatalf("query = %q, want %q", got.Query, tt.want.Query)
			}
		})
	}
}
