package history

import (
	"encoding/json"
	"os"
)

func WriteFile(path string, h File) error {
	return WriteRecords(path, []File{h})
}

func WriteRecords(path string, records []File) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, h := range records {
		if err = encoder.Encode(h); err != nil {
			break
		}
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func AppendLine(path string, h File) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(h)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func ReadFile(path string) ([]File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseRecords(data)
}
