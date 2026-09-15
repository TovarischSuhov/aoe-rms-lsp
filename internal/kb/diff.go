package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiffReport is the aggregated old→new comparison of two kb data
// directories, produced by DiffKB as the semi-automatic "what's new"
// step of the refresh process (docs/kb-refresh.md).
type DiffReport struct {
	Functions EntityDiff
	Constants EntityDiff
	Commands  EntityDiff
}

// EntityDiff lists the added, removed and changed names of one entity kind;
// all three slices are sorted by name.
type EntityDiff struct {
	Added   []string
	Removed []string
	Changed []ChangedEntity
}

// ChangedEntity names one entity and the fields that differ between the two
// data versions, as stable field paths: top-level keys ("desc",
// "since_update"), positional slots ("params[0]") or nested fields
// ("args[terrain].kind").
type ChangedEntity struct {
	Name   string
	Fields []string
}

// Empty reports whether the compared data directories are equivalent.
func (r DiffReport) Empty() bool {
	return r.Functions.empty() && r.Constants.empty() && r.Commands.empty()
}

// Render returns the report as deterministic markdown — stable section
// order, name sorting, fixed field order — so repeated runs on the same
// inputs produce byte-identical output (usable as a PR description).
func (r DiffReport) Render() string {
	var b strings.Builder

	b.WriteString("# KB diff\n")

	renderDiffSection(&b, "XS functions", r.Functions)
	renderDiffSection(&b, "XS constants", r.Constants)
	renderDiffSection(&b, "RMS commands", r.Commands)

	if r.Empty() {
		b.WriteString("\nNo changes.\n")
	}

	return b.String()
}

// empty reports whether the entity kind has no differences.
func (d EntityDiff) empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// renderDiffSection writes one non-empty section of the markdown report.
func renderDiffSection(b *strings.Builder, title string, d EntityDiff) {
	if d.empty() {
		return
	}

	fmt.Fprintf(b, "\n## %s — %d added, %d removed, %d changed\n",
		title, len(d.Added), len(d.Removed), len(d.Changed))

	for _, name := range d.Added {
		fmt.Fprintf(b, "- + %s\n", name)
	}

	for _, name := range d.Removed {
		fmt.Fprintf(b, "- - %s\n", name)
	}

	for _, c := range d.Changed {
		fmt.Fprintf(b, "- ~ %s: %s\n", c.Name, strings.Join(c.Fields, ", "))
	}
}

// DiffKB compares the kb data files of oldDir and newDir (layout by the
// kbdata annotation) and returns the aggregated old→new report. It is a
// data-build utility of the refresh process (docs/kb-refresh.md), not a
// runtime dependency: the server never calls it.
func DiffKB(oldDir, newDir string) (DiffReport, error) {
	oldData, err := loadKbDataDir(oldDir)
	if err != nil {
		return DiffReport{}, fmt.Errorf("load old data: %w", err)
	}

	newData, err := loadKbDataDir(newDir)
	if err != nil {
		return DiffReport{}, fmt.Errorf("load new data: %w", err)
	}

	functions, err := diffRecords(oldData.functions, newData.functions,
		func(f functionWire) string { return f.Name }, diffFunctionFields)
	if err != nil {
		return DiffReport{}, fmt.Errorf("diff functions: %w", err)
	}

	constants, err := diffRecords(oldData.constants, newData.constants,
		func(c constantWire) string { return c.Name }, diffConstantFields)
	if err != nil {
		return DiffReport{}, fmt.Errorf("diff constants: %w", err)
	}

	commands, err := diffRecords(oldData.commands, newData.commands,
		func(c commandWire) string { return c.Name }, diffCommandFields)
	if err != nil {
		return DiffReport{}, fmt.Errorf("diff commands: %w", err)
	}

	return DiffReport{Functions: functions, Constants: constants, Commands: commands}, nil
}

// kbDataDir is the decoded content of one kb data directory.
type kbDataDir struct {
	functions []functionWire
	constants []constantWire
	commands  []commandWire
}

// loadKbDataDir reads and decodes the three kb data files of dir.
func loadKbDataDir(dir string) (kbDataDir, error) {
	var data kbDataDir

	if err := readDataFile(dir, "xs-functions.json", &data.functions); err != nil {
		return data, err
	}

	if err := readDataFile(dir, "xs-constants.json", &data.constants); err != nil {
		return data, err
	}

	if err := readDataFile(dir, "rms-commands.json", &data.commands); err != nil {
		return data, err
	}

	return data, nil
}

// readDataFile decodes one data file of a kb data directory into target.
func readDataFile[T any](dir string, name string, target *[]T) error {
	path := filepath.Join(dir, name)

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	return decodeData(name, raw, target)
}

// diffRecords compares records by name and returns their added, removed and
// changed members; fields reports the differing field paths of two records
// with equal names. Duplicate names are a data error, mirroring the Store
// load validation.
func diffRecords[T any](old []T, new []T, name func(T) string, fields func(T, T) []string) (EntityDiff, error) {
	oldBy, err := indexByName(old, name)
	if err != nil {
		return EntityDiff{}, err
	}

	newBy, err := indexByName(new, name)
	if err != nil {
		return EntityDiff{}, err
	}

	union := make(map[string]struct{}, len(oldBy)+len(newBy))
	for n := range oldBy {
		union[n] = struct{}{}
	}

	for n := range newBy {
		union[n] = struct{}{}
	}

	var diff EntityDiff

	for _, n := range sortedKeys(union) {
		oldRec, inOld := oldBy[n]
		newRec, inNew := newBy[n]

		switch {
		case !inOld:
			diff.Added = append(diff.Added, n)
		case !inNew:
			diff.Removed = append(diff.Removed, n)
		default:
			if changed := fields(oldRec, newRec); len(changed) > 0 {
				diff.Changed = append(diff.Changed, ChangedEntity{Name: n, Fields: changed})
			}
		}
	}

	return diff, nil
}

// indexByName maps records by name, rejecting duplicates.
func indexByName[T any](records []T, name func(T) string) (map[string]T, error) {
	byName := make(map[string]T, len(records))

	for _, rec := range records {
		n := name(rec)
		if _, dup := byName[n]; dup {
			return nil, fmt.Errorf("duplicate name %q", n)
		}

		byName[n] = rec
	}

	return byName, nil
}

// diffFunctionFields returns the paths of fields that differ between two
// function records with equal names.
func diffFunctionFields(a, b functionWire) []string {
	var fields []string

	if a.Category != b.Category {
		fields = append(fields, "category")
	}

	if a.ReturnType != b.ReturnType {
		fields = append(fields, "return_type")
	}

	fields = append(fields, diffParams("params", a.Params, b.Params)...)

	if a.Desc != b.Desc {
		fields = append(fields, "desc")
	}

	if a.SinceUpdate != b.SinceUpdate {
		fields = append(fields, "since_update")
	}

	return fields
}

// diffConstantFields returns the paths of fields that differ between two
// constant records with equal names.
func diffConstantFields(a, b constantWire) []string {
	var fields []string

	if a.Section != b.Section {
		fields = append(fields, "section")
	}

	if a.Value != b.Value {
		fields = append(fields, "value")
	}

	if a.Desc != b.Desc {
		fields = append(fields, "desc")
	}

	if a.SinceUpdate != b.SinceUpdate {
		fields = append(fields, "since_update")
	}

	return fields
}

// diffCommandFields returns the paths of fields that differ between two
// command records with equal names.
func diffCommandFields(a, b commandWire) []string {
	var fields []string

	if a.Section != b.Section {
		fields = append(fields, "section")
	}

	fields = append(fields, diffArgs("args", a.Args, b.Args)...)
	fields = append(fields, diffArgs("attributes", a.Attributes, b.Attributes)...)

	if a.Desc != b.Desc {
		fields = append(fields, "desc")
	}

	if a.GameVersions != b.GameVersions {
		fields = append(fields, "game_versions")
	}

	if a.SinceUpdate != b.SinceUpdate {
		fields = append(fields, "since_update")
	}

	return fields
}

// diffParams compares two positional parameter lists; a length mismatch
// collapses to the bare path because slot names no longer line up.
func diffParams(prefix string, a, b []paramWire) []string {
	if len(a) != len(b) {
		return []string{prefix}
	}

	var fields []string

	for i := range a {
		if a[i].Name != b[i].Name {
			fields = append(fields, fmt.Sprintf("%s[%d]", prefix, i))
			continue
		}

		path := fmt.Sprintf("%s[%s]", prefix, a[i].Name)

		if a[i].Type != b[i].Type {
			fields = append(fields, path+".type")
		}

		if a[i].Required != b[i].Required {
			fields = append(fields, path+".required")
		}

		if a[i].Desc != b[i].Desc {
			fields = append(fields, path+".desc")
		}
	}

	return fields
}

// diffArgs compares two positional argument lists (command args and
// attributes alike); see diffParams for the length-mismatch collapse.
func diffArgs(prefix string, a, b []argWire) []string {
	if len(a) != len(b) {
		return []string{prefix}
	}

	var fields []string

	for i := range a {
		if a[i].Name != b[i].Name {
			fields = append(fields, fmt.Sprintf("%s[%d]", prefix, i))
			continue
		}

		path := fmt.Sprintf("%s[%s]", prefix, a[i].Name)

		if a[i].Kind != b[i].Kind {
			fields = append(fields, path+".kind")
		}

		if a[i].Range != b[i].Range {
			fields = append(fields, path+".range")
		}

		if a[i].Required != b[i].Required {
			fields = append(fields, path+".required")
		}

		if a[i].Desc != b[i].Desc {
			fields = append(fields, path+".desc")
		}
	}

	return fields
}
