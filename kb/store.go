package kb

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
)

// Embedded knowledge base files; see the kbdata annotation in CODEMANIFEST
// for the schema of each file. Regeneration is described in
// kb/.usages/data-pipeline.md.
//
//go:embed data/xs-functions.json data/xs-constants.json data/rms-commands.json
var dataFS embed.FS

// Store is the loaded knowledge base; the single lookup point for all
// consumers. Construct it once with NewStore and share it; after
// construction the data is immutable and safe for concurrent reads.
type Store struct {
	functions       []Function
	functionByName  map[string]Function
	constants       []Constant
	constantByName  map[string]Constant
	constantsBySect map[string][]Constant
	commands        []Command
	commandByName   map[string]Command
	commandsBySect  map[string][]Command
	attrByKey       map[string]CommandArg
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

	return s, nil
}

// newStore returns an empty Store with initialized lookup maps.
func newStore() *Store {
	return &Store{
		functionByName:  make(map[string]Function),
		constantByName:  make(map[string]Constant),
		constantsBySect: make(map[string][]Constant),
		commandByName:   make(map[string]Command),
		commandsBySect:  make(map[string][]Command),
		attrByKey:       make(map[string]CommandArg),
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
			cmd.Args = append(cmd.Args, CommandArg(wa))
		}

		for _, wa := range wcmd.Attributes {
			cmd.Attributes = append(cmd.Attributes, CommandArg(wa))
		}

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
