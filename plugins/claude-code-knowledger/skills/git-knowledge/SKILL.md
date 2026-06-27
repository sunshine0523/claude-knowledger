---
name: git-knowledge
description: Use when the user gives a git URL and says "拉取/clone/添加这个知识库", OR when the user says "更新知识库/同步知识库/pull知识库". Handles cloning a new git KB and pulling updates for existing ones, always followed by reindexing.
version: 1.0.0
triggers:
  - "拉取知识库"
  - "拉取这个知识库"
  - "clone知识库"
  - "添加git知识库"
  - "更新知识库"
  - "同步知识库"
  - "pull知识库"
  - "git知识库"
  - "git knowledge"
  - "pull knowledge base"
  - "update knowledge base"
---

# Git Knowledge Skill

## Two Scenarios

### 1. Add a New Git Knowledge Base

**Trigger**: User provides a git URL + intent to pull/clone/add it as a KB.

**Steps**:
1. Call `git_knowledge_add` with the URL (and optional `id`, `name`, `scope`).
2. Call `index_knowledge` with the returned KB's `id` and `scope` to index it.
3. Report success: KB cloned and indexed.

**Example user messages**: "帮我拉取这个知识库 https://github.com/...", "clone这个git知识库", "添加这个git知识库"

---

### 2. Update Existing Git Knowledge Base(s)

**Trigger**: User says "更新知识库", "同步知识库", "pull知识库", etc.

#### Case A — User specifies a KB name/id

1. Call `git_knowledge_pull` with that `id` (and `scope` if specified).
2. Call `index_knowledge` with that `id` and `scope`.
3. Report success.

#### Case B — No specific KB mentioned

1. Call `git_knowledge_list` to discover all git KBs (global + project).
2. For each KB in the result, call `git_knowledge_pull` with its `id` and `scope`.
3. After all pulls complete, call `index_knowledge` for each KB that was pulled successfully.
4. Report a summary of what was updated.

---

## Tool Reference

| Tool | When to use |
|------|-------------|
| `git_knowledge_add` | Clone a new git repo as a KB |
| `git_knowledge_pull` | Pull updates for an existing git KB |
| `git_knowledge_list` | List all git KBs (global + project dirs) |
| `index_knowledge` | Reindex a KB after clone/pull |

Always reindex after every clone or pull — otherwise new/changed files won't be searchable.
