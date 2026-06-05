# Requirements Document

## Introduction

Migrate the frontend package manager from npm to pnpm and add a dedicated frontend dev container in `docker-compose.dev.yml` that runs the Vite dev server with hot module replacement. Update all project documentation, build scripts, and agent instructions to reflect the new tooling.

## Glossary

- **Frontend_Dev_Container**: A Docker service in `docker-compose.dev.yml` running `pnpm dev` (Vite dev server) for local frontend development with HMR
- **Pnpm**: A fast, disk-efficient Node.js package manager that replaces npm in this project
- **HMR**: Hot Module Replacement — Vite feature that updates browser modules in-place without full page reload when source files change
- **Lockfile**: The `pnpm-lock.yaml` file that pins exact dependency versions (replaces `package-lock.json`)
- **Dockerfile_Frontend_Stage**: The first stage of the multi-stage Dockerfile that builds frontend assets
- **Build_System**: The collection of Makefile targets, Dockerfile stages, and compose services that build and run the project

## Requirements

### Requirement 1: Replace npm with pnpm in the frontend package installation

**User Story:** As a developer, I want the frontend to use pnpm as its package manager, so that I benefit from faster installs and stricter dependency resolution.

#### Acceptance Criteria

1. WHEN the Build_System performs a CI or automated build, THE Build_System SHALL run `pnpm install --frozen-lockfile` instead of `npm ci` for dependency installation in the `frontend/` directory
2. WHEN a developer runs frontend dependency installation on their host machine, THE Build_System SHALL use `pnpm install` instead of `npm install` in the `frontend/` directory
3. THE Lockfile SHALL be `pnpm-lock.yaml` at the `frontend/` directory root and SHALL be committed to version control
4. THE Build_System SHALL NOT reference `package-lock.json` in any Makefile target, Dockerfile stage, compose file, or script
5. WHEN the migration is complete, THE repository SHALL NOT contain a `package-lock.json` file in the `frontend/` directory

### Requirement 2: Update Dockerfile frontend stage to use pnpm (production build unchanged architecturally)

**User Story:** As a developer, I want the production Dockerfile to use pnpm for the frontend build stage, so that container builds are consistent with local development while preserving the single-binary architecture.

#### Acceptance Criteria

1. THE Dockerfile_Frontend_Stage SHALL install pnpm via `corepack enable` on the `node:22-alpine` base image before any dependency installation commands
2. THE Dockerfile_Frontend_Stage SHALL copy `package.json` and `pnpm-lock.yaml` (instead of `package-lock.json`) as the dependency install layer to preserve Docker layer caching
3. THE Dockerfile_Frontend_Stage SHALL run `pnpm install --frozen-lockfile` for dependency installation
4. THE Dockerfile_Frontend_Stage SHALL run `pnpm run build` to produce the static frontend assets
5. WHEN the Dockerfile is built, THE Dockerfile_Frontend_Stage SHALL produce output artifacts at `/src/frontend/build/` within the stage, matching the existing `COPY --from` path used by the Go builder stage
6. THE Dockerfile_Frontend_Stage SHALL NOT contain any `npm` commands (`npm ci`, `npm install`, `npm run`)
7. THE production architecture SHALL remain unchanged: frontend assets are embedded into the Go binary via `COPY --from` into `./internal/frontend/dist/` and served by the backend as a single container

### Requirement 3: Add frontend dev container to docker-compose.dev.yml (development only)

**User Story:** As a developer, I want a dedicated frontend container in the dev compose stack that runs the Vite dev server with HMR, so that frontend changes are reflected immediately in the browser without manual rebuilds during development.

#### Acceptance Criteria

1. THE Frontend_Dev_Container SHALL be defined as a service named `frontend` in `docker-compose.dev.yml`
2. THE Frontend_Dev_Container SHALL use `node:22-alpine` as its base image
3. THE Frontend_Dev_Container SHALL set the working directory to `/app` inside the container
4. WHEN the Frontend_Dev_Container starts, THE Frontend_Dev_Container SHALL enable corepack, run `pnpm install`, and then run `pnpm dev --host` to start the Vite dev server bound to all interfaces
5. IF `pnpm install` fails during container startup, THEN THE Frontend_Dev_Container SHALL exit with a non-zero status code and log an error message indicating the install failure
6. THE Frontend_Dev_Container SHALL bind-mount the host `./frontend` directory to `/app` in the container so that file changes on the host are visible inside the container
7. THE Frontend_Dev_Container SHALL expose container port 5173 mapped to host port 5173 for browser access
8. WHILE the Frontend_Dev_Container is running, THE Frontend_Dev_Container SHALL detect source file changes (`.svelte`, `.ts`, `.js`, `.css` files under `/app/src`) on the host and trigger HMR updates in connected browsers within 2 seconds of the file save
9. THE Frontend_Dev_Container SHALL mount a named volume `frontend_node_modules` at `/app/node_modules` to prevent host-platform binaries from overwriting container-installed dependencies
10. THE Frontend_Dev_Container SHALL declare the named volume `frontend_node_modules` in the top-level `volumes:` section of `docker-compose.dev.yml`
11. THE Frontend_Dev_Container SHALL NOT be defined in `docker-compose.yml`, which continues to use the existing multi-stage Dockerfile-based build for production

### Requirement 4: Update Makefile build targets to use pnpm

**User Story:** As a developer, I want all Makefile targets to use pnpm commands, so that the build system is consistent with the new package manager.

#### Acceptance Criteria

1. THE Build_System SHALL use `pnpm run build` instead of `npm run build` in the `build-frontend` Makefile target
2. THE Build_System SHALL use `pnpm` in place of `npm` for all package manager command invocations across all Makefile targets
3. WHEN `make build-frontend` is executed and pnpm is not found in the system PATH, THEN THE Build_System SHALL exit with a non-zero status and output an error message indicating that pnpm is required
4. WHEN `make build-all` is executed, THE Build_System SHALL invoke the updated `build-frontend` target using pnpm before compiling the Go binary

### Requirement 5: Update README.md documentation

**User Story:** As a developer, I want the README to reference pnpm commands and the new frontend dev container, so that onboarding documentation is accurate.

#### Acceptance Criteria

1. THE README SHALL replace all `npm install` references with `pnpm install`
2. THE README SHALL replace all `npm test` references with `pnpm test`
3. THE README SHALL replace all `npm run dev` references with `pnpm dev`
4. THE README SHALL document the frontend dev container in the Local Setup section, including the service name (`frontend`), its port (5173), and that it provides HMR for frontend development
5. THE README SHALL list `pnpm` (version 9+) in the development prerequisites alongside Node.js, Go, and Make

### Requirement 6: Update AGENTS.md project guidelines

**User Story:** As a developer or AI agent, I want AGENTS.md to reflect the pnpm migration and new dev container, so that automated tooling operates with correct commands.

#### Acceptance Criteria

1. THE AGENTS.md SHALL state pnpm as the package manager in the Frontend Conventions section, including a bullet specifying that `pnpm` is used for dependency installation, script execution, and lockfile management in the `frontend/` directory
2. THE AGENTS.md SHALL document the frontend dev container in the Infrastructure section, including the service name (`frontend`), base image (`node:22-alpine`), exposed port (5173), and its purpose (Vite dev server with HMR for local frontend development)
3. THE AGENTS.md SHALL add a `pnpm test` command entry to the Build and Test section for running frontend unit tests via Vitest
4. THE AGENTS.md SHALL add a `pnpm dev` command entry to the Build and Test section for running the frontend dev server locally outside Docker
5. IF the AGENTS.md contains any references to `npm` for frontend operations, THEN THE AGENTS.md SHALL replace those references with the equivalent `pnpm` commands

### Requirement 7: Update .github/copilot-instructions.md

**User Story:** As a developer using AI assistants, I want copilot instructions to reference the current stack including pnpm and the frontend dev container, so that AI-generated code uses correct tooling.

#### Acceptance Criteria

1. THE copilot-instructions SHALL list pnpm as the frontend package manager in the Expected Stack section alongside the existing frontend tooling (SvelteKit, TypeScript, adapter-static, Tailwind)
2. THE copilot-instructions SHALL describe the Frontend_Dev_Container in the Expected Stack Deployment bullet or a dedicated development section, including its purpose (Vite dev server with HMR), its compose file (`docker-compose.dev.yml`), and its host-accessible port (5173)
3. THE copilot-instructions SHALL NOT reference npm as a frontend package manager anywhere in the file

### Requirement 8: Update docs/MILESTONES.md

**User Story:** As a developer, I want the milestones document to reflect the pnpm migration as a completed enhancement, so that project history is accurate.

#### Acceptance Criteria

1. THE MILESTONES.md SHALL include a new "Post-MVP Enhancements" section (after the existing Milestone H section) that lists the pnpm migration and frontend dev container as completed items with checkmark (✅) indicators
2. THE MILESTONES.md post-MVP entry SHALL state that npm has been replaced with pnpm as the frontend package manager and that a frontend dev container with HMR has been added to `docker-compose.dev.yml`
3. THE MILESTONES.md SHALL preserve all existing milestone content (A through H) and the verification checklist unchanged
