package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const registryFileVersion = 1

type FileStore struct {
	path string
}

type registryFile struct {
	Version        int                    `json:"version"`
	KnowledgeBases []RuntimeKnowledgeBase `json:"knowledge_bases"`
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (f *FileStore) List() ([]RuntimeKnowledgeBase, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("runtime registry %q is empty", f.path)
	}

	var file registryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode runtime registry %q: %w", f.path, err)
	}
	out := make([]RuntimeKnowledgeBase, len(file.KnowledgeBases))
	copy(out, file.KnowledgeBases)
	return out, nil
}

func (f *FileStore) Save(items []RuntimeKnowledgeBase) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	file := registryFile{Version: registryFileVersion, KnowledgeBases: items}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".registry-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, f.path)
}
