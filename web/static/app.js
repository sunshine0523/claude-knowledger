document.documentElement.dataset.knowledger = "ready";

const LANG_KEY = "knowledger.lang";
const DEFAULT_LANG = navigator.language && navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en";

const projectMode = { inProject: false, projectRoot: "", loaded: false };

async function loadProjectMode() {
  try {
    const res = await fetch("/api/project");
    const json = await res.json();
    if (json && json.success && json.data) {
      projectMode.inProject = !!json.data.in_project;
      projectMode.projectRoot = json.data.project_root || "";
    }
  } catch (_) {
    // fallback: not in project mode
  } finally {
    projectMode.loaded = true;
  }
  return projectMode;
}

const translations = {
  en: {
    "nav.dashboard": "Dashboard",
    "nav.kbs": "Knowledge Bases",
    "nav.knowledge": "Knowledge",
    "nav.searchLab": "Search Lab",
    "nav.debug": "Debug",
    "nav.language": "Language",
    "common.actions": "Actions",
    "common.defaultMode": "Default Mode",
    "common.delete": "Delete",
    "common.enabled": "Enabled",
    "common.id": "ID",
    "common.loading": "Loading...",
    "common.name": "Name",
    "common.path": "Path",
    "common.refresh": "Refresh",
    "common.scope": "Scope",
    "common.source": "Source",
    "common.status": "Status",
    "common.tags": "Tags",
    "common.title": "Title",
    "common.type": "Type",
    "common.updated": "Updated",
    "common.yes": "yes",
    "common.no": "no",
    "common.allEnabled": "all enabled",
    "dashboard.title": "Knowledger Dashboard",
    "dashboard.description": "View knowledge base totals, store type distribution, and current search/index configuration.",
    "dashboard.totalKBs": "Total KBs",
    "dashboard.enabled": "Enabled",
    "dashboard.disabled": "Disabled",
    "dashboard.runtime": "Runtime",
    "dashboard.static": "Static",
    "dashboard.storeTypes": "Store Types",
    "dashboard.searchReadiness": "Search Readiness",
    "dashboard.indexingNotes": "Indexing Notes",
    "dashboard.noStoreTypes": "No store types.",
    "dashboard.readiness": "{searchable} searchable KBs; {lexical} lexical; {semantic} semantic configured.{note}",
    "dashboard.loadFailed": "Failed to load dashboard: {message}",
    "kbs.title": "Knowledge Bases",
    "kbs.description": "Manage knowledge base configuration, enabled state, and backend capability. Knowledge bases created in Web are written to the runtime registry; static configuration knowledge bases are read-only.",
    "kbs.addTitle": "Add knowledge base",
    "kbs.backend": "Backend",
    "kbs.scopeAuto": "Auto (project if available, else global)",
    "kbs.scopeProject": "Project",
    "kbs.scopeGlobal": "Global",
    "kbs.projectModeBanner": "Project mode: {root}",
    "kbs.storageLocation": "Storage Location",
    "kbs.addButton": "Add Knowledge Base",
    "kbs.pathHint": "text backend uses a directory path; sqlite backend uses a database path.",
    "kbs.currentTitle": "Current knowledge bases",
    "kbs.empty": "No knowledge bases configured.",
    "kbs.knowledgeCount": "Knowledge Count",
    "kbs.static": "Static",
    "kbs.created": "Knowledge base created.",
    "kbs.deleted": "Knowledge base deleted.",
    "kbs.deleteConfirm": "Delete runtime knowledge base \"{id}\"? Stored data will not be deleted.",
    "knowledge.title": "Knowledge Management",
    "knowledge.description": "Select a knowledge base, inspect its knowledge content, and delete items that are no longer needed.",
    "knowledge.selectKB": "Knowledge Base",
    "knowledge.selectPrompt": "Select a knowledge base to load items.",
    "knowledge.noKBs": "No knowledge bases available.",
    "knowledge.empty": "No knowledge items in this knowledge base.",
    "knowledge.contentTitle": "Knowledge Content",
    "knowledge.contentEmpty": "Select an item to view its content.",
    "knowledge.deleted": "Knowledge item deleted.",
    "knowledge.deleteConfirm": "Delete knowledge item \"{id}\"? This deletes the stored item.",
    "knowledge.loadFailed": "Failed to load knowledge items: {message}",
    "knowledge.view": "View",
    "search.title": "Search Lab",
    "search.description": "Inspect aggregate search requests, hits, score, match mode, and backend. Current sqlite/text backends return lexical hits; semantic/hybrid requests show the actual execution path through warnings or match mode.",
    "search.query": "Query",
    "search.limit": "Limit",
    "search.kbIDs": "KB IDs",
    "search.mode": "Search Mode",
    "search.submit": "Search",
    "search.searching": "Searching...",
    "search.requestSummary": "Request Summary",
    "search.runPrompt": "Run a search to see request details.",
    "search.warnings": "Warnings",
    "search.noWarnings": "No warnings.",
    "search.results": "Results",
    "search.resultPrompt": "Run a search to see results.",
    "search.searchFailed": "Search failed. Fix the request and try again.",
    "search.noHits": "No hits found.",
    "search.queryLabel": "Query",
    "search.limitLabel": "Limit",
    "search.kbIDsLabel": "KB IDs",
    "search.modeLabel": "Search Mode",
    "search.hitCountLabel": "Hit Count",
    "search.kb": "KB",
    "search.score": "Score",
    "search.matchMode": "Match Mode",
    "search.backend": "Backend",
    "search.locator": "Locator",
    "search.snippet": "Snippet",
    "debug.title": "MCP / CLI Debug",
    "debug.description": "View debug requests, responses, warnings, and errors."
  },
  "zh-CN": {
    "nav.dashboard": "仪表盘",
    "nav.kbs": "知识库",
    "nav.knowledge": "知识管理",
    "nav.searchLab": "搜索实验室",
    "nav.debug": "调试",
    "nav.language": "语言",
    "common.actions": "操作",
    "common.defaultMode": "默认模式",
    "common.delete": "删除",
    "common.enabled": "启用",
    "common.id": "ID",
    "common.loading": "加载中...",
    "common.name": "名称",
    "common.path": "路径",
    "common.refresh": "刷新",
    "common.scope": "范围",
    "common.source": "来源",
    "common.status": "状态",
    "common.tags": "标签",
    "common.title": "标题",
    "common.type": "类型",
    "common.updated": "更新时间",
    "common.yes": "是",
    "common.no": "否",
    "common.allEnabled": "全部启用知识库",
    "dashboard.title": "Knowledger 仪表盘",
    "dashboard.description": "查看知识库总览、store type 分布和当前搜索/索引配置状态。",
    "dashboard.totalKBs": "知识库总数",
    "dashboard.enabled": "已启用",
    "dashboard.disabled": "未启用",
    "dashboard.runtime": "运行时",
    "dashboard.static": "静态",
    "dashboard.storeTypes": "存储类型",
    "dashboard.searchReadiness": "搜索就绪状态",
    "dashboard.indexingNotes": "索引说明",
    "dashboard.noStoreTypes": "没有存储类型。",
    "dashboard.readiness": "{searchable} 个可搜索知识库；{lexical} 个 lexical；{semantic} 个 semantic 已配置。{note}",
    "dashboard.loadFailed": "仪表盘加载失败：{message}",
    "kbs.title": "知识库管理",
    "kbs.description": "管理知识库配置、启用状态和后端能力。Web 创建的知识库会写入运行时注册表；静态配置文件中的知识库只读展示。",
    "kbs.addTitle": "新增知识库",
    "kbs.backend": "后端",
    "kbs.scopeAuto": "自动（有项目时为 project，否则为 global）",
    "kbs.scopeProject": "项目",
    "kbs.scopeGlobal": "全局",
    "kbs.projectModeBanner": "项目模式：{root}",
    "kbs.storageLocation": "存储位置",
    "kbs.addButton": "添加知识库",
    "kbs.pathHint": "text 后端使用目录路径；sqlite 后端使用数据库路径。",
    "kbs.currentTitle": "当前知识库",
    "kbs.empty": "暂无知识库。",
    "kbs.knowledgeCount": "知识数量",
    "kbs.static": "静态",
    "kbs.created": "知识库已创建。",
    "kbs.deleted": "知识库已删除。",
    "kbs.deleteConfirm": "删除运行时知识库“{id}”？已存储的数据不会被删除。",
    "knowledge.title": "知识管理",
    "knowledge.description": "选择一个知识库，查看其中的知识内容，并删除不再需要的知识。",
    "knowledge.selectKB": "知识库",
    "knowledge.selectPrompt": "请选择一个知识库来加载知识。",
    "knowledge.noKBs": "没有可用知识库。",
    "knowledge.empty": "该知识库暂无知识。",
    "knowledge.contentTitle": "知识内容",
    "knowledge.contentEmpty": "选择一条知识来查看内容。",
    "knowledge.deleted": "知识已删除。",
    "knowledge.deleteConfirm": "删除知识“{id}”？这会删除已存储的知识条目。",
    "knowledge.loadFailed": "知识加载失败：{message}",
    "knowledge.view": "查看",
    "search.title": "搜索实验室",
    "search.description": "观察聚合搜索请求、命中结果、score、match mode 和 backend。当前 sqlite/text backend 返回 lexical hits；semantic/hybrid 会通过 warning 或 match mode 显示实际执行路径。",
    "search.query": "查询",
    "search.limit": "数量限制",
    "search.kbIDs": "知识库 ID",
    "search.mode": "搜索模式",
    "search.submit": "搜索",
    "search.searching": "搜索中...",
    "search.requestSummary": "请求摘要",
    "search.runPrompt": "运行一次搜索后查看请求详情。",
    "search.warnings": "警告",
    "search.noWarnings": "没有警告。",
    "search.results": "结果",
    "search.resultPrompt": "运行一次搜索后查看结果。",
    "search.searchFailed": "搜索失败。请修正请求后重试。",
    "search.noHits": "未找到结果。",
    "search.queryLabel": "查询",
    "search.limitLabel": "数量限制",
    "search.kbIDsLabel": "知识库 ID",
    "search.modeLabel": "搜索模式",
    "search.hitCountLabel": "命中数量",
    "search.kb": "知识库",
    "search.score": "分数",
    "search.matchMode": "匹配模式",
    "search.backend": "后端",
    "search.locator": "位置",
    "search.snippet": "片段",
    "debug.title": "MCP / CLI 调试",
    "debug.description": "查看调试请求、响应、warning 和 error。"
  }
};

let currentLanguage = localStorage.getItem(LANG_KEY) || DEFAULT_LANG;
if (!translations[currentLanguage]) currentLanguage = "en";

function t(key, params) {
  let value = (translations[currentLanguage] && translations[currentLanguage][key]) || translations.en[key] || key;
  if (!params) return value;
  Object.entries(params).forEach(([name, replacement]) => {
    value = value.replaceAll(`{${name}}`, String(replacement));
  });
  return value;
}

function setLanguage(language) {
  if (!translations[language]) return;
  currentLanguage = language;
  localStorage.setItem(LANG_KEY, language);
  applyTranslations();
  window.location.reload();
}

function applyTranslations() {
  document.documentElement.lang = currentLanguage;
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  const select = document.querySelector("#language-select");
  if (select) select.value = currentLanguage;
}

function initLanguageSwitcher() {
  const select = document.querySelector("#language-select");
  if (select) {
    select.value = currentLanguage;
    select.addEventListener("change", () => setLanguage(select.value));
  }
  applyTranslations();
}

function showMessage(el, message, isError) {
  if (!el) return;
  el.hidden = false;
  el.textContent = message;
  el.className = isError ? "message error" : "message success";
}

function hideMessage(el) {
  if (!el) return;
  el.hidden = true;
  el.textContent = "";
}

function firstAPIError(payload) {
  if (payload && payload.errors && payload.errors.length > 0) {
    return payload.errors[0].message;
  }
  return "Request failed";
}

async function parseAPIResponse(response) {
  const payload = await response.json().catch(() => null);
  if (!response.ok || !payload || !payload.success) {
    throw new Error(firstAPIError(payload));
  }
  return payload;
}

async function apiGet(path) {
  return parseAPIResponse(await fetch(path));
}

async function apiPost(path, payload) {
  return parseAPIResponse(await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  }));
}

async function apiDelete(path) {
  return parseAPIResponse(await fetch(path, { method: "DELETE" }));
}

function tagsFromInput(value) {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function appendTextCell(row, value, asCode) {
  const cell = document.createElement("td");
  const text = value == null || value === "" ? "—" : String(value);
  if (asCode) {
    const code = document.createElement("code");
    code.textContent = text;
    cell.appendChild(code);
  } else {
    cell.textContent = text;
  }
  row.appendChild(cell);
  return cell;
}

function appendTagsCell(row, tags) {
  const cell = document.createElement("td");
  if (!tags || tags.length === 0) {
    cell.textContent = "—";
  } else {
    tags.forEach((tag) => {
      const span = document.createElement("span");
      span.className = "tag";
      span.textContent = tag;
      cell.appendChild(span);
    });
  }
  row.appendChild(cell);
}

function formatBoolean(value) {
  return value ? t("common.yes") : t("common.no");
}

function formatScore(score) {
  const value = Number(score);
  if (!Number.isFinite(value)) return "—";
  return value.toFixed(3);
}

function formatDate(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(currentLanguage);
}

function setText(selector, value) {
  const el = document.querySelector(selector);
  if (!el) return;
  el.textContent = value == null ? "0" : String(value);
}

function showKBMessage(message, isError) {
  showMessage(document.querySelector("#kb-message"), message, isError);
}

async function loadKBs() {
  const payload = await apiGet("/api/kbs");
  return (payload.data && payload.data.knowledge_bases) || [];
}

function setupCreateKBForm(form) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const scopeValue = (data.get("scope") || "").toString().trim();
    const payload = {
      id: data.get("id") || "",
      name: data.get("name") || "",
      store_type: data.get("store_type") || "",
      path: data.get("path") || "",
      enabled: data.get("enabled") === "on",
      semantic_enabled: data.get("semantic_enabled") === "on",
      tags: tagsFromInput(data.get("tags") || "")
    };
    if (scopeValue) payload.scope = scopeValue;

    try {
      await apiPost("/api/kbs", payload);
      showKBMessage(t("kbs.created"), false);
      form.reset();
      applyProjectModeToKBForm();
      await refreshKBPage();
    } catch (error) {
      showKBMessage(error.message, true);
    }
  });
}

function applyProjectModeToKBForm() {
  const sel = document.getElementById("kb-create-scope");
  if (!sel) return;
  if (projectMode.inProject) {
    sel.value = "project";
  }
}

function applyProjectModeBanner() {
  const banner = document.getElementById("project-mode-banner");
  if (!banner) return;
  if (projectMode.inProject) {
    banner.hidden = false;
    banner.textContent = t("kbs.projectModeBanner", { root: projectMode.projectRoot });
  } else {
    banner.hidden = true;
    banner.textContent = "";
  }
}

async function deleteKB(id) {
  if (!window.confirm(t("kbs.deleteConfirm", { id }))) return;
  try {
    await apiDelete(`/api/kbs/${encodeURIComponent(id)}`);
    showKBMessage(t("kbs.deleted"), false);
    await refreshKBPage();
  } catch (error) {
    showKBMessage(error.message, true);
  }
}

function renderKBs(rows) {
  const body = document.querySelector("#kb-table-body");
  const empty = document.querySelector("#kb-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) empty.hidden = rows.length > 0;

  rows.forEach((kb) => {
    const row = document.createElement("tr");
    row.dataset.kbId = kb.id;
    row.dataset.kbSource = kb.source;
    appendTextCell(row, kb.id, true);
    appendTextCell(row, kb.name, false);
    appendTextCell(row, kb.store_type, false);
    appendTextCell(row, kb.path, true);
    appendTextCell(row, formatBoolean(kb.enabled), false);
    appendTextCell(row, kb.source, false);
    appendTagsCell(row, kb.tags || []);
    appendTextCell(row, kb.item_count == null ? 0 : kb.item_count, false);

    const actionCell = document.createElement("td");
    const button = document.createElement("button");
    button.type = "button";
    if (kb.deletable) {
      button.className = "danger kb-delete";
      button.dataset.kbId = kb.id;
      button.textContent = t("common.delete");
      button.addEventListener("click", () => deleteKB(kb.id));
    } else {
      button.disabled = true;
      button.title = "Static knowledge bases are read-only in Web";
      button.textContent = t("kbs.static");
    }
    actionCell.appendChild(button);
    row.appendChild(actionCell);
    body.appendChild(row);
  });
}

async function refreshKBPage() {
  const rows = await loadKBs();
  renderKBs(rows);
}

function initKBPage() {
  const form = document.querySelector("#kb-create-form");
  const body = document.querySelector("#kb-table-body");
  if (!form && !body) return;
  if (form) setupCreateKBForm(form);
  loadProjectMode().then(() => {
    applyProjectModeBanner();
    applyProjectModeToKBForm();
  });
  refreshKBPage().catch((error) => showKBMessage(error.message, true));
}

async function loadDashboard() {
  const message = document.querySelector("#dashboard-message");
  try {
    hideMessage(message);
    const payload = await apiGet("/api/dashboard");
    renderDashboard(payload.data);
  } catch (error) {
    showMessage(message, t("dashboard.loadFailed", { message: error.message }), true);
  }
}

function renderDashboard(data) {
  const summary = data.summary || {};
  setText("#stat-total-kbs", summary.total_kbs);
  setText("#stat-enabled-kbs", summary.enabled_kbs);
  setText("#stat-disabled-kbs", summary.disabled_kbs);
  setText("#stat-runtime-kbs", summary.runtime_kbs);
  setText("#stat-static-kbs", summary.static_kbs);
  renderStoreTypes(summary.store_types || {});
  renderDashboardKnowledgeBases(data.knowledge_bases || []);
  renderReadiness(data.readiness, data.indexing);
  renderStatus("#failures-status", data.failures);
}

function renderStoreTypes(storeTypes) {
  const el = document.querySelector("#store-types");
  if (!el) return;
  el.replaceChildren();
  const entries = Object.entries(storeTypes).sort(([a], [b]) => a.localeCompare(b));
  if (entries.length === 0) {
    const empty = document.createElement("span");
    empty.className = "muted";
    empty.textContent = t("dashboard.noStoreTypes");
    el.appendChild(empty);
    return;
  }
  entries.forEach(([storeType, count]) => {
    const tag = document.createElement("span");
    tag.className = "tag";
    tag.textContent = `${storeType}: ${count}`;
    el.appendChild(tag);
  });
}

function renderDashboardKnowledgeBases(rows) {
  const body = document.querySelector("#dashboard-kbs-body");
  const empty = document.querySelector("#dashboard-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) empty.hidden = rows.length > 0;

  rows.forEach((kb) => {
    const row = document.createElement("tr");
    appendTextCell(row, kb.id, true);
    appendTextCell(row, kb.name, false);
    appendTextCell(row, kb.store_type, false);
    appendTextCell(row, kb.path, true);
    appendTextCell(row, formatBoolean(kb.enabled), false);
    appendTextCell(row, kb.source, false);
    appendTextCell(row, kb.default_search_mode, false);
    appendTagsCell(row, kb.tags || []);
    body.appendChild(row);
  });
}

function renderReadiness(readiness, fallbackStatus) {
  const el = document.querySelector("#indexing-status");
  if (!el) return;
  if (!readiness) {
    renderStatus("#indexing-status", fallbackStatus);
    return;
  }

  const searchable = readiness.searchable_kbs == null ? 0 : readiness.searchable_kbs;
  const lexical = readiness.lexical_configured_kbs == null ? 0 : readiness.lexical_configured_kbs;
  const semantic = readiness.semantic_configured_kbs == null ? 0 : readiness.semantic_configured_kbs;
  const notes = readiness.notes || [];
  const note = notes.length > 0 ? ` ${notes.join(" ")}` : "";
  el.textContent = t("dashboard.readiness", { searchable, lexical, semantic, note });
}

function renderStatus(selector, status) {
  const el = document.querySelector(selector);
  if (!el) return;
  if (!status) {
    el.textContent = "unsupported";
    return;
  }
  el.textContent = `${status.state}: ${status.message}`;
}

function setupSearchLab(form) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const message = document.querySelector("#search-message");
    const button = document.querySelector("#search-submit");
    const payload = searchPayloadFromForm(form);

    try {
      hideMessage(message);
      if (button) {
        button.disabled = true;
        button.textContent = t("search.searching");
      }
      resetSearchResults(t("search.searching"));
      const result = await apiPost("/api/search", payload);
      renderSearchResults(result);
    } catch (error) {
      resetSearchResults(t("search.searchFailed"));
      showMessage(message, error.message, true);
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = t("search.submit");
      }
    }
  });
}

function searchPayloadFromForm(form) {
  const data = new FormData(form);
  const limitValue = data.get("limit");
  return {
    query: data.get("query") || "",
    limit: limitValue === "" ? undefined : Number(limitValue),
    kb_ids: tagsFromInput(data.get("kb_ids") || ""),
    search_mode: data.get("search_mode") || ""
  };
}

function resetSearchResults(message) {
  renderSearchSummary({ query: "", limit: "", kb_ids: [], search_mode: "" }, { hit_count: 0 });
  renderSearchWarnings([]);

  const body = document.querySelector("#search-results-body");
  if (body) body.replaceChildren();

  const empty = document.querySelector("#search-empty");
  if (empty) {
    empty.hidden = false;
    empty.textContent = message;
  }
}

function renderSearchResults(payload) {
  const data = payload.data || {};
  const hits = data.hits || [];
  renderSearchSummary(data, payload.meta || {});
  renderSearchWarnings(payload.warnings || []);

  const body = document.querySelector("#search-results-body");
  const empty = document.querySelector("#search-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) {
    empty.hidden = hits.length > 0;
    empty.textContent = hits.length === 0 ? t("search.noHits") : "";
  }

  hits.forEach((hit) => {
    const row = document.createElement("tr");
    appendTextCell(row, hit.title, false);
    appendTextCell(row, hit.kb_id, true);
    appendTextCell(row, formatScore(hit.score), false);
    appendTextCell(row, hit.match_mode, false);
    appendTextCell(row, hit.source_backend, false);
    appendTextCell(row, hit.locator, true);
    appendTextCell(row, hit.snippet || hit.content_preview, false);
    body.appendChild(row);
  });
}

function renderSearchSummary(data, meta) {
  const el = document.querySelector("#search-summary");
  if (!el) return;
  el.replaceChildren();
  const rows = [
    [t("search.queryLabel"), data.query || ""],
    [t("search.limitLabel"), data.limit == null ? "" : data.limit],
    [t("search.kbIDsLabel"), data.kb_ids && data.kb_ids.length > 0 ? data.kb_ids.join(", ") : t("common.allEnabled")],
    [t("search.modeLabel"), data.search_mode || "auto"],
    [t("search.hitCountLabel"), meta.hit_count == null ? 0 : meta.hit_count]
  ];
  rows.forEach(([key, value]) => {
    const dt = document.createElement("dt");
    dt.textContent = key;
    const dd = document.createElement("dd");
    dd.textContent = String(value);
    el.appendChild(dt);
    el.appendChild(dd);
  });
}

function renderSearchWarnings(warnings) {
  const el = document.querySelector("#search-warnings");
  if (!el) return;
  el.replaceChildren();
  if (warnings.length === 0) {
    const item = document.createElement("li");
    item.className = "muted";
    item.textContent = t("search.noWarnings");
    el.appendChild(item);
    return;
  }
  warnings.forEach((warning) => {
    const item = document.createElement("li");
    item.textContent = warning;
    el.appendChild(item);
  });
}

function initSearchLab() {
  const searchForm = document.querySelector("#search-form");
  if (searchForm) setupSearchLab(searchForm);
}

let knowledgeItems = [];

async function initKnowledgePage() {
  const root = document.querySelector("#knowledge-root");
  if (!root) return;
  const select = document.querySelector("#knowledge-kb-select");
  const refresh = document.querySelector("#knowledge-refresh");
  if (!select) return;

  refresh?.addEventListener("click", () => loadSelectedKnowledgeItems());
  select.addEventListener("change", () => loadSelectedKnowledgeItems());

  try {
    const kbs = await loadKBs();
    renderKnowledgeKBOptions(kbs);
    if (kbs.length > 0) await loadSelectedKnowledgeItems();
  } catch (error) {
    showMessage(document.querySelector("#knowledge-message"), error.message, true);
  }
}

function renderKnowledgeKBOptions(kbs) {
  const select = document.querySelector("#knowledge-kb-select");
  const empty = document.querySelector("#knowledge-empty");
  if (!select) return;
  select.replaceChildren();
  if (kbs.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = t("knowledge.noKBs");
    select.appendChild(option);
    if (empty) {
      empty.hidden = false;
      empty.textContent = t("knowledge.noKBs");
    }
    return;
  }
  kbs.forEach((kb) => {
    const option = document.createElement("option");
    option.value = kb.id;
    option.textContent = `${kb.name || kb.id} (${kb.id})`;
    select.appendChild(option);
  });
}

async function loadSelectedKnowledgeItems() {
  const select = document.querySelector("#knowledge-kb-select");
  if (!select || !select.value) return;
  const message = document.querySelector("#knowledge-message");
  try {
    hideMessage(message);
    const payload = await apiGet(`/api/kbs/${encodeURIComponent(select.value)}/items`);
    knowledgeItems = (payload.data && payload.data.items) || [];
    renderKnowledgeItems(knowledgeItems);
    clearKnowledgeContent();
  } catch (error) {
    showMessage(message, t("knowledge.loadFailed", { message: error.message }), true);
  }
}

function renderKnowledgeItems(items) {
  const body = document.querySelector("#knowledge-items-body");
  const empty = document.querySelector("#knowledge-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) {
    empty.hidden = items.length > 0;
    empty.textContent = items.length === 0 ? t("knowledge.empty") : "";
  }

  items.forEach((item) => {
    const row = document.createElement("tr");
    appendTextCell(row, item.id, true);
    appendTextCell(row, item.title, false);
    appendTextCell(row, item.type, false);
    appendTagsCell(row, item.tags || []);
    appendTextCell(row, formatDate(item.updated_at || item.created_at), false);

    const actionCell = document.createElement("td");
    const viewButton = document.createElement("button");
    viewButton.type = "button";
    viewButton.textContent = t("knowledge.view");
    viewButton.addEventListener("click", () => showKnowledgeContent(item));
    actionCell.appendChild(viewButton);

    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.className = "danger inline-action";
    deleteButton.textContent = t("common.delete");
    deleteButton.addEventListener("click", () => deleteKnowledgeItem(item));
    actionCell.appendChild(deleteButton);

    row.appendChild(actionCell);
    body.appendChild(row);
  });
}

function showKnowledgeContent(item) {
  const title = document.querySelector("#knowledge-content-title");
  const content = document.querySelector("#knowledge-content");
  if (title) title.textContent = item.title || item.id;
  if (content) content.textContent = item.content || "";
}

function clearKnowledgeContent() {
  const title = document.querySelector("#knowledge-content-title");
  const content = document.querySelector("#knowledge-content");
  if (title) title.textContent = t("knowledge.contentEmpty");
  if (content) content.textContent = "";
}

async function deleteKnowledgeItem(item) {
  const select = document.querySelector("#knowledge-kb-select");
  if (!select || !select.value || !item || !item.id) return;
  if (!window.confirm(t("knowledge.deleteConfirm", { id: item.id }))) return;
  try {
    await apiDelete(`/api/kbs/${encodeURIComponent(select.value)}/items/${encodeURIComponent(item.id)}`);
    showMessage(document.querySelector("#knowledge-message"), t("knowledge.deleted"), false);
    await loadSelectedKnowledgeItems();
  } catch (error) {
    showMessage(document.querySelector("#knowledge-message"), error.message, true);
  }
}

initLanguageSwitcher();
initKBPage();
initSearchLab();
initKnowledgePage();

const dashboardRoot = document.querySelector("#dashboard-root");
if (dashboardRoot) loadDashboard();
