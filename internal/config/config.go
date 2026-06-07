package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultSearchMode          = "auto"
	DefaultServerAddress       = ":34125"
	DefaultStoragePath         = "~/.knowledger/db"
	DefaultRuntimeRegistryPath = "~/.knowledger/registry.json"
	DefaultKBID                = "default"
	DefaultKBName              = "Default"
	DefaultChromaProvider      = "chroma"
	DefaultChromaMode          = "persistent"
	DefaultChromaHTTPMode      = "http"
	DefaultChromaStoragePath   = "~/.knowledger/chroma"
	DefaultChromaBaseURL       = "http://127.0.0.1:8000"
	DefaultChromaSyncMode      = "async"
)

type Config struct {
	DefaultSearchMode   string                `yaml:"default_search_mode"`
	RuntimeRegistryPath string                `yaml:"runtime_registry_path"`
	Server              ServerConfig          `yaml:"server"`
	KnowledgeBases      []KnowledgeBaseConfig `yaml:"knowledge_bases"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type KnowledgeBaseConfig struct {
	ID                string         `yaml:"id"`
	Name              string         `yaml:"name"`
	StoreType         string         `yaml:"store_type"`
	StoreConfig       map[string]any `yaml:"store_config"`
	Enabled           bool           `yaml:"enabled"`
	DefaultSearchMode string         `yaml:"default_search_mode"`
	Indexing          map[string]any `yaml:"indexing"`
	Tags              []string       `yaml:"tags"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		DefaultSearchMode: DefaultSearchMode,
		Server:            ServerConfig{Address: DefaultServerAddress},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if err := ApplyDefaults(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Default() (Config, error) {
	cfg := Config{}
	if err := ApplyDefaults(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ApplyDefaults(cfg *Config) error {
	if cfg.DefaultSearchMode == "" {
		cfg.DefaultSearchMode = DefaultSearchMode
	}
	if cfg.RuntimeRegistryPath == "" {
		cfg.RuntimeRegistryPath = DefaultRuntimeRegistryPath
	}
	runtimeRegistryPath, err := expandHomePath(cfg.RuntimeRegistryPath)
	if err != nil {
		return err
	}
	cfg.RuntimeRegistryPath = runtimeRegistryPath
	if cfg.Server.Address == "" {
		cfg.Server.Address = DefaultServerAddress
	}
	if len(cfg.KnowledgeBases) == 0 {
		kb, err := defaultKnowledgeBase()
		if err != nil {
			return err
		}
		cfg.KnowledgeBases = []KnowledgeBaseConfig{kb}
		return nil
	}

	for i := range cfg.KnowledgeBases {
		if err := ApplyKnowledgeBaseDefaults(&cfg.KnowledgeBases[i]); err != nil {
			return err
		}
	}

	return nil
}

func ApplyKnowledgeBaseDefaults(kb *KnowledgeBaseConfig) error {
	if kb.StoreType != "sqlite" {
		return nil
	}
	if kb.StoreConfig == nil {
		kb.StoreConfig = map[string]any{}
	}
	path, ok := kb.StoreConfig["path"]
	if !ok || path == "" {
		defaultPath, err := expandHomePath(DefaultStoragePath)
		if err != nil {
			return err
		}
		kb.StoreConfig["path"] = defaultPath
	} else {
		pathString, ok := path.(string)
		if !ok {
			return fmt.Errorf("knowledge base %q sqlite store_config.path must be a string", kb.ID)
		}
		expanded, err := expandHomePath(pathString)
		if err != nil {
			return err
		}
		kb.StoreConfig["path"] = expanded
	}
	if err := applySQLiteIndexingDefaults(kb); err != nil {
		return err
	}
	return nil
}

func defaultKnowledgeBase() (KnowledgeBaseConfig, error) {
	path, err := expandHomePath(DefaultStoragePath)
	if err != nil {
		return KnowledgeBaseConfig{}, err
	}
	kb := KnowledgeBaseConfig{
		ID:          DefaultKBID,
		Name:        DefaultKBName,
		StoreType:   "sqlite",
		Enabled:     true,
		StoreConfig: map[string]any{"path": path},
	}
	if err := applySQLiteIndexingDefaults(&kb); err != nil {
		return KnowledgeBaseConfig{}, err
	}
	return kb, nil
}

func applySQLiteIndexingDefaults(kb *KnowledgeBaseConfig) error {
	if kb.Indexing == nil {
		kb.Indexing = map[string]any{}
	}
	lexical := ensureMap(kb.Indexing, "lexical")
	setDefault(lexical, "enabled", true)

	semantic := ensureMap(kb.Indexing, "semantic")
	setDefault(semantic, "enabled", true)
	setDefault(semantic, "provider", DefaultChromaProvider)
	collection := kb.ID
	if collection == "" {
		collection = DefaultKBID
	}
	setDefault(semantic, "collection", collection)
	setDefault(semantic, "sync_mode", DefaultChromaSyncMode)
	setDefault(semantic, "auto_download", true)

	value, hasMode := semantic["mode"]
	mode := ""
	if hasMode {
		var ok bool
		mode, ok = value.(string)
		if !ok {
			return fmt.Errorf("knowledge base %q chroma semantic mode must be a string", kb.ID)
		}
	}
	if mode == "" {
		if _, ok := semantic["base_url"]; ok {
			mode = DefaultChromaHTTPMode
		} else {
			mode = DefaultChromaMode
		}
		semantic["mode"] = mode
	}

	if mode == DefaultChromaHTTPMode {
		setDefault(semantic, "base_url", DefaultChromaBaseURL)
		return nil
	}
	if mode != DefaultChromaMode {
		return nil
	}

	path, ok := semantic["path"]
	if !ok {
		basePath, err := expandHomePath(DefaultChromaStoragePath)
		if err != nil {
			return err
		}
		semantic["path"] = filepath.Join(basePath, collection)
		return nil
	}
	pathString, ok := path.(string)
	if !ok || pathString == "" {
		return fmt.Errorf("knowledge base %q chroma semantic path must be a string", kb.ID)
	}
	expanded, err := expandHomePath(pathString)
	if err != nil {
		return err
	}
	semantic["path"] = expanded
	return nil
}

func ensureMap(parent map[string]any, key string) map[string]any {
	value, ok := parent[key]
	if !ok {
		child := map[string]any{}
		parent[key] = child
		return child
	}
	child, ok := value.(map[string]any)
	if ok {
		return child
	}
	child = map[string]any{}
	parent[key] = child
	return child
}

func setDefault(values map[string]any, key string, value any) {
	if _, ok := values[key]; !ok {
		values[key] = value
	}
}

func ExpandHomePath(path string) (string, error) {
	return expandHomePath(path)
}

func expandHomePath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}
