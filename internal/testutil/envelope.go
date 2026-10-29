package testutil

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// EnvelopeItem is one decoded item of a Sentry envelope.
type EnvelopeItem struct {
	Header  map[string]interface{}
	Payload []byte
}

// Gunzip decompresses a gzip request body.
func Gunzip(body string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

// DecodeEnvelope parses envelope bytes: a JSON header line, then item
// header lines each followed by a payload of the header's "length" bytes
// (or, without "length", up to the next newline).
func DecodeEnvelope(data []byte) (map[string]interface{}, []EnvelopeItem, error) {
	r := bufio.NewReader(bytes.NewReader(data))
	line, err := r.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, nil, err
	}
	var header map[string]interface{}
	if err := json.Unmarshal(line, &header); err != nil {
		return nil, nil, fmt.Errorf("envelope header %q: %w", line, err)
	}

	var items []EnvelopeItem
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF && len(bytes.TrimSpace(line)) == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, nil, err
		}
		var ih map[string]interface{}
		if err := json.Unmarshal(line, &ih); err != nil {
			return nil, nil, fmt.Errorf("item header %q: %w", line, err)
		}
		var payload []byte
		if length, ok := ih["length"].(float64); ok {
			payload = make([]byte, int(length))
			if _, err := io.ReadFull(r, payload); err != nil {
				return nil, nil, fmt.Errorf("item payload: %w", err)
			}
			if b, err := r.ReadByte(); err == nil && b != '\n' {
				return nil, nil, fmt.Errorf("payload not followed by a newline")
			}
		} else {
			payload, _ = r.ReadBytes('\n')
			payload = bytes.TrimSuffix(payload, []byte("\n"))
		}
		items = append(items, EnvelopeItem{Header: ih, Payload: payload})
	}
	return header, items, nil
}
