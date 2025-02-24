package criteria

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	AddNotInitialized = iota
	AddExpressionMode
	AddOperationMode
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
	e.parts = []any{}
	var parser func(*expression, any) error
	parser = func(e *expression, data any) error {
		if part, ok := data.(map[string]any); ok {
			if mode, ok := part["mode"].(string); ok {
				e.strategy = Strategy(mode)
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
	err := json.Unmarshal(bytes, &data)
	if err != nil {
		return fmt.Errorf("criteria unmarshal: %v", err)
	}
	return parser(e, data)
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
				e.failed = fmt.Errorf("cannot mix Expressions and operations")
			}
			mode = AddExpressionMode
		} else {
			mode = AddOperationMode
		}
	}
	if mode == AddOperationMode {
		if len(expressions) != 3 {
			e.failed = fmt.Errorf("operations support only 3 argument")
		}
		expressions = []any{expressions}
	}

	e.parts = append(e.parts, expressions...)
	return e
}
