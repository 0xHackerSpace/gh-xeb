# Documentation

| Document | What it holds |
| --- | --- |
| [`memory.md`](memory.md) | Project context that is not in the code: what this repo is for, current state, environment quirks, gotchas. Read this first. |
| [`decisions.md`](decisions.md) | Every decision taken, with the small ones inline and the open questions at the end. |
| [`adr/`](adr/) | Architecture Decision Records — one file per substantial decision. |

Related, outside this folder:

- [`../AGENTS.md`](../AGENTS.md) — the conventions for writing code here, shared
  by Claude Code and GitHub Copilot.
- [`../README.md`](../README.md) — install and usage.

## Which file does a new note go in?

- It explains **why we chose X over Y**, and reversing it would be expensive →
  a new ADR in [`adr/`](adr/).
- It is a small choice whose reasoning is invisible in the code → the
  "Smaller decisions" section of [`decisions.md`](decisions.md).
- It is a fact about the project or the environment that a newcomer would
  otherwise rediscover painfully → [`memory.md`](memory.md).
- It is a rule about how to write code → [`../AGENTS.md`](../AGENTS.md).
