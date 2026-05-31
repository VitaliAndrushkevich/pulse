# Documentation Index

This directory contains the source of truth for project planning and execution.

## Files
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md): milestone-level delivery plan
- [TASKS.md](TASKS.md): execution task board with dependencies and done criteria

## Documentation Maintenance Rule
Docs are part of the deliverable and must be updated in the same change set as code whenever behavior or process changes.

## Required Doc Updates By Change Type
- API endpoint or payload changes:
  - Update OpenAPI artifact and any API docs
- Scheduler, websocket, or performance behavior changes:
  - Update architecture notes and relevant tasks
- Security, auth, token, or secret handling changes:
  - Update security notes and operator guidance
- Build, compose, or local workflow changes:
  - Update root README setup instructions

## Review Checklist
Before merging a change, confirm:
1. Related docs are updated
2. Task state is updated in [TASKS.md](TASKS.md)
3. Any scope changes are reflected in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)
