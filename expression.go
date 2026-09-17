package criteria

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	addNotInitialized = iota
	addExpressionMode
	addOperationMode
)

type Strategy string

func (s Strategy) IsValid() bool {
	return slices.Contains([]Strategy{StrategyAnd, StrategyOr, ""}, s)
}

const (
	StrategyAnd Strategy = "and"
	StrategyOr  Strategy = "or"
)

type Expression interface {
	Add(...any) Expression
}

type Builder interface {
	Strategy() (Strategy, error)
	Params() ([]any, error)
}

func And(expressions ...any) Expression {
	return expr(StrategyAnd).Add(expressions...)
}

func Or(expressions ...any) Expression {
	return expr(StrategyOr).Add(expressions...)
}

func FromBytes(source []byte) (Expression, error) {
	if len(source) == 0 {
		return nil, nil
	}
	e := expr(StrategyAnd)
	err := json.Unmarshal(source, e)
	if err != nil {
		return nil, err
	}
	if _, err = e.Params(); err != nil {
		return nil, err
	}
	return e, nil
}

func expr(strategy Strategy) *expression {
	return &expression{strategy: strategy}
}

type expression struct {
	strategy Strategy
	parts    []any
	failed   error
}

func (e *expression) Strategy() (Strategy, error) {
	if e.failed != nil {
		return "", e.failed
	}
	strategy := e.strategy
	if strategy == "" {
		strategy = StrategyAnd
	}
	if !strategy.IsValid() {
		return "", fmt.Errorf("invalid expression strategy: '%v'", e.strategy)
	}
	return strategy, nil
}

func (e *expression) Params() ([]any, error) {
	if e.failed != nil {
		return nil, e.failed
	}
	return e.parts, nil
}

func (e *expression) UnmarshalJSON(bytes []byte) error {
	data := map[string]any{}
	err := json.Unmarshal(bytes, &data)
	if err != nil {
		return fmt.Errorf("criteria unmarshal: %v", err)
	}
	return e.parse(data)
}

func (e *expression) parse(part map[string]any) error {
	// Nil parts would marshal back to "conditions":null, which parse rejects.
	e.parts = []any{}
	if mode, ok := part["mode"]; ok && mode != nil {
		strategy, ok := mode.(string)
		if !ok {
			return fmt.Errorf("mode must be a string, got %T", mode)
		}
		e.strategy = Strategy(strategy)
	}
	if part["conditions"] == nil {
		return fmt.Errorf("condition is required but missing")
	}
	conditions, ok := part["conditions"].([]any)
	if !ok {
		return fmt.Errorf("conditions must be an array, got %T", part["conditions"])
	}
	for i, condition := range conditions {
		switch c := condition.(type) {
		case map[string]any:
			nested := &expression{}
			err := nested.parse(c)
			if err != nil {
				return fmt.Errorf("condition %d: %w", i, err)
			}
			e.Add(nested)
		case []any:
			if len(c) != 3 {
				return fmt.Errorf("condition %d: operations support only 3 argument, got %d", i, len(c))
			}
			// sql/where.go asserts the operation to string without checking it.
			if _, ok := c[1].(string); !ok {
				return fmt.Errorf("condition %d: operation must be a string, got %T", i, c[1])
			}
			e.Add(c...)
		default:
			return fmt.Errorf("condition %d: must be an object or an array, got %T", i, condition)
		}
	}
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
	mode := addNotInitialized
	for _, p := range expressions {
		if _, ok := p.(Expression); ok {
			if mode == addOperationMode {
				e.failed = fmt.Errorf("cannot mix Expressions and operations")
			}
			mode = addExpressionMode
		} else {
			mode = addOperationMode
		}
	}
	if mode == addOperationMode {
		if len(expressions) != 3 {
			e.failed = fmt.Errorf("operations support only 3 argument")
		}
		expressions = []any{expressions}
	}

	e.parts = append(e.parts, expressions...)
	return e
}
