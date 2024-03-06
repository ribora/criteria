package criteria

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpression(t *testing.T) {
	t.Run(
		"Deserialize",
		func(t *testing.T) {
			q := And()
			compare := Or("id", "lt", 7.).Add(And("id", "eq", nil))
			payload := []byte(`{"mode":"or","conditions":[["id","lt",7],{"mode":"and","conditions":[["id","eq",null]]}]}`)
			err := json.Unmarshal(payload, &q)
			assert.Nil(t, err)
			assert.Equal(t, compare, q)
		},
	)

	t.Run(
		"DeserializeUnknownOperation",
		func(t *testing.T) {
			q := expression{}
			payload := []byte(`{"mode":"prem","conditions":[["id","lt",7]]}`)
			err := json.Unmarshal(payload, &q)
			assert.Nil(t, err)
			strategy, err := q.Strategy()
			assert.EqualError(t, err, "invalid expression strategy: 'prem'")
			assert.Equal(t, Strategy(""), strategy)
		},
	)

	t.Run(
		"Success",
		func(t *testing.T) {
			q := And("field", "eq", 1).
				Add(Or("field2", "eq", "str").Add("field3", "ne", nil)).
				Add("field4", "gt", 3.14).
				Add("field5", "json_eq", []int{7, 5, 4}).
				Add("field6", "eq", []string{"4", "fr"}).
				Add("f", "ne", []int{1})
			qJson, err := json.Marshal(q)
			assert.Nil(t, err)
			assert.Equal(
				t,
				[]byte(`{"conditions":[["field","eq",1],{"conditions":[["field2","eq","str"],["field3","ne",null]],"mode":"or"},["field4","gt",3.14],["field5","json_eq",[7,5,4]],["field6","eq",["4","fr"]],["f","ne",[1]]],"mode":"and"}`),
				qJson,
			)
			strategy, err := q.(Builder).Strategy()
			assert.Nil(t, err)
			parts, err := q.(Builder).Params()
			assert.Nil(t, err)
			assert.Equal(t, StrategyAnd, strategy)
			assert.Len(t, parts, 6)
		},
	)

	t.Run(
		"MixedOperations",
		func(t *testing.T) {
			e := And("field", "eq", 17, And("field", "ne", 6))
			_, err := e.(Builder).Params()
			assert.ErrorContains(t, err, "cannot mix Expressions and operations")
		},
	)

	t.Run(
		"WrongOperationsArgsCount",
		func(t *testing.T) {
			e := And("field", "eq", 17, "fire", "ne", nil)
			_, err := e.(Builder).Params()
			assert.ErrorContains(t, err, "operations support only 3 argument")
		},
	)
}
