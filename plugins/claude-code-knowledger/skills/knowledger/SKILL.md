---
name: knowledger
description: MUST invoke BEFORE answering any question ("how do I", "what is", "how does X work", "what's our convention"), writing code, writing a design/proposal/plan, debugging, or making a technical recommendation — even if the answer seems obvious. Search Knowledger first; project decisions and conventions often live here and nowhere else. MUST also invoke for capture when the user says save/remember/capture/记一下/保存到知识库. Skip ONLY for pure greetings/acknowledgements, trivial single-line typo fixes, and pure git/file operations with no design decisions involved.
version: 2.0.0
triggers:
  # English — recall / lookup
  - "remember"
  - "do you remember"
  - "recall"
  - "we discussed"
  - "we talked about"
  - "last time"
  - "previously"
  - "as I mentioned"
  - "mentioned before"
  - "did we save"
  - "is there a note"
  - "what did we decide"
  - "from our last session"
  # English — capture
  - "save this"
  - "save that"
  - "capture this"
  - "note this"
  - "store this"
  - "remember this"
  - "add to knowledge"
  - "save to knowledger"
  # English — meta
  - "knowledger"
  - "knowledge base"
  - "knowledge bases"
  # 中文 — 回忆 / 查询
  - "记得"
  - "还记得"
  - "记不记得"
  - "我们之前"
  - "上次"
  - "之前提过"
  - "之前提到"
  - "之前说过"
  - "之前讨论"
  - "之前的决定"
  - "查一下知识库"
  - "查知识库"
  - "知识库里"
  - "有没有记录"
  - "有没有保存"
  # 中文 — 捕获
  - "记一下"
  - "记下来"
  - "记下"
  - "保存到知识库"
  - "存到知识库"
  - "添加到知识库"
  - "存一下知识库"
  - "归档到知识库"
  # 中文 — meta
  - "知识库"
---

# Knowledger

Knowledger is a local-first knowledge retrieval and capture system exposed through MCP tools. It holds project decisions, conventions, debugging recipes, and domain references — facts that **grep, file reads, and codegraph cannot recover**. Skipping it means answering from generic knowledge while project-specific guidance sits unused.

## The Rule

**BEFORE answering, writing code, designing, or making any technical recommendation — dispatch a subagent to scan all knowledge bases and inject relevant results into your context.**

Even a 1% chance of relevance means you MUST do this. This is not negotiable.

```dot
digraph knowledger_flow {
  "User message received" [shape=doublecircle];
  "Might KB have relevant knowledge?" [shape=diamond];
  "Dispatch KB-scan subagent" [shape=box];
  "Subagent: list all KBs + items" [shape=box];
  "Subagent: compare items vs full context" [shape=box];
  "Any item ≥1% relevant?" [shape=diamond];
  "Inject relevant items into main context" [shape=box];
  "Answer with full KB context" [shape=doublecircle];
  "Answer directly" [shape=doublecircle];

  "User message received" -> "Might KB have relevant knowledge?";
  "Might KB have relevant knowledge?" -> "Dispatch KB-scan subagent" [label="yes, even 1%"];
  "Might KB have relevant knowledge?" -> "Answer directly" [label="definitely not\n(greeting/ack only)"];
  "Dispatch KB-scan subagent" -> "Subagent: list all KBs + items";
  "Subagent: list all KBs + items" -> "Subagent: compare items vs full context";
  "Subagent: compare items vs full context" -> "Any item ≥1% relevant?";
  "Any item ≥1% relevant?" -> "Inject relevant items into main context" [label="yes"];
  "Any item ≥1% relevant?" -> "Answer with full KB context" [label="no relevant items"];
  "Inject relevant items into main context" -> "Answer with full KB context";
}
```

## Red Flags

These thoughts mean STOP — you are rationalizing:

| Thought | Reality |
|---------|---------|
| "I know this from training" | Generic knowledge ≠ project knowledge. Scan KBs first. |
| "The repo will tell me" | Conventions and decisions are often NOT in the repo. Scan. |
| "Simple coding task" | Simple tasks have project-specific conventions. Scan. |
| "Quick question" | Quick questions have saved answers. Scan. |
| "I'll search if I need to later" | You won't. Scan BEFORE answering. |
| "No obvious KB topic" | Weak signal is not zero signal. Scan. |
| "I already know the answer" | The KB may contradict or refine it. Scan. |
| "The user didn't mention KB" | Users never say "check the KB" — that's your job. |
| "This is just a clarification" | Clarifications shape implementation. Scan first. |

## Subagent KB-Scan Protocol

Dispatch a subagent with this exact mission:

```
1. Call list_knowledge_bases — get every configured KB (id, name, scope).
2. For each KB, call list_knowledge_items to get all item ids and titles.
3. Compare every item title + tags against the main agent's full conversation context.
4. For any item with ≥1% relevance to the current task, call get_knowledge_item for full content.
5. Return ALL retrieved full items to the main agent as structured context.
```

The subagent must err on the side of inclusion — a false positive costs one extra item; a false negative loses critical project context.

## Inject and Apply

When the subagent returns results:
- Treat retrieved knowledge as authoritative project context.
- If it conflicts with the repo or user instructions, surface the conflict — don't silently discard either.
- Cite which KB and item the knowledge came from.

## Capture Durable Knowledge

Perform capture when the user provides:
- A project decision, convention, or reusable note.
- A stable external reference and why it matters.
- Explicit capture intent: "remember this", "save this", "记一下", "保存到知识库".

Before `add_knowledge_item`, confirm the target KB. If unclear, call `list_knowledge_bases` and ask.

## Never Capture

- Secrets, credentials, private tokens, API keys.
- One-off task state, temp logs, stack traces, command output.
- Anything already derivable from the repo or git history.

## Skip Only For

- Pure greetings or acknowledgements with zero task content.
- The immediately preceding assistant message already ran the full KB scan for the same topic.
- The user explicitly says "skip knowledger" / "不用查知识库".

Do not narrate the scan to the user — dispatch the subagent silently, then answer.
