package model

import (
	"encoding/json"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stackdump/gopflow/ptnet"
	"github.com/stackdump/gopflow/statemachine"
)

// a label is the base type
type label = string

// Fn is a state-transition between cells
type Fn = label

// Actors are granted roles in the system
type Actor = label

// Cells designate places in the petri-net model
type Cell = label

// hook to allow for DSL to have syntactic sugar
// where nodes can be chained with arc declarations
type arcFactory func(Arc)

// codify node types
// can be either a Place or a Transition
type nodeType int

// codify arc types
type arcType int

const (
	PLACE      nodeType = 0
	TRANSITION nodeType = 1

	ARC       arcType = 0
	INHIBITOR arcType = 1
)

// nodes are used to generate state machine model
type Node struct {
	Label string
	arcFactory
	nodeType
	*ptnet.Place
	*statemachine.Transition
}

// test if node is a transition
func (n *Node) IsTransition() bool {
	return n.nodeType == TRANSITION
}

// test if node is a place
func (n *Node) IsPlace() bool {
	return n.nodeType == PLACE
}

// declare part of a transaction
func (n *Node) TX(val uint64, cell *Node) *Node {
	n.arcFactory(Arc{
		Source: n,
		Target: cell,
		Weight: val,
		Type:   ARC,
	})
	return n
}

// create inhibitor function
func (n *Node) Inhibitor(weight uint64, target *Node) *Node {
	if n.IsTransition() || target.IsPlace() {
		panic("Inhibitor Arcs must Target Transitions")
	}
	n.arcFactory(Arc{
		Source: n,
		Target: target,
		Weight: weight,
		Type:   INHIBITOR,
	})
	return n
}

// arcs connect nodes
type Arc struct {
	Source *Node
	Target *Node
	Weight uint64
	Type   arcType
}

type ProtoModel interface {
	Freeze() *MetaModel
	Place(ptnet.Place) *Node
	Transition(ptnet.Place) *Node
	Role(string) statemachine.Role
	ToAny() (*any.Any, error)
}

// an MetaModel is used as scaffolding for constructing petri-nets
type MetaModel struct {
	ProtoModel  `json:"-"`
	Schema      string                                           `json:"schema"`
	Places      map[string]*ptnet.Place                          `json:"places"`
	Transitions map[statemachine.Action]*statemachine.Transition `json:"transitions"`
	VectorSize  int                                              `json:"-"`
	vars        []*VarMap
	arcs        []Arc
	frozen      bool
}

func (m *MetaModel) GetVars() []*VarMap {
	return m.vars
}

func (m *MetaModel) IsFrozen() bool {
	return m.frozen == true
}

func (m *MetaModel) assertNotFrozen() {
	if m.frozen {
		panic("frozen model cannot be altered")
	}
}

func (m *MetaModel) AssertFrozen() {
	if !m.frozen {
		panic("expected frozen model")
	}
}

// store are declaration in model
// arcs are indexed only after model is frozen
func (m *MetaModel) AppendArc(a Arc) {
	m.arcs = append(m.arcs, a)
}

func New(Schema string) *MetaModel {
	m := new(MetaModel)
	m.Schema = Schema
	m.Places = make(map[string]*ptnet.Place)
	m.Transitions = make(map[statemachine.Action]*statemachine.Transition)
	m.vars = make([]*VarMap, 0)
	return m
}

func (m *MetaModel) NewVar() Var {
	v := NewVar()
	m.vars = append(m.vars, v.unpack())
	return v
}

func (m *MetaModel) Place(label string, place ptnet.Place) *Node {
	m.assertNotFrozen()
	place.Offset = m.VectorSize
	m.VectorSize++
	m.Places[label] = &place

	return &Node{
		Label:      label,
		nodeType:   PLACE,
		Place:      &place,
		arcFactory: m.AppendArc,
	}
}

func (m *MetaModel) Transition(label string, transition statemachine.Transition) *Node {
	m.assertNotFrozen()
	m.Transitions[statemachine.Action(label)] = &transition

	return &Node{
		Label:      label,
		nodeType:   TRANSITION,
		Transition: &transition,
		arcFactory: m.AppendArc,
	}
}
func (m *MetaModel) Role(label string) statemachine.Role {
	m.assertNotFrozen()
	return statemachine.Role(label)
}

// re-indexes model and marks as frozen
func (m *MetaModel) Freeze() *MetaModel {
	for k, t := range m.Transitions {
		t.Delta = make([]int64, m.VectorSize)
		m.Transitions[k] = t // overwrite
	}

	for _, a := range m.arcs {
		// FIXME deal w/ inhibitors by converting to guards
		if a.Source.IsPlace() && a.Target.IsTransition() {
			p := a.Source
			t := a.Target
			t.Transition.Delta[p.Offset] = 0 - int64(a.Weight)
		} else {
			if a.Target.IsPlace() && a.Source.IsTransition() {
				p := a.Target
				t := a.Source
				t.Transition.Delta[p.Offset] = int64(a.Weight)
			} else {
				panic("bad arc declaration")
			}
		}
	}
	m.frozen = true
	return m
}

// export model as Petri-Net
func (m *MetaModel) PTNet() *ptnet.PTNet {
	if m.frozen != true {
		m.Freeze()
	}
	n := new(ptnet.PTNet)
	n.Places = make(map[string]ptnet.Place)
	n.Transitions = make(map[statemachine.Action]statemachine.Transition)

	for k, v := range m.Places {
		n.Places[k] = *v
	}
	for k, v := range m.Transitions {
		n.Transitions[k] = *v
	}

	return n
}

func (m *MetaModel) ToAny() (n *any.Any, err error) {
	n = new(any.Any)
	n.Value, err = json.Marshal(m)
	return n, err
}

func FromAny(n *any.Any) (m *MetaModel, err error) {
	m = new(MetaModel)
	m.frozen = true
	err = json.Unmarshal(n.GetValue(), m)
	n = new(any.Any)
	return m, err
}

// position on x/y grid for visualization
type Coords struct {
	X int
	Y int
}

// allow relative addressing for variable binding
// Ref{Target, source} points to an arc place -> tx or tx -> place
// Ref{source}  points to a place
type Ref struct {
	Target string
	Source string
}

type binding func() uint64

// begin a new var definition
func NewVar() Var {
	return new(VarMap)
}

// syntactic sugar for variable declaration
type Var interface {
	// set max capacity
	Capacity(t string) Var

	// set input values
	Initial(t string) Var

	// adjust multiple on arc
	Weight(n ...string) Var

	Bind(bindFunc binding)

	// get underlying obj
	unpack() *VarMap
}

type varType int

const InitialVar varType = 0
const WeightVar varType = 1
const CapacityVar varType = 2

// map input vars to MetaModel
type VarMap struct {
	Var
	Ref
	Coords
	Label       string
	Offset      int
	Description string
	binding
	varType
}

func (v *VarMap) Type() varType {
	return v.varType
}

// set max capacity
func (v *VarMap) Capacity(t string) Var {
	v.varType = CapacityVar
	v.Ref.Source = t
	return v
}

// set initial input
func (v *VarMap) Initial(t string) Var {
	v.varType = InitialVar
	v.Ref.Target = t
	return v
}

// set a transacted value Cell -> Fn or Fn -> Cell
func (v *VarMap) Weight(n ...string) Var {
	v.varType = WeightVar
	v.Ref.Source = n[0]
	v.Ref.Target = n[1]
	return v
}

// bind variable to a value producing function
func (v *VarMap) GetVal() uint64 {
	if v.binding == nil {
		panic("unbound")
	}
	return v.binding()
}

// bind variable to a value producing function
func (v *VarMap) Bind(bindFunc binding) {
	v.binding = bindFunc
}

// unpacks data from behind interface
func (v *VarMap) unpack() *VarMap {
	return v
}
