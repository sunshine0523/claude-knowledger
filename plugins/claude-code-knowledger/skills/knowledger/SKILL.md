---
name: knowledger
description: Use this skill when the user asks about project-specific facts, prior decisions, saved references, durable notes, knowledge bases, knowledger, or information that may already be stored in a configured Knowledger knowledge base. Also use it when the user explicitly asks to remember, save, capture, or store durable knowledge.
version: 1.0.0
---

# Knowledger

Knowledger is a local-first knowledge retrieval and capture system exposed through MCP tools. Use it to search configured knowledge bases before answering questions that may depend on saved project context, prior decisions, external references, or reusable notes.

## Available MCP Tools

Use the MCP server named `knowledger`. It exposes these tools:

- `list_knowledge_bases` — list configured knowledge bases before choosing where to search or write.
- `search_knowledge` — search saved knowledge by query, optional knowledge base IDs, limit, and search mode.
- `get_knowledge_item` — fetch a full item when a search result needs more context.
- `add_knowledge_item` — save durable knowledge when the user explicitly asks or clearly confirms capture.

## Search Before Answering

Search Knowledger before answering when the user asks about:

- Project-specific facts, decisions, conventions, incidents, or historical context.
- Prior research, saved references, notes, or documents.
- Information that may have been captured in a knowledge base instead of the repository.
- Whether something is already known, remembered, documented, or saved.
- Knowledger contents, configured knowledge bases, or stored knowledge items.

Use `search_knowledge` with a concise query that includes the user's key terms. If the user names a specific knowledge base, pass that knowledge base ID. If search results are partial or ambiguous, use `get_knowledge_item` before relying on them.

## Avoid Unnecessary Searches

Do not search Knowledger for ordinary coding tasks when the answer should come from the current repository, open files, test output, or the user's message. Do not search only because the word "knowledge" appears in unrelated prose.

## Capture Durable Knowledge

Suggest or perform capture when the user provides durable information such as:

- A project decision, convention, or reusable note.
- A stable external reference and why it matters.
- A fact that should influence future Claude sessions.
- Explicit instructions like "remember this", "save this", or "add this to knowledger".

Before calling `add_knowledge_item`, make sure the target knowledge base is clear. If it is not clear, call `list_knowledge_bases` and ask the user which knowledge base to use. When the user explicitly requests capture and the destination is clear, write without an extra confirmation.

## Do Not Capture

Never store:

- Secrets, credentials, private tokens, API keys, passwords, or sensitive auth material.
- One-off task state, temporary progress, logs, stack traces, or command output that is not durable knowledge.
- Information already derivable from the repository, tests, or git history.
- Personal data unless the user explicitly asks and it is appropriate for the configured knowledge base.

## Using Results

Treat Knowledger results as supporting context, not absolute truth. If retrieved knowledge conflicts with the current repository or current user instructions, trust the current source and mention the conflict briefly.
