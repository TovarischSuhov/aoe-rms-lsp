package include

import (
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClosure_TypesConstruct checks the contract shape: every closure data
// type constructs with its declared fields.
func TestClosure_TypesConstruct(t *testing.T) {
	c := Closure{Root: "file:///main.rms"}
	assert.Equal(t, "file:///main.rms", c.Root)

	r := RmsEntry{URI: "file:///a.rms", File: rms.RmsFile{Name: "a.rms"}}
	assert.Equal(t, "a.rms", r.File.Name)

	x := XsEntry{URI: "file:///lib.xs", File: xs.XsFile{Name: "lib.xs"}}
	assert.Equal(t, "lib.xs", x.File.Name)

	res := ResolvedInclude{Owner: "file:///main.rms", Inc: rms.Include{Path: "a.rms"}, Target: "file:///a.rms"}
	assert.Equal(t, "a.rms", res.Inc.Path)

	m := MissingInclude{Owner: "file:///main.rms", Path: "x.rms"}
	assert.Equal(t, "file:///main.rms", m.Owner)

	tg := Target{URI: "file:///a.rms"}
	assert.NotNil(t, tg.URI)
}

// TestClosure_ExternalDecls checks declaration collection with exclusion.
func TestClosure_ExternalDecls(t *testing.T) {
	decl := func(name string) xs.Decl { return xs.Decl{Kind: xs.DeclFunction, Name: name} }

	c := Closure{
		Xs: []XsEntry{
			{URI: "file:///lib.xs", File: xs.XsFile{Decls: []xs.Decl{decl("sharedFn"), decl("helper")}}},
			{URI: "file:///other.xs", File: xs.XsFile{Decls: []xs.Decl{decl("otherFn")}}},
		},
	}

	all := c.ExternalDecls("")
	require.Len(t, all, 3)
	assert.Equal(t, "sharedFn", all[0].Name)
	assert.Equal(t, "otherFn", all[2].Name) // DFS order preserved

	excluded := c.ExternalDecls("file:///lib.xs")
	require.Len(t, excluded, 1)
	assert.Equal(t, "otherFn", excluded[0].Name)

	assert.Empty(t, Closure{}.ExternalDecls(""))
}
