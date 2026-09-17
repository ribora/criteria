package criteria

import (
	"encoding/json"
	"fmt"
)

type Strategy string

func (s Strategy) IsValid() bool {
	switch s {
	case StrategyAnd, StrategyOr, "":
		return true
	}
	return false
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

type Condition struct {
	Field     string
	Operation string
	Value     any
}

type Node struct {
	Condition Condition
	Nested    Expression
}

func Cond(field, operation string, value any) Condition {
	return Condition{Field: field, Operation: operation, Value: value}
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

func Unpack(e Expression) (Strategy, []Node, error) {
	if e == nil {
		return StrategyAnd, nil, nil
	}
	builder, ok := e.(Builder)
	if !ok {
		return "", nil, fmt.Errorf("expression %T does not implement Builder", e)
	}
	strategy, err := builder.Strategy()
	if err != nil {
		return "", nil, err
	}
	parts, err := builder.Params()
	if err != nil {
		return "", nil, err
	}
	nodes := make([]Node, 0, len(parts))
	for i, part := range parts {
		if nested, ok := part.(Expression); ok {
			nodes = append(nodes, Node{Nested: nested})
			continue
		}
		condition, ok := part.([]any)
		if !ok {
			return "", nil, fmt.Errorf("condition %d: must be an expression or an operation, got %T", i, part)
		}
		if len(condition) != 3 {
			return "", nil, fmt.Errorf("condition %d: operations support only 3 argument, got %d", i, len(condition))
		}
		field, ok := condition[0].(string)
		if !ok {
			return "", nil, fmt.Errorf("condition %d: field must be a string, got %T", i, condition[0])
		}
		operation, ok := condition[1].(string)
		if !ok {
			return "", nil, fmt.Errorf("condition %d: operation must be a string, got %T", i, condition[1])
		}
		nodes = append(nodes, Node{Condition: Condition{Field: field, Operation: operation, Value: condition[2]}})
	}
	return strategy, nodes, nil
}

func Err(e Expression) error {
	builder, ok := e.(Builder)
	if !ok {
		return nil
	}
	_, err := builder.Params()
	return err
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
	nodes := 0
	for _, p := range expressions {
		switch p.(type) {
		case Expression, Condition:
			nodes++
		}
	}
	switch {
	case nodes == len(expressions):
		e.appendNodes(expressions)
	case nodes > 0:
		e.failed = fmt.Errorf("cannot mix Expressions and operations")
		e.appendNodes(expressions)
	default:
		if len(expressions) != 3 {
			e.failed = fmt.Errorf("operations support only 3 argument")
		}
		e.parts = append(e.parts, expressions)
	}
	return e
}

func (e *expression) appendNodes(parts []any) {
	for _, p := range parts {
		if c, ok := p.(Condition); ok {
			p = []any{c.Field, c.Operation, c.Value}
		}
		e.parts = append(e.parts, p)
	}
}
