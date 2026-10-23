package kb

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Embedded knowledge base files; see the kbdata annotation in CODEMANIFEST
// for the schema of each file. Regeneration is described in
// kb/.usages/data-pipeline.md; nuances.json is curated by hand and is not
// regenerated.
//
//go:embed data/xs-functions.json data/xs-constants.json data/rms-commands.json data/nuances.json
var dataFS embed.FS

// Store is the loaded knowledge base; the single lookup point for all
// consumers. Construct it once with NewStore and share it; after
// construction the data is immutable and safe for concurrent reads.
type Store struct {
	functions        []Function
	functionByName   map[string]Function
	constants        []Constant
	constantByName   map[string]Constant
	constantsBySect  map[string][]Constant
	commands         []Command
	commandByName    map[string]Command
	commandsBySect   map[string][]Command
	attrByKey        map[string]CommandArg
	noopAttrByKey    map[string]bool
	aliasByName      map[string]string
	deprecatedByName map[string]marker
	legacyByName     map[string]marker
}

// marker is a deprecated or legacy name entry from nuances.json: the
// stable diagnostic code and the diagnostic text.
type marker struct {
	Code    string
	Message string
}

// NewStore loads the embedded JSON files and validates their schema:
// unknown keys, duplicate names and empty names are load errors.
func NewStore() (*Store, error) {
	s := newStore()

	if err := s.loadFunctions(); err != nil {
		return nil, fmt.Errorf("load xs-functions: %w", err)
	}

	if err := s.loadConstants(); err != nil {
		return nil, fmt.Errorf("load xs-constants: %w", err)
	}

	if err := s.loadCommands(); err != nil {
		return nil, fmt.Errorf("load rms-commands: %w", err)
	}

	if err := s.loadNuances(); err != nil {
		return nil, fmt.Errorf("load nuances: %w", err)
	}

	return s, nil
}

// NewStoreFromDir loads the knowledge base from a directory of JSON data
// files with the same schema as the embedded set (including nuances.json).
// It serves synthetic-fixture tests and offline tooling; NewStore is the
// runtime path. Validation is identical to NewStore.
func NewStoreFromDir(dir string) (*Store, error) {
	s := newStore()

	files := []struct {
		name  string
		index func([]byte) error
	}{
		{"xs-functions.json", s.indexFunctions},
		{"xs-constants.json", s.indexConstants},
		{"rms-commands.json", s.indexCommands},
		{"nuances.json", s.indexNuances},
	}

	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.name, err)
		}

		if err := f.index(raw); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// newStore returns an empty Store with initialized lookup maps.
func newStore() *Store {
	return &Store{
		functionByName:   make(map[string]Function),
		constantByName:   make(map[string]Constant),
		constantsBySect:  make(map[string][]Constant),
		commandByName:    make(map[string]Command),
		commandsBySect:   make(map[string][]Command),
		attrByKey:        make(map[string]CommandArg),
		noopAttrByKey:    make(map[string]bool),
		aliasByName:      make(map[string]string),
		deprecatedByName: make(map[string]marker),
		legacyByName:     make(map[string]marker),
	}
}

// Function looks up an XS function by its exact (case-sensitive) name.
func (s *Store) Function(name string) (Function, bool) {
	fn, found := s.functionByName[name]

	return fn, found
}

// Functions returns all XS functions in data-file order (for completion).
func (s *Store) Functions() []Function {
	return s.functions
}

// Constant looks up an XS constant by its exact (case-sensitive) name.
func (s *Store) Constant(name string) (Constant, bool) {
	c, found := s.constantByName[name]

	return c, found
}

// Constants returns the constants of one section; section "" means all
// sections, in data-file order.
func (s *Store) Constants(section string) []Constant {
	if section == "" {
		return s.constants
	}

	return s.constantsBySect[section]
}

// Command looks up an RMS command by its exact (case-sensitive) name.
func (s *Store) Command(name string) (Command, bool) {
	cmd, found := s.commandByName[name]

	return cmd, found
}

// Commands returns the RMS commands of one section; section "" means all
// sections, in data-file order.
func (s *Store) Commands(section string) []Command {
	if section == "" {
		return s.commands
	}

	return s.commandsBySect[section]
}

// Attribute returns the specification of one attribute of a command;
// found=false when the command or the attribute is unknown.
func (s *Store) Attribute(command string, attr string) (CommandArg, bool) {
	arg, found := s.attrByKey[attrKey(command, attr)]

	return arg, found
}

// NoopAttribute reports whether the attribute is a documented no-op on
// the command: the game most likely ignores it in this position.
func (s *Store) NoopAttribute(command string, attr string) bool {
	return s.noopAttrByKey[attrKey(command, attr)]
}

// Alias returns the correct spelling for a known typo alias;
// found=false when the name is not an alias.
func (s *Store) Alias(name string) (right string, found bool) {
	right, found = s.aliasByName[name]

	return right, found
}

// Deprecated returns the diagnostic code and text for a deprecated name;
// found=false when the name is not marked deprecated.
func (s *Store) Deprecated(name string) (code string, message string, found bool) {
	m, ok := s.deprecatedByName[name]
	if !ok {
		return "", "", false
	}

	return m.Code, m.Message, true
}

// Legacy returns the diagnostic code and text for a legacy name kept for
// compatibility; found=false when the name is not marked legacy.
func (s *Store) Legacy(name string) (code string, message string, found bool) {
	m, ok := s.legacyByName[name]
	if !ok {
		return "", "", false
	}

	return m.Code, m.Message, true
}

// attrKey builds the composite index key for a command attribute.
func attrKey(command string, attr string) string {
	return command + "\x00" + attr
}

// loadFunctions reads and indexes xs-functions.json.
func (s *Store) loadFunctions() error {
	raw, err := dataFS.ReadFile("data/xs-functions.json")
	if err != nil {
		return fmt.Errorf("read data/xs-functions.json: %w", err)
	}

	return s.indexFunctions(raw)
}

// indexFunctions decodes and indexes xs-functions.json content.
func (s *Store) indexFunctions(raw []byte) error {
	var wire []functionWire

	if err := decodeData("data/xs-functions.json", raw, &wire); err != nil {
		return err
	}

	for _, wf := range wire {
		if err := validateName(wf.Name); err != nil {
			return err
		}

		if _, dup := s.functionByName[wf.Name]; dup {
			return fmt.Errorf("duplicate function name %q", wf.Name)
		}

		fn := Function{
			Name:        wf.Name,
			ReturnType:  wf.ReturnType,
			Params:      make([]Param, 0, len(wf.Params)),
			Desc:        wf.Desc,
			SinceUpdate: wf.SinceUpdate,
		}

		for _, wp := range wf.Params {
			fn.Params = append(fn.Params, Param(wp))
		}

		s.functions = append(s.functions, fn)
		s.functionByName[fn.Name] = fn
	}

	return nil
}

// loadConstants reads and indexes xs-constants.json.
func (s *Store) loadConstants() error {
	raw, err := dataFS.ReadFile("data/xs-constants.json")
	if err != nil {
		return fmt.Errorf("read data/xs-constants.json: %w", err)
	}

	return s.indexConstants(raw)
}

// indexConstants decodes and indexes xs-constants.json content.
func (s *Store) indexConstants(raw []byte) error {
	var wire []constantWire

	if err := decodeData("data/xs-constants.json", raw, &wire); err != nil {
		return err
	}

	for _, wc := range wire {
		if err := validateName(wc.Name); err != nil {
			return err
		}

		if _, dup := s.constantByName[wc.Name]; dup {
			return fmt.Errorf("duplicate constant name %q", wc.Name)
		}

		c := Constant(wc)
		s.constants = append(s.constants, c)
		s.constantByName[c.Name] = c
		s.constantsBySect[c.Section] = append(s.constantsBySect[c.Section], c)
	}

	return nil
}

// loadCommands reads and indexes rms-commands.json.
func (s *Store) loadCommands() error {
	raw, err := dataFS.ReadFile("data/rms-commands.json")
	if err != nil {
		return fmt.Errorf("read data/rms-commands.json: %w", err)
	}

	return s.indexCommands(raw)
}

// indexCommands decodes and indexes rms-commands.json content.
func (s *Store) indexCommands(raw []byte) error {
	var wire []commandWire

	if err := decodeData("data/rms-commands.json", raw, &wire); err != nil {
		return err
	}

	for _, wcmd := range wire {
		if err := validateName(wcmd.Name); err != nil {
			return err
		}

		if _, dup := s.commandByName[wcmd.Name]; dup {
			return fmt.Errorf("duplicate command name %q", wcmd.Name)
		}

		cmd := Command{
			Name:         wcmd.Name,
			Section:      wcmd.Section,
			Args:         make([]CommandArg, 0, len(wcmd.Args)),
			Attributes:   make([]CommandArg, 0, len(wcmd.Attributes)),
			Desc:         wcmd.Desc,
			GameVersions: wcmd.GameVersions,
			SinceUpdate:  wcmd.SinceUpdate,
		}

		for _, wa := range wcmd.Args {
			arg := CommandArg(wa)
			mineCommandArg(&arg)
			cmd.Args = append(cmd.Args, arg)
		}

		for _, wa := range wcmd.Attributes {
			attr := CommandArg(wa)
			mineCommandArg(&attr)
			cmd.Attributes = append(cmd.Attributes, attr)
		}

		// Mining happens before indexing so lookups serve mined data.
		s.commands = append(s.commands, cmd)
		s.commandByName[cmd.Name] = cmd
		s.commandsBySect[cmd.Section] = append(s.commandsBySect[cmd.Section], cmd)

		for _, attr := range cmd.Attributes {
			s.attrByKey[attrKey(cmd.Name, attr.Name)] = attr
		}
	}

	return nil
}

// validateName rejects the empty names that would poison the lookup maps.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("empty record name")
	}

	return nil
}

// loadNuances reads and indexes nuances.json (hand-curated, never
// regenerated — see the kbdata annotation).
func (s *Store) loadNuances() error {
	raw, err := dataFS.ReadFile("data/nuances.json")
	if err != nil {
		return fmt.Errorf("read data/nuances.json: %w", err)
	}

	return s.indexNuances(raw)
}

// indexNuances decodes and indexes nuances.json content: no-op
// command-attribute pairs, typo aliases and deprecated/legacy markers.
func (s *Store) indexNuances(raw []byte) error {
	var wire nuancesWire

	if err := decodeData("data/nuances.json", raw, &wire); err != nil {
		return err
	}

	for _, wn := range wire.NoopAttributes {
		if wn.Command == "" || wn.Attribute == "" {
			return fmt.Errorf("empty noop command or attribute")
		}

		key := attrKey(wn.Command, wn.Attribute)
		if s.noopAttrByKey[key] {
			return fmt.Errorf("duplicate noop attribute %q of %q", wn.Attribute, wn.Command)
		}

		s.noopAttrByKey[key] = true
	}

	for _, wa := range wire.Aliases {
		if wa.Wrong == "" || wa.Right == "" {
			return fmt.Errorf("empty alias name")
		}

		if _, dup := s.aliasByName[wa.Wrong]; dup {
			return fmt.Errorf("duplicate alias %q", wa.Wrong)
		}

		s.aliasByName[wa.Wrong] = wa.Right
	}

	for _, wd := range wire.Deprecated {
		if err := indexMarker(s.deprecatedByName, wd, "deprecated"); err != nil {
			return err
		}
	}

	for _, wl := range wire.Legacy {
		if err := indexMarker(s.legacyByName, wl, "legacy"); err != nil {
			return err
		}
	}

	for name := range s.deprecatedByName {
		if _, clash := s.legacyByName[name]; clash {
			return fmt.Errorf("name %q marked both deprecated and legacy", name)
		}
	}

	return nil
}

// indexMarker validates and indexes one deprecated/legacy entry.
func indexMarker(byName map[string]marker, w markerWire, class string) error {
	if w.Name == "" || w.Code == "" || w.Message == "" {
		return fmt.Errorf("empty %s name, code or message", class)
	}

	if _, dup := byName[w.Name]; dup {
		return fmt.Errorf("duplicate %s name %q", class, w.Name)
	}

	byName[w.Name] = marker{Code: w.Code, Message: w.Message}

	return nil
}

// mineCommandArg fills the empty Range and Kind of arg from its Desc
// prose. Structured values win (provenance rule): only empty fields are
// filled, so the helper never overwrites values from extraction and is
// idempotent across the load and extraction pipelines. Shared by
// indexCommands here and buildCommand in extract.go — the identical
// semantics at both call sites are what makes load idempotent with
// extraction.
func mineCommandArg(arg *CommandArg) {
	zero := ValueRange{}

	if arg.Range == zero || arg.Kind == "" {
		kind, r := MineKindRange(arg.Desc)

		if arg.Range == zero {
			arg.Range = r
		}

		if arg.Kind == "" {
			arg.Kind = kind
		}
	}
}

// decodeData decodes one embedded JSON payload with unknown-key rejection.
func decodeData(name string, raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}

	return nil
}

// Wire types mirror the embedded JSON schema exactly (see kbdata); extra
// keys are rejected, so the tags enumerate the full schema.

// functionWire is the JSON shape of one xs-functions.json record.
type functionWire struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	ReturnType  string      `json:"return_type"`
	Params      []paramWire `json:"params"`
	Desc        string      `json:"desc"`
	SinceUpdate string      `json:"since_update"`
}

// paramWire is the JSON shape of one function parameter.
type paramWire struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// constantWire is the JSON shape of one xs-constants.json record.
type constantWire struct {
	Name        string `json:"name"`
	Section     string `json:"section"`
	Value       string `json:"value"`
	Desc        string `json:"desc"`
	SinceUpdate string `json:"since_update"`
}

// commandWire is the JSON shape of one rms-commands.json record.
type commandWire struct {
	Name         string    `json:"name"`
	Section      string    `json:"section"`
	Args         []argWire `json:"args"`
	Attributes   []argWire `json:"attributes"`
	Desc         string    `json:"desc"`
	GameVersions string    `json:"game_versions"`
	SinceUpdate  string    `json:"since_update"`
}

// argWire is the JSON shape of one command argument or attribute.
type argWire struct {
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Range    ValueRange `json:"range"`
	Required bool       `json:"required"`
	Desc     string     `json:"desc"`
}

// nuancesWire is the JSON shape of nuances.json.
type nuancesWire struct {
	NoopAttributes []noopAttrWire `json:"noop_attributes"`
	Aliases        []aliasWire    `json:"aliases"`
	Deprecated     []markerWire   `json:"deprecated"`
	Legacy         []markerWire   `json:"legacy"`
}

// noopAttrWire is one no-op command-attribute pair.
type noopAttrWire struct {
	Command   string `json:"command"`
	Attribute string `json:"attribute"`
}

// aliasWire is one typo alias: the wrong spelling and the correct one.
type aliasWire struct {
	Wrong string `json:"wrong"`
	Right string `json:"right"`
}

// markerWire is one deprecated/legacy name with its diagnostic code and text.
type markerWire struct {
	Name    string `json:"name"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
