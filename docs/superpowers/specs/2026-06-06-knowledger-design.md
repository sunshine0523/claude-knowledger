# Knowledger 设计规格说明

- **日期：** 2026-06-06
- **状态：** 交互式设计已批准；书面规格待审阅
- **产出方式：** 与用户交互式头脑风暴并逐步确认

## 1. 概述

Knowledger 是一个使用 Go 实现、主要面向 agent 使用的知识聚合系统。它通过 MCP 和 CLI 提供一组小而稳定的高层接口，同时提供一个面向人工运维与调试的 Web 控制台。系统会在统一的 Core 抽象之后屏蔽底层存储差异，因此 agent 可以在不了解底层知识库究竟是文本目录、SQLite FTS5，还是带有 Chroma 语义索引增强的 SQLite 的情况下，完成知识检索、列出知识库、写入知识等操作。

这个产品的优先级排序为：

1. 面向 agent 的 API 简单且稳定
2. 对多种知识库与后端类型进行统一管理
3. 检索质量与聚合行为

## 2. 目标

### 核心目标

- 为 agent 提供与后端无关的统一知识接口
- 默认跨多个已启用知识库做聚合搜索
- 在一个 Core API 后面支持多种存储实现
- 通过 CLI、MCP、Web UI 暴露一致的概念模型
- 允许用户同时通过静态配置与运行时管理来维护知识库
- 提供一个完整的 Web 控制台，用于管理、CRUD、调试与检索观察

### 初始版本非目标

- 多用户认证与权限系统
- 分布式任务执行
- 复杂排序系统或 ML reranker
- 超出初始 SQLite + Chroma 模式之外的多种语义后端
- 将 Chroma 作为独立 canonical store 使用

## 3. 已确认的产品决策

### 3.1 agent 接口优先级
系统首先为 agent 消费场景优化，其次才是知识库管理，最后才是更复杂的检索能力。

### 3.2 与存储无关的 add 语义
面向 agent 的 `add` 行为必须与具体存储后端解耦。

- 对于 text 类型知识库，写入应落为 llm-wiki 风格文档
- 对于 SQLite 类型知识库，写入应落为 canonical record / 文本块
- agent 不需要编写任何与后端类型相关的逻辑

### 3.3 统一检索结果模型
检索结果应采用混合统一模型，而不是拆分为仅文档型 API 或仅 chunk 型 API。

### 3.4 知识库管理方式
知识库应同时支持以下两种管理方式：

- 静态配置文件
- CLI 运行时管理

此外，Web 控制台也应具备管理能力。

### 3.5 Web UI 范围
产品包含的是一个完整 Web 控制台，而不只是一个配置页面。它应支持：

- 知识库配置
- 知识 CRUD
- 检索结果观察与调试
- 索引/任务状态可视化
- MCP/CLI 调试

### 3.6 系统结构
系统应按以下方式组织：

- 一个 Go Core Library
- 一个 CLI 适配层
- 一个 MCP 适配层
- 一个 Web 适配层

### 3.7 面向 agent 的操作模型
面向 agent 的接口应保持为一小组高层操作，而不是暴露资源级内部细节。

### 3.8 默认搜索范围
搜索默认应聚合所有已启用知识库，除非显式提供过滤条件。

### 3.9 SQLite + Chroma 建模方式
SQLite 保持独立 canonical backend 的地位。Chroma **不能** 被建模为独立知识库 backend，而应被视为挂接在 SQLite 之上的语义索引 sidecar。

### 3.10 Sidecar 同步策略
SQLite 到 Chroma 的索引同步应采用异步方式。

## 4. 推荐架构

### 4.1 选定方案
推荐方案是：**统一 Core + 多入口 Adapter + 可插拔存储/索引能力**。

选择该方案的原因：

- 能保持稳定的 agent-facing 契约
- 能把后端差异全部收敛在 Core 内部
- 能支持多个入口而不复制业务逻辑
- 能为未来扩展新的后端和检索能力预留空间

### 4.2 分层结构

#### Core 层
Core 负责：

- 知识库注册表加载与解析
- 统一写入语义
- 聚合检索
- 结果归一化
- 路由、过滤、打分与合并逻辑
- 错误归一化
- 索引任务协调

Core 不应该依赖调用方是 CLI、MCP 还是 Web。

#### Adapter 层
系统通过三个适配层包装 Core：

##### CLI Adapter
面向人类操作员和脚本。

职责：
- 命令解析
- 人类可读输出
- 面向自动化的 JSON 输出
- 知识库管理与运维流程支持

##### MCP Adapter
面向 agent。

职责：
- 暴露少量高层工具
- 保持 schema 与语义稳定
- 避免泄露具体 backend 的实现细节

##### Web Adapter
面向人工控制台场景。

职责：
- 管理界面
- 知识项浏览与编辑
- 检索实验与调试
- 索引状态可视化
- MCP/CLI 请求与响应的观察

### 4.3 存储与检索建模拆分
Backend 模型应拆成两个维度。

#### Store Backend
这类后端定义 canonical 数据存储位置，以及写入/读取的主行为。

初始 store backend：
- `text`
- `sqlite`

#### Retrieval / Indexing Capability
这类能力定义知识库如何被检索。

能力族包括：
- lexical retrieval
- semantic retrieval
- hybrid retrieval

这个拆分很关键，因为 Chroma 不是 store backend，而是 SQLite 的语义索引 sidecar。

## 5. 知识库类型

### 5.1 Text Knowledge Base
Text 类型知识库指向一个目录。

行为：
- 将知识保存为 llm-wiki 风格文档
- 支持 list/read/write
- 支持基础 lexical search

### 5.2 SQLite Knowledge Base
SQLite 类型知识库指向一个 SQLite 文件，并作为 canonical record store。

行为：
- 存储 canonical record 与元数据
- 支持 list/read/write
- 支持基于 FTS5 的 lexical search
- 可选挂接 Chroma 语义 sidecar

### 5.3 SQLite + Chroma Hybrid Knowledge Base
这本质上仍然是一个 SQLite 知识库。

行为：
- SQLite 存 canonical record 与 FTS 索引
- Chroma 存用于语义召回的派生向量表示
- 搜索时通过 SQLite FTS 与 Chroma semantic recall 组合成 hybrid retrieval

## 6. 数据模型

### 6.1 KnowledgeBase
表示一个逻辑上的已配置知识库实例。

建议字段：

- `id`
- `name`
- `store_type`（`text` | `sqlite`）
- `store_config`
- `enabled`
- `default_search_mode`
- `indexing`
- `tags` / `labels`

关键含义：
`KnowledgeBase` 是一个逻辑上的、可检索/可写入的知识源，而不只是某个数据库文件名或目录路径。

### 6.2 KnowledgeItem
这是 agent 最核心会接触到的统一知识对象。

建议字段：

- `id`
- `kb_id`
- `type`（`document` | `chunk` | `note`）
- `title`
- `content`
- `summary`
- `source_ref`
- `metadata`
- `tags`
- `created_at`
- `updated_at`

不同后端下的行为：
- text backend 可能将一个 item 直接落为一个文档
- SQLite backend 可能将一个 item 落为一个 canonical record，并在索引时再派生 chunk
- 但对 agent 来说，始终操作的是同一个 `KnowledgeItem` 抽象

### 6.3 SearchHit
表示归一化后的检索命中结果视图。

建议字段：

- `item_id`
- `kb_id`
- `item_type`
- `title`
- `snippet`
- `content_preview`
- `score`
- `match_mode`（`lexical` | `semantic` | `hybrid`）
- `source_backend`
- `locator`
- `metadata`

关键含义：
`SearchHit` 不是存储实体本身，而是命中结果的统一表示。它可能来源于文档命中、chunk 命中、FTS 命中，或向量命中回映后的结果。

### 6.4 IngestionResult 与 IndexStatus
需要独立的运行态结果模型。

#### IngestionResult
应报告：
- canonical 写入是否成功
- 创建/更新了哪个 item
- 是否已投递索引任务
- 是否发生 warning

#### IndexStatus
应报告：
- 当前索引状态
- 最近成功时间
- 最近失败详情
- 队列状态 / backlog 指标（如适用）

## 7. 面向 agent 的高层操作

面向 agent 的接口应有意保持精简。

### 7.1 `search`
用途：
- 默认跨所有已启用知识库搜索
- 支持可选过滤
- 返回统一归一化后的聚合命中结果

建议输入：
- `query`
- `kb_ids?`
- `limit?`
- `filters?`
- `search_mode?`（`auto` | `lexical` | `semantic` | `hybrid`）

行为：
- 默认模式是 `auto`
- 每个知识库按自身能力选择最合适的检索路径
- 所有结果被归一化后再统一合并

### 7.2 `add`
用途：
- 让 agent 在不关心后端差异的前提下写入知识

建议输入：
- `title?`
- `content`
- `kb_id` 或默认目标策略
- `tags?`
- `metadata?`

行为：
- text backend 写入文档
- SQLite backend 写入 canonical record
- 若启用语义索引，则异步投递索引任务

建议返回：
- `item`
- `ingestion_result`
- `index_status`

### 7.3 `list_kbs`
用途：
- 让 agent 或操作员查看当前可用知识库及其能力

建议返回内容包含：
- store 类型
- enabled 状态
- 是否支持写入
- 是否支持 hybrid search
- 索引健康状态

### 7.4 `manage_kb`
用途：
- 提供知识库管理能力
- 更偏向 CLI/Web 使用，但 MCP 也可以暴露

建议子操作：
- create
- update
- enable
- disable
- delete
- reindex
- sync_status
- test_connection

## 8. 检索流程

### 8.1 Step A：选择目标知识库
默认行为：
- 搜索所有已启用知识库

可选过滤：
- 按 knowledge base id
- 按 tags
- 按 store type
- 按 search mode

### 8.2 Step B：按知识库能力路由搜索
在 `search_mode=auto` 下，每个知识库按其最佳能力执行检索。

例如：
- text knowledge base → lexical search
- 不带语义增强的 SQLite knowledge base → FTS5 lexical search
- 挂了 Chroma sidecar 的 SQLite knowledge base → hybrid search

### 8.3 Step C：结果归一化
每种 backend / 检索路径都可能返回不同的原生结果形态，Core 负责将其统一转换为 `SearchHit`。

原生结果可能来源于：
- text 文档命中
- SQLite FTS 记录或 chunk 命中
- Chroma 向量命中后映射回 canonical item

### 8.4 Step D：全局合并与重排
Core 对归一化结果进行全局合并和 Top-N 选择。

初版排序策略应简单稳定：
- 先做各来源分数归一化
- 对 hybrid 命中给予略高于单一路径命中的权重
- 可对精确短语 / 标题 / 标签命中做适度加权
- 最终产出全局 top N

更高级的 reranking 明确不属于初始版本范围。

## 9. SQLite + Chroma 混合模型

### 9.1 Canonical 数据与派生数据
SQLite 是事实源（source of truth）。

Chroma 保存的是来自 SQLite 内容的派生语义检索表示。

也就是说：
- SQLite 持有 canonical record、metadata，以及 lexical index 数据
- Chroma 持有从 canonical 内容派生出来的 semantic retrieval 结构

### 9.2 为什么 Chroma 不是独立 backend
如果把 Chroma 当作独立 backend，会错误地暗示：
- 它可以作为独立可写的 canonical store
- 它应直接作为一个逻辑知识库暴露出去

这与当前设计目标相冲突。正确做法是把 Chroma 放在 SQLite knowledge base 的 indexing 配置内部。

### 9.3 Hybrid Search 行为
对于启用语义索引的 SQLite knowledge base：
- 一条检索路径使用 SQLite FTS
- 一条检索路径使用 Chroma semantic recall
- Core 再将两条结果流合并为统一的 hybrid result stream

## 10. 索引与异步同步

### 10.1 写入路径
当向启用语义索引的 SQLite knowledge base 执行写入时：

1. 先将 canonical record 写入 SQLite
2. 创建索引任务
3. 将任务放入队列
4. 由后台 worker 执行语义索引处理

### 10.2 后台索引 Worker 职责
Worker 应负责：
- 从 SQLite 读取 canonical 内容
- 对内容进行 chunking 或其他语义文档派生
- 生成 embedding
- 将结果 upsert 到 Chroma
- 更新状态与失败信息

### 10.3 为什么优先采用异步
优先采用异步的原因：
- canonical 写入路径更快、更稳定
- Chroma 故障不会阻塞事实存储
- 索引失败可以单独重试
- semantic indexing 被正确建模为派生维护流程

### 10.4 索引状态模型
建议的 item 级状态：
- `not_indexed`
- `queued`
- `indexing`
- `indexed`
- `failed`

建议的 knowledge base 级可视状态：
- 最近同步时间
- 队列积压量
- 最近失败次数
- 当前 embedding provider
- Chroma collection 名称

### 10.5 失败恢复
必须支持的恢复动作：
- 重试单条失败 item
- 对单个 knowledge base 执行 reindex
- 全量重建 semantic index

索引任务必须具备幂等性，确保重试不会产生重复语义副本。

## 11. 错误处理与降级

### 11.1 错误分类
错误应统一归一化为四类：

#### 配置错误
例如：
- 缺少必填字段
- 路径非法
- SQLite 配置格式错误
- Chroma 配置不完整

#### 存储错误
例如：
- text 目录不可写
- SQLite 打不开
- schema 初始化失败
- canonical 写入失败

#### 索引错误
例如：
- embedding 生成失败
- Chroma 不可用
- semantic upsert 失败
- chunking 过程抛错

#### 查询错误
例如：
- 某个 knowledge base 的搜索路径失败
- hybrid search 中 semantic 分支失败
- merge 过程异常

### 11.2 降级行为
系统应在可能时优雅降级。

#### 搜索降级
如果 SQLite hybrid search 中 Chroma 不可用：
- 自动回退到 SQLite FTS
- 返回 warning，说明 semantic path 不可用

#### 写入部分成功
如果 SQLite 写入成功，但 semantic indexing 失败：
- `add` 仍应报告 canonical storage 成功
- 同时明确暴露 indexing warning / status

#### 聚合部分成功
如果全局搜索时某个 knowledge base 失败：
- 仍保留其他知识库的成功结果
- 在响应中标记 partial / degraded
- 报告失败的知识库列表

### 11.3 统一响应包络
CLI、MCP、Web 三个入口都应遵循同一概念上的响应结构：

- `success`
- `data`
- `warnings`
- `errors`
- `meta`

例如 search 的 `meta` 可以包含：
- 实际搜索了哪些知识库
- 用了哪些检索模式
- 总耗时
- 是否发生 fallback

## 12. 配置设计

### 12.1 双层配置模型
因为产品需要同时支持静态声明和运行时管理，所以配置应拆成两层。

#### 静态配置文件
用于 bootstrap / default。

应包含：
- server/web 配置
- MCP 配置
- 默认搜索参数
- 默认索引 / embedding 参数
- 初始 knowledge base 声明

#### 运行时注册表状态
用于持久化运行时管理结果。

应包含：
- 通过 CLI/Web 创建或修改的 knowledge bases
- enable/disable 状态
- 控制平面所需的派生运行信息

### 12.2 有效配置视图
Core 不应直接依赖单一文件，而应消费一个合并后的有效 registry 视图。

好处：
- 保留手写配置的能力
- UI/CLI 动态管理仍是一等能力
- 运行时修改不必绑定到脆弱的 file-only 工作流

### 12.3 SQLite 语义索引配置形态
语义索引必须作为 SQLite knowledge base 下的嵌套配置出现，而不是单独配置为一个 knowledge base。

概念上应类似：
- `store.type = sqlite`
- `indexing.lexical.enabled = true`
- `indexing.semantic.enabled = true`
- `indexing.semantic.provider = chroma`
- `indexing.semantic.collection = ...`
- `indexing.semantic.embedding = ...`
- `indexing.semantic.sync_mode = async`

## 13. Web 控制台设计

Web 控制台是完整操作控制台，而不是简单设置页。

### 13.1 Dashboard
展示：
- knowledge base 总数
- 启用/禁用数量
- 最近搜索情况
- 索引队列状态
- 最近失败任务
- 各知识库健康指标

### 13.2 Knowledge Bases 页面
支持：
- 列表
- 创建 / 编辑 / 删除
- 启用 / 禁用
- 测试连接
- 能力展示
- SQLite / Chroma 语义索引配置

### 13.3 Knowledge Items 页面
支持：
- 按 knowledge base 浏览
- 过滤 / 搜索
- 查看 item 详情
- 新增 / 编辑 / 删除
- 查看 metadata / source references

### 13.4 Search Lab 页面
支持：
- 输入 query
- 选择搜索范围与过滤条件
- 选择模式（`auto`、`lexical`、`semantic`、`hybrid`）
- 展示归一化结果
- 显示 score、match mode、source knowledge base、snippet
- 可选展示原生 backend 结果与归一化结果对照，便于调试

### 13.5 Indexing / Jobs 页面
支持：
- 队列查看
- 失败任务查看
- 重试操作
- 单 knowledge base reindex
- 全量 reindex
- 同步状态可视化

### 13.6 MCP / CLI Debug 页面
支持：
- 最近 MCP 调用
- 最近 CLI 调用
- 请求参数
- 归一化响应
- 错误与 warning
- 手动模拟 Core 操作

## 14. MVP 范围

### 14.1 MVP 必须包含

#### Core
- 统一模型（`KnowledgeBase`、`KnowledgeItem`、`SearchHit`）
- backend/store 抽象
- 聚合搜索
- 统一 add 语义
- 基础知识库列出与管理能力

#### Backends
- text backend
- 带 FTS5 的 SQLite backend

#### Indexing
- 面向 SQLite 的 semantic-sidecar 抽象
- 异步任务模型
- 队列 / 状态 / 重试基础设施
- 若排期允许，可接入第一版 Chroma 集成

#### Adapters
- CLI adapter
- MCP adapter
- Web adapter

#### Web 页面
至少包括：
- Dashboard
- Knowledge Bases
- Search Lab
- Indexing 状态页
- 基础 Debug 页面

### 14.2 MVP 应避免不必要扩张
以下能力应推迟，除非明确需要：
- 高级 reranker
- 丰富的排序策略系统
- 分布式任务编排
- 多用户权限
- 非 SQLite 语义存储
- 过度抽象的 embedding provider 系统

## 15. 测试策略

### 15.1 Core 单元测试
需要覆盖：
- 基于 backend capability 的路由
- 聚合与合并逻辑
- fallback / degraded 行为
- score 归一化
- 配置解析

### 15.2 Backend 合约测试
每种 backend 都应通过同一套概念级 contract tests，覆盖：
- add
- list/get
- search
- metadata handling
- 预期错误行为

这样能减少以后新增 backend 时的行为漂移。

### 15.3 SQLite + Chroma 集成测试
需要覆盖：
- SQLite canonical write
- 索引任务入队
- worker 处理流程
- Chroma upsert 路径
- 失败记录与重试行为
- hybrid search
- semantic path 失败时的 lexical fallback

### 15.4 MCP / CLI Adapter 测试
需要覆盖：
- 参数映射
- 输出 schema 稳定性
- warning / error 映射
- 机器可读输出一致性

### 15.5 Web UI 测试
需要覆盖：
- knowledge base 配置流程
- Search Lab 请求与结果渲染
- indexing 状态更新
- debug 面板显示一致性与正确性

## 16. 面向实现阶段的固定约束

以下约束已经确定，应直接指导 implementation planning：

- 面向 agent 的操作保持高层且数量少
- 搜索默认跨 knowledge base 聚合
- text 与 SQLite 都是一等 canonical store
- Chroma 是 SQLite 的语义 sidecar，而不是独立 backend
- SQLite 到 Chroma 的同步必须异步
- Web 控制台是一等产品界面，而不是附属工具
- 后端差异必须对 agent 消费者透明

## 17. 建议的下一步

下一步应编写 implementation plan，将已批准的设计拆分为可渐进交付的阶段，MVP 可优先按以下顺序推进：

1. Core 抽象与 config/registry
2. text backend 与 SQLite FTS5 backend
3. CLI 与 MCP 高层操作
4. Web 控制台基础与 Search Lab
5. 异步索引框架
6. SQLite + Chroma 语义 sidecar 集成
