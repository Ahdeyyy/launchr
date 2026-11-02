package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// A small arithmetic parser, with no script engine or code execution.
type calcParser struct {
	s          string
	pos, depth int
}

func calculate(s string) (string, error) {
	s = strings.NewReplacer("×", "*", "÷", "/", "−", "-").Replace(strings.TrimSpace(s))
	if len(s) > 2048 {
		return "", fmt.Errorf("expression is too long")
	}
	p := calcParser{s: s}
	v, err := p.expr(0)
	p.space()
	if err != nil {
		return "", err
	}
	if p.pos != len(s) {
		return "", fmt.Errorf("unexpected input at position %d", p.pos+1)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", fmt.Errorf("result is undefined")
	}
	return strconv.FormatFloat(v, 'g', 12, 64), nil
}
func (p *calcParser) space() {
	for p.pos < len(p.s) && unicode.IsSpace(rune(p.s[p.pos])) {
		p.pos++
	}
}
func (p *calcParser) expr(min int) (float64, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 128 {
		return 0, fmt.Errorf("expression is too deeply nested")
	}
	p.space()
	var left float64
	var err error
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("enter an expression")
	}
	ch := p.s[p.pos]
	switch {
	case ch == '+' || ch == '-':
		p.pos++
		left, err = p.expr(3)
		if ch == '-' {
			left = -left
		}
	case ch == '(':
		p.pos++
		left, err = p.expr(0)
		p.space()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
	case ch >= '0' && ch <= '9' || ch == '.':
		start := p.pos
		for p.pos < len(p.s) && ((p.s[p.pos] >= '0' && p.s[p.pos] <= '9') || p.s[p.pos] == '.') {
			p.pos++
		}
		if p.pos < len(p.s) && (p.s[p.pos] == 'e' || p.s[p.pos] == 'E') {
			p.pos++
			if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
				p.pos++
			}
			for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
				p.pos++
			}
		}
		left, err = strconv.ParseFloat(p.s[start:p.pos], 64)
	default:
		start := p.pos
		for p.pos < len(p.s) && p.s[p.pos] >= 'a' && p.s[p.pos] <= 'z' {
			p.pos++
		}
		name := p.s[start:p.pos]
		switch name {
		case "pi":
			left = math.Pi
		case "e":
			left = math.E
		case "sqrt", "abs", "sin", "cos", "tan", "log", "ln":
			p.space()
			if p.pos >= len(p.s) || p.s[p.pos] != '(' {
				return 0, fmt.Errorf("use %s(value)", name)
			}
			p.pos++
			v, e := p.expr(0)
			if e != nil {
				return 0, e
			}
			p.space()
			if p.pos >= len(p.s) || p.s[p.pos] != ')' {
				return 0, fmt.Errorf("missing closing parenthesis")
			}
			p.pos++
			switch name {
			case "sqrt":
				left = math.Sqrt(v)
			case "abs":
				left = math.Abs(v)
			case "sin":
				left = math.Sin(v)
			case "cos":
				left = math.Cos(v)
			case "tan":
				left = math.Tan(v)
			case "log":
				left = math.Log10(v)
			case "ln":
				left = math.Log(v)
			}
		default:
			return 0, fmt.Errorf("use numbers, pi, or a supported function")
		}
	}
	if err != nil {
		return 0, err
	}
	for {
		p.space()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op == '%' {
			left /= 100
			p.pos++
			continue
		}
		prec := 0
		switch op {
		case '+', '-':
			prec = 1
		case '*', '/':
			prec = 2
		case '^':
			prec = 4
		}
		if prec == 0 || prec < min {
			break
		}
		p.pos++
		next := prec + 1
		if op == '^' {
			next = prec
		}
		right, e := p.expr(next)
		if e != nil {
			return 0, e
		}
		switch op {
		case '+':
			left += right
		case '-':
			left -= right
		case '*':
			left *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("cannot divide by zero")
			}
			left /= right
		case '^':
			left = math.Pow(left, right)
		}
	}
	return left, nil
}

type conversion struct {
	Amount   float64
	From, To string
}

func parseConversion(q string) (conversion, bool) {
	f := strings.Fields(strings.ToUpper(q))
	if len(f) != 4 || (f[2] != "TO" && f[2] != "IN") {
		return conversion{}, false
	}
	valid := func(s string) bool {
		if len(s) != 3 {
			return false
		}
		for _, c := range s {
			if c < 'A' || c > 'Z' {
				return false
			}
		}
		return true
	}
	if !valid(f[1]) || !valid(f[3]) {
		return conversion{}, false
	}
	n, err := strconv.ParseFloat(f[0], 64)
	return conversion{n, f[1], f[3]}, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}
