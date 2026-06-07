document.documentElement.dataset.knowledger = "ready";

function showKBMessage(message, isError) {
  const el = document.querySelector("#kb-message");
  if (!el) return;
  showMessage(el, message, isError);
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

function tagsFromInput(value) {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function appendTextCell(row, value, asCode) {
  const cell = document.createElement("td");
  if (asCode) {
    const code = document.createElement("code");
    code.textContent = value == null || value === "" ? "—" : String(value);
    cell.appendChild(code);
  } else {
    cell.textContent = value == null || value === "" ? "—" : String(value);
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

const createForm = document.querySelector("#kb-create-form");
if (createForm) {
  createForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const payload = {
      id: data.get("id") || "",
      name: data.get("name") || "",
      store_type: data.get("store_type") || "",
      path: data.get("path") || "",
      enabled: data.get("enabled") === "on",
      tags: tagsFromInput(data.get("tags") || ""),
    };

    try {
      const response = await fetch("/api/kbs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      await parseAPIResponse(response);
      showKBMessage("Knowledge base created.", false);
      window.location.reload();
    } catch (error) {
      showKBMessage(error.message, true);
    }
  });
}

document.querySelectorAll(".kb-delete").forEach((button) => {
  button.addEventListener("click", async () => {
    const id = button.dataset.kbId;
    if (!id) return;
    if (!window.confirm(`Delete runtime knowledge base "${id}"? Stored data will not be deleted.`)) {
      return;
    }
    try {
      const response = await fetch(`/api/kbs/${encodeURIComponent(id)}`, { method: "DELETE" });
      await parseAPIResponse(response);
      showKBMessage("Knowledge base deleted.", false);
      window.location.reload();
    } catch (error) {
      showKBMessage(error.message, true);
    }
  });
});

async function loadDashboard() {
  const message = document.querySelector("#dashboard-message");
  try {
    hideMessage(message);
    const response = await fetch("/api/dashboard");
    const payload = await parseAPIResponse(response);
    renderDashboard(payload.data);
  } catch (error) {
    showMessage(message, `Failed to load dashboard: ${error.message}`, true);
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
  renderKnowledgeBases(data.knowledge_bases || []);
  renderReadiness(data.readiness, data.indexing);
  renderStatus("#failures-status", data.failures);
}

function setText(selector, value) {
  const el = document.querySelector(selector);
  if (!el) return;
  el.textContent = value == null ? "0" : String(value);
}

function renderStoreTypes(storeTypes) {
  const el = document.querySelector("#store-types");
  if (!el) return;
  el.replaceChildren();
  const entries = Object.entries(storeTypes).sort(([a], [b]) => a.localeCompare(b));
  if (entries.length === 0) {
    const empty = document.createElement("span");
    empty.className = "muted";
    empty.textContent = "No store types.";
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

function renderKnowledgeBases(rows) {
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
    appendTextCell(row, kb.enabled, false);
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
  el.textContent = `${searchable} searchable KBs; ${lexical} lexical; ${semantic} semantic configured.${note}`;
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
        button.textContent = "Searching...";
      }
      resetSearchResults("Searching...");
      const response = await fetch("/api/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const result = await parseAPIResponse(response);
      renderSearchResults(result);
    } catch (error) {
      resetSearchResults("Search failed. Fix the request and try again.");
      showMessage(message, error.message, true);
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = "Search";
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
    search_mode: data.get("search_mode") || "",
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
    empty.textContent = hits.length === 0 ? "No hits found." : "";
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
    ["Query", data.query || ""],
    ["Limit", data.limit == null ? "" : data.limit],
    ["KB IDs", data.kb_ids && data.kb_ids.length > 0 ? data.kb_ids.join(", ") : "all enabled"],
    ["Search Mode", data.search_mode || "auto"],
    ["Hit Count", meta.hit_count == null ? 0 : meta.hit_count],
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
    item.textContent = "No warnings.";
    el.appendChild(item);
    return;
  }
  warnings.forEach((warning) => {
    const item = document.createElement("li");
    item.textContent = warning;
    el.appendChild(item);
  });
}

function formatScore(score) {
  const value = Number(score);
  if (!Number.isFinite(value)) return "—";
  return value.toFixed(3);
}

const dashboardRoot = document.querySelector("#dashboard-root");
if (dashboardRoot) {
  loadDashboard();
}

const searchForm = document.querySelector("#search-form");
if (searchForm) {
  setupSearchLab(searchForm);
}
