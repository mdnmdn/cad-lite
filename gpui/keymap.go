package gpui

import (
	"fmt"
	"reflect"
	"strings"
)

// Action is any named, cloneable, comparable command.
type Action interface {
	Name() string
	Clone() Action
	Equals(other Action) bool
}

// ActionKey is the identity OnAction handlers are registered under: the
// action's concrete type (GPUI's TypeId) plus, for actions built with
// NewAction, their name. NewAction("a") and NewAction("b") are therefore
// distinct, while any two values of a user-defined action type share a key
// (a zero value can register the handler, as in GPUI).
type ActionKey struct {
	Type reflect.Type
	Name string
}

// ActionKeyOf returns the OnAction registration key of a; nil yields the
// zero key.
func ActionKeyOf(a Action) ActionKey {
	if a == nil {
		return ActionKey{}
	}
	k := ActionKey{Type: reflect.TypeOf(a)}
	if n, ok := a.(namedAction); ok {
		k.Name = n.name
	}
	return k
}

// namedAction is the trivial value-type Action built by NewAction.
type namedAction struct{ name string }

// NewAction builds a simple named action (a value type for the POC).
func NewAction(name string) Action { return namedAction{name: name} }

func (a namedAction) Name() string { return a.name }

// Clone returns a copy of the action.
func (a namedAction) Clone() Action { return a }

// Equals reports whether two named actions wrap the same name.
func (a namedAction) Equals(other Action) bool {
	o, ok := other.(namedAction)
	return ok && o.name == a.name
}

// KeyBinding binds one keystroke to an action, active when the focused
// element's key-context stack satisfies Context.
type KeyBinding struct {
	Action     Action
	Keystrokes []Keystroke
	// Context is a predicate over the context stack: "", bare keys and
	// key=value leaves combined with "&&", "||", "!" and parentheses.
	// Empty matches any stack.
	Context string
}

// NewKeyBinding parses keystrokeStr (e.g. "cmd-enter") and context (e.g.
// "editor && !readonly") and returns a binding for the action. It panics on a
// malformed keystroke or context: bindings are construction-time constants,
// so a bad literal is a programming error.
func NewKeyBinding(keystrokeStr string, action Action, context string) KeyBinding {
	ks, err := ParseKeystroke(keystrokeStr)
	if err != nil {
		panic(err)
	}
	if _, err := parseKeyContext(context); err != nil {
		panic(err)
	}
	return KeyBinding{
		Action:     action,
		Keystrokes: []Keystroke{ks},
		Context:    context,
	}
}

// Keymap is the ordered set of active key bindings.
type Keymap struct {
	bindings []KeyBinding
}

// NewKeymap returns an empty keymap.
func NewKeymap() *Keymap { return &Keymap{} }

// AddBindings appends bindings, preserving insertion order.
func (k *Keymap) AddBindings(bindings ...KeyBinding) {
	k.bindings = append(k.bindings, bindings...)
}

// Bindings returns a copy of the bindings in insertion order.
func (k *Keymap) Bindings() []KeyBinding {
	return append([]KeyBinding(nil), k.bindings...)
}

// Len returns the number of bindings.
func (k *Keymap) Len() int { return len(k.bindings) }

// Clear removes every binding.
func (k *Keymap) Clear() { k.bindings = nil }

// MatchKeystroke returns the action of the binding whose last keystroke
// Matches typed and whose context predicate is satisfied by ctxStack, or nil if none
// matches. Bindings are scanned in reverse insertion order, so a binding added
// later wins over an earlier one for the same keystroke and context (later
// bindings are the more specific/override ones).
func (k *Keymap) MatchKeystroke(typed Keystroke, ctxStack []string) Action {
	for i := len(k.bindings) - 1; i >= 0; i-- {
		b := &k.bindings[i]
		if len(b.Keystrokes) == 0 {
			continue
		}
		last := b.Keystrokes[len(b.Keystrokes)-1]
		if !last.Matches(typed) {
			continue
		}
		p, err := parseKeyContext(b.Context)
		if err != nil {
			continue
		}
		if p.matches(ctxStack) {
			return b.Action
		}
	}
	return nil
}

// MatchKeystrokeWithContext is MatchKeystroke returning an explicit ok instead
// of a nil Action.
func (k *Keymap) MatchKeystrokeWithContext(typed Keystroke, ctxStack []string) (Action, bool) {
	a := k.MatchKeystroke(typed, ctxStack)
	return a, a != nil
}

// keyContextPredicate is a parsed Context expression. The zero kind is a leaf.
type keyContextPredicate struct {
	kind    keyContextPredKind
	leaf    string               // predLeaf: the token, e.g. "editor" or "mode=normal"
	operand *keyContextPredicate // predNot
	left    *keyContextPredicate // predAnd / predOr
	right   *keyContextPredicate // predAnd / predOr
}

type keyContextPredKind uint8

const (
	predLeaf keyContextPredKind = iota
	predNot
	predAnd
	predOr
)

// parseKeyContext parses a context predicate. The grammar is:
//
//	or   := and ( "||" and )*
//	and  := unary ( "&&" unary )*
//	unary := "!" unary | primary
//	primary := "(" or ")" | leaf
//
// A leaf is a bare token ("editor") or a key=value token ("mode=normal"); it
// matches when any entry of the context stack equals the token. "!" negates;
// precedence is "!" > "&&" > "||". An empty (or whitespace-only) string parses
// to a nil predicate, which matches everything. Malformed input is an error.
func parseKeyContext(s string) (*keyContextPredicate, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	toks, err := lexKeyContext(s)
	if err != nil {
		return nil, err
	}
	p := &ctxParser{toks: toks, src: s}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, fmt.Errorf("gpui: unexpected %q in key context %q", t.display(), s)
	}
	return e, nil
}

// matches reports whether the predicate is satisfied by the context stack. A
// nil predicate matches everything.
func (p *keyContextPredicate) matches(ctxStack []string) bool {
	if p == nil {
		return true
	}
	switch p.kind {
	case predLeaf:
		for _, e := range ctxStack {
			if e == p.leaf {
				return true
			}
		}
		return false
	case predNot:
		return !p.operand.matches(ctxStack)
	case predAnd:
		return p.left.matches(ctxStack) && p.right.matches(ctxStack)
	case predOr:
		return p.left.matches(ctxStack) || p.right.matches(ctxStack)
	}
	return false
}

type ctxTokKind uint8

const (
	tokLeaf ctxTokKind = iota
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
	tokEOF
)

type ctxToken struct {
	kind ctxTokKind
	text string
}

func (t ctxToken) display() string {
	switch t.kind {
	case tokAnd:
		return "&&"
	case tokOr:
		return "||"
	case tokNot:
		return "!"
	case tokLParen:
		return "("
	case tokRParen:
		return ")"
	case tokEOF:
		return "end of input"
	}
	return t.text
}

func lexKeyContext(s string) ([]ctxToken, error) {
	var toks []ctxToken
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '&' && i+1 < len(s) && s[i+1] == '&':
			toks = append(toks, ctxToken{kind: tokAnd})
			i += 2
		case c == '|' && i+1 < len(s) && s[i+1] == '|':
			toks = append(toks, ctxToken{kind: tokOr})
			i += 2
		case c == '&':
			return nil, fmt.Errorf("gpui: expected \"&&\" in key context %q", s)
		case c == '|':
			return nil, fmt.Errorf("gpui: expected \"||\" in key context %q", s)
		case c == '!':
			toks = append(toks, ctxToken{kind: tokNot})
			i++
		case c == '(':
			toks = append(toks, ctxToken{kind: tokLParen})
			i++
		case c == ')':
			toks = append(toks, ctxToken{kind: tokRParen})
			i++
		default:
			start := i
			for i < len(s) && !strings.ContainsRune(" \t\r\n&|!()", rune(s[i])) {
				i++
			}
			toks = append(toks, ctxToken{kind: tokLeaf, text: s[start:i]})
		}
	}
	toks = append(toks, ctxToken{kind: tokEOF})
	return toks, nil
}

type ctxParser struct {
	toks []ctxToken
	pos  int
	src  string
}

func (p *ctxParser) peek() ctxToken { return p.toks[p.pos] }

func (p *ctxParser) next() ctxToken {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *ctxParser) parseOr() (*keyContextPredicate, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &keyContextPredicate{kind: predOr, left: left, right: right}
	}
	return left, nil
}

func (p *ctxParser) parseAnd() (*keyContextPredicate, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &keyContextPredicate{kind: predAnd, left: left, right: right}
	}
	return left, nil
}

func (p *ctxParser) parseUnary() (*keyContextPredicate, error) {
	if p.peek().kind == tokNot {
		p.next()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &keyContextPredicate{kind: predNot, operand: operand}, nil
	}
	return p.parsePrimary()
}

func (p *ctxParser) parsePrimary() (*keyContextPredicate, error) {
	t := p.next()
	switch t.kind {
	case tokLParen:
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("gpui: expected %q in key context %q", ")", p.src)
		}
		p.next()
		return e, nil
	case tokLeaf:
		return &keyContextPredicate{kind: predLeaf, leaf: t.text}, nil
	default:
		return nil, fmt.Errorf("gpui: unexpected %q in key context %q", t.display(), p.src)
	}
}
