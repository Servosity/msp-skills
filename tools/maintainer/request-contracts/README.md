# Executor request contracts

The nine connectors in issue #351 embed `request_contracts.json` beside their
MCP executor. These schemas describe executor parameters using the checked-in
CLI request construction as the primary evidence. They are metadata, not a
replacement for vendor validation.

Regenerate a connector from the repository root using the standard library only:

```sh
GO111MODULE=off go run tools/maintainer/request-contracts/main.go -slug hudu
GO111MODULE=off go test ./tools/maintainer/request-contracts
```

The extractor parses Go syntax, tracks flag declarations, required-flag guards,
request-map assignments, parsed JSON values, nested maps, converted scalar and
list values, positional substitutions, and header bindings. It resolves catalog
identifiers against exact HTTP method and path when generated CLI identifiers
differ. A function containing multiple annotated commands is rejected. Repeated
annotation IDs on distinct paths remain resolvable by method and path; duplicate
ID/method/path identities across files are rejected. Catalog contracts receive
independent copies, so one endpoint's metadata cannot mutate another's.
Missing catalog coverage or ambiguous matches fail instead of silently
emitting an empty contract.

Body properties keep their exact wire spelling and nesting. Query and header
properties keep CLI public flag names with `x-wire-name` when different.
Catalog wire aliases do not create a second input for an already-bound location
and wire name. Native enum evidence under a wire alias is merged into the source
public property. Callers should prefer the schema public name; if both it and a
legacy wire alias are supplied, the public name takes precedence. A
transport/body name collision uses `path_<name>`, `query_<name>`, or
`header_<name>` for the transport parameter and leaves the body's wire key
intact. Assignment order does not change these aliases or their requiredness. `x-location` records each top-level parameter's
transport location. Catalog array bodies use `body` with `x-raw-body: true` for methods that send
bodies. GET and HEAD catalog array hints do not invent request bodies.
Object schemas allow additional properties because CLI stdin JSON also accepts
vendor fields beyond the generated flags. Arbitrary parsed JSON is unconstrained
unless a source assertion or recorded native schema proves a narrower type.
Array assertions use the declared element type; `[]any` and `[]interface{}`
remain unconstrained instead of being mislabeled as string arrays. Typed slice
flags retain their scalar element type. Source and supplemental required lists
are combined as sets, including lists loaded from JSON.

`supplements.json` records additional evidence from the pre-existing native
Printing Press specifications. Each supplemented endpoint carries a source label,
SHA-256, and endpoint locator. It preserves explicit native enum values for
source-exposed fields and narrows otherwise unconstrained parsed JSON. It does
not change types where the CLI serializes a different type. Two NinjaOne batch
operations require full supplemental contracts: their catalog entries survived,
but their generated CLI constructors were overwritten by command-group parents.
Their native request bodies are arrays of integer template identifiers.

The checked-in supplements make regeneration independent of a developer's local
Printing Press library. When updating them, review the corresponding native
schema against the current CLI request construction, retain the evidence, and
rerun the extractor and connector MCP tests. Enums unavailable in checked-in CLI
source or recorded native evidence remain unspecified. Never invent enum values
from prose such as "An enumeration."

The extractor does not edit CLI commands. Reprints must preserve the separate
hand-fix ledger, regenerate these schemas, and run request serialization tests.
