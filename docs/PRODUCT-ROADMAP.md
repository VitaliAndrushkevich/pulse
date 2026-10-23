# Product opportunities and development roadmap

English | [Русский](PRODUCT-ROADMAP.ru.md)

Draft: 2026-09-20. Planning baseline: `6fe74dd`.

This document proposes ways Pulse could solve additional problems for open-source users and companies. It is a prioritized set of hypotheses, not a release commitment or proof of market demand. The companion [technical-debt backlog](TECHNICAL-DEBT.md) records the reliability prerequisites. No features described here were implemented as part of this review.

## Recommended direction

Build Pulse into a self-hosted tool that helps small engineering teams verify three things:

1. Resources are reachable from the locations that matter.
2. Expected work, such as backups and scheduled imports, actually finishes.
3. Important application workflows return the expected result.

Make that evidence easy to configure, share, and act on through the UI, versioned API, and existing MCP server. The proposed positioning is **self-hosted availability and workflow monitoring for teams that want control over their infrastructure and configuration**.

Pulse already has broad protocol support, encrypted credentials, notifications, tags, history, dashboards, localization, and API/MCP integration. Native Slack, Matrix (unencrypted rooms), and Telegram channels now complement email/webhook, with safe credential editing and testing unsaved settings. See [notification setup](NOTIFICATIONS.md). The opportunity is to turn those capabilities into repeatable operational workflows. Additional protocols should follow concrete user demand.

Keep the core local-first and deployable as one Go application with embedded UI and PostgreSQL/TimescaleDB. Optional remote probes or browser runners would be deliberate architecture extensions requiring a separate design. Team access within one installation does not require multi-tenant SaaS infrastructure.

## Who might adopt it, and why?

These are discovery targets rather than verified customer segments.

| Audience | Problem to validate | Pulse workflow to offer |
|---|---|---|
| Self-hosters and open-source maintainers | A service is reachable, but a backup or scheduled job silently stopped | Availability checks plus heartbeat monitoring and simple notifications |
| Small SaaS/product teams | Login, search, or an API workflow breaks while `/health` still returns 200 | Response assertions, then bounded multi-step API checks |
| Platform/DevOps teams | Monitor configuration drifts across environments and ownership is unclear | Declarative configuration, bulk policies, service owners, deployment annotations |
| Companies running private infrastructure | Central monitoring cannot see the network path employees or internal services use | Outbound-connected private probes and location-specific results |
| Support and customer-success teams | They repeatedly ask engineering whether an incident affects customers | Curated status pages and incident updates |
| Agencies operating several customer installations | They need repeatable setup and customer-specific reports | Templates and separate installations first; evaluate tenant isolation only after demand |

## Opportunity priorities

Effort is relative to the first useful version: M spans several existing components, L adds a substantial model/workflow, and XL changes execution or deployment architecture. Confidence describes technical fit, not proven demand.

| ID | Opportunity | Suggested order | Effort | Principal value |
|---|---|---|---|---|
| PF-01 | Heartbeat and scheduled-job monitoring | Next | M | Catch silent failures outside request/response monitoring |
| PF-02 | Maintenance, alert policies, and incident ownership | Next | M–L | Reduce noise and make alerts actionable |
| PF-03 | Bulk management and configuration as code | Next | M–L | Reproducible setup and lower administration cost |
| PF-04 | Response assertions and API workflows | Next / expand incrementally | M, then L | Detect application failures behind healthy endpoints |
| PF-05 | Status pages and incident communication | After pilot feedback | M–L | Give users and support teams a shared account of impact |
| PF-06 | Team access, SSO, and audit history | Company-adoption gate | L | Shared operation without a shared admin credential |
| PF-07 | Private probes and location-aware monitoring | Validate first | XL | Diagnose private-network and regional availability |
| PF-08 | Availability objectives and reliability reports | After trustworthy coverage | L | Turn observations into reviewable service evidence |
| PF-09 | Deployment context and dependency-aware incidents | After service ownership | L | Shorten investigation and reduce related alert floods |
| PF-10 | Community recipes and useful MCP extensions | Continuous, bounded | M | Easier contribution, onboarding, and automation |

## PF-01: Heartbeat and scheduled-job monitoring

**Problem:** A server can stay online while backups, invoice generation, certificate renewal, synchronization, or nightly imports stop running.

**First useful version:** A heartbeat resource with an expected interval, grace period, last success, and scoped ingestion credential. A simple HTTP call reports success or explicit failure. Persist the next expected deadline and alert when it is missed. Provide shell, cron, and CI examples.

**Follow-up:** Start/success/failure events with run identifiers and duration, followed by timezone-aware cron schedules. Handle concurrent runs, duplicates, late arrivals, and clock boundaries explicitly. A successful backup command is evidence of job completion; it does not prove that a backup can be restored.

**Dependencies:** Durable alerts and freshness semantics (TD-004/005/009). Keep ingestion separate from administration privileges and preserve the API-first contract. An independently hosted heartbeat receiver can also detect loss of Pulse itself; a receiver on the same failed host cannot.

**Pilot:** Three users instrument two real scheduled jobs each and verify missed-run, explicit-failure, and recovery alerts. Measure setup time and false alarms before adding complex schedules.

**Reference:** Healthchecks documents completion pings and grace periods for cron monitoring. This is evidence of an established workflow, not proof of unmet demand for another implementation. [Healthchecks cron monitoring](https://healthchecks.io/docs/monitoring_cron_jobs/).

## PF-02: Maintenance, alert policies, and incident ownership

**Problem:** Brief failures and planned changes create noise; during a real incident, nobody knows whether someone is already investigating.

**First useful version:** Temporary silences with automatic expiry, one-off maintenance windows, independent failure/recovery thresholds, and reusable notification policies assignable to monitors or tag selections. Existing `n_failures_in_row` support is a foundation; define the relationship between raw results, incidents, and notification thresholds. Add an incident acknowledgement and runbook link.

**Follow-up:** Recurring maintenance with timezone/DST semantics, ownership groups, and simple escalation to another destination. Integrate with an existing on-call tool through webhooks before building schedules and rotations.

**Dependencies:** Correct per-binding evaluation, persisted delivery, and recovered reminders (TD-003/004/005). Maintenance should preserve observations while explicitly controlling notification and reporting behavior.

**Pilot:** Run controlled short outages, planned maintenance, and sustained failures. Measure nuisance notifications and whether sustained failures still reach the intended owner within the configured policy. Compare with each participant's baseline.

**Reference:** Gatus exposes separate failure and success thresholds. [Gatus alerting](https://gatus.io/docs/alerting-getting-started).

## PF-03: Bulk management and configuration as code

**Problem:** Creating similar monitors by hand is repetitive, changes drift between environments, and reviewing a monitoring change is difficult.

**First useful version:** Clone monitors, bulk-edit selected settings/tags/policies, and export/import versioned configuration with secret references. Add a small CLI or documented script offering validate, plan, and apply over the existing API. Stable UUIDs and idempotent PUT are already available foundations.

**Semantics to settle:** UI-managed versus file-managed resources, stable identity, partial apply failures, drift reporting, and deletion. Applying the same file twice must not create duplicates. Removal from a file must not silently delete unrelated resources; deletion should be an explicit planned operation. Exports must not reveal stored secret values.

**Follow-up:** CI examples, reusable environment parameters, and importers for formats users actually bring. A Terraform provider is a later option once resource lifecycle and demand are established.

**Dependencies:** Contract consistency (TD-012), notification policy model, and correct pagination (TD-007). Future API changes must co-change affected MCP tools.

**Pilot:** A team provisions a representative environment, reviews a change in a diff, reapplies without drift, and recovers from an interrupted apply. Measure time versus manual setup. Validate importers on real, permission-to-use exported fixtures and report unsupported fields.

## PF-04: Response assertions and API workflows

**Problem:** HTTP 200 or successful protocol negotiation does not establish that login, search, account data, or a business operation works.

**First useful version:** Extend HTTP checks with request bodies, bounded response inspection, JSON-path/value assertions, header assertions, and clear expected-versus-observed failure details. Existing HTTP status/TLS checks and gRPC request support are foundations, not new features to rebuild.

**Follow-up:** A bounded sequence such as obtain a test token → fetch a seeded account → assert a required field. Add variable extraction, per-step results, explicit time budgets, and secret redaction. Prefer read-only or dedicated synthetic accounts and controlled test data. Define cleanup before allowing checks that create business records.

**Later:** Browser journeys using an optional isolated runner. Browser execution adds significant resource usage, version management, sandboxing, and artifact retention; do not place it into the default lightweight core without a design decision.

**Dependencies:** Execution correctness (TD-001/002), credential handling, and bounded response size/execution. Assertion failures should show enough evidence to act without storing sensitive response bodies by default.

**Pilot:** Deliberately break a response field while the endpoint still returns 200. Pulse detects the break and identifies the failed assertion. Then validate one real API workflow end to end, including expired credentials and a slow step.

**Reference:** Checkly describes synthetic monitoring for browser and API workflows. Its existing offering supports the usefulness of the category; Pulse's self-hosted implementation still needs user validation. [Checkly synthetic monitoring](https://www.checklyhq.com/product/synthetic-monitoring/).

## PF-05: Status pages and incident communication

**Problem:** Customers and support teams need a concise view of service impact without access to infrastructure credentials or the administration UI.

**First useful version:** An opt-in read-only page of explicitly selected service components, with current status, maintenance, and manual incident updates. Map internal monitors to public components; keep internal targets, detailed errors, and credentials private. Add an embeddable status badge for open-source projects.

**Follow-up:** Multiple pages, private audience access, custom domains, and subscriber notifications, driven by demand. A page hosted with Pulse shares its failure domain; state that limitation and consider a separately served snapshot for users needing outage-independent communication.

**Dependencies:** Freshness/coverage semantics (TD-009), publication boundaries, and PF-02 incident/maintenance behavior. This is future product scope, not an existing capability.

**Pilot:** A maintainer publishes a page without exposing internal addresses and updates a simulated incident. Ask a non-engineer to identify affected services and the latest update without help.

**Reference:** Uptime Kuma documents public status pages and multiple pages. These are established community expectations rather than a unique differentiator. [Uptime Kuma status pages](https://github.com/louislam/uptime-kuma/wiki/Status-Page).

## PF-06: Team access, SSO, and audit history

**Problem:** Companies need several people and automation clients to operate monitoring with different permissions, while retaining an account of configuration changes.

**First useful version:** Multiple users in one installation, viewer/operator/admin roles, scoped API tokens, and audit events for configuration, credentials, policies, and access changes. Define session/token revocation and enforce authorization in backend handlers, not just UI controls.

**Follow-up:** OIDC SSO and service ownership groups when pilot companies require them. Decide whether access is instance-wide or resource-scoped before adding fine-grained permissions. Keep audit records free of raw credentials. Retention/export should be documented; an audit log alone does not establish compliance.

**Dependencies:** Reviewed authorization model across REST, WebSocket, and MCP; TD-008/011 operational recovery. MCP's existing read/write mode is useful but is not a replacement for backend token scopes.

**Pilot:** A viewer can inspect permitted status but cannot mutate resources through any interface. An operator can manage monitors without administering users. Revocation and audit attribution work across the tested interfaces. Validate the actual identity provider used by a pilot company.

## PF-07: Private probes and location-aware monitoring

**Problem:** A single monitoring location cannot distinguish a resource outage from a local network problem or observe every private network path.

**First useful version:** An optional lightweight outbound-connected agent running the existing protocol checkers. Assign monitors to a named location and show per-location results and probe freshness. Retain local execution as the default.

**Design prerequisites:** Authenticated enrollment and revocation, bounded work leases, credential distribution, version compatibility, clock skew, result identity, offline buffering, and recovery. A disconnected probe means missing evidence, not automatic proof that every assigned target is down.

**Follow-up:** Regional comparisons and explicit quorum policies. Keep individual observations visible when producing an aggregate. Avoid automatic high-availability claims for the central scheduler/database.

**Dependencies:** TD-001/002/005/009/010 and a reviewed protocol/architecture proposal. This deliberately expands the single-deployment model and has the highest maintenance cost among early candidates.

**Pilot:** Recruit a team with a concrete private-network need. Run two locations, disconnect one probe, and break one target from only one location. Users should be able to tell those three outcomes apart.

## PF-08: Availability objectives and reliability reports

**Problem:** Teams need evidence for reliability reviews and customer conversations, including when observation was incomplete.

**First useful version:** Service grouping, an explicitly defined availability objective, and downloadable period reports showing observed availability, check-success ratio, coverage, incidents, maintenance, and methodology. Start with CSV/JSON and a printable view.

**Follow-up:** Error-budget views, trends, and scheduled summaries. Only offer burn-rate alerts after the chosen availability model is precise enough to support them. Customer SLA calculations require agreement on that customer's definitions; do not infer them from a generic uptime percentage.

**Dependencies:** TD-009 is a hard gate. Align configured history retention with report windows; retain appropriate aggregates if raw history expires. PF-02 supplies maintenance rules and PF-06 supplies access controls when reports are shared internally.

**Pilot:** A team can reproduce a report from documented sample data and explain what missing observations mean. Test a new monitor, a week-long collection gap, changing intervals, and a report window longer than retained raw history.

## PF-09: Deployment context and dependency-aware incidents

**Problem:** During an outage, teams spend time correlating changes and sorting many downstream failures caused by one dependency.

**First useful version:** Deployment/change annotations ingested through the API, plus service owner and runbook metadata. Show annotations on the same timeline as observed failures. A nearby deployment is context, not proof of causation.

**Follow-up:** Explicit user-defined dependency links and grouping of related incidents. Preserve all raw results. If notification suppression is offered, show the reason, bound its duration, and provide an override so incorrect dependency configuration does not hide unrelated failures.

**Dependencies:** Reliable incidents (TD-002), PF-02 policies, and stable service identity. Do not start with automatic root-cause inference or autonomous remediation.

**Pilot:** Attach a CI deployment event and investigate a deliberately broken dependency. Measure whether the team finds the owner/runbook faster and receives fewer redundant notifications without losing an independent failure.

## PF-10: Community recipes and useful MCP extensions

**Problem:** Users need a short path from installation to useful monitoring, and contributors need bounded ways to help without learning the whole application.

**First useful version:** Versioned example configurations for common stacks, notification webhook recipes, heartbeat snippets, a contribution guide with a protocol-checker example, and a reproducible local development walkthrough. Add a demo populated with synthetic data and clearly documented setup/reset instructions.

**MCP extension:** Build on existing status/history/incident tools with a structured incident evidence bundle: affected resources, failed assertions, time window, recent deployments, coverage gaps, and links to runbooks. Keep conclusions grounded in observations. Server-enforced scoped tokens should govern access; service responses are untrusted data, not instructions for an assistant.

**Follow-up:** Import adapters and destination presets requested by users. Prefer reviewed data/templates over a runtime arbitrary-code plugin system initially. Templates need schema validation, compatibility notes, safe secret placeholders, and test fixtures. Keep provider-specific retry/response rules explicit where a generic webhook is insufficient.

**Pilot:** An unfamiliar contributor adds a recipe or checker using the guide and CI. A new user deploys a recipe and receives an intentional test alert. An MCP client obtains useful incident evidence without receiving secrets or making an unexpected write.

## Suggested release sequence

| Milestone | Deliverable | Exit gate |
|---|---|---|
| R1: Trustworthy core | P1 debt, failure/restart regressions, initial operational runbook | No known reviewed P1 failure remains; committed alerts recover and shutdown is coordinated |
| R2: Scheduled work and quieter operations | PF-01 interval heartbeats, PF-02 initial controls, a small PF-10 recipe set | Pilot users verify missed jobs and sustained outages with acceptable noise |
| R3: Application correctness and repeatable setup | PF-04 single-request assertions and PF-03 validate/plan/apply | One real workflow failure detected despite HTTP 200; configuration reapply is idempotent |
| R4: Company or community adoption | Select PF-05 or PF-06 from pilot evidence; expand PF-08 only after TD-009 | A named pilot adopts the workflow and validates the acceptance scenario |
| R5: Expanded execution | PF-07 or browser journeys only if validated demand warrants the cost | Architecture and support burden are accepted before implementation |

Run TD-010 capacity work alongside these releases and publish results before making stronger scale claims. Complete TD-011/012 before recommending broad production adoption. The sequence is an ordering proposal, not a calendar estimate; choose one substantial new capability at a time.

## Validate demand before broadening scope

Recruit three to five pilot users across self-hosting, small product teams, and private infrastructure. Ask for a recent incident or missed job and the actual steps they used to detect and resolve it. Avoid starting with a feature wish list.

Record the current workaround, frequency, impact, decision-maker, deployment constraints, and willingness to try a prototype. Observe installation, first monitor, first alert, recovery, and upgrade. Track time to first useful alert, continued use after several weeks, nuisance alerts, setup failures, and configuration effort. Define baseline and target per pilot rather than inventing universal performance or commercial promises.

Advance an opportunity when at least two independent users describe the same recurring problem and one is willing to test the first version on real infrastructure. For a large architecture change, require a concrete deployment scenario and a maintainer willing to own compatibility and upgrades. Revisit these gates if the main objective is personal learning rather than adoption.

## Open-source and company sustainability

Preserve a useful self-hosted core and clear contribution boundaries. Community recipes, translations, adapters, documentation, and protocol tests offer smaller entry points than a broad plugin platform. Publish reproduction instructions and measured deployment limits so maintainers can triage issues efficiently.

If commercial sustainability becomes a goal, validate paid support, upgrade assistance, deployment help, or managed operation before investing in billing and multi-tenancy. Company appeal can come from dependable operation, clear access controls, repeatable configuration, and good documentation within the same open-source installation. Any future packaging or license decision should be explicit; this roadmap does not change the current license or promise a paid edition.

Defer a full observability suite, log warehouse, custom on-call rotation product, automatic remediation, broad protocol expansion, and a general plugin marketplace until concrete use cases justify their maintenance cost. The closest high-value expansion is detecting silent job failures and incorrect application responses with the infrastructure Pulse already has.
