package criteria

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unpack runs on the same untrusted trees FromBytes accepts, so it carries the
// same obligation not to panic. Rejecting one is allowed: a non-string field.
func checkUnpack(t *testing.T, e Expression) {
	t.Helper()
	strategy, nodes, err := Unpack(e)
	if err != nil {
		return
	}
	if strategy != StrategyAnd && strategy != StrategyOr {
		t.Fatalf("Unpack returned strategy %q", strategy)
	}
	parts, err := e.(Builder).Params()
	if err != nil {
		t.Fatalf("Params() failed after Unpack succeeded: %v", err)
	}
	if len(nodes) != len(parts) {
		t.Fatalf("Unpack returned %d nodes for %d parts", len(nodes), len(parts))
	}
	for _, node := range nodes {
		if node.Nested != nil {
			checkUnpack(t, node.Nested)
		}
	}
}

func TestAddLowersAConditionToTheSameTupleAsRawArguments(t *testing.T) {
	typed := And(Cond("status", "eq", []string{"active", "locked"})).
		Add(Cond("facility", "eq", "X1"))
	raw := And("status", "eq", []string{"active", "locked"}).
		Add("facility", "eq", "X1")

	typedParams, err := typed.(Builder).Params()
	require.Nil(t, err)
	rawParams, err := raw.(Builder).Params()
	require.Nil(t, err)
	assert.Equal(t, rawParams, typedParams)

	typedJSON, err := json.Marshal(typed)
	require.Nil(t, err)
	rawJSON, err := json.Marshal(raw)
	require.Nil(t, err)
	assert.Equal(t, string(rawJSON), string(typedJSON))
	assert.Equal(
		t,
		`{"conditions":[["status","eq",["active","locked"]],["facility","eq","X1"]],"mode":"and"}`,
		string(typedJSON),
	)
}

// Params() carrying a Condition would change the wire format for every
// consumer that marshals a tree.
func TestParamsNeverHoldsACondition(t *testing.T) {
	e := And(Condition{Field: "a", Operation: "eq", Value: 1}).
		Add(Or(Condition{Field: "b", Operation: "ne", Value: nil}))

	var walk func(Expression)
	walk = func(e Expression) {
		parts, err := e.(Builder).Params()
		require.Nil(t, err)
		for _, part := range parts {
			if nested, ok := part.(Expression); ok {
				walk(nested)
				continue
			}
			assert.IsType(t, []any{}, part)
		}
	}
	walk(e)
}

func TestAddCatchesAnExpressionInEitherPosition(t *testing.T) {
	trailing := And("field", "eq", 17, And("field", "ne", 6))
	_, err := trailing.(Builder).Params()
	assert.ErrorContains(t, err, "cannot mix Expressions and operations")

	// Three arguments, so the arity check used to let this one through.
	leading := And(And("field", "ne", 6), "eq", 17)
	_, err = leading.(Builder).Params()
	assert.ErrorContains(t, err, "cannot mix Expressions and operations")
}

// Dropping the parts of a failed call would marshal to a tree that parses
// cleanly with the condition silently missing.
func TestAddKeepsThePartsOfAFailedCall(t *testing.T) {
	e := And("a", "eq", 1).Add("b", "eq")
	_, err := e.(Builder).Params()
	require.ErrorContains(t, err, "operations support only 3 argument")

	encoded, err := json.Marshal(e)
	require.Nil(t, err)
	assert.Equal(t, `{"conditions":[["a","eq",1],["b","eq"]],"mode":"and"}`, string(encoded))

	_, err = FromBytes(encoded)
	assert.ErrorContains(t, err, "condition 1: operations support only 3 argument, got 2")
}

// The flat form only exists because Cond frees the three argument slots that a
// raw triple takes up.
func TestCondBuildsTheSameTreeAsChainedRawTriples(t *testing.T) {
	flat := And(
		Cond("status", "eq", "paid"),
		Or(Cond("channel", "eq", "web"), Cond("channel", "eq", "app")),
		Cond("total", "gt", 100),
	)
	chained := And("status", "eq", "paid").
		Add(Or("channel", "eq", "web").Add("channel", "eq", "app")).
		Add("total", "gt", 100)

	flatJSON, err := json.Marshal(flat)
	require.Nil(t, err)
	chainedJSON, err := json.Marshal(chained)
	require.Nil(t, err)
	assert.Equal(t, string(chainedJSON), string(flatJSON))

	assert.Equal(t, Condition{Field: "id", Operation: "eq", Value: 1}, Cond("id", "eq", 1))
}

func TestAFailedTreeStillHoldsConditionsAsTuples(t *testing.T) {
	e := And("a", "eq", 1, Condition{Field: "b", Operation: "ne", Value: nil})
	_, err := e.(Builder).Params()
	require.ErrorContains(t, err, "cannot mix Expressions and operations")

	encoded, err := json.Marshal(e)
	require.Nil(t, err)
	assert.Equal(t, `{"conditions":["a","eq",1,["b","ne",null]],"mode":"and"}`, string(encoded))
}

func TestUnpack(t *testing.T) {
	source := `{"mode":"or","conditions":[["id","lt",7],{"mode":"and","conditions":[["id","eq",null]]}]}`
	e, err := FromBytes([]byte(source))
	require.Nil(t, err)

	strategy, nodes, err := Unpack(e)
	require.Nil(t, err)
	assert.Equal(t, StrategyOr, strategy)
	require.Len(t, nodes, 2)

	assert.Nil(t, nodes[0].Nested)
	assert.Equal(t, Condition{Field: "id", Operation: "lt", Value: 7.}, nodes[0].Condition)

	require.NotNil(t, nodes[1].Nested)
	nestedStrategy, nested, err := Unpack(nodes[1].Nested)
	require.Nil(t, err)
	assert.Equal(t, StrategyAnd, nestedStrategy)
	require.Len(t, nested, 1)
	assert.Equal(t, Condition{Field: "id", Operation: "eq", Value: nil}, nested[0].Condition)
}

func TestUnpackNil(t *testing.T) {
	strategy, nodes, err := Unpack(nil)
	assert.Nil(t, err)
	assert.Equal(t, StrategyAnd, strategy)
	assert.Empty(t, nodes)
}

func TestUnpackNormalisesTheEmptyStrategy(t *testing.T) {
	e, err := FromBytes([]byte(`{"conditions":[["a","eq",1]]}`))
	require.Nil(t, err)
	strategy, _, err := Unpack(e)
	assert.Nil(t, err)
	assert.Equal(t, StrategyAnd, strategy)
}

// FromBytes accepts a non-string field and Params() hands it back untouched;
// only the typed view rejects it, because Condition.Field is a string.
func TestUnpackRejectsANonStringFieldThatFromBytesAccepts(t *testing.T) {
	e, err := FromBytes([]byte(`{"mode":"and","conditions":[[5,"eq",1]]}`))
	require.Nil(t, err)
	params, err := e.(Builder).Params()
	require.Nil(t, err)
	assert.Equal(t, []any{[]any{5., "eq", 1.}}, params)

	_, _, err = Unpack(e)
	assert.ErrorContains(t, err, "condition 0: field must be a string, got float64")
}

func TestUnpackRejectsAHandBuiltNonStringOperation(t *testing.T) {
	_, _, err := Unpack(And("a", 5, 1))
	assert.ErrorContains(t, err, "condition 0: operation must be a string, got int")
}

func TestUnpackReportsADeferredError(t *testing.T) {
	_, _, err := Unpack(And("a", "eq"))
	assert.ErrorContains(t, err, "operations support only 3 argument")
}

func TestUnpackReportsAnInvalidStrategy(t *testing.T) {
	e := expr("prem").Add("a", "eq", 1)
	_, _, err := Unpack(e)
	assert.ErrorContains(t, err, "invalid expression strategy: 'prem'")
}

func TestErr(t *testing.T) {
	assert.Nil(t, Err(nil))
	assert.Nil(t, Err(And("a", "eq", 1)))
	assert.ErrorContains(t, Err(And("a", "eq")), "operations support only 3 argument")
	assert.ErrorContains(t, Err(And("a", "eq", 1, And())), "cannot mix Expressions and operations")
}

func TestStrategyIsValidDoesNotAllocate(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		_ = StrategyOr.IsValid()
	})
	assert.Zero(t, allocs)
}
