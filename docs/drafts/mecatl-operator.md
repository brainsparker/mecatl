# Mecatl operator design proposal

Keep `mecak8s` as the runtime. Add an operator that installs the service and
publishes approved service offerings. Kubernetes should hold configuration
and manage infrastructure. The runtime database should hold sessions, runs,
approvals, and schedule execution state. Private memory and learning also
belong in runtime backing stores, with their own storage and privacy controls.

This is the target design, not existing profile support. It serves platform
engineers who need to offer Mecatl to trusted teams with different guidance,
models, and tools. It does not process agent turns or provide hostile-tenant
isolation.

A simpler fixed deployment remains valid. The operator is justified when
independent owners need to assemble and reuse offerings; full profile and
session binding requires runtime changes that the first release does not have.

## An incident-investigation offering

A platform administrator installs a `Harness` and approves a model endpoint,
a read-only incident tool connection, and a workspace execution template.
The platform owns endpoint trust, credentials, and security limits. A team
maintainer then proposes incident guidance and a `HarnessProfile` that uses
those approved resources. The profile is the service offering, not a Pod
configuration or a named chat agent.

After publication, an engineer signs in through Studio, `mecatui`, or an SDK
without a kubeconfig. The engineer selects the incident offering, investigates
an alert with its guidance and authorized tools, and later resumes the same
session. In the target design, that session retains its configuration
revision. Current permissions still control every new action.

```text
Harness: installation, identity, storage
  └─ HarnessProfile: incident-investigation
       ├─ ContextBundle + Skill: guidance and procedures
       ├─ ModelProvider: approved endpoint and model choices
       ├─ MCPConnection: approved incident tools
       ├─ AccessPolicy + ToolPolicy: who can use what
       └─ ExecutionTemplate: optional workspace recipe
                    │
          resolve, validate, approve
                    ↓
          immutable profile revision
                    ↓ runtime acknowledgement
          session binding in runtime DB
```

The graph shows references, not cascading deletion ownership. The platform
administrator controls the available integrations and limits. The team
maintainer assembles an offering within those limits. The end user consumes
it through runtime APIs. Neither the maintainer nor the user needs authority
to change platform credentials or executor Pods.

## Resources follow ownership

The target inventory separates resources that have different owners or update
cycles. It is not a mandatory set for the first installation.

| Resources | Purpose |
|---|---|
| `Harness`, `HarnessProfile` | Define an installation and an approved offering within it. |
| `ContextBundle`, `Skill`, `AgentDefinition` | Publish typed guidance, reusable procedures, and delegated specialists. |
| `ModelProvider`, `MCPConnection` | Bind approved model endpoints and external tool connections to trust and authentication. |
| `AccessPolicy`, `ToolPolicy` | Set runtime entitlements and enforce tool permission limits. |
| `LearningPolicy` | Govern private learning and the separate path to shared publication. |
| `ExecutionProvider`, `ExecutionTemplate` | Optionally manage a provider and publish workspace recipes. |
| `Automation` | Bridge a platform schedule definition into the runtime scheduler. |

Resources start namespaced, under the provisional group
`platform.mecatl.dev/v1alpha1`. Each profile belongs to one `Harness`.
References stay within the namespace initially, but still require permission
to use the target. A later cross-namespace contract needs target-owner consent
as well as consumer authorization, following the idea behind
[Gateway reference grants](https://gateway-api.sigs.k8s.io/geps/gep-709/).

There is no CR per tool, model, memory record, or turn. Routing starts as typed
profile configuration; a separate routing resource is justified only if it
needs independent ownership. One profile selects one shared persona—the
existing Mecatl soul configuration. That persona is not private user memory
and does not need a separate `Soul` resource. The operator also leaves
databases, PKI, Secrets, Gateways, and ToolHive workloads to their existing
owners.

## Guidance does not grant authority

A `ContextBundle` supplies typed instructions, prompt rules, and commands. It
can also reference independently published skills and agent definitions.
The runtime's five content kinds remain distinct. Commands are prompt
templates, not deterministic workflows. Rules guide the model; runtime policy
enforces restrictions. Skills provide procedures and assets, not tool grants.
An `AgentDefinition` currently describes a specialist child. Publishing one
does not make it a top-level assistant.

The publisher preserves source order, combine or replace behavior, exclusions,
and collision rules from [context composition](../../internal/app/harness_context.go).
It records provenance and trust for each contribution. An acquisition URL is
not a trusted runtime source identity. Mounting content at a platform-owned
path does not raise its trust, and a bundle cannot import adjacent settings.
Existing project guidance and command templates also have different loading
lifetimes; a common publisher does not imply per-run reload for every kind.

Content can be inline, in Git at a full commit, or in OCI at a digest. Small
declarations can stay in the CR; larger bodies and assets belong in artifact
storage, outside etcd and status. All content uses shared acquisition, but
each type retains its own validation and privilege limits. The
[ToolHive skill media type](https://github.com/stacklok/toolhive-core/blob/v0.0.52/oci/skills/mediatypes.go)
with a root `SKILL.md` applies to `Skill` only. It defines neither context
bundles nor agent definitions. Flux integration is optional.

Acquisition runs in an isolated fetch environment. It restricts origins,
checks redirects and authentication challenges before sending credentials, and
blocks unauthorized internal endpoints to prevent server-side request forgery
(SSRF). Extraction limits size and file count and rejects unsafe paths and
links. Fetching executes no scripts or hooks and performs no implicit
submodule downloads. Credentials stay separate from content. Failed
acquisition reports a failure instead of silently selecting another backend,
tag, or revision.

A signature proves provenance under a trust policy, not safety or approval.
Platform approval covers the exact body, assets, and dependencies for a stated
scope. Delegated authors cannot supply host-executed hooks, Secret selectors,
arbitrary integration URLs, or Pod overrides. Approved references select any
permitted hooks or connections. Platform administrators bind credentials to
specific destinations and purposes. These limits matter because current
[agent definitions](../../internal/app/agentdefs.go) can contain privileged host integrations.

## From a proposal to an active revision

**Delegated publication is blocked until the approval and reference-use API is
selected and implemented.** Platform-controlled records must establish who
approved the exact content and dependencies, and who may attach a resource to
an offering. A CR write identifies its actual writer; a GitOps service account
or controller identity is not proof that a human approved the content. These
records bind the verified approver, resource UID, complete content and
dependency digest, scope, and policy revision. Reference-use records also name
the authorized maintainer and target UID. Dependency changes require fresh use
grant checks, and team maintainers cannot edit either authority record.

Publication then resolves the whole reference graph, rejects cycles and
incompatible required capabilities, and checks the approval and reference-use
records. Only then does it publish an immutable revision. The runtime must
acknowledge that revision before advertising it for new sessions. Session
creation persists the exact revision in the database. Listing an offering and
using it rely on the same resolution, but listing alone grants no authority.
An explicitly optional dependency can remain unavailable with a clear reason;
a failed required dependency blocks publication.

Publication approval is different from a per-tool permission decision and from
a learning acceptance decision. `AccessPolicy` runtime grants do not replace
this publication API or Kubernetes RBAC for authoring.

The following YAML illustrates the incident offering's relationships. It is
proposed syntax, not an installable API. The referenced resources are assumed
to exist in the same namespace and to have passed approval and use checks.
The platform attaches its access limits separately so the author cannot
remove them.

```yaml
apiVersion: platform.mecatl.dev/v1alpha1
kind: HarnessProfile
metadata:
  name: incident-investigation
  namespace: operations
spec:
  harnessRef: operations
  contextBundleRefs: [incident-guidance]
  skillRefs: [incident-triage]
  models:
    providerRef: approved-models
    choices: ["<APPROVED_MODEL_ID>"]
    default: "<APPROVED_MODEL_ID>"
  mcpConnectionRefs: [incident-read-only]
  toolPolicyRef: incident-read-only
  learningPolicyRef: learning-off
  execution:
    templateRef: investigation-workspace
  requiredCapabilities: [remote-execution]
```

The example uses remote execution without specialist children. A later variant
that requires both must be rejected while that combination is unsupported.
The declaration does not itself establish availability.

For a routine guidance update, new sessions receive the new acknowledged
revision. Existing sessions keep their binding unless an authorized runtime
operation migrates them after a compatibility check. A failed update leaves
the last accepted revision usable only while current authority permits it.
Credentials rotate independently, and memory and learning remain live data.
The artifact store retains revisions while sessions, schedules, children, or
execution allocations reference them. Durable reference indexes support
collection without an unbounded history in CR status.

Emergency permission changes follow a different path. The target runtime
checks current grants before runs, model requests, tool calls, child admission,
schedule fires, and approval continuation. It fails closed when authorization
data expires, even for an older accepted revision. The maximum permitted age,
authenticated update feed, and treatment of pending approvals need agreement
before implementation. A rollout cannot provide bounded revocation or undo an
external effect already admitted.

This binding model requires runtime changes. An initial operator can mount
existing configuration and restart the deployment, but resumed sessions then
restore against deployment-wide configuration. That first release cannot
promise independent profile selection or immutable per-session configuration.

## Identity and current permission checks

A shared production service needs verified caller OpenID Connect (OIDC) before
it can provide caller isolation. The runtime identifies a person by issuer and
subject. Kubernetes RBAC governs CR access; service mutual TLS (mTLS)
identifies a connecting service. Neither replaces caller identity. A shared
bearer token cannot distinguish users. Private memory further requires
ownership checks and caller-partitioned storage. The separate [agent identity
proposal](agent-identity-model.md) explores delegation; it is not an
implemented guarantee.

The platform attaches `AccessPolicy` limits that a profile author cannot
remove. A caller can use only capabilities allowed by every applicable policy.
`ToolPolicy` preserves deny-dominant permissions and configured Ask. Plan mode
blocks mutations before ordinary permission evaluation. When policies set
numeric ceilings, the lower ceiling wins. The existing [Cedar
adapter](../../internal/adapter/cedarauthority/cedar.go) can only narrow the
tool capabilities already granted to a child; it is not a group-entitlement
service.

Defaults, learning, and retention have separate policy rules. The owning
policy must permit a default override. Lower scopes can tighten learning
limits, but cannot relax them. Conflicting retention obligations need
resolution rather than a generic merge. Posture alone does not establish
project trust.

## Models and external tools

Model inventory reports what a provider offers. A profile selects choices and
routing defaults; entitlements decide which choices a caller can use.
`ModelProvider` binds an endpoint, protocol, trust configuration, and separate
authentication reference. The target runtime checks the final provider and
model after alias resolution, including main, plan, child, and auxiliary calls
such as classification, compaction, checking, and reflection. Schedules follow
the same rules. Fallback stays within the permitted set and fails explicitly
when that set is empty.

The current [`models.allowlist` setting](../../internal/adapter/permconfig/schema.go)
is inert, and unlisted model IDs pass through. Filtering a catalogue cannot
enforce model access. Token caps are runaway brakes, not money caps or global
descendant budgets. Kubernetes quotas do not limit inference spending.
Shared code supports deployment OIDC credentials for providers, but `mecak8s`
does not load them. The first release uses supported service credential paths;
per-user provider OAuth is separate, unsupported work.

`MCPConnection` consumes an approved external streamable-HTTP endpoint or a
ToolHive `MCPServer` or `VirtualMCPServer` endpoint. It does not deploy servers.
ToolHive references need explicit Kubernetes integration and readiness for
the referenced generation; local workload discovery is insufficient.
Direct mode and the embedded broker remain exclusive. An external virtual
endpoint does not replace the embedded broker's session authority.

Each connection declares service authentication or per-user enrollment.
Upstream approval, user consent, and per-call Ask remain separate decisions.
The current broker has process-local attachment state, so deployments need
one replica, `Recreate` updates, and callback routing. Users must re-enroll
after restart. The [deployment guidance](../../user-docs/operating/mecak8s/scale-recover-and-upgrade.md)
explains these limits; adding replicas does not remove them.

## Workspaces and execution ownership

An optional `ExecutionProvider` installs or references a compatible provider.
`ExecutionTemplate` publishes immutable workspace recipes. The existing
private `ExecutionEnvironment` allocation controller remains the sole owner
of allocations and claims. The operator manages its configuration and lifecycle,
not a competing allocation loop.

One installation binds storage identity, Lease coordination, and execution
ownership coherently. Redis uses fixed prefixes, so separate Lease namespaces
do not isolate runtimes that share its keyspace. The
[metadata index](../../adapters/redisstore/metadata_index.go) also constrains Redis
Cluster use. Dedicated state and execution boundaries prevent collisions.
Identity changes require migration that fences old executors before new owners
start mutations; ordinary updates cannot change those identity fields.

[Recipes load at provider startup](../../internal/adapter/executioncontroller/templates.go).
Updates therefore require a maintenance window that stops new work and drains
active mutations. Provider and executor protocols must match. Retain recipe
revisions while allocations use them. Reattachment selects the exact prior
allocation, workspace, and template revision or fails closed. Retained PVCs
have no cascading ownership from disposable resources.

Current remote execution excludes Subagent, Team, Parallel, and repository
source ingestion. It also cannot allocate an independent scheduled workspace.
Required combinations that depend on these features fail publication or
schedule acceptance. An offering label cannot imply support that the runtime
lacks. These limits come from [runtime construction](../../internal/app/build.go).

## Private memory and learning

Memory is structured runtime data, not guidance in a CR. User and project
memory use `tool.MemoryStore` keys and values with revision history. Local
storage uses `memory.json` and `memory.lock` with atomic replacement. Creates
are create-only; updates use compare-and-swap against the current revision.
Forget writes a tombstone. Undo adds a compensating revision rather than
erasing history. Retention limits history access and eventual removal. See
the [memory contract](../architecture/memory.md).

Within an installation, a verified issuer and subject identify the same user's
memory across profiles. Project memory adds the exact server-owned workspace;
it remains private to that caller. A profile neither silently repartitions nor
exports memory. Stronger profile isolation needs a separate contract and
migration. The runtime refreshes the user-memory projection before provider
requests. Project context contains a bounded index without values; `Recall`
retrieves values. Memory remains untrusted data, not instructions or authority.

Current storage needs work before this target is available. `mecak8s` supplies
`UserModelDir`, but not project `MemoryDir`. User-model and learning files do
not become durable merely because sessions use Redis. Scheduled-result
delivery also has an in-memory queue. Owner-enforced runtime configurations
reject unsupported remote memory and learning combinations. A CR alone cannot
safely enable shared learning or substitute an unpartitioned directory.

`LearningPolicy` selects `off`, `review`, or `auto`, with `off` as the default.
Proposal, validation, evaluation, review, and acceptance remain separate.
Review mode needs approval before acceptance. Auto mode accepts private learning
only within approved limits. Automatic fact promotion needs supported explicit
user intent and cannot overwrite conflicting or user-explicit facts. Evidence,
decisions, and resource budgets remain with the private records.

Learned skills have additional limits: changes affect the body only, with no
new assets, tools, or authority. A learned skill may be accepted at the
evaluated level only with `PASS`; validated-level acceptance can permit an
evidence-backed `ABSTAIN` under policy. `FAIL` and errors reject that skill
proposal.

Organization-wide publication requires explicit owner consent, redaction,
review, and publication approval. The operator never turns private records
into shared instructions automatically. This is a separate publication path,
not a side effect of selecting auto learning.

## Platform automation uses the runtime scheduler

`Automation` declares a platform schedule and bridges the runtime's create,
update, pause, and resume operations. Database claims and fire records remain
authoritative. Reconciliation does not call `FireNow`, and there is no second
CronJob scheduler. Personal schedules continue through runtime APIs; external
workflow systems can call those APIs when they already own orchestration.

The bridge derives a deterministic schedule key from installation identity
and the CR UID. If a create response is lost, retry recovers that same schedule.
A name match cannot authorize takeover. The runtime verifies an authenticated
service principal and bridge management identity; a `spec.owner` string cannot
impersonate a person. Safe definition updates need owned-field metadata,
compare-and-swap, and concurrency fencing that current schedule APIs lack.

Acceptance pins the schedule to an exact profile revision. A profile update
does not migrate it; an authorized definition update can select a new revision.
Active fires retain the old binding, and every fire uses current permissions.
The initial defaults prohibit overlap, skip missed fires, and disable automatic
retry, with an explicit time zone. These choices reduce ambiguity but do not
promise exactly-once external effects.

Suspension stops new fires, not active runs. Cancellation is a separate
authorized operation. Deletion suspends first and resolves active or ambiguous
fires before releasing management metadata. It preserves execution evidence.
An active fire can finish under current authority unless cancellation succeeds.

## Installation, recovery, and operational visibility

A `Harness` records dependency bindings and health without taking over the
database, PKI, or ToolHive. New installations use direct server-side apply
with explicit field ownership. Helm adoption needs an inventory, rollback
criteria, and a planned transfer that preserves service and installation
identity. Stop the previous Helm and GitOps writers before adoption; only one
manager owns a field at a time.

Deletion retains databases and workspaces by default. Decommissioning suspends
automation, stops new work, drains execution, and preserves authority metadata
before explicit disposal. Finalizers cannot protect against namespace or CRD
loss. Backups, PVC reclaim policy, and restore procedures must independently
cover sessions and approvals, publication indexes, private history, schedules,
workspaces, and enrollment state. Secret recovery follows separate controls.

During an operator outage, accepted configuration remains usable only while
authority stays fresh, potentially through an independent authority service.
Kubernetes API loss still affects Leases and allocation. Recovery fences old
executors before admitting mutations. Restore checks include paused sessions
and pending approvals. High availability depends on routing, storage,
credentials, and feature state, not replica count alone.

Controllers reconcile configuration idempotently, never each agent turn.
Structural schemas and CEL validate local rules; external downloads and
provider checks run asynchronously, outside admission. Dependency indexes
identify affected profiles. List fields declare apply semantics, and separate
field managers own generated resources and status. Conditions report observed
generation so an old success cannot imply current readiness. Status separates
requested generation, accepted revision, and active runtime-acknowledged
revision. Resource readiness has a specific meaning: available skill content
is not proof that a runtime activated it.

Diagnostics explain failures, tool audit records describe tool execution, and
durable events preserve runtime history. Approval records separately identify
publication decisions. Status contains digests and bounded reasons, not user
content or secrets. Logs exclude content, credentials, and authentication
headers; metrics use bounded labels. CRD evolution needs conversion tests and
storage migration before version removal. Runtime rollout also checks protocol
compatibility and any required execution maintenance.

## Delivery and decisions still needed

**Installation comes first.** Deliver `Harness`, dependency checks, status,
and safe deployment maintenance for a trusted-team service. Qualify real OIDC
login and denial of cross-caller session, schedule, and memory access before
claiming isolation. This stage documents deployment-wide restoration.

**Approved offerings come next.** Add typed content, integrations, policies,
and runtime revision binding after resolving the approval API. Demonstrate an
incident session that survives an update and restart on its original revision.
Reject unauthorized same-namespace references, all cross-namespace references,
and content whose digest, approval scope, redirect, or archive paths fail checks.
Exercise final-model checks, fallback, Ask, and expired authority.

**Execution lifecycle follows.** Qualify exact reattachment through upgrade,
fencing after Lease loss, retained PVCs, and rejection of unsupported remote
combinations. These checks require real-cluster recovery, not only controller
tests or healthy Pod counts.

**Automation and learning complete the later stages.** Recover one schedule
after a crash between creation and mapping persistence, without a duplicate
fire. Verify that definition updates preserve active-fire bindings and that
concurrent writes cannot take over ownership. Prove that private memory or
learned content cannot publish without owner consent and review. These are
future acceptance scenarios, not completed qualification.

Remaining decisions:

- **Publication authority:** Select the authority-record API and its independent
  platform owner before delegated publication.
- **Revocation:** Define authorization freshness, the authenticated update feed,
  and pending-approval behavior before claiming bounded revocation.
- **Storage:** Select artifact and reference-index storage plus caller-scoped
  memory and learning backends before durable binding or private learning.
- **Automation:** Define the runtime management and fencing contract before
  automation manages schedules.
- **Names and identity:** Confirm the API group and resource names, and define
  delegation against the separate identity proposal. Financial quotas and
  hostile tenancy need separate designs.

This document changes no current wire API, Go API, or stored data. Future
profile selection and binding need capability negotiation and a
[wire compatibility assessment](../architecture/api-surface.md#wire-api-compatibility).
`CreateSessionRequest` reserves field 6 and the name `profile`; neither is
available for reuse. Enforcing restrictions on unlisted models can break
existing clients and needs an explicit upgrade assessment and operator approval
before implementation. Go source compatibility and stored-data migration need
separate assessments. Additive CRDs alone do not establish compatibility.

## Design references

The linked implementation sources constrain this proposal. These additional
references explain the principal boundaries and controller conventions:

- [Governance](../architecture/governance.md) and [deployment hardening](../architecture/deployment-and-hardening.md) describe the runtime's security boundaries.
- [Session contract](../../contracts/proto/mecatl/v1/harness.proto), [schedule contract](../../contracts/proto/mecatl/v1/schedule.proto), and [schedule ownership](../../internal/adapter/server/schedule_manager.go) show where runtime extensions belong.
- [Kubernetes custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) and [API conventions](https://github.com/kubernetes/community/blob/main/contributors/devel/sig-architecture/api-conventions.md) inform the configuration boundary and status design.
