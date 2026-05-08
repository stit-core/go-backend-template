# Raw vendor research briefings — 18 parallel streams (May 2026)

Each section is the verbatim output of a research agent that read only vendor-authored or peer-reviewed sources. Citations are inline. The synthesis distilled from these is in `synthesis.md`.

Two of the streams (Anthropic engineering blog, Cognition) reported fabricated `<system-reminder>` content embedded in fetched pages — flagged and ignored.

---

## Stream 1 — Claude Code architecture (Anthropic)

**Skills System** (https://code.claude.com/docs/en/skills.md)

Skills are prompt-based task or knowledge packages stored as `SKILL.md` files with optional supporting files. The architecture distinguishes between model-invoked and user-invoked behaviors: a `description` field makes a skill discoverable to Claude's model; `disable-model-invocation: true` restricts triggering to user commands; `user-invocable: false` hides the `/` command but allows Claude to invoke it. Frontmatter also declares `allowed-tools` (pre-approved tool access during skill execution), `model` overrides, `effort` levels, and `context: fork` to run the skill in an isolated subagent. Dynamic context injection via `` !`command` `` preprocesses shell commands into the prompt before Claude reads it. Skills load on-demand when invoked; their content persists in the context window across turns, with compaction budget: after context pressure, the first 5,000 tokens of each invoked skill stay, with a combined 25,000-token budget across all skills. Skills follow the Agent Skills open standard (https://agentskills.io) with Anthropic extensions. They live at user (`~/.claude/skills/`), project (`.claude/skills/`), or plugin scope, with enterprise overrides. Progressive disclosure: skill names always load (low cost), full content only on use.

**Subagents** (https://code.claude.com/docs/en/subagents.md)

Subagents are isolated agent contexts for delegated work. Each runs in its own window with a custom system prompt, model config, tool set, and independent permissions. When Claude's main conversation would be flooded by exploration, a subagent performs that work and returns a summary. Subagents are declared as markdown files in `.claude/agents/` with a description field. They can preload skills (via a `skills` field), set `permissionMode`, constrain tool access, and maintain their own auto memory.

**Hooks** (https://code.claude.com/docs/en/hooks.md)

Hooks are deterministic event handlers that fire at lifecycle points: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, others. They execute shell commands, HTTP requests, MCP tool calls, or Claude model evals — not to ask the model to do something, but to enforce behavior. A `PreToolUse` hook receives the tool name and input as JSON, returning a decision (`allow`, `deny`, `ask`) or modified input via `hookSpecificOutput`. Exit code 0 = success; exit code 2 = blocking error. Hooks live in `settings.json` at user, project, or local scope. Matchers can target MCP tools with `mcp__<server>__<tool>` syntax.

**Slash Commands** (https://code.claude.com/docs/en/slash-commands.md)

Now merged into the skills system: a `SKILL.md` in `skills/my-command/` creates `/my-command` the same way `.claude/commands/deploy.md` did. Commands accept arguments via `$ARGUMENTS` or named positional. `` !`command` `` shell injection runs preprocessing. `disable-model-invocation: true` makes user-only. Plugin namespacing (`/plugin-name:command`) avoids conflicts.

**CLAUDE.md System** (https://code.claude.com/docs/en/memory.md)

Files are discovered by walking the directory tree from cwd upward; `CLAUDE.md` and `CLAUDE.local.md` in each directory concatenate, with closer-to-cwd content read last. Subdirectory files load on-demand. `.claude/rules/*.md` with `paths` frontmatter loads only when relevant. Files >200 lines hurt adherence. `@path/file` import expands files inline. Managed CLAUDE.md at `/Library/Application Support/ClaudeCode/CLAUDE.md` (macOS) or `/etc/claude-code/CLAUDE.md` (Linux) applies organization-wide. Auto memory at `~/.claude/projects/<project>/memory/MEMORY.md` is machine-local.

**Plan Mode** (https://code.claude.com/docs/en/permission-modes.md)

Plan mode (`default --permission-mode plan`) restricts Claude to reads only. Claude proposes a plan, user approves with `default`/`acceptEdits`/manual. `Shift+Tab` cycles modes; `/plan` prefixes a single message. Design rationale: exploration and implementation are separate cognitive phases.

**MCP Integration** (https://code.claude.com/docs/en/mcp.md)

`claude mcp add --transport <type> <name> <url>`. Transport types: `http` (preferred for remote), `sse` (deprecated), `stdio` (local). Project-level config in `.mcp.json`. MCP tools named `mcp__<server>__<tool>` in permission rules and hooks. No separate permission model.

**Settings Architecture** (https://code.claude.com/docs/en/settings.md)

Four-tier scope (highest to lowest): managed (enterprise), local (`.claude/settings.local.json`), project (`.claude/settings.json`), user (`~/.claude/settings.json`). Permissions: `allow`/`ask`/`deny` arrays. Rules evaluated: deny first, ask, allow. Wildcards/regex supported. Managed cannot be overridden.

**Plugins** (https://code.claude.com/docs/en/plugins.md)

`.claude-plugin/plugin.json` manifest. Skills in `skills/`, agents in `agents/`, hooks in `hooks/hooks.json`, MCP in `.mcp.json`. Skill names namespaced (`/plugin-name:skill-name`). Distributed via marketplaces.

**Claude Agent SDK** (https://platform.claude.com/docs/en/managed-agents/overview)

Separate runtime from Claude Code. Managed agent harness in cloud containers, not a CLI. Long-running **sessions** persist filesystems and history. Sessions reference an **agent** (model + prompt + tools + MCP + skills) and **environment** (container with pre-installed packages). Event-driven via SSE. No interactive permission prompting. Suited for async workflows; Claude Code is for interactive local work.

**Design Rationale**: Skills handle reusable knowledge with on-demand loading; subagents isolate context-heavy work; hooks enforce determinism at fixed points; CLAUDE.md carries persistent guidance; auto memory captures learnings; plan mode separates analysis from execution; settings enforce organization policy; plugins distribute everything. **Progressive disclosure and delegation**: load only what's needed, isolate work when needed, pre-approve or deny rather than asking the model.

---

## Stream 2 — Anthropic engineering blog

### Building Effective Agents (Schluntz & Zhang, Dec 19 2024)
URL: https://www.anthropic.com/research/building-effective-agents

Definitional split:
- **Workflows**: "Systems where LLMs and tools are orchestrated through predefined code paths."
- **Agents**: "Systems where LLMs dynamically direct their own processes and tool usage."

Pattern list:
1. **Augmented LLM** — base building block: LLM + retrieval + tools + memory.
2. **Prompt chaining** — sequential decomposition; trades latency for accuracy.
3. **Routing** — classify input, dispatch to specialized handler.
4. **Parallelization** — *sectioning* (independent subtasks) or *voting* (same task N times).
5. **Orchestrator-workers** — central LLM decomposes, delegates, synthesizes.
6. **Evaluator-optimizer** — generator + critic loop.
7. **Agents** — LLMs in environmental-feedback loop. Open-ended problems with unbounded steps; requires sandboxing.

Rules:
- "Find the simplest solution possible, and only increase complexity when needed."
- "Optimizing single LLM calls with retrieval and in-context examples is usually enough."
- "Tool definitions and specifications should be given just as much prompt engineering attention as your overall prompts."
- Three core principles: **simplicity, transparency, carefully crafted agent-computer interface (ACI)**.

### Multi-Agent Research System (mid-2025)
URL: https://www.anthropic.com/engineering/built-multi-agent-research-system

Architecture: lead agent (Opus) plans → 3-5 parallel subagents (Sonnet) → each searches independently → lead synthesizes → CitationAgent attributes. Plan persisted to **Memory** because "if the context window exceeds 200,000 tokens it will be truncated."

Empirical:
- Multi-agent (Opus lead + Sonnet subagents) beat single-agent Opus by **90.2%** on internal research eval.
- Multi-agent uses **~15× more tokens than chats**.
- On BrowseComp, **token usage explained ~80% of performance variance**.
- Sonnet 3.7 → Sonnet 4 outperformed *doubling the token budget*.
- Improving one tool description: **40% reduction in task completion time**.

When multi-agent helps vs hurts:
- **Helps**: breadth-first queries, problems exceeding one context window, heavy parallelizable tool-use.
- **Hurts**: tasks requiring shared context, dense interdependencies, **most coding work**, real-time coordination.

Parallelism heuristics: simple fact-finding 1 agent / 3-10 calls; comparisons 2-4 subagents / 10-15 calls each; complex research 10+ subagents.

### Effective Context Engineering (Sep 29 2025)
URL: https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents

"LLMs have an 'attention budget' that they draw on when parsing large volumes of context. Every new token introduced depletes this budget by some amount."

Practices:
- **Compaction** — summarize when nearing limits.
- **Structured note-taking / agentic memory** — persistent NOTES.md / memory tools.
- **Sub-agent context isolation** — burn tokens internally, return **1,000-2,000-token summaries**.
- **Just-in-time retrieval** — lightweight identifiers, load through tools at moment of need.

System prompts: "right altitude" — "specific enough to guide behavior… yet flexible enough to provide heuristics rather than brittle if-else logic."

Tool definitions: self-contained, robust, unambiguous, no functional overlap. "If humans can't definitively choose the right tool, neither can agents."

### Claude Code Best Practices
URL: https://code.claude.com/docs/en/best-practices

"Most best practices are based on one constraint: Claude's context window fills up fast."

CLAUDE.md:
- Test for inclusion: "Would removing this cause Claude to make mistakes? If not, cut it."
- "Bloated CLAUDE.md files cause Claude to ignore your actual instructions!"
- For sometimes-relevant workflows, **use a Skill instead**.

Workflow: Explore → Plan → Code → Commit. Plan mode (`Ctrl+G`).

Verification: "the single highest-leverage thing you can do."

Subagents: "Since context is your fundamental constraint, subagents are one of the most powerful tools."

Hooks vs CLAUDE.md: "CLAUDE.md instructions are advisory; hooks are deterministic."

Failure patterns: kitchen-sink session, correction-spiral (after 2 failed corrections, `/clear` and rewrite), over-specified CLAUDE.md, infinite exploration.

### Writing Tools for Agents (Sep 11 2025)
URL: https://www.anthropic.com/engineering/writing-tools-for-agents

"Tools are a new kind of software which reflects a contract between deterministic systems and non-deterministic agents."

Rules:
- **Namespace** by service then resource (`asana_projects_search`).
- **Unambiguous parameter names** (`user_id`, not `user`).
- **Descriptions = onboarding docs**.
- **Errors should be actionable**.
- **Response shape**: prioritize semantic relevance; strip low-signal IDs; offer `response_format` enum.
- **Token efficiency**: pagination, range, filtering. Claude Code caps at **25,000 tokens** by default.

Anti-patterns: 1:1 API wrapping, generic `list_*` tools, overlapping tools, too many tools.

### Code Execution with MCP (Nov 4 2025)
URL: https://www.anthropic.com/engineering/code-execution-with-mcp

Two problems with naive MCP: loading every tool def upfront (~150,000 tokens); intermediate data flows through model twice (50k+ extra tokens).

Code-execution alternative: ~2,000 tokens (98.7% reduction).

Patterns:
- **Progressive disclosure via filesystem** — tools as a file tree.
- **Filter in the execution environment**, not via the model.
- **Control flow as code** — loops/conditionals in sandbox.
- **Privacy-preserving tokenization**.
- **State persistence** in files.

### Agent Skills (Oct 16 2025)
URLs: https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills + https://claude.com/blog/skills

Skill = "a directory containing a SKILL.md file… organized folders of instructions, scripts, and resources."

Three-tier progressive disclosure:
1. **Startup**: `name` and `description` pre-loaded.
2. **On-demand**: SKILL.md loaded when relevant.
3. **Nested**: linked files loaded inside specific scenarios.

Skill vs MCP: Skills teach **procedural knowledge**; MCP servers connect **external tools**.

### Agent SDK (Sep 29 2025)
URL: https://www.anthropic.com/engineering/building-agents-with-the-claude-agent-sdk

"Give your agents a computer, allowing them to work like humans do."

Loop: **gather context → take action → verify work → repeat.**

Component-selection rubric:
- **Tools** — frequent primary actions.
- **Bash & scripts** — flexible work.
- **Code generation** — complex precise operations.
- **Subagents** — parallelization and context isolation.
- **MCP** — pre-built integrations.

### Anthropic's design rules in their own words

1. "Find the simplest solution possible, and only increase complexity when needed."
2. **Workflows ≠ agents.** Predictable steps → workflow. Unbounded path → agent.
3. **Treat context as a finite resource.** Compact, take notes, retrieve just-in-time, isolate in subagents.
4. **Multi-agent is for breadth-first parallel exploration, not coding.** ~15× chat tokens.
5. **Tool definitions deserve as much prompt engineering as your system prompt.**
6. **Build tools for agents, not for humans.** Consolidate workflow tools; cap responses; offer concise/detailed; strip low-signal IDs.
7. **Verification is the single highest-leverage practice.**
8. **CLAUDE.md = always-on; Skills = sometimes; Hooks = never-skip; Subagents = isolation.**
9. **The agentic loop is gather → act → verify → repeat.**
10. **Prefer code execution + progressive-disclosure filesystem over loading every tool definition upfront.**

---

## Stream 3 — Cognition Labs (Devin)

### Walden Yan's anti-multi-agent argument (Jun 12 2025)
URL: https://cognition.ai/blog/dont-build-multi-agents

"libraries such as OpenAI's Swarm and Microsoft's AutoGen actively push concepts which I believe to be the wrong way of building agents."

Failure: "The decision-making ends up being too dispersed and context isn't able to be shared thoroughly enough between the agents." "Running multiple agents in collaboration only results in fragile systems."

Flappy Bird example: "build a Flappy Bird clone" split into background + bird produces inconsistent visuals because subagents "cannot see what the other was doing."

Two principles:
- **Principle 1:** "Share context, and share full agent traces, not just individual messages."
- **Principle 2:** "Actions carry implicit decisions, and conflicting decisions carry bad results."

"I would argue that Principles 1 & 2 are so critical, and so rarely worth violating, that you should by default rule out any agent architectures that don't abide by them."

Recommended: "The simplest way to follow the principles is to just use a single-threaded linear agent." For long-running, summarization: "we introduce a new LLM model whose key purpose is to compress a history of actions & conversation into key details."

"Claude Code is an example of an agent that spawns subtasks. However, it never does work in parallel with the subtask agent."

"At the core of reliability is Context Engineering."

### Multi-Agents: What's Actually Working (Apr 22 2026)
URL: https://cognition.ai/blog/multi-agents-working

"our original observations still hold today for parallel-writer swarms… but we've found a narrower class of patterns that do."

"multi-agent systems work best today when writes stay single-threaded and the additional agents contribute intelligence rather than actions."

Three endorsed patterns:
- **Code review loops** — clean-context reviewer agents. "Devin Review catches an average of 2 bugs per PR, of which roughly 58% are severe."
- **"Smart friend" tool calls** — primary model calls out to stronger model for hard moments.
- **Hierarchical delegation** with central writes.

### Devin architecture

**Devin 2.0** (https://cognition.ai/blog/devin-2): "Spin up multiple parallel Devins, each equipped with its own interactive, cloud-based IDE." Parallelism is across *separate sessions*, not within one task. Interactive Planning: "Devin 2.0 proactively researches your codebase and develops a detailed plan." Devin Search + Devin Wiki (auto-indexes repos every couple hours, creates wikis with architecture diagrams).

**Cloud-agent infrastructure** (https://cognition.ai/blog/what-we-learned-building-cloud-agents): microVMs, not containers — "each workload gets its own kernel, with no shared attack surface." Hypervisor-level snapshots: "snapshotting full machine state at the hypervisor level — memory, process trees, and filesystem." Three load-bearing systems: orchestration, governance/audit, integrations.

**Devin for Terminal** (https://cognition.ai/blog/devin-for-terminal): Local agent with cloud handoff: "hand the session to a cloud agent with its own computer." "Stop worrying about `rm -rf` because the agent works in its own sandbox."

**SWE-1.5 + Cascade harness** (https://cognition.ai/blog/swe-1-5): "End-to-end reinforcement learning (RL) on real task environments using our custom Cascade agent harness." When inference got 10x faster, they "rewrote critical components including lint checking and command execution pipelines."

### Evaluation philosophy

Standard benchmarks insufficient — "much of the complexity in software engineering lies in the messiness of the real world." `cognition-golden` uses "authentic development environments supporting fully autonomous evaluation," with "evaluator agents that have access to Devin's browsing, shell and code editing tools." SWE-1.5 used three reward channels: classical tests, code-quality rubrics, agentic graders with browser-use. "Human experts try to find ways to circumvent the graders" to harden against reward gaming.

### Empirical numbers

- SWE-bench (Mar 2024): 13.86% on random 25%, 23% with unit tests. Runtime cap 45 min.
- SWE-1.5: "near-frontier" SWE-Bench Pro at "up to 950 tokens/second… 6x faster than Claude Haiku 4.5… 13x faster than Claude Sonnet 4.5."
- SWE-Check (Apr 2026): in-distribution F1 to Opus 4.6 closes from 0.09 to 0; OOD from 0.49 to 0.29; "order of magnitude-faster wall-clock."
- Devin Review: 2 bugs/PR, 58% severe.

### Cognition's architecture rules in their own words

- "Share context, and share full agent traces, not just individual messages."
- "Actions carry implicit decisions, and conflicting decisions carry bad results."
- "The simplest way to follow the principles is to just use a single-threaded linear agent."
- "At the core of reliability is Context Engineering."
- "We introduce a new LLM model whose key purpose is to compress a history of actions & conversation."
- "Multi-agent systems work best today when writes stay single-threaded and the additional agents contribute intelligence rather than actions."
- "A shared kernel is a security threat."
- "End-to-end reinforcement learning on real task environments using our custom Cascade agent harness." — model and scaffold are co-designed, not separable.

---

## Stream 4 — OpenAI Codex + Agents SDK

### Codex CLI Architecture
URL: https://developers.openai.com/codex/cli

Open-source, locally-running, Rust. Default mode interactive terminal UI.

**Loop**: Conversational ReAct-style. Plan exposed via togglable `/plan-mode`.

**Tool surface**:
- Shell execution within working directory
- Apply Patch (canonical OpenAI patch format)
- Web Search (cached by default "to reduce prompt injection risks"; live via `--search`)
- Image inputs/generation
- MCP servers
- No browser tool — web is search-only

**Sandbox** (https://developers.openai.com/codex/concepts/sandboxing):
- `read-only`
- `workspace-write` (default for local)
- `danger-full-access`

OS enforcement: **Seatbelt** macOS, **bubblewrap + seccomp** Linux/WSL2, **Windows Sandbox** Windows. Network off by default. `.git`, `.agents`, `.codex` stay read-only.

**Approval policies**:
- `untrusted` — auto-runs known-safe only
- `on-request` — runs in sandbox, asks at boundaries (recommended)
- `on-failure` — escalates after failure
- `never` — no prompts

Approval triggers: edits outside workspace, network access, untrusted commands, "destructive app/MCP tool calls." Optional `auto_review` agent. "Timeouts, parse failures, and review errors fail closed."

**Memory**: transcripts local, `resume` to continue. Cross-run behavior via `AGENTS.md` (project + user).

**Slash commands**: `/permissions`, `/status`, `/plan-mode`, `/feedback`, custom team commands.

### Agents SDK Primitives
URL: https://openai.github.io/openai-agents-python/

"Enough features to be worth useful, but few enough primitives to make it quick to learn."

**Agent**: `instructions`, `tools`, `handoffs`, `output_type` (Pydantic), `model_settings`, `input_guardrails`, `output_guardrails`, `hooks`, `tool_use_behavior`, `mcp_servers`, `reset_tool_choice` (default `True`).

**Runner**: `Runner.run()`, `run_sync()`, `run_streamed()`. Each turn: `final_output` ends loop, `handoff` swaps current agent, `tool calls` execute and re-loop. `max_turns` cap raises `MaxTurnsExceeded`.

**Handoffs**: implemented as tools (e.g. `transfer_to_refund_agent`). New agent "takes over… and gets to see the entire previous conversation history" by default; `input_filter` can prune. Distinct from `Agent.as_tool()` (manager keeps control).

**Guardrails**: parallel checks. Input on first agent, output on last. Tool guardrails wrap function-tool invocations. `tripwire_triggered` raises `InputGuardrailTripwireTriggered` / `OutputGuardrailTripwireTriggered`.

**Sessions**: persistent memory. Backends: `SQLiteSession`, `AsyncSQLiteSession`, `RedisSession`, `SQLAlchemySession`, `MongoDBSession`, `DaprSession`, `OpenAIConversationsSession`, `EncryptedSession`.

**Tracing**: built-in. Traces (workflow) contain spans (operation). Default destination: OpenAI Traces Dashboard. Sensitive-data toggles via `RunConfig.trace_include_sensitive_data`.

**Swarm**: explicitly retired. README states: "Swarm is now replaced by the OpenAI Agents SDK."

### Five "essential tactics" for agents
URL: https://openai.github.io/openai-agents-python/multi_agent/

1. "Invest in good prompts."
2. "Monitor your app and iterate on it."
3. "Allow the agent to introspect and improve."
4. "Have specialized agents that excel in one task, rather than having a general purpose agent."
5. "Invest in evals."

**Handoff vs Agent-as-Tool**:
- **Handoff** when "routing itself is part of the workflow."
- **Agent-as-Tool** when "a specialist should help with a bounded subtask but should not take over the user-facing conversation."

### Empirical
- GPT-5.3-Codex: 85.0% SWE-bench Verified
- GPT-5.2: 80.0%
- GPT-4o (original Codex): 33.2%

OpenAI publicly stopped reporting SWE-bench Verified, citing ≥59.4% of audited problems with flawed tests + train/eval contamination risk. Now recommend SWE-bench Pro.

### OpenAI's stated rules
1. Specialized agents over generalists.
2. Make tools legible.
3. Single agent owns user when continuity matters.
4. Default to least privilege.
5. Two-layer safety (OS sandbox + approval).
6. Loops must terminate.
7. Tracing non-optional.
8. Evals over vibes — retire broken ones.
9. Self-improvement loop.
10. Plan transparency.

---

## Stream 5 — GitHub Copilot Workspace + Coding Agent

### Copilot Workspace
Pipeline: **Task → (Spec / Brainstorm) → Plan → Implementation → Verify → PR**, "fully editable" at every stage.

- **Topic / Brainstorm**: thought-partner exploration.
- **Plan**: Copilot "figures out which files it must alter, and exactly what it must accomplish in each file."
- **Implementation**: streams suggestions.
- **Verification**: built-in terminal/Codespace.
- **PR / Share**: shareable workspace links.

"Everything that GitHub Copilot Workspace proposes—from the plan to the code—is fully editable… You retain all of the autonomy."

### Copilot Coding Agent (Project Padawan)

**Loop**:
1. Issue assignment from github.com / GitHub Mobile / Agents panel / VS Code / Copilot Chat
2. Adds 👀 emoji reaction
3. Opens draft PR tagged `[WIP]`, pushes commits
4. Explores via GitHub code search RAG, edits, runs tests/linters in sandbox
5. Updates PR + tags requester for review; CI/CD only after human approval
6. Iterates until subtasks complete

**Sandbox**:
- "Ephemeral development environment, powered by GitHub Actions"
- Read-only repo by default; writes only via draft PR branch
- **Firewall-restricted internet access** — admins allowlist hosts
- `.github/workflows/copilot-setup-steps.yml` for reproducible setup
- Self-hosted runners supported

**Tool surface**:
- GitHub code search RAG
- Built-in MCP: GitHub MCP Server + Playwright MCP
- Shell, build, test, lint inside Actions VM
- Hooks, Skills, Custom Agents

### Custom Instructions
- `.github/copilot-instructions.md` (repo-wide)
- `.github/instructions/<name>.instructions.md` with `applyTo:` glob, `excludeAgent:`
- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` at repo root
- Personal + Organization instructions
- Precedence: Personal > Repository > Organization

**Should**: repo summary, build/test, architecture, file structure, env, workarounds.
**Should not**: task-specific guidance; >2 pages.

### AGENTS.md (Aug 28 2025)
GitHub announced full support. The Coding Agent reads `AGENTS.md` (root or nested) + continues to honor `copilot-instructions.md` + `.instructions.md` + `CLAUDE.md` + `GEMINI.md`.

Empirical study of 2,500+ repos identified six core sections: **Commands, Testing, Project Structure, Code Style, Git Workflow, Boundaries**, with "three-tier boundary" (always do / ask first / never do). Most common boundary: "Never commit secrets."

### GitHub's principles
1. Editable artifacts at every stage.
2. Human-in-the-loop by default.
3. Transparency through git primitives.
4. Sandboxed, least-privileged execution.
5. Repo-resident context (AGENTS.md, copilot-setup-steps.yml, MCP config) versioned with code.
6. Specialist over generalist (study favors narrow + boundaries).
7. "AI helps with the things you don't want to do, so you have more time for the things you do."

---

## Stream 6 — Cursor

### Agent
Three components: **Instructions**, **Tools**, **Model**. Harness "automatically optimizes" prompts and tool formats per model.

**Loop**: Default autonomous ReAct (no stated tool-call limit). Three modes via Shift+Tab or `--mode`:
- **Agent** (full tools)
- **Plan** (research + clarifying questions, builds markdown plan first)
- **Ask** (read-only)

**File reading**: ranged but model-driven. Early guardrails removed as models improved.

**Edit mechanism**: per-model. "OpenAI's models are trained to edit files using a patch-based format, while Anthropic's models are trained on string replacement." Each gets training-aligned format.

**Shell**: monitored output. CLI has `/sandbox` toggle. Internal training infra "Anyrun" runs "hundreds of thousands of sandboxed coding environments."

**Approvals**: Checkpoints (auto-snapshot before significant changes, separate from git). Queued messages (Enter queues, Cmd+Enter interrupts). MCP tools require approval; auto-run via `~/.cursor/permissions.json`. Privileged shell triggers masked password prompt over secure IPC.

### Context engine
Shifted "from static context injection toward dynamic agent-driven retrieval."

Tools: Semantic search, Search files/folders, Read files, Web (search + fetch), Browser, Fetch Rules.

Server-side index. Privacy Mode = ZDR contracts. Context Usage Breakdown (May 2026) shows per-turn token spend.

### Rules
Four mechanisms with Team → Project → User precedence:
- **Project Rules** — `.cursor/rules/*.mdc`, version-controlled
- **User Rules** — global Settings
- **Team Rules** — org-wide
- **AGENTS.md** — markdown alternative; nested dirs

Four attachment modes:
- **Always Apply** (`alwaysApply: true`)
- **Apply Intelligently** (description-only, agent decides)
- **Apply to Specific Files** (glob)
- **Apply Manually** (`@my-rule`)

≤500 lines per rule. Reference external files; don't duplicate codebase docs. `.cursorrules` superseded.

### Cloud Agents (formerly Background Agents)
Isolated cloud VMs, clone repo, work on branch, push merge-ready PR with artifacts. Triggers: Cursor Web/Desktop, Slack, GitHub, Linear, mobile PWA, SDK. Always Max Mode. Local↔cloud handoff (Cursor 3). CLI: `&` prefix.

**Build in Parallel** (May 7 2026, v3.3): subagents for independent tasks; configurable models; `/multitask`. Multi-agent kernels post: "the entire coordination protocol lived in a single markdown file that specified the output format, rules, and tests" — 3 weeks unattended on 235 problems.

### Customization
**Plan/Agent/Ask** built-ins, **Skills** (`.cursor/skills/<name>/SKILL.md` with frontmatter — `name`, `description`, optional `paths`, `disable-model-invocation`), **Subagents** (named, custom prompt and model), **Hooks** (`.cursor/hooks.json`).

### MCP
`.cursor/mcp.json` (project) or `~/.cursor/mcp.json` (global). Three transports: stdio, SSE, Streamable HTTP. Tools, Prompts, Resources, Roots, Elicitation, Apps.

### Cursor's principles
1. Trust the model with context selection.
2. Match tool formats to model training.
3. Measure what users keep — "Keep Rate" + A/B; CursorBench.
4. Drive tool calls to ≥2-3 nines reliability.
5. Train the agent in its own harness — Composer 2 RL "occurs in realistic Cursor sessions with the same tools and harness the deployed model uses."
6. Plan-first when ambiguous.
7. Snapshots over guardrails — Checkpoints give cheap rollback.
8. Layer customization: Rules / Skills / Subagents / Hooks / MCP.

---

## Stream 7 — Sourcegraph (Cody + Amp)

Sourcegraph spun out Amp into a separate company; Sourcegraph keeps code-search platform.

### Amp architecture

**Modes**:
- `smart` (Opus 4.7, 300k tokens, unconstrained)
- `rush` (faster/cheaper)
- `deep` (GPT-5.5 extended thinking)

**Tool surface**: file ops, shell, git, web, code analysis. Three extension paths:
1. **Toolboxes** — local executable scripts using `TOOLBOX_ACTION` env vars + JSON on stdin/stdout
2. **MCP servers** (local or HTTP/streamable) with `includeTools` filters
3. **TypeScript plugins** that call `amp.registerTool()` and hook events

**Subagents (Task tool)**: "multi-step tasks broken into independent parts" or "operations producing extensive output not needed after completion." Parallel ("one subagent per file"). Cannot communicate, cannot guide mid-task, start fresh, parent sees only summary.

**Oracle**: separate reasoning model (originally o3, now GPT-5/GPT-5.4 high-reasoning). For "review, debugging, analyzing." Own context window. Minimal prompting "to avoid unnecessarily increasing costs or slowing you down." Explicit invocation ("use the oracle to review").

**Subagents shipped**: Librarian (searches public + private GitHub via Sourcegraph code search), Agentic Review, Finder/Oracle/Librarian/Kraken (refactor) per Beyang.

**Threads**: complete records of messages/context/tool calls. Sync to ampcode.com. Shareable, forkable, `@thread-id` referenceable, labeled, Thread Map visualized.

**Memory**: `AGENTS.md` / `AGENT.md` searched hierarchically: cwd, parents to `$HOME`, subtrees, system-wide. `@`-mentions with optional `globs`.

**Skills**: "Skills let the agent lazily-load specific instructions on how to use local tools." `.agents/skills/`, can bundle MCP via `mcp.json`.

**Sandbox**: executes commands directly in workspace. **No mandatory approval by default** — security trust-based. Optional plugin event hooks (`tool.call`, `tool.result`). Git staging area is recommended safety net.

### Cody architecture

"Context is king." Multi-product: autocomplete, chat, edit, test gen, agentic chat.

**Retrieval**: Two parallel strategies via Reciprocal Rank Fusion: keyword search (entity extraction + BM25 + learned signals); embeddings. Autocomplete uses Jaccard similarity on chunks from current/surrounding/recently-edited files.

**Three-tier context**: Local file → repository → remote repository (`@`-mention repos). Scales to "300,000 repositories and 90GB monorepos."

**Agentic chat**: "Think twice, answer once." Five tools: Code Search, Codebase Files, Terminal (user-approved), Web Browser, OpenCtx (Linear/Jira/Slack).

### Code search vs embeddings

Sourcegraph dropped global embeddings for three reasons: privacy ("sending code to a 3rd party"), admin complexity, vector-DB scaling cost. Native code search more accurate for known-item retrieval.

Three-role split: **Code Search** (precise), **Deep Search** (agentic NL), **MCP** (programmatic).

### CodeScaleBench (Mar 2026)
1M+ LoC enterprise codebases, multi-language, multi-repo. Two splits: SDLC (150 patch-based) and Org (220 `answer.json`).

MCP vs baseline: file recall 0.127 → 0.277, Precision@5 0.140 → 0.478, F1@5 0.099 → 0.262. Kubernetes monorepo: baseline timed out at 2h; MCP completed in 89s with 0.90 reward.

### Sourcegraph's principles
1. Context is king.
2. Context is a system, not a string.
3. 200k tokens is plenty — favor short focused threads.
4. Frontier-driven, minimalist curation.
5. Paint-by-numbers, human in loop.
6. Evals as regression tests, not decision tools — "evals are primarily useful as unit tests and regression tests, to ensure a new model doesn't regress some existing behavior."
7. Tools = general; skills = lazily-loaded task-specific.
8. Subagents are isolated, not collaborative.
9. Specialization by model.
10. Code search > embeddings for known-item retrieval at enterprise scale.

---

## Stream 8 — Google DeepMind / Gemini CLI / Jules

### Gemini CLI architecture

Open source, TypeScript. Three-layer stack inside `packages/core`:
- `GeminiClient` — entry point, conversation lifecycle, loop detection, chat compression, tool-output masking, hook firing
- `Turn` — converts streaming SDK responses into typed events, accumulates `pendingToolCalls` via Scheduler
- `GeminiChat` — wraps `@google/genai`, mid-stream retries

Loop "can span dozens of turns — sending requests to the model, receiving streaming responses, dispatching tool calls, feeding results back, and repeating."

**Tool surface**: file system (read/write), shell, web (fetch + Google Search grounding). Custom tools via MCP only.

**Sandbox** (https://geminicli.com/docs/cli/sandbox/): five backends — macOS Seatbelt, Docker/Podman, Windows Native Sandbox, gVisor/runsc, LXC/LXD. Trusted Folders policy gates execution.

**Memory**: `GEMINI.md` (project + user). Checkpointing, token caching, chat compression.

**Configuration**: `.gemini/settings.json` (project) overrides `~/.gemini/settings.json` (user).

**MCP**: First-class. Configured in `settings.json` under `mcpServers`.

**Subagents** (v0.38.1+): Markdown in `.gemini/agents/*.md`. Frontmatter: `name`, `description`, `kind: local|remote`, `tools`, `model`, `temperature`, `max_turns`, `timeout_mins`. "Each subagent runs in its own isolated context loop, with independent history that does not bloat the main agent's context." **Recursion forbidden**. Built-ins: `codebase_investigator`, `cli_help`, `generalist`, experimental `browser_agent`.

**Hooks**: 12 event types — Tool (`BeforeTool`, `AfterTool`), Agent (`BeforeAgent`, `AfterAgent`), Model (`BeforeModel`, `BeforeToolSelection`, `AfterModel`), Lifecycle (`SessionStart`, `SessionEnd`, `Notification`, `PreCompress`). JSON over stdin/stdout, exit code 2 = "system block." Fields: `decision: "allow"|"deny"`, `reason`, `hookSpecificOutput`, `continue: false`, `systemMessage`. Synchronous in loop.

### Jules

Three-step workflow: select repo → develop plan → review/create PR. "Jules fetches your repository, clones it to a Cloud VM, and develops a plan utilizing the latest Gemini 3 Pro model." Plan editable "before, during, and after execution." A "critic agent" "now steps in more reliably and re-engages after replanning."

GitHub-native: issue labeled `jules` becomes a task. Multi-surface (Web UI, Jules Tools, Jules API, Gemini CLI extension).

**Sandbox**: "Secure Google Cloud virtual machine" — full Linux. VM destroyed after PR. "Doesn't train on your private code."

`jules-action` GitHub Action: thin wrapper, auth + forward.

### AlphaCode → AlphaEvolve

**AlphaCode 2** (Dec 2023): Sample-and-filter. Up to **1M code samples** per problem with randomized temperature; execute on test inputs; cluster by output signature; keep 10 largest; second model scores; best per cluster → 10 final submissions. Solved 1.7× more, 85th percentile CodeForces.

**AlphaEvolve** (May 2025): Four components — prompt sampler, model ensemble (Gemini Flash for breadth, Gemini Pro for depth), evaluator pool, programs database with evolutionary algorithm. Discovered 4×4 complex matmul in 48 scalar multiplications (Strassen 1969 was beaten); recovered 0.7% Borg compute; 23% Gemini training kernel speedup; 32.5% FlashAttention speedup.

**Lesson**: evaluator-driven evolutionary loops over an ensemble, not megasample brute force.

### Empirical
- Gemini 2.5 Pro: 63.8% SWE-bench Verified
- Gemini 3 Pro: 76.2% SWE-bench
- Gemini 3 Flash: 78%
- Gemini 3.1 Pro: 80.6% Verified, 68.5% Terminal-Bench 2.0, 33.5% APEX-Agents, 90.8% τ²-bench Retail, 99.3% τ²-bench Telecom

### Google/DeepMind's principles
1. Bake reasoning into the model.
2. Native tool use at model layer.
3. Small grounded built-in tool set, extend via MCP.
4. Subagents = isolation, not collaboration. No recursion.
5. Long-horizon work async, off-terminal, in destroyable cloud VM.
6. Show plan, let user edit, keep critic.
7. Evolutionary search with ensemble (Flash + Pro) for research-grade.
8. Hooks belong in loop, synchronous, JSON-driven.
9. Sandbox by default with multiple backends.
10. One model surface, many client surfaces.

---

## Stream 9 — Microsoft (AutoGen + Magentic-One + Semantic Kernel)

### AutoGen v0.4 (actor-model rewrite)

Original v0.2 had limits: "users struggled with architectural constraints, an inefficient API compounded by rapid growth, and limited debugging."

**v0.4 = complete redesign on actor model**. Three layers:
- **`autogen-core`** — message passing, event-driven agents, local + distributed runtime, Python + .NET
- **`autogen-agentchat`** — opinionated high-level API; "closest experience to v0.2"
- **`autogen-ext`** — first/third-party extensions

Primitives: **Agents**, **Messages**, **Topics + Subscriptions** (pub/sub), **Runtime** (local or distributed).

"Event-driven communication moves message delivery away from agents to a centralized component, making it easier to observe and debug."

**Orchestration patterns**:
- `RoundRobinGroupChat` — fixed sequence, shared context
- `SelectorGroupChat` — model picks next speaker
- `Swarm` — `HandoffMessage` for explicit transfer
- `MagenticOneGroupChat` — generalist orchestrator

**Termination conditions** first-class.

**Tools / code execution**: `LocalCommandLineCodeExecutor`, `DockerCommandLineCodeExecutor`, Jupyter executors.

**Observability**: Native OpenTelemetry, AutoGen Bench, AutoGen Studio.

**Caveat**: AutoGen "now in maintenance mode with no new features planned" — directed to **Microsoft Agent Framework** under Semantic Kernel umbrella.

### Magentic-One

Lead **Orchestrator** + 4 specialized agents.

**Orchestrator**: two nested loops
- **Outer loop**: Task Ledger (facts, guesses, plan)
- **Inner loop**: Progress Ledger (self-reflection, agent assignment, updates)

Stalls return control to outer loop, plan rewritten.

**Specialized agents**:
- **WebSurfer** — Chromium browser
- **FileSurfer** — markdown-based local file/dir nav
- **Coder** — writes/analyzes code
- **ComputerTerminal** — executes programs

Performance: 40-60% on GAIA / AssistantBench / WebArena vs ~7-16% raw GPT-4. Human ~78-92%.

Rationale: "Encapsulating distinct skills in separate agents simplifies development and reuse, similar to object-oriented programming."

### Semantic Kernel

Production-grade SDK (.NET, Python, Java; v1.0+ stability).

**Abstractions**:
- `Agent` (abstract base) — `ChatCompletionAgent`, `OpenAIAssistantAgent`, `AzureAIAgent`, `OpenAIResponsesAgent`, `CopilotStudioAgent`
- `AgentThread` — stateful service-side or local
- **Plugins** (formerly "skills") — collections of native or prompt functions invoked through function calling
- **Planners** — `ActionPlanner`, `SequentialPlanner`, `StepwisePlanner` — **deprecated** in favor of native function calling

Orchestration patterns: Concurrent, Sequential, Handoff, Group Chat, **Magentic** (ported from Magentic-One).

### "Skills" terminology — two different things

- **SK's original "Skills"** = function bundles. Renamed to **plugins** in 2023: "Skills to plugins: fully embracing the OpenAI plugin spec in Semantic Kernel." After v1.0, "any function you add to a Kernel will get a Plugin."
- **Anthropic's Skills** = filesystem-backed behavior bundles (SKILL.md + scripts).

Different mechanism, coincidentally collides with deprecated SK vocabulary.

### MS Research findings

AutoGen paper: classical multi-agent debate + LangChain ReAct **underperformed vanilla GPT-4** on small test sets.

### Microsoft's principles
1. Use multi-agent for modularity, not "debate."
2. Separate orchestration from agents (Magentic-One Task + Progress Ledgers).
3. Event-driven, actor-model runtime.
4. Function calling over hand-built planners.
5. Layer for audience: Core / AgentChat / Extensions.
6. Standardize observability — OpenTelemetry non-negotiable.
7. Plugins/tools = substrate; agents = tools + instructions + thread.

---

## Stream 10 — Replit Agent

### Version timeline
Agent v1 (Sep 2024), v2 (Feb 2025, Claude 3.7), Agent 3 (Sep 2025), 3.5/4.5 (Nov 2025, Claude Sonnet/Opus 4.5), Agent 4 (Mar 2026).

### Pipeline

"What looks like one agent is actually a pipeline of specialized models — one for exploration, one for testing, one for the sub-agents handling parallelism."

Roles:
- **Plan mode (planner)** — opt-in, "breaks down complex projects into ordered task lists"
- **Builder (main agent)** — generates code, installs deps, runs
- **Tester subagent** — split out specifically "to solve context pollution issues"
- **Debugger** — print-style debugging via captured logs

V2 hypothesis-driven: "forms a hypothesis, searches for the right files, and only starts making changes when it has enough information."

Agent 4: plan-while-building with parallel sub-agents in isolated forks, auto-merge resolves ~90% conflicts.

### Multi-agent stance
Sub-agents only "where context-isolation is needed" (testing, parallel forks). Specialized sub-agents resolve merge conflicts when forks recombine.

### Tool surface
File ops (CSVs, PDFs, PowerPoint, MD, code), package install, run, browser automation via Playwright in JS sandbox ("execute JavaScript in a sandbox and inject helper functions"), database read queries, log capture, 30+ connectors.

Code-execution-based browser control over Computer Use because complex sequences "become simple loops" — "3x faster and 10x more cost-effective than Computer Use models."

### Sandbox
Repl = VM. Snapshot engine: "instant filesystem forks for reversible operations, versioned databases, isolated sandboxes." Checkpoints "as it works, so you can roll back to any previous state." Agent 4 parallel tasks in "isolated environments that are exact copies." Historical: omegajail.

### Course-correction
Plan mode + checkpoint rollback + Queue (edit/reorder messages, per-task model + Extended Thinking) + Agent 4 Design Canvas + Dynamic Intelligence per-request flags.

"Decision-time guidance — injecting situational instructions at key moments rather than front-loading all rules."

### Long-running
Agent 3: 200 minutes autonomous, $0.20/test session. Pro: 10 concurrent background tasks.

### Empirical
2025 Review: Agent v2 "2-3x speed improvements"; system prompt optimization "2-3 seconds faster per action"; multi-tool calling "15% cost savings and 30% faster results." Agent 4: "10x faster than Agent 3."

### Replit's principles
1. Specialized sub-agents only where context-pollution warrants.
2. Code execution beats imperative tool calls.
3. Decision-time guidance over front-loaded rules.
4. Hybrid safety: deterministic static analysis + dependency scans + LLM, never AI-only.
5. Snapshots + isolated forks make autonomy safe.
6. Plan-while-building beats plan-then-build; parallelism with auto-merge.
7. Autonomy opt-in and tunable.

---

## Stream 11 — Princeton SWE-agent + SWE-bench

### SWE-bench (ICLR 2024)
URL: https://arxiv.org/abs/2310.06770

**Task**: GitHub issue + repo state → patch that passes hidden `FAIL_TO_PASS` while preserving `PASS_TO_PASS`. **2,294 instances** from 12 Python repos.

**Eval**: containerized harness, Docker per instance (June 2024 rewrite eliminated cross-test contamination).

**Headline**: Claude 2 with retrieval scored **1.96%**. Real issue resolution was essentially unsolved in late 2023.

### SWE-agent (NeurIPS 2024)
URL: https://arxiv.org/abs/2405.15793

**Central thesis**: "LM agents represent a new category of end users with their own needs and abilities, and benefit from custom interfaces." **Agent-Computer Interface (ACI)** as first-class object.

**Why "give the LLM a shell" fails**: hallucinating paths/lines (cat floods context); looping on broken edits (long stack traces); edit corruption (sed/redirect writes broken code).

**ACI design**:
1. **Custom file viewer with 100-line window**, scrollable, line-numbered. Replaces `cat`.
2. **`edit <start>:<end>` with integrated linter**: rejects + does not apply if syntactically invalid. "We add a linter that runs when an edit command is issued, and do not let the edit command go through if the code isn't syntactically correct."
3. **`search_dir` and `search_file`**: one line per matching file, not per match.
4. **Empty-output normalization**: success+silent → "Your command ran successfully and did not produce any output."

**Four ACI principles**:
- Actions simple and easy
- Actions compact and efficient
- Feedback informative but concise
- Guardrails to mitigate error propagation

**Empirical**:
- SWE-agent + GPT-4 Turbo: **12.47%** on full SWE-bench. Prior best (RAG-only): **3.8%**. Same model, 3× score.
- SWE-agent + GPT-4 Turbo on Lite (300): **18.0%**. Claude 3 Opus: **11.7%**.
- HumanEvalFix: **87.7%** pass@1.

**Ablation (§4)**: removing ACI for vanilla shell drops GPT-4 Turbo by **~10.7 percentage points**. Linter alone causes measurable drop because invalid edits poison context.

**Action / token budget**: ~25-50 actions, per-instance compute budget.

### EnIGMA (2024)
URL: https://arxiv.org/abs/2409.16165

ACI scaffold for CTFs. **Interactive Agent Tools** wrap interactive programs (debugger, netcat). NYU CTF: **13.5%**, 3× prior best. Sets SOTA on Intercode-CTF and CyBench.

New failure mode: **"soliloquizing"** — model hallucinates fake observations.

### SWE-bench Verified (Aug 2024)
URL: https://openai.com/index/introducing-swe-bench-verified

Three defects in original:
1. Overly specific tests (rejected valid alternatives)
2. Underspecified issues (38.3% flagged)
3. Setup flakiness (61.1% flagged)

Fix: 93 engineers re-annotated. **500 instances** retained.

GPT-4o jumped from ~16% to **33.2%** on Verified — **measured score nearly doubled without changing the model**, because the original was systematically *underestimating* capability.

### Variants
- **Lite**: 300 instances
- **Multimodal** (https://arxiv.org/abs/2410.03859): 619 JS visual/UI tasks. Top Python systems crater on M; Agentless required 15h manual rewrites; SWE-agent (language-agnostic) transfers cleaner. "Generalizable LM-based systems for software engineering will emphasize interaction, not problem solving."
- **Multilingual**: 300 across 9 languages. SWE-agent + Claude 3.7 Sonnet at $2.50/task: **43%**.
- **Live** (https://arxiv.org/abs/2505.23419): 1,319 tasks since 2024. **RepoLaunch** automated curation, refreshed monthly. Contamination-resistant.
- **SWE-Lancer** (https://arxiv.org/abs/2502.12115): 1,400+ Upwork tasks ($1M total). Best (Claude 3.5 Sonnet): **26.2%** on independent coding.

### Princeton's empirically-grounded design rules
1. **Treat the interface as a first-class artifact** — ablation shows ~10.7pp from interface.
2. **Cap the window, number the lines** — 100-line file viewer with stable line numbers.
3. **Reject invalid actions at the boundary** — edit-linter prevents broken state entering context.
4. **Compress feedback to be informative-but-concise** — `search_dir` returns one line per file.
5. **Reasoning on model, mechanics on tools** — don't bake AST/repo heuristics; they don't transfer.
6. **Hygiene before scale** — Verified's 16→33.2 jump from *fixing the benchmark, not the model*.
7. **Budget actions, not just tokens** — 25-50 turn limits, per-instance dollar caps.

---

## Stream 12 — Aider

### Repo-map
URL: https://aider.chat/docs/repomap.html

"Concise map of your whole git repository" with classes, functions, types, signatures.
- Tree-sitter parsing per language
- Graph ranking on dependency graph; identifies "most often referenced" symbols
- `--map-tokens` budget (default 1k); dynamic based on chat state
- "Sending whole files is a bulky way to send code context, wasting the precious context window."

### Edit formats
URL: https://aider.chat/docs/more/edit-formats.html

- **whole** — full file rewrite. Slow/costly; safe for weak models.
- **diff** — search/replace blocks like git merge-conflict markers.
- **diff-fenced** — variant for Gemini.
- **udiff** — simplified unified diff for GPT-4 Turbo.
- **editor-diff / editor-whole** — architect mode variants.

**Udiff case** (https://aider.chat/docs/unified-diffs.html): "GPT acts more like it's writing textual data intended to be read by a program, not talking to a person." Four principles: familiar, simple, high-level, flexible. **20% → 61%** on laziness benchmark; 3× fewer placeholders.

**Anti-JSON case** (https://aider.chat/2024/08/14/code-in-json.html): "all of the models did worse on the benchmark when asked to return code in a structured JSON response." "Would *you* write better code by typing it out normally or typing it as a properly escaped JSON string?"

### Polyglot benchmark (Dec 2024)
225 hard Exercism exercises across C++/Go/Java/JS/Python/Rust. Tracks "Percent correct" + "Correct edit format" separately.

Top: gpt-5 (high) 88.0% / 91.6%; o3-pro (high) 84.9% / 97.8%.

### Architect mode (Sep 2024)
"The model has to split its attention between solving the coding problem and conforming to the edit format." Reasoning model (Architect) describes solution; cheaper Editor formats edits. o1-preview + DeepSeek-as-editor: 85% (vs o1-preview alone 79.7%).

### Philosophy
Pair programming, not autonomy. Git-auto-commits, easy diff/undo, lint/test feedback. "Notably avoids claims about autonomous agents or single-shot solutions."

### CONVENTIONS file
URL: https://aider.chat/docs/usage/conventions.html

Markdown bullet list. Examples: "Prefer httpx over requests." / "Use types everywhere possible."

Loaded via `/read CONVENTIONS.md` or `aider --read CONVENTIONS.md` or `.aider.conf.yml`. **Marked read-only, cached if prompt caching enabled.**

### Aider's principles
1. Pair programming, not autonomy.
2. Edit formats are load-bearing — familiar, simple, high-level, flexible; avoid JSON-wrapped code.
3. Match format to model.
4. Decompose attention — Architect/Editor split.
5. Repo map over embeddings — tree-sitter ASTs + graph rank, sized to budget.
6. Benchmark realistically — measure correctness AND format adherence; refresh when saturated; span languages.
7. Conventions in versioned, cached file via `/read`.

---

## Stream 13 — OpenHands (formerly OpenDevin)

URL: https://arxiv.org/abs/2407.16741 (Wang et al. 2024)

### Core architecture: action-observation event stream

Stateless loop over immutable, chronological event stream. Each `step()` produces one Action; runtime executes; Observation event appended; loop until `AgentFinishAction`.

**Action types**:
- `CmdRunAction` — bash in sandbox
- `IPythonRunCellAction` — arbitrary Python in persistent Jupyter kernel
- `FileReadAction` / `FileWriteAction`
- `BrowseURLAction` / `BrowserInteractiveAction` — Playwright Chromium via BrowserGym DSL
- `AgentDelegateAction`
- `AgentFinishAction`
- `MessageAction`

Each has a paired Observation. Event stream is the *only* state — agents are stateless reducers, makes condensation/replay/fork/security-analysis tractable. `EventStream` writer/reader pattern routes events through `Condenser` and `SecurityAnalyzer`.

### CodeAct (https://arxiv.org/abs/2402.01030)

**Thesis**: LLM emits *executable Python*, interpreter runs, returns stdout/tracebacks as observations.

Why code beats JSON:
1. *Control and data flow* — loops, conditionals, variables native.
2. *Existing tools* — directly use software packages.
3. *Automated feedback* — traceback already there.
4. *Pre-training alignment* — fluent.

Empirical: M3ToolEval **+20%** absolute success, **30% fewer actions**. Outperformed text/JSON on 12 of 17 LLMs.

### AgentSkills

Python functions auto-imported into IPython namespace. Selection rule: skill added when "(1) it is not readily achievable for [the] LLM to write code directly… and/or (2) it involves calling an external model."

Skills: `open_file`, `goto_line`, `edit_file_by_replace`, `parse_pdf`, `parse_image` (vision-backed), `parse_docx`. Composable inside loops, recoverable from tracebacks.

### Multi-agent delegation

`AgentDelegateAction` "enables an agent to delegate a specific subtask." Hierarchical-by-default — single orchestrator with single-shot delegations.

Predefined: CodeActAgent (default), BrowsingAgent, DummyAgent / PlannerAgent, ReadOnlyAgent / VerifierAgent.

### Runtime / sandbox

Sandbox = separate process via REST API. Default: **Docker** with action-execution server, persistent bash, persistent IPython kernel, headless Chromium. Also: Apptainer (HPC), Process (local dev), Remote (E2B, Modal, RunPod).

Persistence per-conversation. Workspace bind-mounted/copied.

### Microagents → Skills

`.openhands/microagents/*.md` → `.agents/skills/*` (newer SDK rebrand).

Frontmatter:
```yaml
name: kubernetes
trigger_type: keyword       # always | keyword | manual
keywords: [kubernetes, k8s, kubectl]
mcp_location: <optional>
```

Two main classes: General/Repo (always loaded into `<REPO_CONTEXT>`); Keyword-Triggered (loaded into `<EXTRA_INFO>` only when match).

### SWE-bench

CodeActAgent v1.8 + Claude 3.5 Sonnet: **26%** on Lite. v2.x/v3 stack on Verified: high-60s pass@1; +8-12 with 5-sample majority voting.

### OpenHands' principles
1. Agents act in the world the way humans do — code, shells, browsing — not bespoke JSON.
2. One unified action space (executable code) > N discrete tool schemas.
3. Event stream is the only state — stateless agent + immutable history.
4. Sandbox first — Docker by default.
5. Skills = Python functions in IPython namespace.
6. Repo-local context first-class — microagents/skills.

---

## Stream 14 — Documentation patterns comparative

### File location & naming

| Vendor | Filename | Project loc | User loc | Org loc |
|---|---|---|---|---|
| Anthropic Claude Code | `CLAUDE.md` (+ `.claude/rules/*.md`) | `./CLAUDE.md` or `./.claude/CLAUDE.md` | `~/.claude/CLAUDE.md` | `/Library/.../ClaudeCode/CLAUDE.md`, `/etc/claude-code/CLAUDE.md` |
| AGENTS.md spec | `AGENTS.md` | repo root + nested | n/s | n/s |
| GitHub Copilot | `copilot-instructions.md` (+ `.instructions.md`) | `.github/copilot-instructions.md` | n/a | n/a |
| Cursor | `.mdc`/`.md` in `.cursor/rules/` | `.cursor/rules/*.mdc` | "User Rules" UI | "Team Rules" |
| Sourcegraph Amp | `AGENTS.md` (or `AGENT.md`/`CLAUDE.md`) | cwd + parents to `$HOME` + subtrees | `~/.config/amp/AGENTS.md` | `/etc/ampcode/AGENTS.md` |
| Google Gemini CLI | `GEMINI.md` (overridable) | repo root + parents until `.git` | `~/.gemini/GEMINI.md` | n/a |
| Aider | `CONVENTIONS.md` | project | n/a | n/a |
| Continue.dev | `*.md` in `.continue/rules/` | `.continue/rules/` | `~/.continue/rules/` | Hub |
| Devin | UI "Knowledge" items | per-repo | per-user | Org/Enterprise tabs |

### Format

| Vendor | Plain MD | Frontmatter | Imports | Length |
|---|---|---|---|---|
| Claude Code | yes | YAML `paths:` only on `.claude/rules/*.md` | `@path`, max 5 hops | "<200 lines" |
| AGENTS.md spec | yes | "no required fields" | n/s | unspecified |
| Copilot | yes | none in repo file | none | first 4,000 chars used by review |
| Cursor | `.mdc` | `alwaysApply`, `globs`, `description` | `@file` refs | "<500 lines" |
| Amp | yes | YAML with `globs` | `@`-mentions | unspecified |
| Gemini CLI | yes | none documented | `@file.md` | unspecified |
| Aider | yes | none | n/a | "concise" |
| Continue.dev | `.md` or `.yaml` | `name`, `globs`, `regex`, `description`, `alwaysApply` | none | unspecified |
| Devin | n/a (UI fields) | trigger description mandatory | n/a | concise triggers |

### Loading semantics

- **Always-loaded model**: Claude Code (cwd→root concatenation, root-first), Gemini CLI (walk to `.git`), Amp (cwd→`$HOME`), Copilot (always-on for repo).
- **Closest-wins precedence**: AGENTS.md spec, Claude Code, Amp.
- **Path-scoped**: Cursor "Auto Attached", Continue.dev `globs`/`regex`, Claude Code `.claude/rules/`, Amp `globs`, GitHub `.github/instructions/*.instructions.md`.
- **Description-matched**: Cursor "Apply Intelligently", Continue.dev `description`-only, Devin trigger.
- **Manual**: Cursor `@rule-name`, Aider `--read`.
- **Org override**: Anthropic + Amp non-excludable system files; GitHub Copilot org policies; Cursor Team Rules; Devin Enterprise.

### What goes in (universally)
- Build/test/lint commands with flags (every vendor + GitHub study)
- Coding style/conventions (every vendor)
- Repo structure (everyone except Aider)
- Boundaries / "don't touch X" zones (AGENTS.md spec, GitHub study, Cursor, Anthropic)

### What doesn't go in
- **Anthropic**: anything Claude can read from code, language standards, detailed API docs, frequently-changing info, file-by-file descriptions, "self-evident practices."
- **GitHub Copilot**: external resource refs, style-based response constraints, complex conditional logic.
- **Cursor**: whole style guides (use linters), common tool commands, codebase docs duplicates.
- **AGENTS.md spec**: README content stays in README.
- **Devin**: only repeated material.

### AGENTS.md as emerging standard
agents.md lists 20+ tools: OpenAI Codex, Google Jules, Anthropic Claude Code, GitHub Copilot, Cursor, VS Code, Devin, Aider, Zed, Warp, Sourcegraph Amp. Claims 60,000+ repos.

Cross-compatibility:
- **Amp**: reads `AGENTS.md`, `AGENT.md`, `CLAUDE.md` natively.
- **Claude Code**: doesn't read `AGENTS.md` directly; documents `@AGENTS.md` import or `ln -s`. `/init` ingests `AGENTS.md`, `.cursorrules`, `.windsurfrules`.
- **Gemini CLI**: filename overridable via `settings.json`'s `context.fileName`.
- **Codex CLI**: native AGENTS.md.

Practical state: AGENTS.md is lingua franca; Anthropic + Google maintain branded filenames with documented bridges.

### GitHub 2,500-repo study findings
Six top sections: commands, testing, project structure, code style, git workflow, boundaries.

- Specific job/persona beats "helpful coding assistant"
- Executable commands early, including flags
- "One real code snippet showing your style beats three paragraphs describing it."
- Three-tier boundary: always-do / ask-first / never-do
- Most common boundary: "never commit secrets"
- Most failures = missing structure, not missing model capability

### Vendors agree on
- Markdown file at known location, auto-loaded, beats hidden config
- Build/test/lint with exact flags
- Clear boundaries / forbidden zones
- Code style scoped (refer to linters, don't restate)
- Closer files override farther in monorepos
- Concrete examples beat abstract instructions
- Length restraint helps
- Org-managed override layer is now standard

### Vendors disagree on
- **Filename** — AGENTS.md vs vendor-specific
- **Frontmatter** — required vs optional
- **Loading model** — eager vs split modes vs manual
- **Architecture content scope** — yes (Amp, Anthropic) vs no (Cursor, AGENTS.md → README)
- **Imports vs directories**
- **Single big file vs many small files**
- **Length budget** — Cursor 500 / Anthropic 200 / Copilot 4k chars / spec silent

---

## Stream 15 — Skills primitive

### Anthropic's Skill — definition

"Organized folder of instructions, scripts, and resources that agents can discover and load dynamically." Unit = directory; entry = `SKILL.md`.

Layout:
```
skill-name/
├── SKILL.md
├── scripts/
├── references/
└── assets/
```

**Open-standard frontmatter** (agentskills.io):
- `name` — required, 1-64 chars, lowercase
- `description` — required, ≤1024 chars; describe *what* and *when*
- `license`, `compatibility`, `metadata`, `allowed-tools`

**Claude Code superset**: `when_to_use`, `argument-hint`, `arguments`, `disable-model-invocation`, `user-invocable`, `model`, `effort`, `context: fork`, `agent`, `hooks`, `paths`, `shell`.

Locations: enterprise > personal (`~/.claude/skills/`) > project (`.claude/skills/`) > plugin.

Slash commands merged into skills: `.claude/commands/foo.md` and `.claude/skills/foo/SKILL.md` both produce `/foo`.

**Progressive disclosure**:
1. **Discovery (~100 tokens/skill)**: name + description in system prompt.
2. **Activation (<5,000 tokens; SKILL.md <500 lines)**: full body when relevant.
3. **Execution (on-demand)**: bundled files read/executed as needed.

**Claude Code budgets**:
- Description+when_to_use truncated at **1,536 chars**.
- Total skill-listing budget: **1% of context window**, **8,000-char fallback**, tunable via `SLASH_COMMAND_TOOL_CHAR_BUDGET`.
- Post-compaction: **first 5,000 tokens** of each invoked skill, **25,000-token combined cap**, most-recent-first.
- Loaded SKILL.md "stays in context for the rest of the session."

**Invocation**: model judgment on description match, OR user `/name`, OR `paths:` glob match. `disable-model-invocation: true` removes from context. `user-invocable: false` hides from `/` menu.

### Skill vs neighboring primitives

| Primitive | Trigger | Lifetime | When to use |
|---|---|---|---|
| **Skill** | Model OR user `/name` | Loaded on activation, sticky | Procedural workflow, multi-step recipe, sometimes-needed knowledge |
| **Tool / MCP** | Model decides per call | Schemas always loaded; bodies don't | External capabilities — verbs, not knowledge |
| **CLAUDE.md** | Always | Permanent | Always-on facts |
| **Subagent** | Delegated by parent | Separate context | Fresh context, parallel exploration, isolation |
| **Slash command** | User-typed | Same as skill body | Subsumed: `.claude/commands/*.md` is skill without bundles |
| **Hook** | Deterministic on events | Out-of-band | Runtime invariants — skills suggest, hooks enforce |

**Crisp boundary**: Skills = procedural knowledge model chooses to apply; Tools/MCP = external capabilities model calls; CLAUDE.md = always-on context; Hooks = runtime invariants.

### Other vendors

- **Cursor Skills**: native spec implementation. "Rules are declarative and always-on; Skills are procedural and on-demand." Deprecating legacy rules.
- **Amp Skills**: primary `.agents/skills/`, also reads `.claude/skills/` and `~/.claude/skills/`. Same SKILL.md.
- **OpenHands microagents**: keyword-triggered + always-on + on-demand. Migrated `.openhands/microagents/` → `.agents/skills/`.

Compatibility: spec-conforming skills port. Claude-Code-specific frontmatter silently ignored elsewhere.

### Microsoft "Skills" history (don't confuse)
SK 2022-2023 used "skills" = function bundles. Renamed Skill → Plugin in 2023 ahead of v1.0.0. SK skill = callable function bundle (Tool/MCP-like). Anthropic's 2025 Skill is different.

### When NOT to use a Skill
- **Always-on fact** → CLAUDE.md
- **Function with params/return** → Tool/MCP
- **One-line tweak** → just type it
- **Need deterministic enforcement** → Hook
- **Description triggers wrong** → tighten or `disable-model-invocation: true`

### Vision
"Enable agents to create, edit, and evaluate Skills on their own." `skill-creator` skill ships. Trajectory: agents author their own procedural memory, version-controlled, shareable.

**Operating rule**: Skill when *procedure* (>3 steps) + *sometimes needed* + *worth bundling resources*. Otherwise: always-on → CLAUDE.md; capability → Tool/MCP; deterministic gate → Hook; one-off → type it.

---

## Stream 16 — Memory architectures

### Five categories
- **Short-term / working** — in-context history. Trivial.
- **Episodic** — past sessions chronologically. LangGraph "thread-scoped" checkpoints; ChatGPT "Reference chat history"; Zep timestamps with valid-from/valid-to.
- **Semantic** — extracted, deduplicated facts. ChatGPT saved memories, Mem0 fact store, Letta core memory, Cursor Memories.
- **Procedural** — workflows, skills, system-prompt updates. LangGraph procedural memory; Voyager JS skill library; Anthropic Skills.
- **Scratchpad / working memory** — files agent reads/writes. NOTES.md, `/memories/`, Letta core memory blocks.

### MemGPT / Letta (https://arxiv.org/abs/2310.08560)
LLM as kernel with virtual memory. Three tiers:
- **Main context** = working RAM (system, FIFO queue, *core memory* blocks)
- **Core memory**: small, always-in-context, editable via `core_memory_append` / `core_memory_replace`. Persona + human blocks.
- **External**: **recall memory** (full history, `conversation_search`) + **archival memory** (vector store, `archival_memory_insert/search`)

Agent decides paging. Heartbeats + memory-pressure warning.

### Mem0 (https://arxiv.org/abs/2504.19413)
Two-phase pipeline:
- **Extraction**: LLM pulls candidate facts; existing related retrieved.
- **Update**: second LLM call performs `ADD`/`UPDATE`/`DELETE`/`NOOP` against store.

API: `add`/`search`/`update`/`delete`/`get_all`, scoped by `user_id`/`agent_id`/`run_id`.

Mem0g graph variant. Hybrid (vector + graph + key-value).

LOCOMO benchmark: ~91% latency reduction vs full-context.

### Anthropic
Two mechanisms:
- **Memory tool**: client-side `view`/`create`/`str_replace`/`insert`/`delete`/`rename` over `/memories`. Anthropic ships contract; storage is dev's.
- **Claude Code memory**: enterprise > project (`./CLAUDE.md`) > user (`~/.claude/CLAUDE.md`). `@path` imports. `#` shortcut + `/memory` command.
- **NOTES.md pattern**: structured note file as working memory across sub-agent boundaries.

### ChatGPT Memory
April 2024: opt-in saved memories. April 2025: "Reference chat history" (implicit episodic).

### Devin Knowledge
User-curated trigger/content pairs. Devin suggests new entries after sessions.

DeepWiki (separate): auto-indexes repo into structured wiki.

### Cursor Memories
Per-project, generated from conversations, surfaced for user approval before save.

### Generative Agents (https://arxiv.org/abs/2304.03442)
Memory stream with recency + importance + relevance scoring. Periodic *reflection* into higher-level abstractions.

### Failure modes
- **Pollution** — ChatGPT + Cursor gate writes through user-visible items / approval. Mem0 `UPDATE`/`DELETE` for contradictions.
- **Staleness** — Zep/Graphiti bi-temporal. Aider rebuilds repo-map.
- **Bloat / poor recall** — Anthropic warns against unbounded notes; LangGraph "memory profiles" / "collections."
- **Cross-user leakage** — Mem0 mandates scope IDs. Letta scopes per-agent. Claude Code separates user/project. Cursor scopes per project.

### Convergent design for coding agent memory
Four distinct stores:
- Checked-in **project file** (CLAUDE.md / replit.md / `.cursorrules`) — procedural, human-edited
- **User file** for cross-project preferences
- **Agent-curated semantic store** with explicit add/update/delete + contradiction handling
- **Task scratchpad** (NOTES.md / `/memories/`) — working memory

Repo understanding (Aider repo-map, DeepWiki) is regenerated, not remembered.

Every write inspectable + reversible. Every fact scoped + timestamped where possible.

---

## Stream 17 — Evaluation methodology

### Public benchmarks

**SWE-bench (Princeton, ICLR 2024)** — 2,294 GitHub issue→PR pairs across 12 Python repos. Functional, repo-grounded, unit-test-graded. Original Claude 2: 1.96%.

**SWE-bench Verified (OpenAI + Princeton, Aug 2024)** — 500 human-curated. Anthropic Claude Opus 4.6 system card: **80.84%** (averaged 25 trials, adaptive thinking, max-effort sampling).

**SWE-bench Pro (Scale AI, Sep 2025)** — 1,865 problems / 41 repos. Three partitions: 11 public GPL, 12 held-out GPL, 18 commercial. GPL/private = contamination defense. HITL: env, spec augmentation, test verification. Top public: Claude Opus 4.5 45.9%, Sonnet 4.5 43.6%, Gemini 3 Pro 43.3%, GPT-5 (High) 41.8%. **Same models score ~80% on Verified** — gap is contamination penalty.

**Aider polyglot** — 225 Exercism C++/Go/Java/JS/Python/Rust. Pass@1 + Pass@2; format adherence separate. GPT-5 (high): 88.0% Pass@2 / 52.0% Pass@1. Limit: Exercism public + old.

**LiveCodeBench** — contest problems with release dates; restrict to post-cutoff. Code gen + self-repair + test-output prediction + code execution. Demonstrated DeepSeek's drop on post-Sep-2023 problems. Only major coding benchmark with real contamination control.

**BigCodeBench** — 1,140 tasks (+ 150 Hard Set), realistic library use vs algo puzzles. Complete + Instruct modes. Calibrated Pass@1 greedy. Open-data badges per model.

**Cybench / HELM** — Cybench is CTF cyberoffense. HELM aggregates older benchmarks; mostly saturated.

### Private / internal evals

**Anthropic** — Multi-Agent post: LLM-as-judge 0.0-1.0 across factual accuracy, citation accuracy, completeness, source quality, tool efficiency. Start with ~20 representative queries, sufficient to discriminate prompt changes 30% → 80%. Human eval for hallucinations, source-bias, emergent failure modes. On BrowseComp: 3 factors explained 95% variance, token usage alone 80%.

**Cognition (cognition-golden)** — tasks inspired by real customer use on codebases up to millions of lines. Devin: 74.2% without seeing eval tasks during training. Two commitments: reproducible fully-autonomous environments (real Linux machines, live web servers, simulated users releasing info conditionally); agentic graders (browsing, shell, code-edit tools, visually verify outputs e.g. logging into Grafana). "Messy real world."

**Cursor (CursorBench-3)** — sourced via Cursor Blame: traces committed code in Cursor's own engineering back to agent request. Median 181 lines changed, 390-char prompts vs 7-10 lines / 1,185-3,055 chars on public benchmarks. Agentic graders: correctness, code quality, efficiency, interaction. Refreshed every few months. Live-traffic A/B. Framing: "what users keep."

**OpenAI** — internal evals not publicly documented. Has retired SWE-bench Verified.

**Sourcegraph (CodeScaleBench)** — 370 tasks: 150 SDLC patch-graded + 220 Org artifact-graded. Enterprise codebases ~1M+ LOC, multi-language, cross-repo. Six-dimension agentic checklist: Task Validity, Outcome Validity, Reporting, Reproducibility, Tool Effectiveness, Statistical Validity. Started 20 tasks/suite, sized via DoE for distinguishing 0.05 deltas. Replayable transcripts. "Evals as regression tests."

### METR uplift studies

**July 2025 RCT** — 16 experienced OSS devs, repos 22k+ stars / 1M+ LOC, with personal contribution history. 246 issues, randomized AI/no-AI (mostly Cursor Pro + Claude 3.5/3.7 Sonnet). Self-reported timing, screen-recorded. Headline: **AI users took 19% longer**, subjectively estimated 20% faster, forecasted 24% speedup beforehand.

**Feb 2026 update** — substantially walks it back. Same devs: −18% [−38%, +9%]. New devs: −4% [−15%, +9%]. Three design problems: developers refused participation rather than work without AI (selection bias against AI-strong beneficiaries); 30-50% avoided submitting AI-accelerated tasks (task-selection bias); time-tracking broke with concurrent agents. "Only very weak evidence."

### SWE-bench problems

OpenAI retirement post + Latent Space writeup with Mia Glaese / Olivia Watkins:

1. **Direct training contamination** — any model trained on GitHub data after June 2024 likely saw the 500 Verified problems including solutions. GPT-5.2's CoT references Django release notes describing the specific `edit_only` parameter the test expects, not in issue text.
2. **Flawed test design** — of 138 audited problematic tasks: 49 too narrow (reject correct alternatives), 26 too wide (demand undocumented features). External work (UTBoost, PatchDiff) estimates ~6-7pp leaderboard inflation.
3. **Memorization signature** — "SWE-Bench Illusion" paper (arXiv 2506.12286): models hit 76% file-path identification from issue text alone (no repo access) on Verified, dropping to 53% on OOD repos. Verbatim 5-gram similarity 35% on Verified vs 18% comparable.

OpenAI framing: a single 80% on Verified can hide that the model didn't improve at coding — improved at retrieving the gold patch.

### Reproducibility

Per Epoch AI: full 500 SWE-bench Verified takes 62-73 min on one GitHub Actions VM (32 cores, 128 GB), ~8s/sample, 300k-token cumulative budget. Image footprint: 30 GiB optimized vs 189 GiB original. 484/500 Verified reproduce on Epoch's infra; 99.8% of full SWE-bench builds on x86_64.

Why third-party reproductions diverge:
- Build images depend on apt + PyPI fetched at build time → not bit-identical
- Agent scaffolding (system prompt, tools, retries, sampling) rarely fully published
- Per-run sampling temperature + best-of-N inflates self-reports

Anthropic averages 25 trials for headline numbers; most third-party use 1.

### What "evidence the agent works" means to a serious lab

Layered standard:
- **Contamination-resistant external benchmark** — SWE-bench Pro (held-out + commercial), or LiveCodeBench post-cutoff. Lower bound on capability.
- **Private internal eval bound to product** — cognition-golden, CursorBench-3, CodeScaleBench. Refreshed quarterly, intentionally underspecified.
- **Agentic grading**, not just unit tests — agents with browsing/shell/code-edit verify outcomes.
- **Statistical rigor on small samples** — Anthropic 20-query early dev, Sourcegraph DoE-sized for 0.05 deltas, Anthropic 25-trial headline averages.
- **Live-traffic confirmation** — Cursor online A/B, Cognition deployed customer engagements. Eval lab + product floor must agree.
- **Resistance to METR result** — no public lab claims dev-time speedup based on RCTs; existing evidence is "very weak" by METR's own admission. Internal evals measure capability; uplift is open question.

Serious labs no longer treat single benchmark score as evidence. Artifact = portfolio: contamination-resistant public number + refreshed private eval + agentic-graded transcripts + statistical CIs + live-traffic regression history + explicit acknowledgement none of this yet proves end-user productivity.

---

## Stream 18 — Tool design + MCP

### Anthropic's tool-design rules

- **Namespacing**: prefix grouping; "Namespacing… can help delineate boundaries between lots of tools."
- **Unambiguous parameters**: `user_id` not `user`.
- **Description as onboarding doc**: "Think of how you would describe your tool to a new hire."
- **Errors as actionable hints**: "Specific and actionable improvements, rather than opaque error codes or tracebacks."
- **Response shape**: high-signal info; `response_format` enum (concise/detailed); strip low-signal IDs.
- **Anti-patterns**: 1:1 API wrap; generic `list_*`; overlapping tools; "More tools don't always lead to better outcomes."

### Code execution as consolidation

150k → 2k tokens (98.7% reduction).
- **Progressive-disclosure filesystem**: tools as tree (`servers/google-drive/...`).
- **Filter inside sandbox**: 10,000 spreadsheet rows → 5 filtered before any token.
- **Tradeoff**: "Secure execution environment with appropriate sandboxing, resource limits, and monitoring."

### SWE-agent ACI principles
- Custom file viewer over `cat` — 100-line window prevents drowning in 5,000-line dump.
- Edit + linter — failed edits return error with lint hint instead of silently writing broken code.
- Search returns file lists, not blobs — "showing the model more context about each match proved to be too confusing."
- Empty output → "Your command ran successfully and did not produce any output."
- "Good ACI design leads to much better results."

### MCP spec essentials (2025-06-18)

**Server features**: Resources (read-only), Prompts (templates), Tools (model-invokable). Negotiated at init.

**Client features (server-initiated)**: Sampling (server asks client for completion; "intentionally limits server visibility into prompts"; user approval); Roots (URI/filesystem boundaries); Elicitation (mid-flow info request).

**Apps**: interactive UI surfaces.

**Transports**:
- **stdio** — server as subprocess. "Clients SHOULD support stdio whenever possible." Local servers.
- **Streamable HTTP** — single endpoint POST + GET, optional SSE upgrade. Replaces deprecated 2024-11-05 HTTP+SSE. Remote servers.

DNS-rebinding mitigations mandated for local HTTP.

**Authorization**: "Authorization is OPTIONAL." When used, OAuth 2.1 resource server. PKCE MUST. Dynamic Client Registration (RFC 7591) SHOULD. `resource` parameter (RFC 8707) MUST bind tokens. Servers MUST NOT pass tokens to upstream APIs.

### MCP ecosystem

**Servers**: Anthropic reference (Drive, Slack, GitHub, Git, Postgres, Puppeteer). GitHub official (~80 tools across 24 toolsets, "dynamic tool discovery" beta — "enabling only the toolsets you need can help the LLM with tool choice and reduce context size"). Stripe (~30 tools, OAuth + restricted-key). Cloudflare (Workers + Durable Objects + Streamable HTTP + OAuth Provider). Notion, Slack, AWS.

**Clients**: Claude Desktop + Claude Code, ChatGPT, Cursor, VS Code Copilot Chat, Codex, Gemini CLI, Amp, OpenHands, Continue, Zed, Replit, Cody. Coverage uneven — Tools universal; Resources/Prompts common; Sampling/Elicitation rarer (Claude Desktop).

### Edit-format research

**Aider polyglot**: "Plain text edit formats worked best." Weak models: `whole` ~46%. Strong: "GPT-4 gets comparable results with `whole` and `diff`, but `whole` significantly increases costs and latency." Function-calling: "It makes GPT write worse code."

**Aider unified diffs**: GPT-4 Turbo SEARCH/REPLACE → udiff: 20% → 61%. "Diffs are usually consumed by the patch program, which is fairly rigid. This seems to encourage rigor."

**Cursor**: split — strong "sketch" model emits imperfect change; custom-trained Apply model integrates at ~1000 tok/s.

**Recommended formats today**: OpenAI ships `apply_patch`. Anthropic published Claude system prompts use search-replace blocks. Both deliberately avoid line numbers, favor unambiguous before/after anchors.

**Tool-overload failure**: stacking standard MCP servers (Playwright + GitHub + IDE) "consumes over 20% of context window before agent even started." GitHub MCP alone ~42k tokens. Lost-in-the-middle degradation. Empirical case for code-execution + progressive-disclosure response.

### Convergent tool-design rules

1. **Fewer, higher-level tools beat many low-level ones.**
2. **Names + parameters self-documenting.**
3. **Return shaped, paginated, filtered results — never raw blobs.**
4. **Errors are prompts.**
5. **Progressive disclosure beats flat tool list.**
6. **Edit formats should match patch program, not JSON schemas.**
7. **Sandbox the side effects.**
