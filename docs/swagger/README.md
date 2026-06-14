# OneFS OpenAPI spec

`make schemas` reads the full OneFS OpenAPI spec from this directory (default
`11035-9.14.0.json`) to regenerate `internal/powerscale/testdata/onefs_schemas.json`,
which the schema-drift guard (`internal/powerscale/schema_guard_test.go`) checks fixtures
against.

The spec itself is git-ignored (≈5 MB). Obtain it from Dell's PowerScale OneFS API
documentation / `isilon_sdk` for the target release and place it here before running
`make schemas`.
