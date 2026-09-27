# ADR-007: Whole-Token Templates and Shell-Free Translation

## Status
Accepted

## Context
`ArgsTemplate` has been a JSON array since M3. M6 must define how placeholders
are substituted and how config-author mistakes are handled. Every placeholder
in the shipped `trace` example occupies a complete token.

Contrary to the continuation prompt summary, the repository's `system_info`
template is not empty; it contains `-a`. The repository definition is retained,
and the empty-template case is covered independently.

## Decision
- Unlike earlier constructors that cannot fail, `translation.New(cfg)` returns
  `(*Translator, error)`. Before serving requests, it checks the Config with
  `ValidateConfig`, validates templates using the following rules, and compiles
  an independent internal map. This affects stage construction only; the
  existing `agent.Translator` interface does not change.
- A token without braces is a literal. A token containing braces is accepted
  only when the entire token is exactly `{name}` and `name` identifies a
  defined, non-duplicate parameter. Combined templates such as
  `--host={host}`, brace escaping, conditions, wildcards, and shell syntax are unsupported.
- Every token produces exactly one argument. Repeated placeholders are allowed.
  An input value is copied only; it is not reinterpreted as a template, split,
  joined, or quoted.
- Command is copied exactly from config. No part of it comes from Arguments or Metadata.
- An empty template produces an empty argument list. A missing placeholder
  value, including for an optional parameter, is an error. Tokens are not
  omitted and no implicit default exists. An optional parameter may be absent
  only when the template does not reference it.
- Translate runs only after Validation and Policy. It does not repeat type,
  range, pattern, or authorization checks. It still defensively rejects nil
  requests, unknown operations, and missing placeholder values without
  returning partial output.
- The internal snapshot does not depend on mutable config or caller-owned
  outputs. Core remains unchanged, and isolated M2/M4/M5 tests keep their placeholders.

## Alternatives Considered
1. **Build a shell string and perform textual replacement** — Rejected because
   it destroys argument boundaries and creates a command-injection path.
2. **Replace placeholders within any part of a token** — Rejected for M6 because
   whole tokens meet the current need without requiring more escaping and composition rules.
3. **Validate templates only for each request** — Rejected because config errors
   can be detected before serving traffic. Missing request values are still checked by Translate.
4. **Automatically remove the token for an absent optional parameter** —
   Rejected because it may change the meaning of an adjacent argument or flag.
   Conditional behavior needs a separate requirement and decision.

## Consequences
- Startup wiring must check Translator construction errors and must not build
  Core without a valid Translator.
- `ArgsTemplate` remains inert data in `LoadConfig`. Grammar validation occurs
  in `translation.New`, avoiding a reverse dependency from config.
- Config is the trusted source of command definitions. This stage does not
  establish that an executable or its tool-specific options are safe. Process
  execution belongs to M7.
- Input must already be validated, and roles remain unauthenticated under ADR-006.
