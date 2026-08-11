package calc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Eval evaluates a mathematical expression string and returns the result as float64.
func Eval(expr string) (float64, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	if len(tokens) == 0 {
		return 0, errors.New("empty expression")
	}
	p := &parser{tokens: tokens, pos: 0}
	val, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.tokens) {
		return 0, fmt.Errorf("unexpected token: %s", p.tokens[p.pos])
	}
	return val, nil
}

// FormatResult formats a float64 math result cleanly without trailing decimal zeros.
func FormatResult(val float64) string {
	if math.IsNaN(val) {
		return "NaN"
	}
	if math.IsInf(val, 1) {
		return "+Infinity"
	}
	if math.IsInf(val, -1) {
		return "-Infinity"
	}

	// Check if integer
	if val == math.Trunc(val) && math.Abs(val) < 1e15 {
		return fmt.Sprintf("%.0f", val)
	}
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func tokenize(expr string) ([]string, error) {
	var tokens []string
	runes := []rune(expr)
	n := len(runes)
	i := 0

	for i < n {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}

		if r == '+' || r == '-' || r == '*' || r == '/' || r == '%' || r == '^' || r == '(' || r == ')' || r == ',' {
			tokens = append(tokens, string(r))
			i++
			continue
		}

		// Number (including decimals)
		if unicode.IsDigit(r) || r == '.' {
			start := i
			hasDot := (r == '.')
			i++
			for i < n && (unicode.IsDigit(runes[i]) || (!hasDot && runes[i] == '.')) {
				if runes[i] == '.' {
					hasDot = true
				}
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			continue
		}

		// Identifiers (functions or constants: sqrt, sin, cos, pi, e, etc.)
		if unicode.IsLetter(r) {
			start := i
			i++
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) {
				i++
			}
			tokens = append(tokens, strings.ToLower(string(runes[start:i])))
			continue
		}

		return nil, fmt.Errorf("invalid character: '%c'", r)
	}

	return tokens, nil
}

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *parser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) parseExpression() (float64, error) {
	return p.parseAddSub()
}

func (p *parser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}

	for {
		op := p.peek()
		if op == "+" {
			p.next()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left += right
		} else if op == "-" {
			p.next()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left -= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseMulDiv() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}

	for {
		op := p.peek()
		if op == "*" {
			p.next()
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			left *= right
		} else if op == "/" {
			p.next()
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			left /= right
		} else if op == "%" {
			p.next()
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, errors.New("modulo by zero")
			}
			left = math.Mod(left, right)
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parsePower() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}

	if p.peek() == "^" {
		p.next()
		right, err := p.parsePower() // right-associative
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}

	return left, nil
}

func (p *parser) parseUnary() (float64, error) {
	if p.peek() == "+" {
		p.next()
		return p.parseUnary()
	}
	if p.peek() == "-" {
		p.next()
		val, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (float64, error) {
	token := p.next()
	if token == "" {
		return 0, errors.New("unexpected end of expression")
	}

	// Number
	if num, err := strconv.ParseFloat(token, 64); err == nil {
		return num, nil
	}

	// Parentheses
	if token == "(" {
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if p.next() != ")" {
			return 0, errors.New("missing closing parenthesis ')'")
		}
		return val, nil
	}

	// Constants
	switch token {
	case "pi":
		return math.Pi, nil
	case "e":
		return math.E, nil
	}

	// Functions
	if p.peek() == "(" {
		p.next() // consume '('
		arg1, err := p.parseExpression()
		if err != nil {
			return 0, err
		}

		var arg2 float64
		hasArg2 := false
		if p.peek() == "," {
			p.next() // consume ','
			arg2, err = p.parseExpression()
			if err != nil {
				return 0, err
			}
			hasArg2 = true
		}

		if p.next() != ")" {
			return 0, fmt.Errorf("missing ')' for function %s", token)
		}

		switch token {
		case "sqrt":
			if arg1 < 0 {
				return 0, errors.New("sqrt of negative number")
			}
			return math.Sqrt(arg1), nil
		case "abs":
			return math.Abs(arg1), nil
		case "round":
			return math.Round(arg1), nil
		case "floor":
			return math.Floor(arg1), nil
		case "ceil":
			return math.Ceil(arg1), nil
		case "sin":
			return math.Sin(arg1), nil
		case "cos":
			return math.Cos(arg1), nil
		case "tan":
			return math.Tan(arg1), nil
		case "log", "ln":
			if arg1 <= 0 {
				return 0, errors.New("log of non-positive number")
			}
			return math.Log(arg1), nil
		case "log10":
			if arg1 <= 0 {
				return 0, errors.New("log10 of non-positive number")
			}
			return math.Log10(arg1), nil
		case "pow":
			if !hasArg2 {
				return 0, errors.New("pow requires 2 arguments: pow(base, exp)")
			}
			return math.Pow(arg1, arg2), nil
		case "min":
			if !hasArg2 {
				return 0, errors.New("min requires 2 arguments: min(a, b)")
			}
			return math.Min(arg1, arg2), nil
		case "max":
			if !hasArg2 {
				return 0, errors.New("max requires 2 arguments: max(a, b)")
			}
			return math.Max(arg1, arg2), nil
		default:
			return 0, fmt.Errorf("unknown function: '%s'", token)
		}
	}

	return 0, fmt.Errorf("unknown identifier: '%s'", token)
}
