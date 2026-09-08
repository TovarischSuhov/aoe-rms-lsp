// Package kb embeds the AoE2 RMS+XS knowledge base and serves read-only
// lookups for the analysis and server cells of aoe2-lsp.
package kb

// Function is a single XS function from the knowledge base.
type Function struct {
	// Name is the exact, case-sensitive function name.
	Name string
	// ReturnType is the XS return type (e.g. "void", "int").
	ReturnType string
	// Params are the declared parameters in source order.
	Params []Param
	// Desc is the human-readable description.
	Desc string
	// SinceUpdate is the DE update id that introduced the function
	// ("" when unknown — treat as always existed).
	SinceUpdate string
}

// Param is a single parameter of an XS function. Required=false means the
// parameter has a default value and may be omitted at call sites.
type Param struct {
	// Name is the parameter name.
	Name string
	// Type is the XS parameter type.
	Type string
	// Required reports whether the parameter must be passed.
	Required bool
	// Desc is the human-readable parameter description.
	Desc string
}

// Constant is a single XS constant from the knowledge base.
type Constant struct {
	// Name is the exact, case-sensitive constant name.
	Name string
	// Section is the Constants.xs section the constant belongs to.
	Section string
	// Value is the string representation of the constant value.
	Value string
	// Desc is the human-readable description.
	Desc string
	// SinceUpdate is the DE update id that introduced the constant
	// ("" when unknown — treat as always existed).
	SinceUpdate string
}

// Command is a single RMS command from the knowledge base.
type Command struct {
	// Name is the exact, case-sensitive command name.
	Name string
	// Section is the lowercase RMS section the command belongs to
	// (e.g. "land_generation", "global").
	Section string
	// Args are the positional arguments of the command.
	Args []CommandArg
	// Attributes are the attributes allowed inside the command block.
	Attributes []CommandArg
	// Desc is the human-readable description.
	Desc string
	// GameVersions lists the game versions the command works in
	// (e.g. "All", "UP/DE", "DE only").
	GameVersions string
	// SinceUpdate is the DE update id that introduced the command
	// ("" when unknown — treat as always existed).
	SinceUpdate string
}

// CommandArg is the specification of one positional argument or attribute
// of an RMS command.
type CommandArg struct {
	// Name is the argument or attribute name.
	Name string
	// Kind is the expected value shape: number / percent / const / ... .
	// Empty after load only when the prose does not parse (flag attributes).
	Kind string
	// Range is the mined value bounds; empty — not mined.
	Range ValueRange
	// Required reports whether the argument must be present.
	Required bool
	// Desc is the human-readable description.
	Desc string
}

// ValueRange is the mined value bounds of a command argument, extracted
// from its Desc prose. Construct-and-use data: no mutation.
type ValueRange struct {
	// Min is the lower bound exactly as written in the prose
	// ("" — not mined).
	Min string `json:"min"`
	// Max is the upper bound exactly as written in the prose
	// ("" — not mined).
	Max string `json:"max"`
}
