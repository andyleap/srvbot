package main

import (
	"fmt"
	"strconv"

	. "github.com/andyleap/parser"
)

type Compute interface {
	Run(values map[string]float64) (float64, error)
	GetVars() []*ComputeVariableFactor
}

type ComputeBase struct {
	Expr Compute
}

func (b ComputeBase) Run(values map[string]float64) (float64, error) {
	a, err := b.Expr.Run(values)
	if err != nil {
		return 0, err
	}
	return a, nil
}

func (b ComputeBase) GetVars() []*ComputeVariableFactor {
	return b.Expr.GetVars()
}

type ComputeOperand struct {
	Op      string
	Operand Compute
}

type ComputeTerm struct {
	Start Compute
	Ops   []ComputeOperand
}

func (t ComputeTerm) Run(values map[string]float64) (float64, error) {
	v, err := t.Start.Run(values)
	if err != nil {
		return 0, err
	}
	for _, op := range t.Ops {
		opv, err := op.Operand.Run(values)
		if err != nil {
			return 0, err
		}
		switch op.Op {
		case "+":
			v += opv
		case "-":
			v -= opv
		case "*":
			v *= opv
		case "/":
			v /= opv
		}
	}
	return v, nil
}

func (t ComputeTerm) GetVars() []*ComputeVariableFactor {
	vars := t.Start.GetVars()
	for _, op := range t.Ops {
		vars = append(vars, op.Operand.GetVars()...)
	}
	return vars
}

type ComputeExprFactor struct {
	Expr Compute
}

func (ef ComputeExprFactor) String() string {
	return fmt.Sprintf("(%d)", ef.Expr)
}

func (ef ComputeExprFactor) Run(values map[string]float64) (float64, error) {
	a, err := ef.Expr.Run(values)
	if err != nil {
		return 0, err
	}
	return a, nil
}

func (ef ComputeExprFactor) GetVars() []*ComputeVariableFactor {
	return ef.Expr.GetVars()
}

type ComputeNumberFactor struct {
	Number float64
}

func (nf ComputeNumberFactor) String() string {
	return fmt.Sprint(nf.Number)
}

func (nf ComputeNumberFactor) Run(values map[string]float64) (float64, error) {
	return nf.Number, nil
}

func (nf ComputeNumberFactor) GetVars() []*ComputeVariableFactor {
	return []*ComputeVariableFactor{}
}

type ComputeVariableFactor struct {
	Monitor  string
	Variable string
}

func (vf ComputeVariableFactor) String() string {
	return fmt.Sprintf("%s.%s", vf.Monitor, vf.Variable)
}

func (vf ComputeVariableFactor) Run(values map[string]float64) (float64, error) {
	m, ok := values[fmt.Sprintf("%s.%s", vf.Monitor, vf.Variable)]
	if !ok {
		return 0, fmt.Errorf("Can't find variable %s.%s", vf.Monitor, vf.Variable)
	}
	return m, nil
}

func (vf ComputeVariableFactor) GetVars() []*ComputeVariableFactor {
	return []*ComputeVariableFactor{&vf}
}

var (
	computeGrammar *Grammar
)

func init() {
	number := And(Mult(0, 1, Lit("-")), Mult(1, 0, Set("0-9")), Mult(0, 1, And(Lit("."), Mult(0, 0, Set("0-9")))))
	number.Node(func(m Match) (Match, error) {
		v, err := strconv.ParseFloat(String(m), 64)
		if err != nil {
			return nil, err
		}
		return ComputeNumberFactor{Number: v}, nil
	})

	variable := And(
		Tag("Monitor", Mult(1, 0, Set("a-zA-Z"))),
		Lit("."),
		Tag("Variable", Mult(1, 0, Set("a-zA-Z"))))

	variable.Node(func(m Match) (Match, error) {
		mtag := GetTag(m, "Monitor")
		vtag := GetTag(m, "Variable")
		return ComputeVariableFactor{
			Monitor:  String(mtag.Match),
			Variable: String(vtag.Match),
		}, nil
	})

	expr := &Grammar{}

	parenexpr := And(Lit("("), Tag("Expr", expr), Lit(")"))
	parenexpr.Node(func(m Match) (Match, error) {
		etag := GetTag(m, "Expr")
		return etag.Match, nil
	})

	factor := Or(parenexpr, variable, number)

	term := And(Tag("Start", factor), Tag("Ops", Mult(0, 0, And(Tag("Op", Set("*/")), Tag("Operand", factor)))))
	term.Node(func(m Match) (Match, error) {
		fmt.Println(m)
		start := GetTag(m, "Start").Match.(Compute)
		ops := []ComputeOperand{}
		for _, op := range GetTag(m, "Ops").Match.(MatchTree) {
			ops = append(ops, ComputeOperand{
				Op:      string(GetTag(op, "Op").Match.(MatchString)),
				Operand: GetTag(op, "Operand").Match.(Compute),
			})
		}
		if len(ops) == 0 {
			return start, nil
		}
		return ComputeTerm{
			Start: start,
			Ops:   ops,
		}, nil
	})

	expr.Set(And(Tag("Start", term), Tag("Ops", Mult(0, 0, And(Tag("Op", Set("-+")), Tag("Operand", term))))))
	expr.Node(func(m Match) (Match, error) {
		start := GetTag(m, "Start").Match.(Compute)
		ops := []ComputeOperand{}
		for _, op := range GetTag(m, "Ops").Match.(MatchTree) {
			ops = append(ops, ComputeOperand{
				Op:      string(GetTag(op, "Op").Match.(MatchString)),
				Operand: GetTag(op, "Operand").Match.(Compute),
			})
		}
		if len(ops) == 0 {
			return start, nil
		}
		return ComputeTerm{
			Start: start,
			Ops:   ops,
		}, nil
	})

	computeGrammar = expr
}

func Decode(expr string) (Compute, error) {
	comp, err := computeGrammar.ParseString(expr)
	if err != nil {
		return nil, err
	}
	return comp.(Compute), nil
}

type ComputedVariable struct {
	compute Compute
	Name    string
}

func RunComputeds(cvs []ComputedVariable) map[string]float64 {
	vars := make(map[string]map[string]bool)
	for _, cv := range cvs {
		for _, v := range cv.compute.GetVars() {
			mvars, ok := vars[v.Monitor]
			if !ok {
				mvars = make(map[string]bool)
				vars[v.Monitor] = mvars
			}
			mvars[v.Variable] = true
		}
	}

	values := make(map[string]float64)

	for m, v := range vars {
		vs := make([]string, 0)
		for vn := range v {
			vs = append(vs, vn)
		}
		vals := Config.Monitors[m].monitor.GetValues(vs)
		for k, val := range vals {
			var fval float64
			switch tt := val.(type) {
			case float64:
				fval = tt
			case float32:
				fval = float64(tt)
			case uint32:
				fval = float64(tt)
			case uint64:
				fval = float64(tt)
			case int32:
				fval = float64(tt)
			case int64:
				fval = float64(tt)
			default:
				continue
			}
			values[fmt.Sprintf("%s.%s", m, k)] = fval
		}
	}

	vals := make(map[string]float64)

	for _, cv := range cvs {
		val, err := cv.compute.Run(values)
		if err != nil {
			vals[cv.Name] = val
		}
	}

	return vals
}

func RunCompute(c Compute) (float64, error) {
	vars := make(map[string]map[string]bool)

	for _, v := range c.GetVars() {
		mvars, ok := vars[v.Monitor]
		if !ok {
			mvars = make(map[string]bool)
			vars[v.Monitor] = mvars
		}
		mvars[v.Variable] = true
	}

	values := make(map[string]float64)

	for m, v := range vars {
		vs := make([]string, 0)
		for vn := range v {
			vs = append(vs, vn)
		}
		vals := Config.Monitors[m].monitor.GetValues(vs)
		for k, val := range vals {
			var fval float64
			switch tt := val.(type) {
			case float64:
				fval = tt
			case float32:
				fval = float64(tt)
			case uint32:
				fval = float64(tt)
			case uint64:
				fval = float64(tt)
			case int32:
				fval = float64(tt)
			case int64:
				fval = float64(tt)
			default:
				continue
			}
			values[fmt.Sprintf("%s.%s", m, k)] = fval
		}
	}

	val, err := c.Run(values)
	if err != nil {
		return 0, err
	}
	return val, nil
}
