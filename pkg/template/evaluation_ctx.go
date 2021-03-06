package template

import (
	"fmt"

	"github.com/get-ytt/ytt/pkg/structmeta"
	"github.com/get-ytt/ytt/pkg/template/core"
	"go.starlark.net/starlark"
)

type EvaluationCtx struct {
	dialect EvaluationCtxDialect

	nodes     *Nodes
	ancestors Ancestors

	pendingAnnotations map[NodeTag]NodeAnnotations

	rootInit       bool
	rootNode       EvaluationNode
	parentNodes    []EvaluationNode
	parentNodeTags []NodeTag
}

type EvaluationNode interface {
	GetValues() []interface{}
	SetValue(interface{}) error
	AddValue(interface{}) error
	ResetValue()
	GetAnnotations() interface{}
	SetAnnotations(interface{})
	DeepCopyAsInterface() interface{} // expects that result implements EvaluationNode
}

type EvaluationCtxDialect interface {
	PrepareNode(parentNode EvaluationNode, val EvaluationNode) error
	Replace(parentNodes []EvaluationNode, val interface{}) error
	ShouldWrapRootValue(val interface{}) bool
	WrapRootValue(val interface{}) interface{}
}

func (e *EvaluationCtx) TplReplace(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() != 1 {
		return starlark.None, fmt.Errorf("expected exactly one argument")
	}

	nodes := append([]EvaluationNode{e.rootNode}, e.parentNodes...)
	val := core.NewStarlarkValue(args.Index(0)).AsInterface()

	err := e.dialect.Replace(nodes, val)
	if err != nil {
		return starlark.None, err
	}

	return &core.StarlarkNoop{}, nil
}

// args(nodeTag, value Value)
func (e *EvaluationCtx) TplSetNode(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() > 1 {
		if _, noop := args.Index(1).(*core.StarlarkNoop); !noop {
			err := e.parentNodes[len(e.parentNodes)-1].SetValue(core.NewStarlarkValue(args.Index(1)).AsInterface())
			if err != nil {
				return starlark.None, err
			}
		}
		return starlark.None, nil
	}

	// use default value from AST since no user provided value was given
	nodeTag, err := NewNodeTagFromStarlarkValue(args.Index(0))
	if err != nil {
		return starlark.None, err
	}

	node, ok := e.nodes.FindNode(nodeTag)
	if !ok {
		return starlark.None, fmt.Errorf("expected to find %s", nodeTag)
	}

	for _, val := range node.GetValues() {
		err := e.parentNodes[len(e.parentNodes)-1].AddValue(val)
		if err != nil {
			return starlark.None, err
		}
	}

	return starlark.None, nil
}

// args(args..., kwargs...)
func (e *EvaluationCtx) TplCollectNodeAnnotation(
	thread *starlark.Thread, f *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	result := starlark.Tuple{args}
	for _, kwarg := range kwargs {
		result = append(result, kwarg)
	}
	return result, nil
}

// args(nodeTag, name, values)
func (e *EvaluationCtx) TplStartNodeAnnotation(
	thread *starlark.Thread, f *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if args.Len() != 3 {
		return starlark.None, fmt.Errorf("expected exactly 3 arguments")
	}

	nodeTag, err := NewNodeTagFromStarlarkValue(args.Index(0))
	if err != nil {
		return starlark.None, err
	}

	annNameStr, err := core.NewStarlarkValue(args.Index(1)).AsString()
	if err != nil {
		return starlark.None, err
	}

	annName := structmeta.AnnotationName(annNameStr)
	annVals := args.Index(2).(starlark.Tuple)

	kwargs = []starlark.Tuple{}
	for _, val := range annVals[1:] {
		kwargs = append(kwargs, val.(starlark.Tuple))
	}

	if _, found := e.pendingAnnotations[nodeTag]; !found {
		e.pendingAnnotations[nodeTag] = NodeAnnotations{}
	}

	// TODO overrides last set value
	e.pendingAnnotations[nodeTag][annName] = NodeAnnotation{
		Args:   annVals[0].(starlark.Tuple),
		Kwargs: kwargs,
	}

	return starlark.None, nil
}

// args(nodeTag)
func (e *EvaluationCtx) TplStartNode(
	thread *starlark.Thread, _ *starlark.Builtin,
	args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if args.Len() != 1 {
		return starlark.None, fmt.Errorf("expected exactly one argument")
	}

	nodeTag, err := NewNodeTagFromStarlarkValue(args.Index(0))
	if err != nil {
		return starlark.None, err
	}

	return starlark.None, e.startNode(nodeTag)
}

func (e *EvaluationCtx) startNode(nodeTag NodeTag) error {
	node, ok := e.nodes.FindNode(nodeTag)
	if !ok {
		return fmt.Errorf("expected to find %s", nodeTag)
	}

	nodeVal := node.DeepCopyAsInterface().(EvaluationNode)
	nodeVal.ResetValue()

	if nodeAnns, found := e.pendingAnnotations[nodeTag]; found {
		e.pendingAnnotations[nodeTag] = nil
		nodeVal.SetAnnotations(nodeAnns)
	}

	if !e.rootInit {
		if e.dialect.ShouldWrapRootValue(nodeVal) {
			err := e.startNode(e.ancestors.FindParentTag(nodeTag))
			if err != nil {
				return err
			}
		} else {
			e.rootInit = true
			e.rootNode = nodeVal
		}
	}

	if len(e.parentNodes) > 0 {
		commonParentTag := e.ancestors.FindCommonParentTag(
			e.parentNodeTags[len(e.parentNodeTags)-1], nodeTag)
		e.unwindToTag(commonParentTag)

		err := e.dialect.PrepareNode(e.parentNodes[len(e.parentNodes)-1], nodeVal)
		if err != nil {
			return err
		}

		err = e.parentNodes[len(e.parentNodes)-1].AddValue(nodeVal)
		if err != nil {
			return err
		}
	}

	e.parentNodeTags = append(e.parentNodeTags, nodeTag)
	e.parentNodes = append(e.parentNodes, nodeVal)

	return nil
}

func (e *EvaluationCtx) RootNode() interface{} { return e.rootNode }

func (e *EvaluationCtx) RootNodeAsStarlarkValue() starlark.Value {
	val := e.dialect.WrapRootValue(e.rootNode)
	if typedVal, ok := val.(starlark.Value); ok {
		return typedVal
	}
	return core.NewGoValue(val, false).AsStarlarkValue()
}

func (e *EvaluationCtx) unwindToNode(node EvaluationNode) {
	for i, parentNode := range e.parentNodes {
		if parentNode == node {
			e.parentNodes = e.parentNodes[:i+1]
			e.parentNodeTags = e.parentNodeTags[:i+1]
			return
		}
	}
	panic(fmt.Sprintf("expected to find node %T when unwinding", node))
}

func (e *EvaluationCtx) unwindToTag(tag NodeTag) {
	for i, parentTag := range e.parentNodeTags {
		if parentTag.Equals(tag) {
			e.parentNodes = e.parentNodes[:i+1]
			e.parentNodeTags = e.parentNodeTags[:i+1]
			return
		}
	}
	panic(fmt.Sprintf("expected to find %s when unwinding", tag))
}
