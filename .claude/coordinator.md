# Credo Review Agents — Shared Standards

## Finding Taxonomy (Required)

Every finding from every agent MUST include this header:

```
- Category: MODEL | SECURITY | EFFECTS | TRACEABILITY | READABILITY | TESTING | PERFORMANCE | CONTRACT
- Key: stable dedupe ID (e.g., SECURITY:client_secret:rotation:replay)
- Confidence: 0.0–1.0
- Action: CODE_CHANGE | TEST_ADD | DOC_ADD | ADR_ADD | MEASURE_ADD
```

**Category ownership (enforced routing):**

| Category      | Owner Agent       | Others MAY NOT emit |
|---------------|-------------------|---------------------|
| MODEL         | ddd-review        | ✗                   |
| SECURITY      | secure-design     | ✗                   |
| EFFECTS       | balance-review    | ✗                   |
| TRACEABILITY  | balance-review    | ✗                   |
| READABILITY   | complexity-review | ✗                   |
| TESTING       | testing-review    | ✗                   |
| PERFORMANCE   | performance-review| ✗                   |
| CONTRACT      | qa                | ✗                   |

If an agent discovers something outside its category, it notes a **handoff** rather than a finding:

```
Handoff to [agent]: [one-line description]
```

## Execution Order

Run agents in this sequence to minimize churn:

1. **QA** — contract trapdoors, missing transitions (no structural changes)
2. **Secure-by-design** — trust boundaries, invariants, TOCTOU, transaction scope
3. **DDD** — purity + orchestration + aggregate shape (after security constraints known)
4. **Balance (PASS C + D only)** — hop budget + effects visibility
5. **Performance** — measurement and load test plans (call paths now stable)
6. **Testing** — map findings to scenarios and integration tests
7. **Complexity (last)** — local readability polish after structure stabilizes

## Shared Non-negotiables

All agents enforce these rules; individual agents add domain-specific rules.

### Type-safe IDs
- Distinct types for each ID kind (`UserID`, `SessionID`, `TokenID`).
- No raw `string` or `uuid.UUID` as ID types in domain signatures.

### Parse at boundaries
- Use `Parse*` functions at trust boundaries (HTTP handlers, message consumers).
- Parse functions return `(T, error)`, not `T` with internal panic.
- Validation order: Origin → Size → Lexical → Syntax → Semantics.

### Error taxonomy
- Domain errors: typed, safe messages, stable codes.
- Infrastructure errors: wrapped with `%w`, not exposed to clients.
- No panic in production paths; `Must*` only in tests/init.

### Interface placement
- Interfaces live at the consumer site.
- Only create interfaces when: 2+ implementations exist OR hard boundary (test double, external system).

### Store purity
- Stores do I/O only—no business logic, no state transition decisions.
- Stores return `*Entity` (pointer), not copies.
- Domain construction goes through service layer using constructors.

### Domain purity
- `internal/domain/*` has no I/O imports (`database/sql`, `net/http`, `os`).
- Domain functions do not take `context.Context`.
- No `time.Now()` or `rand.*` in domain—receive as parameters.

### Transaction scope
- Transactions guard multi-step writes only.
- No external I/O inside transactions.
- Use outbox pattern for events.

## Output Deduplication

Orchestrator behavior:
1. Collect all findings from all agents.
2. Drop duplicates with identical `Key`.
3. When same `Key` from multiple agents (shouldn't happen with routing), keep highest `Confidence`.
4. Present findings grouped by `Category`, then by `Action`.

## Handoff Protocol

When an agent finds something outside its scope:

```markdown
### Handoffs

- → secure-design: Token refresh endpoint lacks replay protection
- → balance-review: Service method has 6 hops to complete flow
```

Do NOT elaborate on handoffs. The receiving agent will investigate.
