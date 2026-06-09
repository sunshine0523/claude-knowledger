# Retrieval Snippets 与全文读取接口设计

## 目标

检索结果不再返回完整 `content`，而是返回围绕检索词的短片段，降低 agent 上下文消耗；同时新增通过知识 ID 读取完整内容的接口和 CLI 命令，保证用户需要阅读全文时有明确入口。

## 决策

采用方案：**service 统一生成轻量 snippet，并新增 full-content 获取接口与 CLI 子命令**。

对比过的方案：

1. **只在 CLI 输出层截断**  
   改动最小，但 service/API 仍会携带全文，无法保护 agent 或其他调用方的上下文。
2. **在 backend 搜索阶段生成 snippet**  
   能较早瘦身，但 lexical、semantic、hybrid 的片段行为容易分散，semantic 命中仍需要回查 canonical SQLite 内容。
3. **在 service 聚合后统一瘦身，并新增 full-content 获取接口**  
   推荐并采用。它能让所有搜索模式输出一致片段，同时保留明确的全文读取路径。

## 接口边界

`core.StoreBackend` 新增：

```go
GetItem(context.Context, KnowledgeBase, string) (KnowledgeItem, error)
```

职责：

- backend 负责从 canonical store 按 `kb_id + item_id` 读取完整 `KnowledgeItem`。
- SQLite backend 使用 SQLite `knowledge_items` 表作为 canonical 来源。
- service 负责参数校验、KB/backend 路由，以及搜索结果瘦身。

`service.Service` 新增：

```go
GetKnowledgeItem(ctx context.Context, kbID string, itemID string) (core.KnowledgeItem, error)
```

职责：

- `kbID` 为空时返回配置错误。
- `itemID` 为空时返回配置错误。
- 找不到 KB、backend 未注册、item 不存在时返回明确错误。

CLI 新增：

```bash
knowledger get --kb <kb-id> --id <item-id>
```

输出完整 `core.KnowledgeItem` JSON，包括完整 `Content`。

## Search 输出行为

`service.Search` 保持原有搜索流程：

1. 遍历启用的 KB。
2. 解析 effective search mode。
3. 调用 backend `Search`。
4. 保留 semantic/hybrid fallback 与 warning 行为。
5. 聚合和排序 hits。
6. 应用最终 limit。
7. 将最终 hits 统一转换为轻量片段。

搜索结果中的 `Snippet` 和 `ContentPreview` 都不再包含完整 content。它们应被设置为相同的片段文本，便于旧调用方读取任一字段都不会拿到全文。

`Title`、`ItemID`、`KBID`、`Score`、`MatchMode`、`SourceBackend`、`Metadata` 等字段保持不变。

## Snippet 生成规则

默认窗口为命中词前后各 120 个字符，按 Unicode rune 计数，避免中文内容被按字节截断。

处理流程：

1. 对用户 query 做简单分词：按空白和常见标点切分，去掉空词。
2. 在完整 content 中大小写不敏感地查找第一个出现的 query 词。
3. 找到时，截取命中位置前 120 个字符到命中词后 120 个字符。
4. 如果片段不是从 content 开头开始，在前面加省略号。
5. 如果片段不是到 content 结尾结束，在后面加省略号。
6. 如果 query 为空，或没有任何 query 词能在 content 中字面匹配，返回 content 开头 240 个字符。
7. 如果 content 长度不超过要返回的窗口范围，直接返回完整 content；这是短内容，不违反搜索结果不返回长全文的目标。

Semantic/hybrid 命中也按同一规则处理：service 通过 `GetItem` 回查 canonical item 的完整 content 后生成 snippet。这样 Chroma hit 即使携带完整或部分 content，也不会决定最终输出长度。

## 回查失败行为

搜索结果瘦身阶段如果单条 hit 回查完整 item 失败：

- 不让整次 search 失败。
- 对该 hit 使用已有 `Snippet`，如果为空则使用 `ContentPreview`。
- 将 fallback 文本截断到 240 个字符。
- 追加 warning，说明该 hit 无法回查 full content 生成 query-centered snippet。

明确全文读取接口 `GetKnowledgeItem` 和 CLI `get` 不采用 fallback：参数缺失、KB 不存在、item 不存在都直接返回错误。

## SQLite GetItem 行为

SQLite backend 按 `kb_id` 和 `id` 查询 `knowledge_items`：

- 成功时返回完整 `core.KnowledgeItem`，字段解析规则与 `ListItems` 一致。
- item ID 必须限定在当前 KB 内，不能跨 KB 读取。
- 未找到时返回明确错误。

## 测试范围

需要覆盖：

- SQLite `GetItem` 成功返回完整 content。
- SQLite `GetItem` 按 KB 隔离，不能读取其他 KB 的 item。
- SQLite `GetItem` 未找到时返回错误。
- Service `GetKnowledgeItem` 校验空 `kbID`、空 `itemID`。
- Service `GetKnowledgeItem` 委托正确 backend 并返回完整 content。
- Service `Search` 命中 query 时返回前后各 120 字符片段，不返回全文。
- Service `Search` 找不到 query 字面匹配时返回 content 开头 240 字符。
- Service `Search` 对 semantic/hybrid hit 回查 canonical content 后生成片段。
- Service `Search` 回查失败时保留该 hit、截断 fallback 文本，并追加 warning。
- CLI `get --kb --id` 输出完整 item JSON。
- CLI `search` 输出 snippet，而不是完整 content。

## 非目标

- 不改变 ranking、score、hybrid merge 逻辑。
- 不实现句子级摘要或语义摘要。
- 不新增分页、range 读取或 partial content API。
- 不改变 Chroma sidecar 的索引内容。
- 不修改现有 add/delete/list-kbs 的用户界面。
