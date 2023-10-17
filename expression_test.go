package expr

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/gorib/try"
)

func TestExpression(t *testing.T) {
	t.Run(
		"Serialize",
		func(t *testing.T) {
			q := And()
			compare := Or("id", "lt", 7.).Add(And("id", "eq", nil))
			payload := []byte(`{"mode":"or","conditions":[["id","lt",7],{"mode":"and","conditions":[["id","eq",null]]}]}`)
			try.ThrowError(json.Unmarshal(payload, &q))
			assert.Equal(t, compare, q)
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
			qJson := try.Throw(json.Marshal(q))
			assert.Equal(
				t,
				[]byte(`{"conditions":[["field","eq",1],{"conditions":[["field2","eq","str"],["field3","ne",null]],"mode":"or"},["field4","gt",3.14],["field5","json_eq",[7,5,4]],["field6","eq",["4","fr"]],["f","ne",[1]]],"mode":"and"}`),
				qJson,
			)
		},
	)

	t.Run(
		"MixedOperations",
		func(t *testing.T) {
			assert.PanicsWithError(t, "cannot mix Expressions and operations", func() { And("field", "eq", 17, And("field", "ne", 6)) })
		},
	)

	t.Run(
		"WrongOperationsArgsCount",
		func(t *testing.T) {
			assert.PanicsWithError(t, "operations support only 3 argument", func() { And("field", "eq", 17, "fire", "ne", nil) })
		},
	)
}
