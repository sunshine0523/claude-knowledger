# Dashboard 与 Search Lab MVP 设计

日期：2026-06-06

## 背景

Knowledger 当前提供了基础 Web 控制台，其中 `/kbs` 已经可以管理知识库。Dashboard 路由 `/` 和 Search Lab 路由 `/search-lab` 目前只渲染静态占位模板，没有任何数据。Web API 当前只暴露知识库管理接口：`GET /api/kbs`、`POST /api/kbs`、`DELETE /api/kbs/{id}`。搜索能力已经存在于 service 层：`Service.Search(ctx, core.SearchOptions)`，并且 CLI 已经在使用它，但 Web adapter 还没有暴露搜索接口。

## 目标

实现一个 MVP，让 Dashboard 和 Search Lab 从占位页变成可用页面，同时不超出现有 service 能力范围。

MVP 会实现：

- 新增真实可用的 Search Lab，并由 `Service.Search` 驱动。
- 新增 Dashboard 知识库维度汇总指标。
- API 响应风格保持与现有 Web API wrapper 一致。
- UI 不引入新依赖，继续使用现有 Go templates 和 `web/static/app.js`。
- 为新增 Web API 行为和路由/模板标记补充 Go 测试。

## 非目标

MVP 不会实现：

- 暴露真实索引队列指标。
- 暴露最近索引失败记录。
- 统计每个知识库的 item 数量。
- 展示最近文档。
- 实现或修改 Chroma semantic query/upsert 行为。
- 新增 JavaScript 构建系统、npm 依赖或前端测试框架。
- 改变 CLI 或 MCP 行为。
- 删除或迁移现有知识库数据。

## 选定方案

直接在 Web adapter 中实现 MVP 所需 endpoints。

这个方案符合当前 Web API 模式：`/api/kbs` 已经在 `internal/adapters/web/server.go` 中直接包装 service 和 registry 数据。Dashboard MVP 指标只从 `ListKnowledgeBaseRecords()` 派生，目前还不需要新增 service 层 `DashboardSummary` 抽象。如果后续 Dashboard 需要 item count、worker metrics、Chroma health 或 failure history，再把这些能力提升到 service 层。

## 后端 API 设计

### `POST /api/search`

新增 Web API 路由：

```text
POST /api/search
```

请求体：

```json
{
  "query": "sqlite default storage",
  "limit": 10,
  "kb_ids": ["default"],
  "search_mode": "lexical"
}
```

请求规则：

- `query` 必填，并且 trim 空白后不能为空。
- `limit` 可选，默认值为 10。
- `limit` 必须为正数，且不能超过 100。
- `kb_ids` 可选；省略或空数组表示搜索所有启用的知识库。
- `search_mode` 可选；空值表示使用现有 service/backend 默认行为。
- 显式传入的 `search_mode` 只接受 `lexical`、`semantic`、`hybrid`。

handler 调用：

```go
s.svc.Search(r.Context(), core.SearchOptions{
    Query: query,
    Limit: limit,
    KBIDs: kbIDs,
    SearchMode: searchMode,
})
```

响应体沿用现有 `apiResponse` wrapper：

```json
{
  "success": true,
  "data": {
    "query": "sqlite default storage",
    "limit": 10,
    "hits": [
      {
        "item_id": "...",
        "kb_id": "default",
        "item_type": "",
        "title": "Default DB",
        "snippet": "...",
        "content_preview": "...",
        "score": 1,
        "match_mode": "lexical",
        "source_backend": "sqlite",
        "locator": "",
        "metadata": {}
      }
    ]
  },
  "warnings": ["..."],
  "errors": [],
  "meta": {
    "hit_count": 1
  }
}
```

错误处理：

- Web server 没有兼容 service 时返回 `503 service_unavailable`。
- 请求体不是合法 JSON 时返回 `400 invalid_json`。
- `query` trim 后为空时返回 `400 invalid_query`。
- `limit` 小于 1 或大于 100 时返回 `400 invalid_limit`。
- `search_mode` 非空且不属于允许值时返回 `400 invalid_search_mode`。
- 搜索执行错误尽量使用现有错误映射；没有匹配时返回 `500 search_failed`。

### `GET /api/dashboard`

新增 Web API 路由：

```text
GET /api/dashboard
```

handler 通过 `ListKnowledgeBaseRecords()` 读取知识库记录，使用现有 KB view 映射转换数据，并计算 Dashboard summary。

响应体：

```json
{
  "success": true,
  "data": {
    "summary": {
      "total_kbs": 3,
      "enabled_kbs": 2,
      "disabled_kbs": 1,
      "runtime_kbs": 1,
      "static_kbs": 2,
      "store_types": {
        "sqlite": 2,
        "text": 1
      }
    },
    "knowledge_bases": [
      {
        "id": "default",
        "name": "Default",
        "store_type": "sqlite",
        "path": "~/.knowledger/db",
        "enabled": true,
        "default_search_mode": "hybrid",
        "tags": [],
        "source": "static",
        "deletable": false
      }
    ],
    "indexing": {
      "state": "unsupported",
      "message": "Index queue metrics are not exposed in the web dashboard MVP."
    },
    "failures": {
      "state": "unsupported",
      "message": "Recent indexing failures are not exposed in the web dashboard MVP."
    }
  }
}
```

错误处理：

- Web server 没有兼容 service 时返回 `503 service_unavailable`。
- 列出知识库记录失败时返回 `500 list_kbs_failed`。

## Service 边界

扩展 Web adapter 内部 service interface，让同一个注入 service 同时支持知识库管理和搜索：

- `ListKnowledgeBaseRecords()`
- `CreateKnowledgeBase(ctx, input)`
- `DeleteKnowledgeBase(ctx, id)`
- `Search(ctx, core.SearchOptions)`

具体实现 `*service.Service` 已经具备这些能力。测试可以使用实现该 interface 的 fake service。

## Dashboard UI 设计

`web/templates/dashboard.html` 会从占位文字改成数据驱动的页面骨架。

页面包含：

- 页面标题和简短说明。
- `#dashboard-root` 作为功能根节点。
- loading/error message 区域。
- 顶部统计卡片：
  - Total KBs
  - Enabled
  - Disabled
  - Runtime
  - Static
- store type 分布区域。
- 知识库明细表，列包括：
  - ID
  - Name
  - Store Type
  - Path
  - Enabled
  - Source
  - Default Search Mode
  - Tags
- 两个轻量状态块：
  - Index Queue：`unsupported`
  - Recent Failures：`unsupported`

Dashboard 页面加载后通过 `GET /api/dashboard` 获取数据。没有配置知识库时显示空状态；API 失败时显示错误消息。

## Search Lab UI 设计

`web/templates/search_lab.html` 使用已选定的“查询栏 + 调试表格”布局。

页面包含：

- 搜索表单 `#search-form`。
- Query 文本输入框，必填。
- Limit 数字输入框，默认 10。
- KB IDs 文本输入框，使用逗号分隔；留空表示所有启用知识库。
- Search Mode select：
  - default / empty
  - lexical
  - semantic
  - hybrid
- Request summary 区域，展示规范化后的请求和 hit count。
- Warning 区域，展示 `SearchResult.Warnings`。
- Results table，列包括：
  - Title
  - KB
  - Score
  - Match Mode
  - Backend
  - Locator
  - Snippet

提交时，JavaScript 阻止默认导航，禁用提交按钮，显示 searching 状态，调用 `POST /api/search`，然后渲染响应。没有结果时显示 `No hits found.`；失败时显示可见错误消息。Score 格式化为三位小数。

## JavaScript 设计

继续使用 `web/static/app.js`。保留已有 KB 管理行为，并增加小的 feature-specific 函数。

通用 API helper 保持：

- `firstAPIError(payload)`
- `parseAPIResponse(response)`

Dashboard 函数：

- `loadDashboard()`
- `renderDashboard(payload)`
- `renderStoreTypes(storeTypes)`
- `renderKnowledgeBases(rows)`

Search Lab 函数：

- `setupSearchLab(form)`
- `searchPayloadFromForm(form)`
- `renderSearchResults(payload)`
- `renderSearchWarnings(warnings)`
- `formatScore(score)`

通过 DOM 是否存在来启用对应功能：

```js
const dashboardRoot = document.querySelector("#dashboard-root");
if (dashboardRoot) loadDashboard();

const searchForm = document.querySelector("#search-form");
if (searchForm) setupSearchLab(searchForm);
```

所有来自用户或 backend 的动态内容都必须通过 `textContent` 或 text node 插入，不能用 HTML 字符串拼接。这可以防止 snippet、metadata 或 title 注入页面 markup。

## 测试计划

### Web API 测试

在 `internal/adapters/web/server_test.go` 中新增测试，或拆分为专门的 Web API 测试文件。

`POST /api/search` 测试：

- service 不可用时返回 `503 service_unavailable`。
- 非法 JSON 返回 `400 invalid_json`。
- 空 query 返回 `400 invalid_query`。
- limit 为负数、0 或超过上限时返回 `400 invalid_limit`。
- 无效 search mode 返回 `400 invalid_search_mode`。
- 合法请求能把 query、limit、KB IDs 和 search mode 正确传给 fake service。
- 合法响应包含 hits、warnings 和 `meta.hit_count`。

`GET /api/dashboard` 测试：

- service 不可用时返回 `503 service_unavailable`。
- 合法响应能正确计算 `total_kbs`、`enabled_kbs`、`disabled_kbs`、`runtime_kbs`、`static_kbs` 和 `store_types`。
- 响应包含使用现有 view shape 的 `knowledge_bases`。
- 响应包含 `indexing.state = unsupported` 和 `failures.state = unsupported`。

### 路由/模板测试

- `GET /` 返回 200，并包含 `dashboard-root`。
- `GET /search-lab` 返回 200，并包含 `search-form`。
- 现有 `/kbs` 路由和 API 行为继续通过。

### 验证命令

运行：

```bash
go test ./...
CGO_ENABLED=1 go test -tags fts5 ./...
```

手动 smoke 验证：

```bash
go run ./cmd/knowledger serve
```

然后打开：

- `http://127.0.0.1:34125/`
- `http://127.0.0.1:34125/search-lab`

检查 Dashboard 统计、Search Lab 成功搜索、空结果、空 query 错误和非法 limit 错误。

## 实施阶段

### Phase 1：后端 API

- 扩展 Web adapter service interface，让它支持 search。
- 新增 search request/view types。
- 注册 `POST /api/search`。
- 注册 `GET /api/dashboard`。
- 实现 search validation 和 handler。
- 实现 dashboard summary 计算。
- 新增 Web API 测试。

### Phase 2：模板

- 将 Dashboard 占位内容替换为数据容器和表格骨架。
- 将 Search Lab 占位内容替换为表单、summary、warnings 和 results table 骨架。
- 确保两个页面都加载 `/static/app.js`。
- 增加稳定 DOM ID，供测试和 JavaScript 使用。

### Phase 3：前端 JavaScript

- 新增 Dashboard loading 和 rendering。
- 新增 Search Lab form handling 和 rendering。
- 保留现有 KB create/delete 行为。
- 使用 `textContent` 安全渲染所有动态内容。

### Phase 4：验证

- 运行 Go tests。
- 当 CGO 可用时运行 FTS5 tagged tests。
- 在浏览器中运行本地手动 smoke 验证。

## 后续扩展

MVP 之后自然的下一步包括：

- 如果 CLI/MCP 也需要 Dashboard summary，将 summary 逻辑移动到 service 层。
- 通过扩展 backend list/count API 增加 item counts 和 recent documents。
- 从 indexing package 暴露索引队列指标。
- 持久化 indexing failures，并暴露最近失败记录。
- 将真实 Chroma semantic query results 接入 backend search。
- 当 Web UI 超出简单 DOM 行为后，再补充前端测试。
