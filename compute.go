package main

import (
	"strings"
	"fmt"
		
	"github.com/skelterjohn/gopp"
)

const computegopp = `
Base => {type=ComputeBase} {field=Expr} <<Expr>>
# An Expr is either the sum of two terms,
Expr => {type=ComputeSum} {field=First} <<Term>> {field=Op} <sum> {field=Second} <<Expr>>
# or just another term.
Expr => <Term>
# A Term is either the product of two factors,
Term => {type=ComputeProduct} {field=First} <<Factor>> {field=Op} <product> {field=Second} <<Term>>
# or just another factor.
Term => <Factor>
# A factor is either a parenthesized expression,
Factor => {type=ComputeExprFactor} '(' {field=Expr} <<Expr>> ')'
# or a variable,
Factor => {type=ComputeVariableFactor} {field=Monitor} <identifier> '.' {field=Variable} <identifier>
# or just a number.
Factor => {type=ComputeNumberFactor} {field=Number} <number>
# A number is a string of consecutive digits.
number = /([-\d.]+)/
# An identifier is a string of letters
identifier = /([-_a-zA-Z]+)/
product = /([\/*])/
sum = /([-+])/
`

type Compute interface {
	Run(values map[string]float64) (float64, error)
	GetVars() []*ComputeVariableFactor
}

type ComputeBase struct {
	Expr Compute
}

func (b ComputeBase) String() string {
	return fmt.Sprintf("%d", b.Expr)
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

type ComputeSum struct {
	First, Second Compute
	Op string
}

func (s ComputeSum) String() string {
	return fmt.Sprintf("%d%s%d", s.First, s.Op, s.Second)
}

func (s ComputeSum) Run(values map[string]float64) (float64, error) {
	a, err := s.First.Run(values)
	if err != nil {
		return 0, err
	}
	b, err := s.Second.Run(values)
	if err != nil {
		return 0, err
	}
	if s.Op == "+" {
		return a + b, nil
	}
	return a - b, nil
}

func (s ComputeSum) GetVars() []*ComputeVariableFactor {
	return append(s.First.GetVars(), s.Second.GetVars()...)
}

type ComputeProduct struct {
	First, Second Compute
	Op string
}

func (p ComputeProduct) String() string {
	return fmt.Sprintf("%d%s%d", p.First, p.Op, p.Second)
}

func (p ComputeProduct) Run(values map[string]float64) (float64, error) {
	a, err := p.First.Run(values)
	if err != nil {
		return 0, err
	}
	b, err := p.Second.Run(values)
	if err != nil {
		return 0, err
	}
	if p.Op == "*" {
		return a * b, nil
	}
	return a / b, nil
}

func (p ComputeProduct) GetVars() []*ComputeVariableFactor {
	return append(p.First.GetVars(), p.Second.GetVars()...)
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
	Monitor string
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
	computeDecoder *gopp.DecoderFactory
)

func init() {
	computeDecoder, _ = gopp.NewDecoderFactory(computegopp, "Base")

	computeDecoder.RegisterType(ComputeBase{})
	computeDecoder.RegisterType(ComputeExprFactor{})
	computeDecoder.RegisterType(ComputeNumberFactor{})
	computeDecoder.RegisterType(ComputeSum{})
	computeDecoder.RegisterType(ComputeProduct{})
	computeDecoder.RegisterType(ComputeVariableFactor{})
}

func Decode(expr string) (Compute, error) {
	dec := computeDecoder.NewDecoder(strings.NewReader(expr))
	var compute ComputeBase
	err := dec.Decode(&compute)
	if err != nil {
		return nil, err
	}
	return compute, nil
}

type ComputedVariable struct {
	compute Compute
	Name string
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
			switch tt := val.(type){
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
			switch tt := val.(type){
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
