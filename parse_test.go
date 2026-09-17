package criteria

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkShape asserts the contract sql/where.go relies on: a part is either a
// nested Expression or a 3-element []any whose operation is a string.
func checkShape(t *testing.T, e Expression) {
	t.Helper()
	b, ok := e.(Builder)
	if !ok {
		t.Fatalf("expression %T does not implement Builder", e)
	}
	parts, err := b.Params()
	if err != nil {
		t.Fatalf("Params() on a tree FromBytes accepted: %v", err)
	}
	for _, part := range parts {
		if nested, ok := part.(Expression); ok {
			checkShape(t, nested)
			continue
		}
		v, ok := part.([]any)
		if !ok {
			t.Fatalf("condition %#v is not []any", part)
		}
		if len(v) != 3 {
			t.Fatalf("condition %#v has %d elements, want 3", v, len(v))
		}
		if _, ok := v[1].(string); !ok {
			t.Fatalf("condition %#v has non-string operation %T", v, v[1])
		}
	}
}

func TestFromBytesRejects(t *testing.T) {
	cases := []struct {
		name   string
		source string
		err    string
	}{
		// A syntax error is caught by encoding/json before UnmarshalJSON runs,
		// so it carries no "criteria unmarshal" prefix.
		{"NotJSON", `{`, "unexpected end of JSON input"},
		{"TrailingGarbage", `{"conditions":[]}x`, "invalid character"},
		{"TopLevelArray", `[1,2,3]`, "criteria unmarshal"},
		{"TopLevelString", `"filter"`, "criteria unmarshal"},
		{"TopLevelNumber", `7`, "criteria unmarshal"},
		{"TopLevelBool", `true`, "criteria unmarshal"},

		{"NoConditions", `{}`, "condition is required but missing"},
		{"NullDocument", `null`, "condition is required but missing"},
		{"NullConditions", `{"mode":"and","conditions":null}`, "condition is required but missing"},
		{"ModeOnly", `{"mode":"or"}`, "condition is required but missing"},

		{"ConditionsNumber", `{"mode":"and","conditions":5}`, "conditions must be an array, got float64"},
		{"ConditionsString", `{"mode":"and","conditions":"x"}`, "conditions must be an array, got string"},
		{"ConditionsBool", `{"mode":"and","conditions":true}`, "conditions must be an array, got bool"},
		{"ConditionsObject", `{"mode":"and","conditions":{}}`, "conditions must be an array, got map[string]interface {}"},

		{"ConditionString", `{"mode":"and","conditions":["x"]}`, "condition 0: must be an object or an array, got string"},
		{"ConditionNumber", `{"mode":"and","conditions":[5]}`, "condition 0: must be an object or an array, got float64"},
		{"ConditionBool", `{"mode":"and","conditions":[false]}`, "condition 0: must be an object or an array, got bool"},
		{"ConditionNull", `{"mode":"and","conditions":[null]}`, "condition 0: must be an object or an array, got <nil>"},
		{"SecondConditionBad", `{"mode":"and","conditions":[["a","eq",1],"x"]}`, "condition 1: must be an object or an array, got string"},

		{"ArityZero", `{"mode":"and","conditions":[[]]}`, "condition 0: operations support only 3 argument, got 0"},
		{"ArityOne", `{"mode":"and","conditions":[["a"]]}`, "condition 0: operations support only 3 argument, got 1"},
		{"ArityTwo", `{"mode":"and","conditions":[["a","eq"]]}`, "condition 0: operations support only 3 argument, got 2"},
		{"ArityFour", `{"mode":"and","conditions":[["a","eq",1,2]]}`, "condition 0: operations support only 3 argument, got 4"},

		{"OperationNumber", `{"mode":"and","conditions":[["a",5,1]]}`, "condition 0: operation must be a string, got float64"},
		{"OperationNull", `{"mode":"and","conditions":[["a",null,1]]}`, "condition 0: operation must be a string, got <nil>"},
		{"OperationBool", `{"mode":"and","conditions":[["a",true,1]]}`, "condition 0: operation must be a string, got bool"},
		{"OperationArray", `{"mode":"and","conditions":[["a",["eq"],1]]}`, "condition 0: operation must be a string, got []interface {}"},
		{"OperationObject", `{"mode":"and","conditions":[["a",{},1]]}`, "condition 0: operation must be a string, got map[string]interface {}"},

		{"ModeNumber", `{"mode":5,"conditions":[["a","eq",1]]}`, "mode must be a string, got float64"},
		{"ModeArray", `{"mode":[],"conditions":[["a","eq",1]]}`, "mode must be a string, got []interface {}"},
		{"ModeObject", `{"mode":{},"conditions":[["a","eq",1]]}`, "mode must be a string, got map[string]interface {}"},

		{
			"NestedBadOperation",
			`{"mode":"and","conditions":[{"mode":"or","conditions":[["a",5,1]]}]}`,
			"condition 0: condition 0: operation must be a string, got float64",
		},
		{
			"NestedBadArity",
			`{"mode":"and","conditions":[["a","eq",1],{"mode":"or","conditions":[["b","eq"]]}]}`,
			"condition 1: condition 0: operations support only 3 argument, got 2",
		},
		{
			"NestedMissingConditions",
			`{"mode":"and","conditions":[{"mode":"or"}]}`,
			"condition 0: condition is required but missing",
		},
		{
			"DeeplyNestedBadOperation",
			`{"conditions":[{"conditions":[{"conditions":[["a",1,2]]}]}]}`,
			"condition 0: condition 0: condition 0: operation must be a string, got float64",
		},
	}

	for _, c := range cases {
		t.Run(
			c.name,
			func(t *testing.T) {
				e, err := FromBytes([]byte(c.source))
				assert.Nil(t, e)
				assert.ErrorContains(t, err, c.err)
			},
		)
	}
}

func TestFromBytesAccepts(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		strategy Strategy
		params   []any
	}{
		{
			"Empty",
			``,
			"",
			nil,
		},
		{
			"NoConditions",
			`{"mode":"and","conditions":[]}`,
			StrategyAnd,
			[]any{},
		},
		{
			"Single",
			`{"mode":"and","conditions":[["id","eq",1]]}`,
			StrategyAnd,
			[]any{[]any{"id", "eq", 1.}},
		},
		{
			"MissingMode",
			`{"conditions":[["id","eq",1]]}`,
			StrategyAnd,
			[]any{[]any{"id", "eq", 1.}},
		},
		{
			"NullMode",
			`{"mode":null,"conditions":[["id","eq",1]]}`,
			StrategyAnd,
			[]any{[]any{"id", "eq", 1.}},
		},
		{
			"Or",
			`{"mode":"or","conditions":[["id","eq",1]]}`,
			StrategyOr,
			[]any{[]any{"id", "eq", 1.}},
		},
		{
			"ValueTypes",
			`{"mode":"and","conditions":[["a","eq",null],["b","eq",true],["c","eq","s"],["d","eq",1.5],["e","in",[1,2]],["f","json_in",{"k":"v"}]]}`,
			StrategyAnd,
			[]any{
				[]any{"a", "eq", nil},
				[]any{"b", "eq", true},
				[]any{"c", "eq", "s"},
				[]any{"d", "eq", 1.5},
				[]any{"e", "in", []any{1., 2.}},
				[]any{"f", "json_in", map[string]any{"k": "v"}},
			},
		},
		{
			"NonStringField",
			`{"mode":"and","conditions":[[5,"eq",1]]}`,
			StrategyAnd,
			[]any{[]any{5., "eq", 1.}},
		},
		{
			"UnknownOperationAlias",
			`{"mode":"and","conditions":[["a","totally_unknown",1]]}`,
			StrategyAnd,
			[]any{[]any{"a", "totally_unknown", 1.}},
		},
	}

	for _, c := range cases {
		t.Run(
			c.name,
			func(t *testing.T) {
				e, err := FromBytes([]byte(c.source))
				require.Nil(t, err)
				if c.source == "" {
					assert.Nil(t, e)
					return
				}
				require.NotNil(t, e)
				checkShape(t, e)

				b, ok := e.(Builder)
				require.True(t, ok)
				strategy, err := b.Strategy()
				assert.Nil(t, err)
				assert.Equal(t, c.strategy, strategy)
				params, err := b.Params()
				assert.Nil(t, err)
				assert.Equal(t, c.params, params)
			},
		)
	}
}

func TestFromBytesNested(t *testing.T) {
	source := `{"mode":"or","conditions":[["id","lt",7],{"mode":"and","conditions":[["id","eq",null]]}]}`
	e, err := FromBytes([]byte(source))
	require.Nil(t, err)
	checkShape(t, e)

	assert.Equal(t, Or("id", "lt", 7.).Add(And("id", "eq", nil)), e)

	b, ok := e.(Builder)
	require.True(t, ok)
	params, err := b.Params()
	require.Nil(t, err)
	require.Len(t, params, 2)
	assert.Equal(t, []any{"id", "lt", 7.}, params[0])

	nested, ok := params[1].(Expression)
	require.True(t, ok, "a sub-expression must stay a criteria.Expression in Params")
	nestedBuilder, ok := nested.(Builder)
	require.True(t, ok)
	nestedStrategy, err := nestedBuilder.Strategy()
	assert.Nil(t, err)
	assert.Equal(t, StrategyAnd, nestedStrategy)
	nestedParams, err := nestedBuilder.Params()
	assert.Nil(t, err)
	assert.Equal(t, []any{[]any{"id", "eq", nil}}, nestedParams)
}

// An invalid strategy stays deferred to Strategy(), as the README documents.
func TestFromBytesDefersInvalidStrategy(t *testing.T) {
	e, err := FromBytes([]byte(`{"mode":"prem","conditions":[["a","eq",1]]}`))
	require.Nil(t, err)
	require.NotNil(t, e)

	b, ok := e.(Builder)
	require.True(t, ok)
	params, err := b.Params()
	assert.Nil(t, err)
	assert.Equal(t, []any{[]any{"a", "eq", 1.}}, params)

	strategy, err := b.Strategy()
	assert.EqualError(t, err, "invalid expression strategy: 'prem'")
	assert.Equal(t, Strategy(""), strategy)
}

// FromBytes must never hand back a tree whose Params() reports a deferred error.
func TestFromBytesNeverReturnsAFailedTree(t *testing.T) {
	broken := And("field", "eq", 17, "fire", "ne", nil)
	b, ok := broken.(Builder)
	require.True(t, ok)
	_, err := b.Params()
	require.ErrorContains(t, err, "operations support only 3 argument")

	source, err := json.Marshal(broken)
	require.Nil(t, err)
	e, err := FromBytes(source)
	assert.Nil(t, e)
	assert.ErrorContains(t, err, "operations support only 3 argument, got 6")
}

// A tree FromBytes accepted must survive its own encoding: an empty nested
// expression used to marshal to "conditions":null, which parse then rejected.
func TestFromBytesRoundTripsEmptyNested(t *testing.T) {
	source := `{"mode":"and","conditions":[["a","eq",1],{"mode":"or","conditions":[]}]}`
	e, err := FromBytes([]byte(source))
	require.Nil(t, err)

	encoded, err := json.Marshal(e)
	require.Nil(t, err)
	assert.Equal(
		t,
		`{"conditions":[["a","eq",1],{"conditions":[],"mode":"or"}],"mode":"and"}`,
		string(encoded),
	)

	again, err := FromBytes(encoded)
	assert.Nil(t, err)
	assert.Equal(t, e, again)
}

func nestJSON(depth int) string {
	return strings.Repeat(`{"mode":"or","conditions":[`, depth) +
		`{"mode":"and","conditions":[["f","eq",1]]}` +
		strings.Repeat(`]}`, depth)
}

func TestFromBytesDeepNesting(t *testing.T) {
	t.Run(
		"Accepted",
		func(t *testing.T) {
			e, err := FromBytes([]byte(nestJSON(500)))
			require.Nil(t, err)
			checkShape(t, e)

			depth := 0
			for {
				b, ok := e.(Builder)
				require.True(t, ok)
				params, err := b.Params()
				require.Nil(t, err)
				require.Len(t, params, 1)
				nested, ok := params[0].(Expression)
				if !ok {
					assert.Equal(t, []any{"f", "eq", 1.}, params[0])
					break
				}
				e = nested
				depth++
			}
			assert.Equal(t, 500, depth)
		},
	)

	// encoding/json caps nesting at 10000 levels, and one criteria level costs
	// two of them. That ceiling is what bounds parse's recursion, so no input
	// can drive it deep enough to overflow the stack.
	t.Run(
		"DecoderCapsRecursion",
		func(t *testing.T) {
			_, err := FromBytes([]byte(nestJSON(4998)))
			assert.Nil(t, err)
			_, err = FromBytes([]byte(nestJSON(4999)))
			assert.EqualError(t, err, "exceeded max depth")
			_, err = FromBytes([]byte(nestJSON(50000)))
			assert.EqualError(t, err, "exceeded max depth")
		},
	)

	t.Run(
		"UnbalancedBrackets",
		func(t *testing.T) {
			_, err := FromBytes([]byte(strings.Repeat(`{"conditions":[`, 50000)))
			assert.NotNil(t, err)
		},
	)
}

// randomJSON builds an arbitrary JSON value, biased towards the shapes criteria
// cares about so the generator reaches the parser rather than bouncing off
// encoding/json.
func randomJSON(r *rand.Rand, depth int) any {
	if depth <= 0 {
		return nil
	}
	switch r.Intn(10) {
	case 0:
		return nil
	case 1:
		return r.Float64()
	case 2:
		return r.Intn(2) == 0
	case 3:
		return fmt.Sprintf("s%d", r.Intn(5))
	case 4, 5:
		n := r.Intn(5)
		out := make([]any, n)
		for i := range out {
			out[i] = randomJSON(r, depth-1)
		}
		return out
	case 6, 7:
		out := map[string]any{}
		for i := 0; i < r.Intn(4); i++ {
			out[fmt.Sprintf("k%d", r.Intn(4))] = randomJSON(r, depth-1)
		}
		return out
	default:
		out := map[string]any{}
		if r.Intn(4) > 0 {
			modes := []any{"and", "or", "prem", "", 5., nil, []any{}}
			out["mode"] = modes[r.Intn(len(modes))]
		}
		if r.Intn(8) > 0 {
			out["conditions"] = randomJSON(r, depth-1)
		}
		return out
	}
}

func TestFromBytesNeverPanics(t *testing.T) {
	r := rand.New(rand.NewSource(20260917))
	for i := 0; i < 20000; i++ {
		value := randomJSON(r, 6)
		source, err := json.Marshal(value)
		require.Nil(t, err)

		e, err := FromBytes(source)
		if err != nil {
			assert.Nil(t, e, "a failed parse must not return an expression: %s", source)
			continue
		}
		require.NotNil(t, e, "%s", source)
		checkShape(t, e)
	}
}

func TestFromBytesNeverPanicsOnArbitraryBytes(t *testing.T) {
	r := rand.New(rand.NewSource(17092026))
	alphabet := []byte(`{}[]",:0123456789.truefalsnl \x-+eE`)
	for i := 0; i < 20000; i++ {
		source := make([]byte, r.Intn(64))
		for j := range source {
			source[j] = alphabet[r.Intn(len(alphabet))]
		}
		e, err := FromBytes(source)
		if err != nil {
			continue
		}
		if e != nil {
			checkShape(t, e)
		}
	}
}

func FuzzFromBytes(f *testing.F) {
	seeds := []string{
		``,
		`null`,
		`{}`,
		`{"mode":"and","conditions":[]}`,
		`{"mode":"and","conditions":5}`,
		`{"mode":"and","conditions":["x"]}`,
		`{"mode":"and","conditions":[["a",5,1]]}`,
		`{"mode":"and","conditions":[["a","eq"]]}`,
		`{"mode":"and","conditions":[[]]}`,
		`{"mode":5,"conditions":[["a","eq",1]]}`,
		`{"mode":"prem","conditions":[["a","eq",1]]}`,
		`{"mode":"or","conditions":[["id","lt",7],{"mode":"and","conditions":[["id","eq",null]]}]}`,
		`{"conditions":[{"conditions":[{"conditions":[["a","eq",1]]}]}]}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(
		func(t *testing.T, source []byte) {
			e, err := FromBytes(source)
			if err != nil {
				if e != nil {
					t.Fatalf("FromBytes(%q) returned both an expression and an error %v", source, err)
				}
				return
			}
			if e == nil {
				return
			}
			checkShape(t, e)

			// A parsed tree must survive a round trip through its own encoding.
			source, err = json.Marshal(e)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			again, err := FromBytes(source)
			if err != nil {
				t.Fatalf("re-parsing %q failed: %v", source, err)
			}
			checkShape(t, again)
		},
	)
}
