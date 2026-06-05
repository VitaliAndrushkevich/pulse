# Implementation Plan: pnpm Frontend Dev Container Migration

## Overview

Migrate the Pulse frontend from npm to pnpm and add a dedicated frontend dev container with Vite HMR support. Tasks are ordered by dependency: lockfile generation first (everything else depends on it), then Dockerfile and compose changes, then Makefile, and finally documentation updates which can be parallelized.

## Tasks

- [x] 1. Generate pnpm lockfile and remove npm lockfile
  - [x] 1.1 Generate `pnpm-lock.yaml` from existing `package.json` in `frontend/`
    - Run `pnpm import` in `frontend/` to convert `package-lock.json` to `pnpm-lock.yaml`, or run `pnpm install` to generate it fresh
    - Verify `frontend/pnpm-lock.yaml` exists and is valid
    - _Requirements: 1.3_
  - [x] 1.2 Delete `frontend/package-lock.json`
    - Remove the npm lockfile from the repository
    - Verify `frontend/package-lock.json` no longer exists
    - _Requirements: 1.4, 1.5_

- [x] 2. Update Dockerfile frontend stage to use pnpm via corepack
  - [x] 2.1 Modify Dockerfile Stage 1 to use corepack and pnpm
    - Add `RUN corepack enable` after the `WORKDIR` instruction
    - Change `COPY frontend/package.json frontend/package-lock.json ./` to `COPY frontend/package.json frontend/pnpm-lock.yaml ./`
    - Replace `RUN npm ci` with `RUN pnpm install --frozen-lockfile`
    - Replace `RUN npm run build` with `RUN pnpm run build`
    - Ensure output path remains `/src/frontend/build/` for the Go builder stage `COPY --from`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

- [x] 3. Add frontend dev container to docker-compose.dev.yml
  - [x] 3.1 Add `frontend` service definition to `docker-compose.dev.yml`
    - Use `node:22-alpine` as the image
    - Set `container_name: pulse-frontend-dev`
    - Set `working_dir: /app`
    - Set command: `["sh", "-c", "corepack enable && pnpm install && pnpm dev --host"]`
    - Map port `5173:5173`
    - Bind-mount `./frontend:/app`
    - Mount named volume `frontend_node_modules` at `/app/node_modules`
    - Add `depends_on` with `backend: condition: service_started`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9_
  - [x] 3.2 Declare `frontend_node_modules` in the top-level `volumes:` section
    - Add `frontend_node_modules:` alongside existing `pulse-postgres-dev-data:`
    - _Requirements: 3.10_

- [x] 4. Update Makefile build-frontend target to use pnpm with guard check
  - [x] 4.1 Add pnpm availability guard and replace npm with pnpm in `build-frontend`
    - Add `@command -v pnpm >/dev/null 2>&1 || { echo "Error: pnpm is required but not found in PATH. Install it: https://pnpm.io/installation"; exit 1; }` as the first line
    - Replace `cd frontend && npm run build` with `cd frontend && pnpm run build`
    - Verify `build-all` target still works (it depends on `build-frontend`)
    - _Requirements: 4.1, 4.2, 4.3, 4.4_

- [x] 5. Checkpoint - Verify infrastructure changes
  - Ensure Dockerfile builds successfully with `docker build -t pulse:test .`
  - Ensure `docker compose -f docker-compose.dev.yml config` validates without errors
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Update README.md documentation
  - [x] 6.1 Update README.md with pnpm references and frontend dev container docs
    - Replace `npm install` with `pnpm install` in all occurrences
    - Replace `npm test` with `pnpm test` in all occurrences
    - Replace `npm run dev` with `pnpm dev` in all occurrences
    - Add `pnpm` (version 9+) to the Development Prerequisites section
    - Document the frontend dev container in the Local Setup section: service name `frontend`, port 5173, HMR support
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_

- [x] 7. Update AGENTS.md project guidelines
  - [x] 7.1 Update AGENTS.md with pnpm and frontend dev container information
    - Add pnpm as the package manager in the Frontend Conventions section (dependency installation, script execution, lockfile management)
    - Document the frontend dev container in the Infrastructure section: service name `frontend`, base image `node:22-alpine`, port 5173, purpose (Vite dev server with HMR)
    - Add `pnpm test` command to the Build and Test section for frontend unit tests via Vitest
    - Add `pnpm dev` command to the Build and Test section for running frontend dev server locally
    - Replace any remaining `npm` references for frontend operations with `pnpm`
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

- [x] 8. Update .github/copilot-instructions.md
  - [x] 8.1 Update copilot-instructions.md with pnpm and frontend dev container
    - Add pnpm as the frontend package manager in the Expected Stack section alongside SvelteKit, TypeScript, adapter-static, Tailwind
    - Document the Frontend_Dev_Container: purpose (Vite dev server with HMR), compose file (`docker-compose.dev.yml`), host port 5173
    - Remove any `npm` references as a frontend package manager
    - _Requirements: 7.1, 7.2, 7.3_

- [x] 9. Update docs/MILESTONES.md
  - [x] 9.1 Add Post-MVP Enhancements section to MILESTONES.md
    - Add a new "Post-MVP Enhancements" section after the existing Milestone H section
    - Include pnpm migration and frontend dev container as completed items with ✅ indicators
    - State that npm has been replaced with pnpm and that a frontend dev container with HMR has been added to `docker-compose.dev.yml`
    - Preserve all existing milestone content (A through H) and verification checklist unchanged
    - _Requirements: 8.1, 8.2, 8.3_

- [x] 10. Final checkpoint - Ensure all changes are consistent
  - Verify no remaining `npm` references in modified files (Dockerfile, Makefile, docker-compose.dev.yml)
  - Ensure `frontend/pnpm-lock.yaml` exists and `frontend/package-lock.json` does not
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- No tasks are marked optional because there are no property-based tests or unit tests to write — this is a tooling/config migration
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Documentation tasks (6–9) are independent of each other and can be executed in parallel
- The frontend dev container is development-only; production `docker-compose.yml` remains unchanged
- Existing 141 frontend unit tests must continue passing after migration (verified via `pnpm test`)

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "3.1"] },
    { "id": 3, "tasks": ["3.2", "4.1"] },
    { "id": 4, "tasks": ["6.1", "7.1", "8.1", "9.1"] }
  ]
}
```
