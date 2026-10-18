# REQ-027: Independent JSON resources and unified activation

1. Install resources.d/TEMPLATE.json and English README.md/README.txt; never open,
   parse or apply the exact template filename. Preserve custom definitions during
   update, refreshing references only. Ignore inactive/non-.json files.
2. Accept config-only, log-only, service-only and combined standard JSON definitions.
   Require one section and explicit role references. Reject unknown/duplicate
   properties, duplicate IDs/paths, ambiguous names, unsafe links/permissions and
   excessive input before activation.
3. Configuration fields use the existing typed, owner/ACL/comment-preserving
   writer. Service-less configuration does not activate any application service.
4. Each log is one fixed-path streaming operation with independent roles and
   verified read access. Registration changes no OS privileges/ACLs/ownership.
5. Each service action is one explicit signed unit/action pair. Missing actions
   grant nothing; no caller-selected unit or shell command is accepted.
6. termctl check validates without signing or changing active configuration.
   termctl apply validates, signs and activates the entire generated definition
   set. Support local and exact-match offline signing without trust rotation.
7. Journal registry, writer sandbox, generation manifest and catalog together;
   restore previous active state on failure, retaining failed recovery for retry.
   Idempotent apply does not restart services. Share existing lifecycle guards.
8. Preserve legacy catalog/registry and source files. Track only generated IDs for
   replacement/removal. Removing a source unregisters its generated capabilities
   after apply without deleting data or stopping the external application service.
9. SSH and Console expose permission-filtered service/log paths and the config
   editor through the existing authenticated gateway/manager boundaries.
