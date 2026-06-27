package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

type gitKnowledgeAddInput struct {
	Scope string `json:"scope,omitempty"`
	URL   string `json:"url"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
}

type gitKnowledgePullInput struct {
	Scope string `json:"scope,omitempty"`
	ID    string `json:"id"`
}

func (s *Server) handleGitKnowledgeAdd(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input gitKnowledgeAddInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return mcpgo.NewToolResultError("url is required"), nil
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = gitIDFromURL(url)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = id
	}
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	clonePath, err := gitKnowledgePath(s.svc, scope, id)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	if _, err := os.Stat(clonePath); err == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("path already exists: %s", clonePath)), nil
	}
	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	out, err := exec.CommandContext(ctx, "git", "clone", url, clonePath).CombinedOutput()
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("git clone failed: %v\n%s", err, out)), nil
	}
	record, err := s.svc.CreateKnowledgeBase(ctx, service.CreateKnowledgeBaseInput{
		Scope:     scope,
		ID:        id,
		Name:      name,
		StoreType: "text",
		Path:      clonePath,
	})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"knowledge_base": record}), nil
}

func (s *Server) handleGitKnowledgePull(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input gitKnowledgePullInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	id := strings.TrimSpace(input.ID)
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	var kbPath string
	for _, kb := range s.svc.ListKnowledgeBases() {
		if kb.ID == id && kb.Scope == scope {
			kbPath, _ = kb.StoreConfig["path"].(string)
			break
		}
	}
	if kbPath == "" {
		return mcpgo.NewToolResultError(fmt.Sprintf("knowledge base %q (scope: %s) not found or has no path", id, scope)), nil
	}
	out, err := exec.CommandContext(ctx, "git", "-C", kbPath, "pull").CombinedOutput()
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("git pull failed: %v\n%s", err, out)), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"output": string(out), "id": id, "scope": scope}), nil
}

func (s *Server) handleGitKnowledgeList(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	type entry struct {
		Scope string `json:"scope"`
		ID    string `json:"id"`
		Path  string `json:"path"`
	}
	var results []entry
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".knowledger", "git-knowledge")
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					results = append(results, entry{"global", e.Name(), filepath.Join(dir, e.Name())})
				}
			}
		}
	}
	if s.svc != nil && s.svc.HasProjectScope() {
		dir := filepath.Join(s.svc.ProjectRoot(), ".knowledger", "git-knowledge")
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					results = append(results, entry{"project", e.Name(), filepath.Join(dir, e.Name())})
				}
			}
		}
	}
	if results == nil {
		results = []entry{}
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"git_knowledge_bases": results}), nil
}

func gitKnowledgePath(svc *service.Service, scope, id string) (string, error) {
	if scope == core.ScopeProject {
		root := svc.ProjectRoot()
		if root == "" {
			return "", fmt.Errorf("not in a project directory")
		}
		return filepath.Join(root, ".knowledger", "git-knowledge", id), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".knowledger", "git-knowledge", id), nil
}

var gitIDRegexp = regexp.MustCompile(`[^a-z0-9-]+`)

func gitIDFromURL(url string) string {
	u := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	seg := u[strings.LastIndexAny(u, "/:")+1:]
	seg = gitIDRegexp.ReplaceAllString(strings.ToLower(seg), "-")
	seg = strings.Trim(seg, "-")
	if seg == "" {
		return "git-knowledge"
	}
	return seg
}
