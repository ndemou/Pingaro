package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func ParseRecords(data []byte) ([]File, error) {
	var records []File
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var h File
		err := decoder.Decode(&h)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid history JSON: %w", err)
		}
		records = append(records, h)
	}
	return records, nil
}
