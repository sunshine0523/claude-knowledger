package app

import (
	"fmt"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
	"github.com/kindbrave/knowledger/internal/backends/sqlite"
	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)

func Run(configPath string, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	return RunWithConfig(cfg, args)
}

func RunDefault(args []string) error {
	cfg, err := config.Default()
	if err != nil {
		return err
	}
	return RunWithConfig(cfg, args)
}

func RunWithConfig(cfg config.Config, args []string) error {
	if err := config.ApplyDefaults(&cfg); err != nil {
		return err
	}
	svc, err := BuildServiceFromConfig(cfg)
	if err != nil {
		return err
	}
	return runService(svc, cfg.Server.Address, args)
}

func BuildService(configPath string) (*service.Service, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return BuildServiceFromConfig(cfg)
}

func BuildDefaultService() (*service.Service, error) {
	cfg, err := config.Default()
	if err != nil {
		return nil, err
	}
	return BuildServiceFromConfig(cfg)
}

func BuildServiceFromConfig(cfg config.Config) (*service.Service, error) {
	if err := config.ApplyDefaults(&cfg); err != nil {
		return nil, err
	}
	r := registry.New(cfg.KnowledgeBases, registry.NewFileStore(cfg.RuntimeRegistryPath))
	return service.NewManaged(r, buildBackends)
}

func buildBackends(kbs []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
	backends := map[string]core.StoreBackend{
		"text": text.New(),
	}
	path, hasSQLite, err := sqlitePath(kbs)
	if err != nil {
		return nil, err
	}
	if hasSQLite {
		sqliteBackend, err := sqlite.New(path)
		if err != nil {
			return nil, err
		}
		backends["sqlite"] = sqliteBackend
	}
	return backends, nil
}

func runService(svc *service.Service, address string, args []string) error {
	cmd := cli.NewRootCommandWithAddress(svc, address)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func sqlitePath(kbs []core.KnowledgeBase) (string, bool, error) {
	var selected string
	for _, kb := range kbs {
		if kb.StoreType != "sqlite" {
			continue
		}
		path, ok := kb.StoreConfig["path"].(string)
		if !ok || path == "" {
			return "", false, fmt.Errorf("knowledge base %q sqlite store_config.path is required", kb.ID)
		}
		if selected == "" {
			selected = path
			continue
		}
		if path != selected {
			return "", false, fmt.Errorf("multiple sqlite database paths are not supported: %q and %q", selected, path)
		}
	}
	if selected == "" {
		return "", false, nil
	}
	return selected, true, nil
}
