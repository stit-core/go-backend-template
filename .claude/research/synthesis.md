# Cross-vendor synthesis — what the major AI labs actually do

Compiled May 2026 from 18 parallel research streams into Anthropic, OpenAI, Cognition, Google DeepMind, Microsoft, GitHub, Cursor, Sourcegraph, Replit, Aider, Princeton (SWE-agent / SWE-bench), All-Hands AI (OpenHands), plus cross-cutting work on memory, evaluation, tool design, and documentation patterns. All claims trace to vendor-authored or peer-reviewed sources — see `raw-briefings.md` for the citations.

The point of this document: capture the load-bearing claims that **every** serious player makes (so the architecture doesn't have to argue them), and list the genuine disagreements (so the architecture can pick a side and defend it).

---

## 1. Where the major labs converge

These claims appear, in some form, in every vendor's published material. Treat as load-bearing — they are the closest thing to industry consensus.

### 1.1 Architecture & loop

- **Start with a single agent. Add complexity only when measured.** Anthropic ("find the simplest solution possible, and only increase complexity when needed"); Cognition ("the simplest way to follow the principles is to just use a single-threaded linear agent"); OpenAI ("specialized agents that excel in one task, rather than... general purpose"); Microsoft (post-AutoGen, naive multi-agent debate underperformed strong single-agent baselines on their own evals).
- **The agentic loop is gather → act → verify → repeat.** Anthropic Agent SDK names this explicitly. SWE-agent's empirical work shows verification (the linter that rejects bad edits *before* they enter context) is what stops looping pathologies. Replit's V2 hypothesis-driven exploration is the same idea by another name.
- **For *coding*, multi-agent parallelism rarely helps.** Anthropic's Multi-Agent Research System paper says it explicitly: "most coding work" has "fewer parallelizable subtasks" than research and they do not recommend the orchestrator-worker pattern there. Cognition's anti-multi-agent post is built around the Flappy Bird example (parallel agents make incompatible style choices). Replit only forks subagents for clear context-isolation cases (testing, parallel-file edits with auto-merge).
- **Subagents = context isolation, not collaboration.** Gemini CLI explicitly forbids subagent recursion; Amp subagents cannot communicate mid-task; Claude Code subagents return summaries to the parent; OpenHands' AgentDelegate is hierarchical-by-default. The pattern is: **fan out for read-heavy investigation, fan in with a 1–2k-token summary**, never loop multiple writers across a shared artifact.

### 1.2 Context engineering (the "who has what info" question)

- **Treat context as a finite budget.** Anthropic's "Effective context engineering" post and Sourcegraph's "Context is king" frame this identically. Every token spent on irrelevant content reduces the attention available for the actual task — "context rot" is documented.
- **Four distinct stores, matched to cadence:**
  - **Always-on facts** → CLAUDE.md / AGENTS.md / GEMINI.md (every vendor).
  - **Sometimes-needed procedures** → Skills (Anthropic SKILL.md spec, adopted by Cursor, Amp, OpenHands, Codex; spec at agentskills.io).
  - **External capabilities** → Tools / MCP servers (universal).
  - **Working memory during a task** → scratchpad files (NOTES.md, `/memories/`, Anthropic memory tool, Letta core memory, Mem0).
- **Just-in-time retrieval beats pre-loading.** Cursor moved from static context injection to model-driven retrieval as models improved. Anthropic's "Code Execution with MCP" shows pre-loading every tool def costs ~150k tokens; loading only what's needed costs ~2k. The pattern: agent carries lightweight identifiers (file paths, queries) and retrieves through tools at the moment of need.
- **Compaction is not optional for long-running work.** Cognition fine-tuned a smaller summarizer model. Anthropic ships auto-compact in Claude Code with structured note retention. Sourcegraph favors short threads and explicit fork over giant ones.

### 1.3 Tool design

- **Fewer, higher-level tools beat many low-level ones.** Anthropic ("more tools don't always lead to better outcomes"); GitHub MCP ships toolsets you opt into; SWE-agent uses a hand-curated handful. Tool overload is empirically a top failure mode.
- **Tool descriptions are part of the prompt.** Anthropic's measured 40% reduction in task time from improving *one* tool description. Treat tool docs like onboarding for a new hire.
- **Errors are prompts.** Lint failures on edits, pagination hints on truncation, MCP's structured 401s — every error must include the next move.
- **Edit formats matter more than edit models.** Aider's polyglot benchmark: GPT-4 Turbo went 20% → 61% by switching from SEARCH/REPLACE to unified diff. Asking models to wrap code in JSON consistently hurts. Cursor matches edit format to model training (patch for OpenAI, search-replace for Anthropic).
- **Sandbox by default.** SWE-agent's edit-linter, MCP's audience-bound tokens, Codex's two-layer (OS + approval), Gemini CLI's five sandbox backends, Cognition's microVMs. Tool calls are untrusted code.

### 1.4 The documentation file

`AGENTS.md` is the multi-vendor convergence point — endorsed by OpenAI Codex, Cursor, Sourcegraph Amp, GitHub Copilot Coding Agent, Google Jules, Aider, Zed, Warp, and others. Anthropic uses `CLAUDE.md` but documents bridging via `@AGENTS.md` import or symlink. GitHub's empirical study of 2,500+ repos identified six load-bearing sections: **Commands · Testing · Project Structure · Code Style · Git Workflow · Boundaries**.

What every vendor agrees belongs in the file:
- Build/test/lint commands with exact flags
- Coding style (with reference to linter, not the rules themselves)
- Project structure / module map
- Hard boundaries ("never do X")

What every vendor agrees does **not** belong:
- Anything the agent can derive from reading the code
- Detailed API docs (link, don't copy)
- Frequently-changing info
- Self-evident generic advice ("write clean code")

GitHub's three-tier boundary framework (always-do / ask-first / never-do) is the most cited shape. Anthropic targets ≤200 lines per file; Cursor caps rules at 500 lines; Copilot's review uses only first 4,000 chars. Everyone agrees: **bloat reduces adherence**.

### 1.5 Verification and evaluation

- **Verification is the highest-leverage practice.** Anthropic Best Practices: "the single highest-leverage thing you can do." Cognition's three reward channels (tests + rubrics + agentic graders). Sourcegraph's six-dimension agentic checklist.
- **Public benchmarks have known flaws.** SWE-bench Verified suffers from training-data contamination (GPT-5.2's CoT references the exact `edit_only` parameter the test expects, not in the issue text); 49 of 138 audited Verified tasks reject correct alternative solutions. **OpenAI publicly retired SWE-bench Verified** in 2026 for that reason.
- **Real labs use a portfolio.** Contamination-resistant external benchmark (SWE-bench Pro on held-out + commercial repos, LiveCodeBench post-cutoff) **plus** a private internal eval refreshed quarterly (cognition-golden, CursorBench-3, Sourcegraph CodeScaleBench) **plus** statistical rigor on small samples (Anthropic averages 25 trials per Verified score; Sourcegraph sized via Design of Experiments for 0.05 deltas) **plus** live-traffic regression history.
- **Evals as regression tests, not as decision tools.** Sourcegraph (Beyang Liu): "evals are primarily useful as unit tests and regression tests, to ensure a new model doesn't regress." A small set of representative tasks (~20, per Anthropic's own admission) is enough to discriminate prompt changes.
- **METR's RCT (July 2025) found AI made experienced devs 19% slower while they thought they were 20% faster.** The Feb 2026 update walks this back to "very weak evidence" due to selection bias. **No serious lab claims developer-time speedup based on RCT evidence.** Capability is measured; uplift is treated as open.

---

## 2. Genuine disagreements among the labs

These are not consensus — pick a side with rationale.

### 2.1 Multi-agent for coding: does it ever help?

- **Cognition (Don't Build Multi-Agents):** Single-threaded by default. Multi-agent is acceptable only when "writes stay single-threaded and the additional agents contribute intelligence rather than actions" (review loops, "smart friend" model escalation, hierarchical delegation with central writes).
- **Anthropic (Multi-Agent Research):** Multi-agent is great for **breadth-first research**, costs ~15× tokens, *not recommended for coding*. Subagents in Claude Code are for **context isolation**, not parallel writing.
- **Sourcegraph Amp:** Specialist subagents (Oracle = stronger reasoning model, Librarian = code search). Reads parallel, writes serial.
- **Replit Agent 4:** Parallel sub-agents in **isolated forks** with auto-merge resolving conflicts ~90% of time. The exception that proves the rule — Replit invests heavily in merge tooling because parallel writers break things.
- **Microsoft Magentic-One:** Generalist orchestrator + specialists for open-ended computer use. Their *own evidence* says debate-style multi-agent underperforms strong single-agent on coding.

**Recommendation:** for a backend-template developer agent, follow Cognition + Anthropic. Single primary writer. Subagents only for read-heavy investigation (Explore-style) and post-hoc review (Review-style with fresh context). No parallel writers.

### 2.2 Embeddings vs code-search-as-retrieval

- **Sourcegraph:** Dropped global embeddings explicitly. Code-graph queries (definitions, references, dependencies) plus hybrid keyword + on-demand embeddings. "Privacy + admin complexity + vector-DB scaling cost."
- **Cursor:** Server-side index, semantic search exposed as a tool. Privacy Mode adds ZDR contracts. The mechanism is opaque but documented as embeddings-backed.
- **Aider:** Tree-sitter AST + PageRank-style importance ranking, no embeddings. "Sending whole files is a bulky way to send code context."
- **Claude Code:** No first-party indexing. Relies on `grep` / `find` + glob via tools. Aider-style repo-map exists in some skills.

**Recommendation:** for a single-repo Go template, embeddings are overkill. Use ripgrep + Go toolchain (`go list`, `go doc`, AST tools) as the agent's retrieval surface. Cheap, deterministic, reproducible.

### 2.3 Edit format

- **Aider:** Plain text edit formats (whole-file for weak models, search-replace for strong, unified diff for strongest). JSON wrapping consistently hurts.
- **OpenAI:** Custom `apply_patch` tool format.
- **Anthropic:** Trained Claude on string-replace; that's what Claude Code uses natively.
- **Cursor:** Per-model — patch for OpenAI, string-replace for Anthropic.

**Recommendation:** Claude Code already uses Anthropic-trained string-replace via the Edit tool. Don't fight it. The only thing to add is a `gofmt` hook to deterministically clean output.

### 2.4 Where rules attach

- **Always-on (every CLAUDE.md / AGENTS.md):** simplest, but bloats context.
- **Path-globbed (Cursor `globs:`, Anthropic `.claude/rules/*.md` `paths:`, Amp frontmatter, GitHub `.github/instructions/*.instructions.md`):** rule loads only when matching files enter context.
- **Description-matched (Cursor "Apply Intelligently", Devin Knowledge):** model decides relevance from the description.
- **Manual (Aider `--read`, Cursor `@rule-name`):** user invokes explicitly.

**Recommendation:** path-globbed for Go domain rules (e.g., `.claude/rules/sqlc-migrations.md` with `paths: migrations/*.sql`). Always-on for the small core (build commands, hard boundaries). No "manual" except for one-off references.

---

## 3. Architecture proposal for `go-backend-template`

Grounded in the consensus above. Six discrete pieces, each with a purpose only it can serve.

### 3.1 `AGENTS.md` at repo root (with `CLAUDE.md` symlink)

Single source of truth. Six sections per GitHub's empirical study:

```
1. Commands       — exact `make` / `go test` / `golangci-lint` invocations
2. Testing        — how tests are organised, what `make test-unit` vs `test-integration` does
3. Project structure — internal/domain/, pkg/, cmd/, migrations/ in one paragraph each
4. Code style     — link to .golangci.yml; don't restate rules
5. Git workflow   — conventional commits prefix list, branch model, squash-merge
6. Boundaries     — three tiers:
   - Always do: run `make pre-commit` before claiming done; respect existing domain boundaries
   - Ask first: anything touching migrations/, pkg/policy
   - Never do: skip pre-commit (--no-verify); commit secrets; modify go.mod without explanation
```

Target: **under 150 lines.** `CLAUDE.md` symlinked to `AGENTS.md` so both vendors read the same file.

### 3.2 `.claude/rules/` for path-scoped knowledge

Files attached only when matching paths enter context. One file per domain concern, ≤80 lines each:

- `domain-architecture.md` — `paths: internal/domain/**` — the ports/adapters convention you already have
- `sqlc-and-migrations.md` — `paths: internal/domain/*/repo/sqlc/**, migrations/**` — sqlc workflow, migration rules
- `tests-pipeline.md` — `paths: **/*_test.go, internal/testkit/**` — TDD expectations, mockery v3
- `outbox-events.md` — `paths: internal/domain/*/events/**` — outbox pattern
- `worker-subscribers.md` — `paths: internal/worker/**` — subscriber conventions

Justification: Anthropic, Cursor, Amp, GitHub all support glob-attached rules; this is the consensus mechanism for "domain-specific guidance that shouldn't pollute every session."

### 3.3 `.claude/skills/` for procedural workflows

Skills only for procedures that meet the test: **>3 steps, sometimes needed, benefits from bundled scripts**. Candidates from the existing workflow:

- `add-domain` — full new-entity E2E (already had a skill for this; restore with refined content)
- `add-migration` — pgkit + sqlc regeneration + checksum update
- `run-pre-merge` — the canonical "before I claim done" checklist (`make lint && make test && go vet ./...`)
- `release-cut` — if you have one (semantic versioning, changelog, tags)

Each skill = `SKILL.md` (≤200 lines) + optional `scripts/`. Conform to **agentskills.io** open spec so they port to Cursor/Amp/Codex/etc. without changes.

### 3.4 Hooks for deterministic enforcement

The patterns Anthropic and Gemini CLI converge on. In `.claude/settings.json`:

- `PostToolUse(Edit|Write|MultiEdit, *.go)` → run `gofmt -w` + fast `go vet` on changed files.
- `PreToolUse(Bash, "*git commit*")` → block if `--no-verify` flag present.
- `Stop` → run a quick sanity check (e.g., `go build ./...`) and append warning to chat if it fails.

Hooks are non-negotiable behaviors — putting them in CLAUDE.md is "advisory; hooks are deterministic and guarantee the action happens" (Anthropic Best Practices).

### 3.5 Subagents for context isolation only

Three subagents, no more. Each in `.claude/agents/`:

- `explorer` — read-only, large tool budget, returns 1-2k-token summaries (Anthropic Multi-Agent pattern: "fan out read-heavy work, fan in summary"). Use for "where does X live" / "find all callers of Y" / "summarise this domain."
- `reviewer` — read-only, fresh context, runs after the implementer is done. Cognition's Devin Review pattern: "catches an average of 2 bugs per PR, of which 58% severe."
- `migration-author` — narrow specialist for SQL migrations + sqlc regeneration. Justified because it needs different tools (`make migrate`, `make sqlc`) and a different mental model from regular Go code.

The main agent does all writing. **No parallel writers, no swarm.**

### 3.6 Verification baked into the loop

Every multi-step change must end with `make pre-commit` or equivalent. Hook-enforced via Stop hook. This is Anthropic Best Practices' "highest-leverage thing." Without verification, the agent is the only feedback loop, and its bias toward its own code is documented.

### 3.7 Memory

- `MEMORY.md` (per Claude Code's auto-memory at `~/.claude/projects/<proj>/memory/`) for cross-session context. Already in use.
- **No** separate Letta/Mem0 stack. Overkill for a single-repo dev workflow. Auto-memory + AGENTS.md is enough.

---

## 4. What to build first (MVP)

Order matters. Each piece adds capability and is testable independently.

1. **`AGENTS.md` + `CLAUDE.md` symlink** (1-2h). Six sections per GitHub study. ≤150 lines.
2. **One real skill: `add-domain`** (2-3h). Use the workflow you already have documented. SKILL.md + maybe a `scripts/new-domain.sh` that the skill teaches the agent to use.
3. **Three hooks: gofmt / vet / pre-commit-on-stop** (1-2h). Test by editing a `.go` file and watching it auto-format.
4. **Three subagents: explorer / reviewer / migration-author** (3-4h). Each ≤80 line system prompt, with a clear contract.
5. **Path-scoped rules** (2-3h). Four to six rule files in `.claude/rules/` with `paths:` frontmatter.
6. **Eval harness** (4-6h). A `scripts/eval/` directory with 8-12 representative tasks (Cognition's "messy real-world" framing — start small, refresh quarterly). Each task has a fixture branch + assertion script. Run weekly to detect regressions.

Total: ~15-20 hours of focused work, all reversible (git-tracked, no infra).

---

## 5. How to validate it works (defensible reproducibility)

Per Sourcegraph's "evals as regression tests" stance:

1. Pick **8-12 real tasks** from your own commit history — actual issues you fixed in the last 3 months. Each gets a fixture branch + an `assertion.sh` script that returns 0 if the agent's solution is acceptable.
2. Bind the eval to a `make eval` target that runs each task in `claude --headless` mode, scores pass/fail, and writes a JSON report.
3. Run weekly. Compare to last week's report. Regressions → investigate.
4. **Do not** report this number publicly. It's a private regression test, not a marketing benchmark. (This is what every serious lab actually does — Anthropic's 20-query early eval, Cursor's CursorBench, Cognition's cognition-golden are all the same shape.)
5. Optionally add a public-benchmark baseline: SWE-bench Pro on a held-out subset, or LiveCodeBench post-cutoff. Use only as a *floor*. Do not chase the score.

---

## 6. What this rebuild deliberately does NOT do

Each is a "no" with a citation:

- **No multi-agent swarm.** Cognition's "Don't Build Multi-Agents" + Anthropic's Multi-Agent Research caveat about coding. Single-threaded writer.
- **No `team-lead → task-lead → workers` hierarchy.** The previous `experimental Agent Teams` pattern. Cognition's Principle 1 ("share context, share full traces") is harder to enforce in a hierarchy than in a single agent with subagent isolation.
- **No 22 specialist agents.** GitHub's 2,500-repo study favors narrow boundaries, not narrow agents. A handful of broad agents + path-scoped rules covers the same ground with less interference.
- **No embeddings / vector DB.** Sourcegraph's privacy-and-cost reasoning + Aider's repo-map proof-of-concept. ripgrep + AST tools are cheap and deterministic.
- **No autopilot plans-state machine.** Pure overhead for a single-developer project. Use Plan mode (Anthropic) when ambiguous; otherwise just go.
- **No bloated CLAUDE.md.** "Bloated CLAUDE.md files cause Claude to ignore your actual instructions" (Anthropic). Hard cap 150 lines.
- **No JSON-wrapped edit formats.** Aider's measured ~3× harm. Native string-replace via Edit tool.

---

## 7. The two prompt-injection attempts seen during research

Two of the 18 research agents reported that fetched pages contained fabricated `<system-reminder>` blocks attempting to alter agent behavior ("Auto Mode Active" injected into Anthropic engineering blog content during fetch). Agents flagged and ignored. Mentioned here for transparency; not a vulnerability in your repo.

---

## Bottom line

The convergent picture across Anthropic, OpenAI, Cognition, Google DeepMind, Microsoft, GitHub, Cursor, Sourcegraph, Replit, and academic groups (Princeton SWE-agent, OpenHands) is narrower than the marketing suggests:

- One primary agent, single-threaded for writes.
- Subagents for context isolation only.
- Hooks for invariants. Skills for procedures. Rules for context-scoped knowledge. AGENTS.md for always-on facts.
- Verify everything. Evals are regression tests, not scores to chase.
- Tools matter more than models, and edit formats matter more than tools.
- Reproducibility is a portfolio, not a number.

The detailed citations are in `raw-briefings.md`. The next step is to ship MVP §4 and let the eval harness tell us whether the architecture survives contact with real tasks.
