# Mecatl operator design proposal

Status: proposal for review. Author: Mecatl. Review date: 2026-11-06.

The proposed operator manages platform configuration for `mecak8s`.
`mecak8s` remains the runtime. Runtime APIs and backing stores retain
application data and execution authority. This document proposes a target
architecture and delivery phases. This document does not describe a shipped
operator.

## Decision summary

| Area | Proposed decision |
|---|---|
| Runtime | Keep `mecak8s`. Keep sessions, runs, approvals, and learning records outside Kubernetes custom resources. |
| Installation | Use one `Harness` for one coherent installation identity. Use direct server-side apply for new installations. |
| Offerings | Use `HarnessProfile` to compose approved capabilities. A profile is neither a Deployment nor a YAML overlay. |
| Ownership | Create a resource only for an independently owned lifecycle. Do not create one resource per configuration struct. |
| Scope | Use namespaced resources in provisional group `platform.mecatl.dev/v1alpha1`. Reject cross-namespace references initially. |
| Authorization | Separate author, approver, attacher, and runtime user authority. Require authorized use for same-namespace references too. |
| Publication | Resolve and approve a complete graph before publishing an immutable revision. Add runtime acknowledgement and durable session binding. |
| Isolation | Start with trusted teams. Do not introduce a `Tenant` resource or claim hostile-tenant isolation. |
| Delivery | Ship useful slices. Do not require every proposed resource for the first installation. |

## Goals and non-goals

The platform team publishes capabilities with explicit security limits.
An application-team maintainer assembles an approved offering.
An end user signs in at one service address through Studio, `mecatui`, or an
SDK. The end user needs no kubeconfig.

The operator must make configuration ownership, activation, and failure
states visible. The operator must preserve existing permission checks and
execution boundaries. The operator must support recovery without losing
session bindings or granting old executors new authority.

The operator does not process agent turns. The operator does not replace
ToolHive, an identity provider, a database operator, or the runtime scheduler.
This proposal does not promise financial quotas, exactly-once tool effects,
or high availability from a replica count alone.

## Terms and roles

| Term | Meaning in this proposal |
|---|---|
| Custom resource (CR) | A Kubernetes object that declares platform configuration. |
| Custom resource definition (CRD) | The Kubernetes schema and API registration for a CR type. |
| Common Expression Language (CEL) | The expression language for local CRD validation rules. |
| Model Context Protocol (MCP) | The protocol that connects the runtime to external tools. |
| Open Container Initiative (OCI) artifact | Registry content that a digest can identify immutably. |
| OpenID Connect (OIDC) | The identity protocol used to verify authentication claims. |
| PersistentVolumeClaim (PVC) | A Kubernetes request for persistent storage. |
| Principal | A verified human or service identity that can receive authority. |
| Quiescence | A maintenance state with no new work and no active mutations. |
| Capability | A runtime function with explicit prerequisites and availability. |
| Offering | The approved capabilities and defaults that a profile presents to users. |
| Publication | An immutable, validated configuration graph and its retained content. |
| Revision | The digest that identifies an immutable publication or template. |
| Binding | A durable association between a runtime object and a publication revision. |
| Entitlement | Current permission to consume a capability or selected model. |
| Attachment | A reference that adds an approved capability to an offering. |
| Quarantine | A restricted environment for fetching and validating unpublished content. |
| Fencing | A check that prevents an obsolete owner from starting new mutations. |
| Authorization freshness | The maximum age of authority information that the runtime can accept. |
| Compare-and-swap (CAS) | A write that succeeds only when the expected revision still matches. |
| Server-side apply (SSA) | Kubernetes field management with explicit field owners. |

| Role | Authority |
|---|---|
| Platform administrator | Own installation identity, security limits, endpoint trust, and approved integrations. |
| Content author | Propose instructions, procedures, or specialist definitions. |
| Approver | Approve an exact digest for a stated scope and policy. |
| Application-team maintainer | Attach approved capabilities and choose permitted defaults. |
| End user | Consume entitled offerings through runtime APIs. |
| Automation principal | Run a platform schedule under a verified service identity. |

Kubernetes write access grants author authority only. An author cannot
self-approve content unless a separate grant explicitly permits approval.
An attacher needs use authority for every reference. Runtime users need
current entitlements, even when an approver accepted the publication earlier.

A CR write does not record approval. A GitOps service account identifies the
writer, not a human reviewer. Reconciliation must not substitute controller
identity for approval or permitted reference use. `AccessPolicy` runtime
grants alone do not authorize authoring, approval, or attachment.

The platform must control authoritative approval and reference-use records.
Those records must remain independent of application-team control.
Application teams must not modify those records. Each record must bind the
verified approver identity, resource UID, full content and dependency digest,
scope, and policy revision. A reference-use record must also bind the
authorized attacher and target resource UID. The publisher must check current
reference-use grants after any dependency changes.

The final API for these records remains undecided. Phase 2 must select and
implement that API before it permits delegated publication. Controller
credentials and GitOps review history alone cannot meet this prerequisite.

## Current behavior and design gaps

The following facts constrain the target design. The evidence section links
the implementation and architecture sources.

| Current behavior | Required design response |
|---|---|
| `harness_context` composes five content kinds with ordered sources. | Preserve combine, replace, exclusions, and collision rules. |
| Source IDs identify trusted registrations. | Keep acquisition URLs separate from runtime source identities. |
| Live sessions retain loaded project guidance. Commands use template sources. | Define activation per content kind. Do not claim uniform per-run reload. |
| Soul selection uses one process user slot. | Keep one selected persona as a profile field or reference. Do not model authenticated-user memory as soul. |
| Agent definitions can include host hooks and inline MCP configuration. | Define a restricted application-team subset before accepting definitions. |
| `models.allowlist` has no effect. Unlisted model IDs pass through. | Add runtime entitlement enforcement before promising restricted model offerings. |
| Token caps act as boundary-checked runaway brakes. | Do not describe token caps as financial or global descendant quotas. |
| `mecak8s` supplies `UserModelDir`, but not project `MemoryDir`. | Define capability-specific durable storage before advertising memory availability. |
| Ownership enforcement rejects remote memory and learning combinations. | Keep those combinations unavailable until compatible storage and isolation exist. |
| Redis persists sessions, but scheduled-result delivery uses an in-memory queue. | Treat result delivery as a separate recovery gap. |
| Execution recipes load at provider startup. | Coordinate quiescence for recipe changes and provider upgrades. |
| Remote execution excludes Subagent, Team, Parallel, and repository-source ingestion. | Reject profiles that require these combinations. |
| Profile selection and durable publication binding are not current session contracts. | Add runtime APIs and stored bindings in a later phase. |

The private `execution.mecatl.dev/v1alpha1` `ExecutionEnvironment` resource
belongs to the execution provider. Its controller owns infrastructure
allocation and claims. The proposed operator must not compete for those
fields or create an alternative allocation controller.

## Resource ownership and composition

```text
Platform administrator                     Application-team maintainer
  Harness + approved capability resources    HarnessProfile + content proposals
                 \                           /
                  -> resolve, authorize, approve
                     -> immutable publication
                        -> mecak8s acknowledgement
                           -> runtime offering catalogue
                              -> user session and DB binding

ExecutionTemplate -> provider recipe catalogue -> ExecutionEnvironment controller
Automation -> runtime schedule definition      -> DB scheduler and execution claims
ToolHive -> MCPServer / VirtualMCPServer        -> MCPConnection endpoint reference
```

The graph shows configuration dependencies, not cascading deletion ownership.
A profile must identify one `Harness`. Each capability reference must resolve
within that installation's approved scope. A capability can serve several
profiles without copying its content.

| Resource | Owner and lifecycle | Main contract |
|---|---|---|
| `Harness` | Platform administrator; installation lifetime. | Installation identity, storage bindings, coordination, identity, exposure, and maintenance. |
| `HarnessProfile` | Application-team maintainer; offering lifetime. | Approved capability references, defaults, one persona, and availability requirements. |
| `ContextBundle` | Content owner; publication lifetime. | Typed instructions, prompt rules, commands, and optional manifest references. |
| `AgentDefinition` | Specialist owner; independent revision lifetime. | Delegated specialist definition with approved integration references. |
| `Skill` | Procedure owner; independent reuse lifetime. | Reusable procedure and its assets. |
| `ModelProvider` | Platform administrator; endpoint and trust lifetime. | Protocol, endpoint, trust configuration, and separate authentication reference. |
| `MCPConnection` | Integration owner; endpoint consumption lifetime. | ToolHive endpoint reference, transport, readiness, and authentication mode. |
| `AccessPolicy` | Platform security owner; entitlement lifetime. | Runtime consumption grants. Kubernetes RBAC still controls CR authorship. |
| `ToolPolicy` | Platform security owner; tool permission lifetime. | Tool limits that preserve Deny and configured Ask. |
| `LearningPolicy` | Data and security owner; learning governance lifetime. | Learning mode, evidence, evaluation, and publication limits. |
| `ExecutionProvider` | Platform administrator; optional managed installation. | Install or reference one compatible provider in an execution namespace. |
| `ExecutionTemplate` | Execution administrator; recipe publication lifetime. | Canonical immutable recipe revisions and selection policy. |
| `Automation` | Platform automation owner; schedule definition lifetime. | Bridge a declaration into the existing runtime scheduler. |

The operator can introduce a reusable `ModelRoutingPolicy` when several
owners share routing independently. Guardrail, hook, and retention resources
need the same ownership test. Until then, typed fields or approved references
are sufficient. Generic configuration fragments do not justify new CRs.

`HarnessProfile` must not accept arbitrary Pod patches, service accounts,
volumes, hooks, or Secret selectors. Application teams must not combine an
arbitrary URL with a platform credential. Platform administrators must bind
credentials to approved destinations and purposes.

External operators retain ownership of databases, PKI, Secrets, Gateways,
and ToolHive workloads. A `Harness` records required bindings and health.
A `Harness` must not imply that one `databaseRef` externalizes all state.

## Content acquisition and publication

Substantive content can use bounded inline data, Git at a full commit, or
OCI at a digest. The default inline limit is 16 KiB per resource.
Larger bodies and assets belong in immutable artifact storage, outside etcd
and CR status. A GitOps-authored CR does not need `source.git` when the CR
already contains the complete small declaration.

All content types must use the same typed resolver and immutable publisher.
Each payload type still needs its own schema, validation, and privilege
rules. A `Skill` can consume the ToolHive artifact type
`dev.toolhive.skills.v1` with `SKILL.md` at its root.
That artifact describes a procedure, not an executable workload.
That media type does not define agent or context bundle formats.

Flux can provide acquisition integration. Flux is not a required dependency.
The operator must verify the selected immutable artifact and its provenance.
An unavailable acquisition backend must produce an explicit failure.
The operator must not silently switch backends or resolve a different tag.

Fetching must occur in quarantine with restricted network origins.
The fetcher must validate redirects and authentication challenges before
sending credentials. The fetcher must block unauthorized internal endpoints.
Extraction must enforce size, file-count, path, and link limits.
Fetching must not execute scripts, hooks, or implicit submodule downloads.
Credentials must remain separate from content and publication records.

A signed artifact proves provenance under a trust policy.
A signature does not prove content safety or publication authority.
Approval must bind the full body and assets by digest, scope, and policy.
Private learned content must never enter shared publications automatically.

### Preserve typed content boundaries

A `ContextBundle` must preserve the five `harness_context` kinds:
instructions, commands, rules, skills, and agent definitions.
A bundle can reference independently published skill and agent manifests.
A bundle must not import adjacent settings or grant tool access.

The resolver must retain ordered combine or replace semantics.
The resolver must apply exclusions before collision selection.
The publication must record source provenance and trust for each contribution.
Mounting low-trust content at a platform path must not raise its trust.

Prompt rules guide the model. Runtime policy enforces restrictions.
Commands are prompt templates, not deterministic action workflows.
A profile selects one soul persona through a field or approved reference.
A separate `Soul` CR adds no independent lifecycle and is unnecessary.

An `AgentDefinition` describes a delegated specialist.
Publication must not automatically expose each specialist as a top-level
assistant. Application-team definitions must exclude host-executed hooks,
inline credentials, and arbitrary MCP endpoints. Proposed integration
references must select platform-approved hooks or connections instead.

### Publication and activation contract

The publisher must perform these operations in order:

1. Resolve the complete dependency graph to immutable content.
2. Reject cycles, incompatible capabilities, and unauthorized references.
3. Validate required dependencies and explicit optional dependencies.
4. Verify authoritative approval and current reference-use records for the
   resolved graph, scope, and policy revision.
5. Publish the complete artifact without changing the current revision.
6. Obtain runtime acknowledgement of the publication revision.
7. Advertise the revision for new sessions.
8. Persist each new session binding in the runtime database.

The requested CR generation identifies the requested configuration.
The accepted publication revision identifies the validated publication.
The runtime-acknowledged publication revision identifies the publication
that the runtime acknowledged. Status must keep these three fields distinct.

Required dependencies block publication. An author must explicitly mark an
optional dependency. The runtime must show an unavailable optional capability
with its exact reason. Listing and use must share the same approved
resolution. Successful listing alone must not grant use authority.

Durable per-session binding is target runtime work.
Phase 1 can mount existing configuration and restart the deployment.
Rehydrated sessions can then use deployment-wide configuration.
Phase 1 must not claim immutable per-session pinning.

| Domain | Activation contract in the target design |
|---|---|
| Content, routing defaults, and persona | New sessions bind the new publication. Existing sessions retain their binding. |
| Explicit session migration | Validate compatibility and record a new binding through a runtime operation. |
| Current entitlements and security revocation | Recheck authority at runtime boundaries within the declared freshness limit. |
| Credentials | Rotate through the credential provider. Never freeze secret values in publications. |
| Memory and learning | Read live owned records under the applicable current policy. |
| Execution recipes | Retain exact revisions. Coordinate provider updates until dynamic publication exists. |
| Automation definitions | Acceptance pins an exact publication revision. Each fire inherits that revision. |

Garbage collection must retain revisions referenced by paused sessions,
schedules, children, and execution allocations. The backing store must hold
reference indexes. CR status must not accumulate an unbounded revision list.

## Policy and identity

Policy composition must follow each domain's rules.
A generic deep merge cannot represent these security contracts.

| Domain | Required composition |
|---|---|
| Tool permissions | Deny overrides Ask and Allow. Configured Ask remains effective. |
| Plan mode | Block mutations before ordinary permission evaluation. |
| Entitlements | Intersect the grants from all applicable authority boundaries. |
| Numeric ceilings | Select the lower permitted ceiling. |
| Defaults | Allow an override only when the owning policy permits the override. |
| Content | Use ordered composition with explicit exclusions and collisions. |
| Learning | Permit lower scopes to tighten policy only. |
| Retention | Resolve explicit preservation and deletion obligations. Reject conflicts. |

Posture alone must not trust a headless project.
Platform-side security attachments must remain effective when an
application-team maintainer changes a profile. An application-team maintainer
must not detach those limits.
The existing optional Cedar adapter only attenuates carried tool capabilities.
That adapter does not implement general group entitlements.

Initially, cross-namespace references must fail closed.
A later export contract must require target-owner consent and consumer use
checks. Gateway API's reference-grant model informs this direction.
A namespace match must never replace use authorization.

Workload identity identifies the serving process or automation principal.
End-user identity identifies the authenticated caller by issuer and subject.
Neither identity substitutes for the other. The separate
[agent identity proposal](agent-identity-model.md) explores future delegation.
That draft is not evidence of implemented identity guarantees.

Phase 1 production use with multiple callers requires caller OIDC.
The runtime must verify the issuer and subject for each caller. Kubernetes
RBAC, a shared bearer token, and transport mutual TLS (mTLS) do not replace
caller OIDC. Private memory also requires runtime ownership enforcement and
a caller-partitioned store. Until these prerequisites hold, Phase 1 must not
claim multi-caller isolation or enable private memory for multiple callers.

Immutable configuration does not freeze current authority.
The proposed default authorization freshness limit is 60 seconds.
The runtime must check authority before new runs, model requests, tool calls,
child admission, schedule fires, and approval continuation.
A check must fail closed after the authority information expires.
Revocation must invalidate cached grants within that limit.
Already admitted external effects can finish.
Revocation cannot undo effects.

This bound requires runtime enforcement and an authenticated authority feed.
A Kubernetes rollout alone cannot provide the bound.
Until those prerequisites exist, the operator must not advertise bounded
revocation. A security revocation must override last-good configuration.

## Models, MCP, and execution

### Model selection

Observed provider inventory, curated offerings, permissions, and routing serve
different purposes. Inventory reports models. A profile presents approved
choices. Entitlements authorize use. Routing selects among permitted choices.

The runtime must check the final provider and model after alias resolution.
The check must cover main runs, plan runs, children, classifiers, compactors,
checkers, reflection, auxiliary calls, and schedules.
Routing fallback must stay within the caller's entitlement set.
An empty eligible set must fail explicitly.

Current unlisted-model passthrough remains a compatibility concern.
The operator cannot enforce restrictions by filtering a catalogue alone.
Existing token caps are runaway brakes, not financial quotas or global
budgets across all descendants. Kubernetes `ResourceQuota` limits cluster
resources, not inference spending. Financial quotas need a separate design
for reservations, provider charges, reconciliation, and failure handling.

Shared code supports deployment-scoped OIDC credentials for model providers.
`mecak8s` does not currently load those credentials.
Per-user provider OAuth is not an existing supported capability.
The first operator slice must use supported deployment credential paths.

### MCP consumption

`MCPConnection` can reference an approved external HTTP endpoint or a
ToolHive-managed `MCPServer` or `VirtualMCPServer` endpoint. For ToolHive
resources, readiness checks must match the referenced resource generation.
Local ToolHive workload discovery does not discover
Kubernetes CRs. The operator needs an explicit Kubernetes integration.

Connections must use streamable HTTP. Direct mode and embedded broker mode
must remain exclusive choices. An external virtual MCP endpoint does not
replace the embedded broker's session authority contract.

The connection must declare service credentials or per-user enrollment.
Upstream approval, user consent, and per-call Ask are separate decisions.
The operator must not treat any one decision as approval for the others.

The current broker's outer attachment is process-local.
Broker deployments currently require one replica and `Recreate` updates with
callback routing. Owners must re-enroll after restart.
The operator must expose this interruption and recovery requirement.
A higher replica count cannot repair this state model.

### Execution installation and templates

An execution namespace must have one provider owner.
A `Harness` installation binding must combine storage identity, coordination
namespace, and execution ownership. Redis uses fixed key prefixes.
Independent Lease namespaces do not isolate a shared Redis keyspace.
The operator must reject bindings that permit cross-runtime collisions.
The metadata index also limits Redis Cluster assumptions.

Installation identity fields must be immutable during ordinary updates.
Storage, coordination, or execution identity changes require a migration.
The migration must fence old executors before activating replacement owners.

`ExecutionTemplate` must publish canonical immutable recipe revisions.
Current startup-loaded recipes require coordinated quiescence for updates.
Provider and executor protocol versions must match.
The operator must retain revisions while allocations reference those revisions.
Exact reattachment must fail closed, with no template or workspace fallback.
Retained PVCs must not have cascading ownership from disposable CRs.

A profile that requires specialists and remote execution is incompatible with
the current runtime. The operator must reject that profile until runtime
extensions support the combination. The operator must also reject required
remote repository-source ingestion. A label such as "coding" cannot imply
support for every capability.

## Illustrative resource declarations

These examples show proposed relationships, not supported syntax.
No CRDs in this proposal make these examples installable.
All referenced resources must exist in namespace `engineering` and pass use
checks. Placeholders identify content that an implementer must supply.

```yaml
apiVersion: platform.mecatl.dev/v1alpha1
kind: ModelProvider
metadata:
  name: approved-models
  namespace: engineering
spec:
  protocol: openai-compatible
  endpoint: https://models.example.com/v1
  trustRef: model-endpoint-trust
  authenticationRef: model-service-credential
---
apiVersion: platform.mecatl.dev/v1alpha1
kind: AgentDefinition
metadata:
  name: review-specialist
  namespace: engineering
spec:
  source:
    git:
      url: https://git.example.com/platform/specialists.git
      commit: "<FULL_COMMIT_ID>"
      path: review
  toolPolicyRef: read-only-review
  integrationRefs: []
---
apiVersion: platform.mecatl.dev/v1alpha1
kind: HarnessProfile
metadata:
  name: engineering-review
  namespace: engineering
spec:
  harnessRef: engineering
  contextBundleRefs: [engineering-guidance]
  agentDefinitionRefs: [review-specialist]
  skillRefs: [review-checklist]
  models:
    providerRef: approved-models
    choices: ["<APPROVED_MODEL_ID>"]
    default: "<APPROVED_MODEL_ID>"
  toolPolicyRef: read-only-review
  learningPolicyRef: private-review
  execution:
    mode: no-filesystem
  requiredCapabilities: [delegation]
---
apiVersion: platform.mecatl.dev/v1alpha1
kind: Automation
metadata:
  name: weekly-review-summary
  namespace: engineering
spec:
  profileRef: engineering-review
  servicePrincipalRef: review-automation
  cadence: "0 9 * * 1"
  timeZone: Etc/UTC
  prompt: Summarize the approved review records.
  overlap: forbid
  missedFire: skip
  retry: none
  suspend: true
```

The platform administrator creates the provider and credential binding.
The content owner proposes the specialist at an exact commit.
The approver accepts the resolved digest and permitted scope.
The maintainer attaches the approved content to the profile.
The runtime must refuse an unapproved model even if the provider accepts it.
The no-filesystem example avoids claiming remote specialist support.

## Platform and user journeys

The platform administrator creates a `Harness` with external dependency
bindings and one public address. The operator checks installation identity,
endpoint trust, storage contracts, and runtime compatibility.
The platform administrator then publishes approved capabilities.

The application-team maintainer selects those capabilities in a profile.
A missing grant blocks publication with `UseNotAuthorized`.
An incompatible required capability blocks publication with
`IncompatibleCapabilities`. A failed content update leaves the previous
publication available only while current authorization permits use.

The user signs in through Studio, `mecatui`, or an SDK.
The runtime lists entitled offerings and exact availability reasons.
The user selects an offering. The runtime binds the session to the
runtime-acknowledged publication revision and records the caller.
The operator takes no part in each turn.

The user can create personal schedules through runtime APIs.
The user can maintain private memory under runtime ownership checks.
Neither action requires a CR or Kubernetes access.

## Automation bridge

An `Automation` declares a platform-owned schedule definition.
The runtime database remains authoritative for execution and claims.
The bridge must use the existing create, update, pause, and resume concepts.
The bridge must never call `FireNow` on each reconciliation.
A second CronJob scheduler would create competing execution authority.

The bridge must derive a deterministic schedule key from the immutable
installation identity and the `Automation` CR UID. It must persist the
CR UID-to-schedule mapping. An ambiguous create retry must use the same key
and observe the same schedule. Before update or adoption, the runtime must
verify both the service-principal owner and the bridge management identity.
A matching name never permits takeover of an unrelated schedule.

The runtime must verify the automation service principal.
A `spec.owner` string must not impersonate a human.
Definition writes must be idempotent and limited to bridge-owned fields.
Current schedule APIs do not supply generic CAS and owned-field metadata.
The automation phase must add runtime ownership checks, management identity,
CAS, and concurrency fencing before it enables the bridge.

At definition acceptance, `profileRef` resolves to one exact accepted
publication revision. The runtime must persist that binding with the schedule.
Every fire inherits that revision. A profile update must not migrate an
accepted automation. An authorized definition update must select a new
publication revision to migrate the automation. Active fires retain their
previous revision. The runtime must recheck current entitlements at each fire
and at runtime authority boundaries. These bindings are target runtime work.
Phase 1 does not provide automation publication pinning.

The initial policy is no overlap, skip missed fires, and no automatic retry.
The declaration must specify a time zone and these execution policies.
Later retry policies must account for ambiguous external effects.
The scheduler cannot promise exactly-once tool execution.

Suspension prevents new fires. Suspension does not cancel an active run.
Cancellation requires a separate authorized runtime operation.
Deletion must suspend first and inspect active or ambiguous fires.
The bridge must retain ownership metadata until disposition is known.
An active fire can finish under current authority unless cancellation succeeds.
The bridge must not delete execution evidence to complete finalization.

Current remote execution cannot allocate an independent scheduled workspace.
Profiles that require that behavior must remain unavailable for automation.
Personal schedules continue to use runtime APIs without operator ownership.

## State, privacy, and recovery

User-model memory and project memory both use `tool.MemoryStore`.
The store uses structured keys and values with revision history.
An absent expected version permits create-only writes. An update must use
CAS with the exact current revision. `Forget` creates a tombstone.
`Undo` creates a compensating revision; it does not erase history.
Undo and history access stop at the configured retention boundary.
Retention policy must govern final removal of records and history.
The local adapter uses `memory.json` and `memory.lock` with atomic replacement.
`MEMORY.md` is not the primary memory architecture.
See the [memory contract](../architecture/memory.md).

Within one installation, the same verified caller shares user-model memory
across profiles. The caller key uses the verified issuer and subject.
Project memory adds the exact server-owned workspace to that caller scope.
Project memory is not shared with other callers. A profile must not silently
partition memory again or export memory. Stronger profile isolation requires
an explicit migration and a future contract.

The runtime projects a bounded project-memory index without values.
The agent must use `Recall` to retrieve values. The runtime refreshes the
user-model projection before provider requests. Memory facts remain untrusted
data. They must not become instructions or permissions. Live memory remains
outside immutable publications.

`mecak8s` supplies a user-model directory, but does not configure project
memory through `MemoryDir`. Local learning repositories use the user-model
directory. Redis session persistence does not externalize these files.
Ownership enforcement rejects unsupported remote memory and learning.
The operator must report unavailable capabilities instead of enabling an
unsafe shared directory.

| State class | Required ownership and backup contract |
|---|---|
| Sessions, messages, runs, approvals | Runtime store; consistent backup with identity and execution bindings. |
| Publications and reference indexes | Immutable artifact store plus durable indexes; retain every referenced revision. |
| User memory and learning records | Caller-scoped backing store; preserve history, consent, and access metadata. |
| Project memory and workspaces | Caller-and-workspace-scoped storage; preserve exact allocation and revision associations. |
| Schedules and fire records | Runtime scheduler store; preserve definitions, claims, and bridge ownership. |
| Scheduled-result delivery | Separate durability contract; current in-memory delivery cannot claim lossless recovery. |
| Credentials and enrollment | External secret owner or runtime enrollment store; use separate access and recovery controls. |

A learning policy selects `off`, `review`, or `auto`.
The default is `off`. Learning must retain separate proposal, validation,
evaluation, review, and acceptance stages. `review` requires approval before
acceptance. `auto` can accept private learning only within approved limits.
Automatic fact promotion requires supported explicit user intent. It must not
overwrite a conflicting fact or a user-explicit fact. Learned skill changes
must affect the body only. Assets, permissions, and tools must stay unchanged.

Acceptance at the `evaluated` level requires an evaluation result of `PASS`.
Acceptance at the `validated` level can permit an evidence-backed `ABSTAIN`
under policy. `FAIL` or an error must reject the proposal.
Learning must preserve evidence, validation results, evaluation results,
review decisions, and resource budgets.
Private proposals belong in runtime backing stores.
Learned state must never become permission policy.

Private-to-organization export requires explicit owner consent, redaction,
review, and publication approval. The operator must not watch private records
and automatically publish shared content.

`Harness` deletion must retain databases and workspaces by default.
Decommissioning must suspend automation, stop admission, drain execution,
and preserve authority metadata. An explicit disposal procedure can remove
retained data after backup and ownership checks.
A finalizer cannot protect data from namespace or CRD deletion.
Backup policy, PVC reclaim policy, and restore procedures must address those
failure modes independently.

During an operator outage, the runtime can use accepted configuration only
within its declared authorization freshness contract. An independent
authority service can keep authorization fresh during that outage.
A Kubernetes API outage still affects Leases and execution allocation.
Existing external calls can finish after ownership loss.
Recovery must fence old executors before admitting new mutations.

High availability requires compatible routing, storage, credentials, and
capability state. Broker restrictions limit available deployment topologies.
Restore qualification must include paused sessions and pending approvals,
not just healthy Pod counts.

## Reconciliation and observability

CRDs must use structural schemas and CEL for local validation.
Admission webhooks must not download content or contact providers.
Controllers must resolve external references asynchronously.
Each list field must declare its SSA semantics: atomic, set, or map.

Reconcilers must be idempotent. Dependency indexes must identify affected
profiles without scanning all content. Controllers must not write unchanged
objects. Separate field managers must own generated resources and status.
Status subresources must prevent spec writers from asserting readiness.

Conditions must include observed generation and bounded reasons:
`Accepted`, `ResolvedRefs`, `Ready`, and `Available`.
For `HarnessProfile`, `Ready` means the runtime acknowledged the intended
publication. For `Skill`, `Ready` means the approved bundle is available.
Skill readiness does not confirm activation in a runtime.
`Available` reports service capability under current dependencies and policy.
A previous generation's condition must not imply current readiness.
Status must contain digests and summaries, not content or credentials.

Diagnostics, tool-call audit records, and durable events have separate contracts.
Diagnostics explain failures. Tool-call audit records describe executed tools.
Separate approval records identify publication decisions and their approvers.
Durable events preserve runtime history. The operator must not log content,
credential values, or authentication headers. Metrics must use bounded labels.

New installations must use direct SSA with explicit ownership.
Existing Helm installations require a planned ownership transfer.
The transfer must preserve service and installation identities.
Helm and the operator must not manage the same fields concurrently.
The migration must include an inventory and rollback criteria.
Administrators must stop Helm releases and GitOps reconciliation before they
transfer field ownership. Only then can the operator adopt resources.

CRD versions need conversion tests and a storage migration plan before removal.
A storage version change must preserve resource identity and retained bindings.
Runtime upgrades must check protocol compatibility before rollout.
Execution upgrades must include coordinated quiescence where current loaders
require a restart.

## Delivery phases and acceptance criteria

Each phase must state its supported capabilities and unavailable combinations.
The following criteria define future qualification work, not completed tests.

| Phase | Useful scope | Acceptance criteria |
|---|---|---|
| 1: installation | `Harness`, external bindings, supported deployment configuration, and status. | A real cluster installs, drains, restores, and upgrades one trusted-team service. Identity collisions fail. Production multi-caller use requires verified caller OIDC. Caller A cannot access caller B's sessions, schedules, or memory. Private memory requires ownership enforcement and caller-partitioned storage. Rehydration documents deployment-wide configuration. |
| 2: approved offerings | Profiles, typed content, providers, MCP references, access and tool policies, publication, and runtime binding. | The authoritative approval and reference-use API gates delegated publication. CR writes and controller identity cannot supply approval. Application teams cannot modify authority records. Dependency changes trigger current use-grant checks. Same-namespace unauthorized use fails. Cross-namespace use fails. Tampered content fails. Sessions retain revisions across restart. Every model path enforces final-target entitlements. |
| 3: execution lifecycle | Optional provider installation and independent template publication. | Exact reattachment survives upgrade. Old executors cannot mutate after fencing. Retained PVCs survive CR deletion. Incompatible remote profiles fail. |
| 4: automation and learning | Schedule bridge, private learning policy, and explicit export. | A crash after schedule creation but before mapping persistence still yields one schedule per installation and CR UID. Retries verify owner and management identity. Concurrent edits cannot overwrite authority. Profile updates do not migrate automation. Authorized definition updates select a new publication revision; active fires retain the previous revision. Fires recheck current entitlements. Deletion handles ambiguous fires. Private data cannot publish without consent. |

Phase 1 does not promise multiple independently pinned profiles.
Phase 2 can ship without managed execution providers or automation.
Phase 2 must reject an authority record if the approver, resource UID,
content or dependency digest, scope, or policy revision does not match.
A revoked reference-use grant must block publication after a dependency change.
Unsupported optional capabilities must remain visible with exact reasons.

Controller qualification must include envtest for reconciliation, status,
SSA ownership, and dependency changes. Negative tests must cover reference
use, approval scope, namespace boundaries, fetch redirects, and extraction.
Runtime tests must cover Ask preservation, model routing fallback, stale
authority, schedule fencing, and publication retention.

Real-cluster tests must cover volumes, callback routing, provider upgrades,
Lease loss, restore, and Helm ownership transfer.
End-to-end tests must exercise real user login, offering selection, approval,
session recovery, and automation under a service principal.
These criteria do not report completed runtime qualification.

## Alternatives and remaining decisions

| Alternative | Decision and reason |
|---|---|
| Put sessions and learning records in CRs | Reject. Application data needs runtime transactions, privacy controls, and independent retention. |
| Reconcile Helm values indefinitely | Reject for new installations. Competing ownership complicates drift and migration. |
| Use one generic configuration CR | Reject. Independent owners need separate approval and lifecycle boundaries. |
| Create a CR for every struct | Reject. Internal implementation types do not define ownership. |
| Require Flux | Reject. Acquisition integration must remain optional. |
| Add `Tenant` now | Defer. Trusted-team isolation needs explicit limits before a stronger tenancy contract. |
| Reuse ToolHive workloads | Select. The operator consumes endpoints instead of duplicating workload management. |
| Use a CronJob per automation | Reject. The runtime scheduler must remain the execution authority. |

Reviewers must decide the following bounded questions before implementation:

- Confirm the provisional API group and the initial resource names.
- Select the authoritative approval and reference-use record API before
  Phase 2 delegated publication. Define independent platform control and
  checks for identity, resource UID, full content and dependency digest,
  scope, and policy revision.
- Select the artifact store and transactional reference-index interface.
- Validate the proposed 60-second authorization freshness default.
- Specify the authority feed and the treatment of in-flight approvals.
- Select the durable backend for capability-specific memory and learning.
- Define schedule ownership metadata and fencing in the runtime API.
- Define delegated identity integration with the separate identity proposal.

These questions do not reopen the runtime/operator boundary or the prohibition
on automatic private-content publication. Financial quotas require a separate
proposal before an offering can advertise spending enforcement.

## API compatibility assessment

This documentation change alters no wire API, Go API, or stored data.
Future profile selection, entitlement discovery, and durable binding require
runtime capabilities and compatibility negotiation.
`CreateSessionRequest` reserves field number 6 and name `profile`.
Implementers must not reuse either reservation.

Restricting current unlisted-model passthrough can break conforming clients.
Implementers must assess affected clients, major-version requirements, and
upgrade behavior before implementation. A breaking change requires explicit
operator approval under the [wire API policy](../architecture/api-surface.md#wire-api-compatibility).
New capability identifiers must distinguish unsupported features from denied
or temporarily unavailable features.

Stored-data migration needs a separate assessment for publication bindings,
schedule ownership, and reference indexes. Go source compatibility also needs
an independent assessment. Additive CRDs do not prove runtime compatibility.

## Evidence and design references

Current behavior comes from these implementation sources and current guides.
Other drafts remain proposals.

- [Context schema](../../internal/adapter/permconfig/schema.go): five kinds near line 58; retention near 554; learning near 1399; inert allowlist near 1776.
- [Context composition](../../internal/app/harness_context.go): registered sources and provenance near lines 54 and 95.
- [Agent source](../../engine/tool/agentsource.go), [command source](../../engine/prompt/commandsource.go), and [prompt rules](../../engine/prompt/rule.go): distinct content contracts.
- [Project guidance lifetime](../../user-docs/features/agent-behavior/project-instructions-and-rules.md) and [soul selection](../../internal/app/soulselect.go): live guidance and process persona selection.
- [`mecak8s` composition](../../cmd/mecak8s/flags.go): application configuration near line 698 and directory wiring near 777.
- [Runtime construction](../../internal/app/build.go): result delivery near 4116; memory and learning near 5760; remote restrictions near 8758.
- [Memory store](../../internal/adapter/memory/store.go), [memory partitioning](../../internal/adapter/memory/namespaced_store.go), and [learning ownership](../../internal/app/learningdriver.go).
- [Agent integrations](../../internal/app/agentdefs.go) and [Cedar attenuation](../../internal/adapter/cedarauthority/cedar.go).
- [Session contract](../../contracts/proto/mecatl/v1/harness.proto), [schedule contract](../../contracts/proto/mecatl/v1/schedule.proto), and [schedule ownership](../../internal/adapter/server/schedule_manager.go).
- [Execution recipes](../../internal/adapter/executioncontroller/templates.go), [Redis prefixes](../../adapters/redisstore/redisstore.go), and [metadata index limits](../../adapters/redisstore/metadata_index.go).
- [Scale and recovery](../../user-docs/operating/mecak8s/scale-recover-and-upgrade.md): routing, broker, and deployment limits.
- [Governance](../architecture/governance.md), [memory](../architecture/memory.md), [observability](../architecture/observability.md), and [deployment hardening](../architecture/deployment-and-hardening.md): owning architecture chapters.

External references inform design choices, not claims of Mecatl support:

- [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) separates configuration from application data.
- [Kubernetes API conventions](https://github.com/kubernetes/community/blob/main/contributors/devel/sig-architecture/api-conventions.md) and [Kubebuilder practices](https://book.kubebuilder.io/reference/good-practices) inform reconciliation and status.
- [CRD validation](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules) and [CRD versioning](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/) inform schema evolution.
- [Gateway reference grants](https://gateway-api.sigs.k8s.io/geps/gep-709/) informs target-owner consent. It does not supply universal authorization.
- [Gateway policy attachment](https://gateway-api.sigs.k8s.io/geps/gep-713/) informs attachment design. Related patterns have mixed maturity, including experimental work.
- [KubeVela roles and composition](https://kubevela.io/docs/getting-started/core-concept/) informs role separation, not arbitrary application-team templating.
- [Flux OCI acquisition](https://fluxcd.io/flux/components/source/ocirepositories/) informs optional immutable source integration.
- [ToolHive skill media types](https://github.com/stacklok/toolhive-core/blob/v0.0.52/oci/skills/mediatypes.go) and [packaging](https://github.com/stacklok/toolhive-core/blob/v0.0.52/oci/skills/packager.go) define the referenced skill artifact.
- [Argo scheduled workflows](https://argo-workflows.readthedocs.io/en/latest/cron-workflows/) illustrates explicit cadence and concurrency policy, not exactly-once effects.
- [SPIFFE identity](https://spiffe.io/docs/latest/spiffe-about/overview/) distinguishes workload identity from end-user identity.
