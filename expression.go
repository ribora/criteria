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
	e := And()
	err := json.Unmarshal(source, &e)
	if err != nil {
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
