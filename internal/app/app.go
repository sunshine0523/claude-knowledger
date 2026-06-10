package app

import (
	"github.com/kindbrave/knowledger/internal/adapters/cli"
	mcpadapter "github.com/kindbrave/knowledger/internal/adapters/mcp"
	"github.com/kindbrave/knowledger/internal/backends/sqlite"
	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)

type MCPRunner func(*service.Service) error

var runMCPServer MCPRunner = func(svc *service.Service) error {
	return mcpadapter.NewServer(svc).ServeStdio()
}

func SetMCPRunnerForTest(runner MCPRunner) func() {
	previous := runMCPServer
	runMCPServer = runner
	return func() { runMCPServer = previous }
}

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
	defer func() { _ = svc.Close() }()
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
	if hasStoreType(kbs, "sqlite") {
		sqliteBackend, err := sqlite.NewMulti(kbs)
		if err != nil {
			return nil, err
		}
		backends["sqlite"] = sqliteBackend
	}
	return backends, nil
}

func runService(svc *service.Service, address string, args []string) error {
	cmd := cli.NewRootCommandWithAddressAndMCPRunner(svc, address, func() error {
		return runMCPServer(svc)
	})
	cmd.SetArgs(args)
	return cmd.Execute()
}

func hasStoreType(kbs []core.KnowledgeBase, storeType string) bool {
	for _, kb := range kbs {
		if kb.StoreType == storeType {
			return true
		}
	}
	return false
}
