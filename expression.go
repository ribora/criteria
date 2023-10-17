package expr

import (
	"encoding/json"
	"fmt"

	"gitlab.com/gorib/ptr"
	"gitlab.com/gorib/try"
)

const (
	AddNotInitialized = iota
	AddExpressionMode
	AddOperationMode
)

const (
	StrategyAnd = "and"
	StrategyOr  = "or"
)

type Expression interface {
	Add(...any) Expression
}

func And(expressions ...any) Expression {
	return ptr.Pointer(expression{StrategyAnd, []any{}}).Add(expressions...)
}

func Or(expressions ...any) Expression {
	return ptr.Pointer(expression{StrategyOr, []any{}}).Add(expressions...)
}

func NewBuilder(e Expression) (string, []any) {
	if ex, ok := e.(*expression); ok {
		return ex.strategy, ex.parts
	}
	panic("unsupported object")
}

type expression struct {
	strategy string
	parts    []any
}

func (e *expression) UnmarshalJSON(bytes []byte) error {
	e.parts = []any{}
	var parser func(*expression, any) error
	parser = func(e *expression, data any) error {
		if part, ok := data.(map[string]any); ok {
			if mode, ok := part["mode"].(string); ok {
				e.strategy = mode
			}
			if part["conditions"] == nil {
				return fmt.Errorf("condition is required but missing")
			}
			for _, condition := range part["conditions"].([]any) {
				if _, ok := condition.(map[string]any); ok {
					nested := &expression{}
					err := parser(nested, condition)
					if err != nil {
						return err
					}
					e.Add(nested)
				} else {
					e.Add(condition.([]any)...)
				}
			}
		}
		return nil
	}
	data := map[string]any{}
	try.ThrowError(json.Unmarshal(bytes, &data))
	try.ThrowError(parser(e, data))
	return nil
}

func (e *expression) MarshalJSON() ([]byte, error) {
	data := map[string]any{
		"mode":       e.strategy,
		"conditions": e.parts,
	}
	return json.Marshal(data)
}

func (e *expression) Add(expressions ...any) Expression {
	// 0 — not initialized; 1 — Expression; 2 — operation
	mode := AddNotInitialized
	for _, p := range expressions {
		if _, ok := p.(Expression); ok {
			if mode == AddOperationMode {
				panic(fmt.Errorf("cannot mix Expressions and operations"))
			}
			mode = AddExpressionMode
		} else {
			mode = AddOperationMode
		}
	}
	if mode == AddOperationMode {
		if len(expressions) != 3 {
			panic(fmt.Errorf("operations support only 3 argument"))
		}
		expressions = []any{expressions}
	}

	e.parts = append(e.parts, expressions...)
	return e
}
