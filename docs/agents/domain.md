# Domain Docs

This repo uses a single-context layout: `GLOSSARY.md` at the repo root and architecture decision records in `docs/adr/`.

## Before exploring

- Read the root `GLOSSARY.md`, when present.
- Read ADRs in `docs/adr/` that affect the area being explored.

If these files do not exist, proceed silently. The `/domain-modeling` skill creates them lazily when terms or decisions are resolved.

## Use the glossary's vocabulary

When naming domain concepts in issues, proposals, hypotheses, or tests, use the glossary's terms and avoid synonyms it explicitly rejects.

If a needed concept is missing, reconsider whether the project uses that concept. Note real vocabulary gaps for `/domain-modeling`.

## Flag ADR conflicts

If a proposal contradicts an existing ADR, identify the ADR and explain why its decision should be reconsidered.
