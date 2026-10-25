# Configurable logical configuration CLI — discussion draft

Status: Historical discussion draft, superseded for implemented behavior by
ADR-037 and configs/resources.d/README.md. The JSON sketch below was not the final
schema: use config.contexts with inline named fields, as the installed guide shows.

## Confirmed direction

Use FortiOS-like logical configuration contexts rather than exposing physical
file names/section names as the only navigation model. Keep the existing Tab
behavior (common-prefix completion and listing), not cycling. Remove root show;
retain show inside configuration contexts. Rename the default trace operation to
capture and remove packaged nslookup/traceroute operations and executable bindings.
Execution catalog extensibility and existing permission/isolation boundaries stay.
Configuration saves must not automatically restart/reload services; service
management is always explicit through execute service. The current editor
activation feature is not removed by this discussion draft.

## Proposed model

Register each physical file once using the existing resources.d config definition.
Retain its canonical path, format, fields, types, constraints and permissions.
Add declarative CLI views over its registered coordinates. A context path is a
list of tokens. Every field alias points to an existing (section,key) coordinate;
views never create raw filesystem or arbitrary-key access.

An illustrative mapping, proposed new properties rather than current schema:

```json
{
  "cli": {
    "contexts": [
      {
        "path": ["tunnel"],
        "fields": {
          "tcpmss": {"section": "Tunnel", "key": "TCPMSS"}
        }
      }
    ]
  }
}
```

The referenced physical field is separately registered with its actual type and
constraints (for example integer with administrator-selected bounds). Format is
still ini. Changing the logical alias never changes the INI key. This avoids
mixing validation definitions with multiple competing view definitions.

Proposed terminal experience:

```text
keyhan # config tunnel
keyhan (tunnel) # show
    set tcpmss 1360
keyhan (tunnel) # set tcpmss 1400
keyhan (tunnel) # diff
keyhan (tunnel) # end
```

This is a singleton context and does not require edit. An application context
with predeclared entries may instead expose edit cat/edit dog. The choice of
singleton versus entry table must be explicit in the schema, not guessed from
physical INI sections. Creation/deletion remains disallowed unless a future
explicit policy and writer capability are designed and approved.

Build contextual ?, completion and visibility from this model and session roles.
Reject duplicate/prefix-conflicting context routes, duplicate field aliases,
unregistered coordinates, missing references and ambiguous mappings during
termctl check. View-level restrictions may narrow existing resource permissions;
they cannot grant access to a field denied by the underlying resource.

Multiple views can refer to different sections of one physical file. One file
has one writer/version authority. Saves check its version, update only authorized
registered coordinates, validate the final document and atomically replace it
while preserving comments/ownership/ACLs. No multi-file atomicity is implied.

## Remaining design contracts

- Exact configuration-view schema, singleton/table navigation and aliases.
- Field syntax is selected: config tunnel followed by set tcpmss 1400.
  Configuration-local show and diff remain available. The exact declarative
  schema remains open; avoid requiring duplicate validation definitions.
- FortiOS-like next/end save boundaries and abort behavior for shared files;
  current SafeTerm still saves the whole resource draft at end.
- Defaults and unset: unset may restore only a declared default and must not
  silently mean empty/delete. Existing allow_empty controls set FIELD "".
- Save behavior selected: validate/store only; service actions are always
  explicit. Remove automatic on-change activation from the redesigned config
  flow once implementation is authorized.
- Immediate ? handling and contextual descriptions without changing Tab behavior.

No editor storage/navigation behavior is changed until these contracts are agreed.
