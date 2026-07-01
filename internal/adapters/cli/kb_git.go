package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newKBGitKnowledgeAddCommand(svc *service.Service) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:   "kb-git-knowledge-add <url>",
		Short: "Clone a git repository as a text knowledge base",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			if id == "" {
				id = idFromGitURL(url)
			}
			if name == "" {
				name = id
			}
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			clonePath, err := gitKnowledgePath(svc, scope, id)
			if err != nil {
				return err
			}
			if _, err := os.Stat(clonePath); err == nil {
				return fmt.Errorf("path already exists: %s", clonePath)
			}
			if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
				return err
			}
			out, err := exec.CommandContext(cmd.Context(), "git", "clone", url, clonePath).CombinedOutput()
			if err != nil {
				return fmt.Errorf("git clone failed: %w\n%s", err, out)
			}
			record, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{
				Scope:     scope,
				ID:        id,
				Name:      name,
				StoreType: "text",
				Path:      clonePath,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"knowledge_base": record})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "knowledge base id (derived from repository name if omitted)")
	cmd.Flags().StringVar(&name, "name", "", "human-readable name (defaults to id)")
	return cmd
}

func newKBGitKnowledgePullCommand(svc *service.Service) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "kb-git-knowledge-pull",
		Short: "Pull latest changes for a git-knowledge knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			var kbPath string
			for _, kb := range svc.ListKnowledgeBases() {
				if kb.ID == id && kb.Scope == scope {
					kbPath, _ = kb.StoreConfig["path"].(string)
					break
				}
			}
			if kbPath == "" {
				return fmt.Errorf("knowledge base %q (scope: %s) not found or has no path", id, scope)
			}
			out, err := exec.CommandContext(cmd.Context(), "git", "-C", kbPath, "pull").CombinedOutput()
			fmt.Fprint(cmd.OutOrStdout(), string(out))
			if err != nil {
				return fmt.Errorf("git pull failed: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "knowledge base id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
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

func newKBGitKnowledgeListCommand(svc *service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "kb-git-knowledge-list",
		Short: "List all git-knowledge knowledge bases (global and project)",
		RunE: func(cmd *cobra.Command, args []string) error {
			type entry struct {
				Scope string `json:"scope"`
				ID    string `json:"id"`
				Path  string `json:"path"`
			}
			// Build a set of registered git-knowledge paths so orphaned
			// directories (whose KB record was already deleted) are not
			// surfaced.
			registered := make(map[string]struct{})
			if svc != nil {
				for _, kb := range svc.ListKnowledgeBases() {
					if kb.StoreType != "text" {
						continue
					}
					path, _ := kb.StoreConfig["path"].(string)
					if path == "" {
						continue
					}
					clean := filepath.Clean(path)
					if filepath.Base(filepath.Dir(clean)) == "git-knowledge" {
						registered[clean] = struct{}{}
					}
				}
			}
			var results []entry
			if home, err := os.UserHomeDir(); err == nil {
				dir := filepath.Join(home, ".knowledger", "git-knowledge")
				if entries, err := os.ReadDir(dir); err == nil {
					for _, e := range entries {
						if !e.IsDir() {
							continue
						}
						full := filepath.Join(dir, e.Name())
						if _, ok := registered[filepath.Clean(full)]; ok {
							results = append(results, entry{"global", e.Name(), full})
						}
					}
				}
			}
			if svc != nil && svc.HasProjectScope() {
				dir := filepath.Join(svc.ProjectRoot(), ".knowledger", "git-knowledge")
				if entries, err := os.ReadDir(dir); err == nil {
					for _, e := range entries {
						if !e.IsDir() {
							continue
						}
						full := filepath.Join(dir, e.Name())
						if _, ok := registered[filepath.Clean(full)]; ok {
							results = append(results, entry{"project", e.Name(), full})
						}
					}
				}
			}
			if results == nil {
				results = []entry{}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
		},
	}
}

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)

func idFromGitURL(url string) string {
	u := strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	seg := u[strings.LastIndexAny(u, "/:")+1:]
	seg = nonAlphanumDash.ReplaceAllString(strings.ToLower(seg), "-")
	seg = strings.Trim(seg, "-")
	if seg == "" {
		return "git-knowledge"
	}
	return seg
}
