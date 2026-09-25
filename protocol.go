package graphqlsse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
)

type operationRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

func parseOperation(r *http.Request, maxRead int64) (operationRequest, error) {
	switch r.Method {
	case http.MethodGet:
		if maxRead > 0 && int64(len(r.URL.RawQuery)) > maxRead {
			return operationRequest{}, errRequestTooLarge
		}
		return parseGetOperation(r.URL.Query())
	case http.MethodPost:
		if r.Body == nil {
			return operationRequest{}, fmt.Errorf("request body is required")
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "" {
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil || mediaType != "application/json" {
				return operationRequest{}, fmt.Errorf("content type must be application/json")
			}
		} else {
			return operationRequest{}, fmt.Errorf("content type must be application/json")
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRead+1))
		if err != nil {
			return operationRequest{}, fmt.Errorf("read request: %w", err)
		}
		if int64(len(body)) > maxRead {
			return operationRequest{}, errRequestTooLarge
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		var operation operationRequest
		if err := decoder.Decode(&operation); err != nil {
			return operationRequest{}, fmt.Errorf("decode request: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return operationRequest{}, fmt.Errorf("request body must contain one JSON object")
		}
		if operation.Query == "" {
			return operationRequest{}, fmt.Errorf("query is required")
		}
		return operation, nil
	default:
		return operationRequest{}, fmt.Errorf("method must be GET or POST")
	}
}

func parseGetOperation(values url.Values) (operationRequest, error) {
	operation := operationRequest{
		Query:         values.Get("query"),
		OperationName: values.Get("operationName"),
	}
	if operation.Query == "" {
		return operationRequest{}, fmt.Errorf("query is required")
	}
	if variables := values.Get("variables"); variables != "" {
		if err := json.Unmarshal([]byte(variables), &operation.Variables); err != nil {
			return operationRequest{}, fmt.Errorf("decode variables: %w", err)
		}
	}
	return operation, nil
}
