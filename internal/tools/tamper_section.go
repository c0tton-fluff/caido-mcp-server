package tools

import (
	"fmt"
	"sort"
	"strings"
)

// This file owns the single source of truth for how a Match & Replace
// (tamper) section maps onto the Caido GraphQL TamperSectionInput tree.
//
// Every level of that tree is a GraphQL oneof (section -> operation ->
// matcher -> replacer), and the accepted shape varies PER OPERATION, not
// per section: a header `raw` op takes a raw matcher (regex/value/full),
// a header `update` op takes a name matcher, and a method `update` op
// takes no matcher at all. Encoding that in a table rather than a switch
// is what lets one builder serve create/update/test and keeps the
// per-section mode validation from drifting into a second hand-maintained
// list.
//
// Verified against Caido 0.57.0 via the testTamperRule mutation: the
// server rejects unknown input fields outright (e.g. sending `matcher`
// to TamperOperationMethodUpdateInput fails variable coercion), so the
// table must be exact rather than permissive.

// tamperMatcherKind is which matcher input an operation variant accepts.
type tamperMatcherKind int

const (
	// matcherNone means the operation declares no matcher field. Sending
	// one is a coercion error. The operation applies unconditionally.
	matcherNone tamperMatcherKind = iota
	// matcherRaw is TamperMatcherRawInput: a oneof of regex/value/full.
	matcherRaw
	// matcherName is TamperMatcherNameInput: {name: String!}. Used by the
	// header and query add/update/remove operations.
	matcherName
)

// Tool-facing operation kinds. These mirror the Caido GUI vocabulary
// rather than the raw GraphQL field names, which differ per section
// (SNI's value-set operation is spelled `raw` in the schema).
const (
	opUpdateRaw   = "updateRaw"
	opUpdateValue = "updateValue"
	opAdd         = "add"
	opRemove      = "remove"
)

// tamperOpSpec is the exact shape of ONE operation variant.
type tamperOpSpec struct {
	// gqlKey is the field name inside the operation oneof.
	gqlKey string
	// matcher is which matcher input this variant declares.
	matcher tamperMatcherKind
	// replacer reports whether this variant declares a replacer field.
	// False for remove, which takes a matcher only.
	replacer bool
}

// tamperSectionSpec is a tool-facing section name plus the operation
// variants its GraphQL section field accepts.
type tamperSectionSpec struct {
	// gqlKey is the field name inside TamperSectionInput.
	gqlKey string
	// ops maps a tool-facing operation kind to its shape.
	ops map[string]tamperOpSpec
}

// rawOnlyOps is the operation set for sections whose only variant is a
// raw regex/value/full match with a replacer: all, body, firstLine,
// path, and the websocket message sections.
func rawOnlyOps() map[string]tamperOpSpec {
	return map[string]tamperOpSpec{
		opUpdateRaw: {gqlKey: "raw", matcher: matcherRaw, replacer: true},
	}
}

// namedFieldOps is the operation set for sections addressing named
// fields -- request/response headers and query parameters. These are the
// only sections that accept all four GUI modes.
func namedFieldOps() map[string]tamperOpSpec {
	return map[string]tamperOpSpec{
		opUpdateRaw:   {gqlKey: "raw", matcher: matcherRaw, replacer: true},
		opUpdateValue: {gqlKey: "update", matcher: matcherName, replacer: true},
		opAdd:         {gqlKey: "add", matcher: matcherName, replacer: true},
		opRemove:      {gqlKey: "remove", matcher: matcherName, replacer: false},
	}
}

// valueOnlyOps is the operation set for sections that carry a bare
// replacer and no matcher: method, status code, SNI. The operation
// always applies, so there is nothing to match against.
func valueOnlyOps(gqlKey string) map[string]tamperOpSpec {
	return map[string]tamperOpSpec{
		opUpdateValue: {gqlKey: gqlKey, matcher: matcherNone, replacer: true},
	}
}

// tamperSections is the section table. Adding a section here is all that
// is required to expose it; the builder, the validation errors and the
// tool descriptions all derive from this map.
var tamperSections = map[string]tamperSectionSpec{
	"requestAll":       {gqlKey: "requestAll", ops: rawOnlyOps()},
	"requestBody":      {gqlKey: "requestBody", ops: rawOnlyOps()},
	"requestFirstLine": {gqlKey: "requestFirstLine", ops: rawOnlyOps()},
	"requestPath":      {gqlKey: "requestPath", ops: rawOnlyOps()},
	"requestHeader":    {gqlKey: "requestHeader", ops: namedFieldOps()},
	"requestQuery":     {gqlKey: "requestQuery", ops: namedFieldOps()},
	"requestMethod":    {gqlKey: "requestMethod", ops: valueOnlyOps("update")},
	// SNI's value-set operation is spelled `raw` in the schema even
	// though it declares no matcher. The table records the discrepancy so
	// callers never have to.
	"requestSNI":         {gqlKey: "requestSNI", ops: valueOnlyOps("raw")},
	"responseAll":        {gqlKey: "responseAll", ops: rawOnlyOps()},
	"responseBody":       {gqlKey: "responseBody", ops: rawOnlyOps()},
	"responseFirstLine":  {gqlKey: "responseFirstLine", ops: rawOnlyOps()},
	"responseHeader":     {gqlKey: "responseHeader", ops: namedFieldOps()},
	"responseStatusCode": {gqlKey: "responseStatusCode", ops: valueOnlyOps("update")},
}

// TamperOperation selects and parameterizes one Match & Replace
// operation mode. It is optional on the create/update tools: when
// omitted, the section's default mode is used with the legacy top-level
// match/replace fields.
type TamperOperation struct {
	Kind string `json:"kind,omitempty" jsonschema:"Operation mode: updateRaw (regex/literal over the raw section) updateValue (set a named header or query param) add (insert a named field) remove (delete a named field). Defaults to updateRaw where supported."`
	// Name is the header or query parameter name for
	// updateValue/add/remove.
	Name string `json:"name,omitempty" jsonschema:"Header or query parameter name. Required for updateValue add and remove."`
	// Match is the pattern for updateRaw.
	Match string `json:"match,omitempty" jsonschema:"Pattern for updateRaw. Interpreted per match_kind."`
	// MatchKind selects the raw matcher variant.
	MatchKind string `json:"match_kind,omitempty" jsonschema:"How to interpret match for updateRaw: regex (default) value (literal substring no escaping needed) or full (the entire section)."`
	// Value is the replacement text.
	Value string `json:"value,omitempty" jsonschema:"Replacement text. Not allowed for remove."`
	// WorkflowID replaces the matched content with a workflow's output.
	WorkflowID string `json:"workflow_id,omitempty" jsonschema:"Use a convert workflow as the replacement instead of value. Mutually exclusive with value."`
}

// tamperSectionNames returns the tool-facing section names, sorted, for
// use in error messages and tool descriptions.
func tamperSectionNames() []string {
	names := make([]string, 0, len(tamperSections))
	for name := range tamperSections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// tamperOpNames returns a section's supported operation kinds, sorted.
func tamperOpNames(spec tamperSectionSpec) []string {
	names := make([]string, 0, len(spec.ops))
	for name := range spec.ops {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// defaultTamperOpKind picks the operation used when the caller does not
// name one. updateRaw is preferred so existing match/replace callers keep
// their behavior; sections without it have exactly one mode, so that mode
// is unambiguous.
func defaultTamperOpKind(spec tamperSectionSpec) string {
	if _, ok := spec.ops[opUpdateRaw]; ok {
		return opUpdateRaw
	}
	names := tamperOpNames(spec)
	if len(names) == 1 {
		return names[0]
	}
	// Unreachable with the current table: every section either supports
	// updateRaw or has a single mode. Guard so a future table entry fails
	// loudly here rather than silently picking a mode by map order.
	return ""
}

// buildTamperSection constructs the GraphQL TamperSectionInput value.
//
// Maps are used rather than the generated input structs because every
// level is a GraphQL oneof and genqlient v0.8.1 drops omitempty from
// nullable pointer fields, serializing unset variants as explicit null
// and violating the oneof. A map only carries the keys we set. Keep this
// approach until a genqlient release fixes omitempty on nullable pointer
// fields (v0.8.1 is current; there is no v0.9.x).
func buildTamperSection(
	section string, op TamperOperation,
) (map[string]any, error) {
	spec, ok := tamperSections[section]
	if !ok {
		return nil, fmt.Errorf(
			"unknown section %q: valid sections are %s",
			section, strings.Join(tamperSectionNames(), ", "),
		)
	}

	kind := op.Kind
	if kind == "" {
		kind = defaultTamperOpKind(spec)
	}
	opSpec, ok := spec.ops[kind]
	if !ok {
		return nil, fmt.Errorf(
			"section %q does not support operation %q: it supports %s",
			section, kind, strings.Join(tamperOpNames(spec), ", "),
		)
	}

	body := map[string]any{}

	matcher, err := buildTamperMatcher(section, kind, opSpec, op)
	if err != nil {
		return nil, err
	}
	if matcher != nil {
		body["matcher"] = matcher
	}

	replacer, err := buildTamperReplacer(kind, opSpec, op)
	if err != nil {
		return nil, err
	}
	if replacer != nil {
		body["replacer"] = replacer
	}

	return map[string]any{
		spec.gqlKey: map[string]any{
			"operation": map[string]any{opSpec.gqlKey: body},
		},
	}, nil
}

// buildTamperMatcher builds the matcher variant for an operation, or
// returns nil when the operation declares no matcher field. Fields that
// do not belong to the chosen mode are rejected locally so the caller
// gets an actionable message instead of a GraphQL coercion error.
func buildTamperMatcher(
	section, kind string, opSpec tamperOpSpec, op TamperOperation,
) (map[string]any, error) {
	switch opSpec.matcher {
	case matcherNone:
		if op.Name != "" {
			return nil, fmt.Errorf(
				"operation %q on section %q takes no name: it always applies",
				kind, section,
			)
		}
		if op.Match != "" {
			return nil, fmt.Errorf(
				"operation %q on section %q takes no match: it always applies, "+
					"set value only", kind, section,
			)
		}
		return nil, nil

	case matcherName:
		if op.Name == "" {
			return nil, fmt.Errorf(
				"operation %q on section %q requires name", kind, section,
			)
		}
		if op.Match != "" {
			return nil, fmt.Errorf(
				"operation %q matches by name, not by pattern: "+
					"drop match or use updateRaw", kind,
			)
		}
		return map[string]any{"name": op.Name}, nil

	case matcherRaw:
		if op.Name != "" {
			return nil, fmt.Errorf(
				"operation %q matches by pattern, not by name: "+
					"drop name or use updateValue", kind,
			)
		}
		return buildTamperRawMatcher(op)

	default:
		return nil, fmt.Errorf("unhandled matcher kind for operation %q", kind)
	}
}

// buildTamperRawMatcher builds the TamperMatcherRawInput oneof.
func buildTamperRawMatcher(op TamperOperation) (map[string]any, error) {
	matchKind := op.MatchKind
	if matchKind == "" {
		matchKind = "regex"
	}
	switch matchKind {
	case "regex":
		if op.Match == "" {
			return nil, fmt.Errorf("match is required when match_kind is regex")
		}
		return map[string]any{"regex": map[string]any{"regex": op.Match}}, nil
	case "value":
		if op.Match == "" {
			return nil, fmt.Errorf("match is required when match_kind is value")
		}
		return map[string]any{"value": map[string]any{"value": op.Match}}, nil
	case "full":
		if op.Match != "" {
			return nil, fmt.Errorf(
				"match_kind full replaces the entire section: drop match",
			)
		}
		return map[string]any{"full": map[string]any{"full": true}}, nil
	default:
		return nil, fmt.Errorf(
			"unknown match_kind %q: use regex, value or full", matchKind,
		)
	}
}

// buildTamperReplacer builds the TamperReplacerInput variant, or returns
// nil for operations that declare no replacer field.
func buildTamperReplacer(
	kind string, opSpec tamperOpSpec, op TamperOperation,
) (map[string]any, error) {
	if !opSpec.replacer {
		if op.Value != "" || op.WorkflowID != "" {
			return nil, fmt.Errorf(
				"operation %q deletes the field and takes no value "+
					"or workflow_id", kind,
			)
		}
		return nil, nil
	}
	if op.WorkflowID != "" {
		if op.Value != "" {
			return nil, fmt.Errorf(
				"value and workflow_id are mutually exclusive",
			)
		}
		return map[string]any{
			"workflow": map[string]any{"id": op.WorkflowID},
		}, nil
	}
	// An empty value is meaningful: it blanks the matched content.
	return map[string]any{"term": map[string]any{"term": op.Value}}, nil
}

// resolveTamperOperation merges the legacy top-level match/replace
// fields into an operation. An explicit operation always wins; the
// legacy fields only apply when no operation was supplied, which is what
// keeps pre-existing callers working unchanged.
func resolveTamperOperation(
	op *TamperOperation, match, replace string,
) TamperOperation {
	if op != nil {
		return *op
	}
	return TamperOperation{Match: match, Value: replace}
}

// TamperSectionModes returns every section mapped to the operation modes
// it supports. Exported so tests can walk the whole table and assert each
// section/mode pair against the real GraphQL schema, rather than sampling
// a few entries by hand.
func TamperSectionModes() map[string][]string {
	out := make(map[string][]string, len(tamperSections))
	for name, spec := range tamperSections {
		out[name] = tamperOpNames(spec)
	}
	return out
}

// TamperSectionGQLKey returns a section's GraphQL field name, or "" when
// the section is unknown. Exported for the table-walking test.
func TamperSectionGQLKey(section string) string {
	return tamperSections[section].gqlKey
}

// TamperMatcherKindFor reports which matcher a section/mode pair takes as
// one of "raw", "name" or "none", or "" when the pair is unknown.
// Exported for the table-walking test.
func TamperMatcherKindFor(section, kind string) string {
	spec, ok := tamperSections[section]
	if !ok {
		return ""
	}
	op, ok := spec.ops[kind]
	if !ok {
		return ""
	}
	switch op.matcher {
	case matcherRaw:
		return "raw"
	case matcherName:
		return "name"
	case matcherNone:
		return "none"
	default:
		return ""
	}
}

// TamperHasReplacer reports whether a section/mode pair takes a replacer.
// Exported for the table-walking test.
func TamperHasReplacer(section, kind string) bool {
	spec, ok := tamperSections[section]
	if !ok {
		return false
	}
	return spec.ops[kind].replacer
}

// tamperSectionDoc renders the section list for tool descriptions,
// annotating the sections that accept more than one operation mode.
func tamperSectionDoc() string {
	var multi []string
	for _, name := range tamperSectionNames() {
		if len(tamperSections[name].ops) > 1 {
			multi = append(multi, name)
		}
	}
	return fmt.Sprintf(
		"sections: %s. All four modes (updateRaw/updateValue/add/remove) "+
			"are available on %s; every other section supports only its "+
			"single mode",
		strings.Join(tamperSectionNames(), " "),
		strings.Join(multi, " "),
	)
}
