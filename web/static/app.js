document.documentElement.dataset.knowledger = "ready";

function showKBMessage(message, isError) {
  const el = document.querySelector("#kb-message");
  if (!el) return;
  el.hidden = false;
  el.textContent = message;
  el.className = isError ? "message error" : "message success";
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
