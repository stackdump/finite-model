package dsl

// This package provides a user-facing api
// for working with the finite MetaModel framework

import (
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stackdump/finite-model/model"
	"github.com/stackdump/gopflow/ptnet"
	"github.com/stackdump/gopflow/statemachine"
)

type Node = model.Node

type StateVector = statemachine.StateVector

type StateMachine = statemachine.StateMachine

type Token = uint64 // base token type

type Model interface {
	Var() Var
	GetVars() []*model.VarMap
	StateMachine() *StateMachine
	PTNet() *ptnet.PTNet
	Marshal() (*any.Any, error)
}

type Vector = statemachine.Delta

// codify node types
type Cell = ptnet.Place

// role def
type Role = statemachine.Role

// transition def
type Defun = statemachine.Transition

//action def
type Action = statemachine.Action

// define a new place
type CellDef func(CellRef, Cell) *model.Node

// define a new transition
type FnDef func(Fn, Defun) *model.Node

// define a new role
type RoleDef func(Actor) Role

// bind outside data to the model with Vars
// Vars are applied when a model is loaded
type Var = model.Var

// model builder function
type ModelDeclaration func(RoleDef, CellDef, FnDef)

// build new model instance and return it's interface
func NewModel(schema string, m ModelDeclaration) Model {
	i := new(instance)
	i.schema = schema
	i.MetaModel = model.New(i.schema)
	m(i.Role, i.Place, i.Transition) // re-index state machine
	return i
}

// Model Instance
type instance struct {
	schema string
	*model.MetaModel
}

func (i *instance) Marshal() (*any.Any, error) {
	if !i.IsFrozen() {
		i.Freeze()
	}
	i.AssertFrozen()
	return i.ToAny()
}

func (i *instance) Var() model.Var {
	return i.NewVar()
}

// runtime error when assembling dsl vars
func assertOK(ok bool) {
	if !ok {
		panic("bad dsl var")
	}
}

// construct state machine and evaluate variable bindings
func (i *instance) StateMachine() *statemachine.StateMachine {
	i.MetaModel.Freeze()
	net := i.PTNet()

	var t statemachine.Transition
	var p ptnet.Place
	var ok bool

	for _, v := range i.GetVars() {
		switch v.Type() {
		case model.CapacityVar:
			p, ok = net.Places[v.Source]
			assertOK(ok)
			p.Capacity = v.GetVal()
			net.Places[v.Source] = p // overwrite
		case model.WeightVar:
			var ok bool

			t, ok = net.Transitions[Action(v.Source)]
			if !ok {
				// place(source)->transition(target)
				t, ok = net.Transitions[Action(v.Target)]
				assertOK(ok)
				p, ok = net.Places[v.Source]
				assertOK(ok)
				t.Delta[p.Offset] = int64(0 - v.GetVal()) // p->t represents removal of a token
				net.Transitions[Action(v.Target)] = t     // overwrite
			} else {
				// transition(source)->place(target)
				p, ok = net.Places[v.Target]
				assertOK(ok)
				t.Delta[p.Offset] = int64(0 + v.GetVal()) // p->t represents addition of a token
				net.Transitions[Action(v.Source)] = t     // overwrite
			}
			//fmt.Printf("Var %v", t)
		case model.InitialVar:
			p, ok = net.Places[v.Target]
			assertOK(ok)
			p.Initial = v.GetVal()
			net.Places[v.Target] = p // overwrite
		default:
			panic("Unknown Type")
		}

	}
	return net.StateMachine()
}

// load json model from {Any.Value: []byte}
func Unmarshal(a *any.Any) (*model.MetaModel, error) {
	return model.FromAny(a)
}

// TX Function pointer
type Fn = model.Fn

// Cell reference
type CellRef = model.Cell

// Actor/User type declaration
type Actor = model.Actor
