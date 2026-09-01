# gorib/criteria

Filter expressions: `and`/`or` trees of `[field, operation, value]` triples that
round-trip through JSON. An API accepts one from a client, `sql` turns it into a
`WHERE` clause.

The package is deliberately ignorant of SQL. It knows the shape of a filter —
nesting, strategy, arity — and nothing about what a field or an operation means.
That belongs to whoever consumes the tree.

## Building

```go
q := criteria.And("status", "eq", "paid").
	Add(criteria.Or("channel", "eq", "web").Add("channel", "eq", "app")).
	Add("total", "gt", 100)
```

`And` and `Or` both return an `Expression`, and `Add` accepts either three
values — one operation — or any number of nested `Expression`s. The two cannot
be mixed in a single `Add` call.

```go
criteria.And("id", "eq", 1, criteria.And("x", "ne", 2)) // error: cannot mix
criteria.And("id", "eq")                                // error: 3 arguments
```

## Errors arrive later

`Add` returns the expression, never an error — a malformed call sets a failure
flag on the tree instead. The flag surfaces when a consumer reads the tree
through the `Builder` interface:

```go
type Builder interface {
	Strategy() (Strategy, error)
	Params() ([]any, error)
}
```

Both methods return the recorded error. This keeps building fluent — an
expression assembled across ten lines of filter mapping has no error to thread
through — at the cost of moving the failure to the point of use. `sql` reports
it from `Build()`.

An empty strategy reads as `and`; anything other than `and` or `or` is
`invalid expression strategy`.

## JSON

```json
{
  "mode": "or",
  "conditions": [
    ["id", "lt", 7],
    {"mode": "and", "conditions": [["id", "eq", null]]}
  ]
}
```

An object is a nested expression, an array is an operation. `FromBytes` parses
this form, returning `(nil, nil)` for empty input so an absent filter parameter
needs no special case at the call site:

```go
expr, err := criteria.FromBytes(raw)
if err != nil {
	return nil, err
}
q := sql.Select().From(sql.Table("orders")).Where(expr) // nil = no WHERE
```

Numbers arrive as `float64`, as they do from any `encoding/json` decode into
`any`. Nothing converts them back — the driver binds whatever it is given.

## Operations

The vocabulary is the consumer's. For `sql` on pgx it is:

| Alias | SQL |
|---|---|
| `eq`, `in`, `=` | `=`, or `IN (…)` for a slice, or `IS NULL` for nil |
| `ne`, `neq`, `nin`, `!=` | `!=` / `NOT IN` / `IS NOT NULL` |
| `gt`, `ge`, `gte`, `lt`, `le`, `lte` | the corresponding comparison |
| `between` | `BETWEEN a AND b` — the value must be a two-element slice |
| `like`, `ilike` | pattern match |
| `json_in` (`@>`), `json_eq` (`?`) | JSONB containment and key existence |

An unknown alias is an error from the driver, not from this package. Field
names are sanitized by `sql` (camelCase to snake_case, injection characters
rejected), also not here.

## What it does not do

- No validation of fields, operations or value types.
- No evaluation — nothing here can test a struct against an expression.
- Not concurrency-safe: build a tree in one goroutine, then treat it as
  read-only.
