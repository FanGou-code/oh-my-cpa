# ADR 0035: Agents may run read-only SQL over a classified schema

- Status: Accepted
- Date: 2026-09-28

## Context

The capability registry deliberately exposed no SQL: every read went through a declared,
aggregating capability, and `docs/agent-capabilities.md` listed SQL among the things an agent must
not reach. In use, operators asked questions the declared reads were never shaped for - joins
between requests and pricing versions, a count grouped by a column no capability groups by, the
history of an ingest gap - and the agent could only say it could not see.

The database holds material no answer should include: the CPA management key (encrypted, in
`cpa_instances`), raw usage payloads that can carry client keys (`usage_inboxes`), the Agent's own
encrypted sessions and operations (`agent_documents`), operator preferences including the
Playground's saved conversation (`ui_preferences`), and raw upstream error bodies
(`error_events.body`).

## Decision

Two read capabilities, `database_schema` and `database_query`, give agents SQL under a policy the
repository owns (`internal/repository/readonly_query.go`), in four independent layers:

1. **The connection cannot write.** Queries run on a separate pool opened with `mode=ro` and
   `query_only`, with `SQLITE_LIMIT_ATTACHED` at zero and bounded value, statement, compound and
   expression sizes.
2. **The text is one SELECT.** A small tokenizer finds the real statement boundary (quotes and
   comments included) and accepts exactly one `SELECT` or `WITH` statement.
3. **The compiled program is checked, not the text.** Before running, the statement's `EXPLAIN`
   program is read: every b-tree it opens must belong to a table classified readable, every column
   it reads - through a table or an index - must not be a redacted one, and any write opcode,
   write transaction, virtual table or non-`main` database is refused. Checking the program is
   what makes aliases, views, subqueries and CTEs unable to route around the policy; the price is
   that table-valued functions (`json_each`, `pragma_*`) are unavailable.
4. **The result is bounded and masked.** At most 200 rows and 24 KiB, 500 characters per cell and
   5 seconds per query; text shaped like a credential is replaced and email addresses are masked.

Tables are opt-in. Every table is either readable (with its redacted columns named) or hidden with
a recorded reason, and a test fails when a migration adds a table that is neither, so a new table
is never readable by default.

Both capabilities are exposed to the built-in Agent and to MCP clients, whose management key is
already administrator-equivalent.

## Consequences

An agent can answer ad-hoc questions about OMC's own records without a new capability per
question, and the declared capabilities remain the preferred path - the system prompt says so,
because they aggregate on the server and carry the console's semantics. The query results still go
to the selected model and its upstream, as every capability result does (ADR 0027). Adding a table
now includes a classification decision, and adding a column that can carry a secret to a readable
table must add it to that table's redacted list in the same change.
