package graphqlsse

import (
	"net/http"
)

type errorPayload struct {
	Errors []errorItem `json:"errors"`
}

type errorItem struct {
	Message string `json:"message"`
}

func writeHTTPError(w http.ResponseWriter, status int, err error) {
	http.Error(w, err.Error(), status)
}

func writeStreamError(w *frameWriter, err error) error {
	return w.writeEvent("next", errorPayload{Errors: []errorItem{{Message: err.Error()}}})
}

func acceptsEventStream(r *http.Request) bool {
	for _, value := range r.Header.Values("Accept") {
		for _, mediaType := range splitHeader(value) {
			if mediaType == "text/event-stream" {
				return true
			}
		}
	}
	return false
}

func splitHeader(value string) []string {
	result := make([]string, 0, 2)
	for _, part := range splitComma(value) {
		for len(part) > 0 && (part[0] == ' ' || part[0] == '\t') {
			part = part[1:]
		}
		if i := indexByte(part, ';'); i >= 0 {
			part = part[:i]
		}
		for len(part) > 0 && (part[len(part)-1] == ' ' || part[len(part)-1] == '\t') {
			part = part[:len(part)-1]
		}
		result = append(result, part)
	}
	return result
}

func splitComma(value string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == ',' {
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	return append(parts, value[start:])
}

func indexByte(value string, target byte) int {
	for i := range value {
		if value[i] == target {
			return i
		}
	}
	return -1
}
