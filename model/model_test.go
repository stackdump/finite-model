package model_test

import (
	"testing"

	"github.com/stackdump/gopflow/statemachine"
	"github.com/stretchr/testify/assert"

	. "github.com/stackdump/finite-model/model/dsl"
)

// invariants
const (
	INC0 Fn = "INC0"
	DEC0 Fn = "DEC0"
	INC1 Fn = "INC1"
	DEC1 Fn = "DEC1"

	user Actor = "default"

	p0 CellRef = "00"
	p1 CellRef = "01"
)

// counter model declaration
func modelDef(role RoleDef, cell CellDef, fn FnDef) {
	userRole := role(user)

	dec0 := fn(DEC0, Defun{Role: userRole})
	dec1 := fn(DEC1, Defun{Role: userRole})

	p00 := cell(p0, Cell{Initial: 0}).TX(1, dec0)
	p01 := cell(p1, Cell{Initial: 1}).TX(1, dec1)

	fn(INC0, Defun{Role: userRole}).TX(1, p00)
	fn(INC1, Defun{Role: userRole}).TX(1, p01)
}

func TestModelBinding(t *testing.T) {
	m := NewModel("Counter", modelDef)
	v := m.Var // var constructor

	// change max capacity
	v().Capacity(p0).Bind(func() uint64 { return 5 })

	// set initial value
	v().Initial(p0).Bind(func() uint64 { return 1 })

	// update Fn value to incBy 2
	v().Weight(INC0, p0).Bind(func() uint64 { return 2 })

	sm := m.StateMachine()
	assert.Equal(t, sm.Initial, statemachine.StateVector{1, 1})
	//t.Log(sm.Initial)
	out, _, _ := sm.Transform(sm.Initial, INC0, 1)
	_ = out
	//t.Log(out)
	assert.Equal(t, out, []int64{3, 1})

	out, _, _ = sm.Transform(sm.Initial, INC0, 2) // at capacity
	assert.Equal(t, out, []int64{5, 1})

	vout, role, err := sm.Transform(sm.Initial, INC0, 3) // exceeds capacity
	//t.Logf("role: %s error: %s", role, err)
	assert.Equal(t, "overflow", err.Error())
	assert.Equal(t, statemachine.Role("default"), role)
	assert.Equal(t, vout, []int64{7, 1})
}
