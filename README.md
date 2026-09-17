# gorib/criteria

An AST for filter expressions: `and`/`or` trees of `[field, operation, value]`
triples that round-trip through JSON. A service accepts one from a client, and a
backend — `sql`, an Elasticsearch builder, whatever comes next — compiles it
into a query.

The package knows the shape of a filter: nesting, strategy, arity. It knows
nothing about what a field or an operation *means*. That belongs to whoever
consumes the tree.

## Building

```go
q := criteria.And("status", "eq", "paid").
	Add(criteria.Or("channel", "eq", "web").Add("channel", "eq", "app")).
	Add("total", "gt", 100)
```

`And` and `Or` both return an `Expression`, and `Add` accepts either three
values — one operation — or any number of nested `Expression`s and
`Condition`s. A single call cannot mix the two forms.

```go
criteria.And("id", "eq", 1, criteria.And("x", "ne", 2)) // error: cannot mix
criteria.And("id", "eq")                                // error: 3 arguments
```

A raw triple takes up all three argument slots, so everything else has to be
chained on. `Cond` frees them, and the tree can be written as one:

```go
q := criteria.And(
	criteria.Cond("status", "eq", "paid"),
	criteria.Or(
		criteria.Cond("channel", "eq", "web"),
		criteria.Cond("channel", "eq", "app"),
	),
	criteria.Cond("total", "gt", 100),
)
```

A `Condition` is lowered into the same triple the package stores everywhere
else, so both forms build the same tree and encode to the same JSON — a
`Condition` never appears in `Params()`.

## Errors arrive later

`Add` returns the expression, never an error — a malformed call sets a failure
flag on the tree instead. The flag surfaces when a consumer reads the tree:

```go
type Builder interface {
	Strategy() (Strategy, error)
	Params() ([]any, error)
}
```

Both methods return the recorded error, as does `Unpack`; `criteria.Err(expr)`
asks for it directly. This keeps building fluent — an expression assembled
across ten lines of filter mapping has no error to thread through — at the cost
of moving the failure to the point of use, wherever the backend compiles it.

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
expr, err := criteria.FromBytes(request.Criteria)
if err != nil {
	return nil, err
}
return repo.List(ctx, expr) // nil expression = no filter
```

Numbers arrive as `float64`, as they do from any `encoding/json` decode into
`any`. Nothing converts them back — the backend binds whatever it is given.

## Reading a tree

A backend does not destructure the tree by hand:

```go
strategy, nodes, err := criteria.Unpack(expr)
if err != nil {
	return err
}
for _, node := range nodes {
	if node.Nested != nil {
		// recurse into the sub-expression
		continue
	}
	// node.Condition.Field, .Operation, .Value
}
```

`Unpack` normalises the strategy, reports the deferred error, and guarantees
the shape: three elements, a string field, a string operation. `Unpack(nil)`
returns an empty list and no error, so an absent filter needs no special case
at the call site.

Note the asymmetry with `FromBytes`, which accepts `[5, "eq", 1]` and hands the
field back untouched through `Params()`. The typed view is stricter because
`Condition.Field` is a string; the untyped path is unchanged.

## Operations stay opaque

The operation is a string this package never interprets. That is deliberate:
the vocabularies do not nest.

| consumer | operations |
|---|---|
| `sql` constructors | `eq`, `ne`, `gt`, `ge`, `lt`, `le`, `between`, `like`, `ilike`, `json_in`, `json_eq` |
| `sql` drivers | `=`, `in`, `!=`, `not in`, `between`, the JSONB key operators |
| an Elasticsearch builder | `eq`, `ne`, `in`, `nin`, `exists`, `missing` |

`exists` means nothing in SQL, `between` means nothing in Elasticsearch, and
`nin` and `not in` are the same thing under two names. An enum here would be
the union of every backend, with each backend rejecting most of it. Negation is
the same story: `must_not` is an Elasticsearch idea, not a filter one.

Each consumer documents its own vocabulary, rejects what it does not know, and
handles field naming and escaping itself.

## What it does not do

- No validation of what a field, operation or value *means*. `Unpack` checks
  that a field and an operation are strings; nothing checks that they exist or
  suit each other.
- No evaluation — nothing here can test a struct against an expression.
- Not concurrency-safe: build a tree in one goroutine, then treat it as
  read-only.
