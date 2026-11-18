package include

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolver_Referrers checks the reverse include index: every resolved
// directive across the open documents' closures pointing at the target —
// one hit per (owner, directive range), sorted by owner and position,
// including hits discovered transitively (the referrer itself is only
// reachable through an intermediate include). The target's own closure
// is not a root, and files outside every open closure stay invisible.
func TestResolver_Referrers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		// root.rms → shared.rms directly; sibling.rms → mid.rms → shared.rms
		"root.rms":    "<PLAYER_SETUP>\n#include \"shared.rms\"\n#include \"mid.rms\"\n",
		"mid.rms":     "<PLAYER_SETUP>\n#include \"shared.rms\"\n",
		"sibling.rms": "<PLAYER_SETUP>\n#include \"mid.rms\"\n",
		"shared.rms":  "<PLAYER_SETUP>\n",
		"orphan.rms":  "<PLAYER_SETUP>\n#include \"shared.rms\"\n",
	})

	// Open documents are the index roots: root (direct + transitive) and
	// sibling (transitive through mid). orphan.rms is not open and not
	// reachable — its directive stays invisible.
	source := fakeSource{uris["root.rms"]: "<PLAYER_SETUP>\n#include \"shared.rms\"\n#include \"mid.rms\"\n", uris["sibling.rms"]: "<PLAYER_SETUP>\n#include \"mid.rms\"\n"}
	r := NewResolver(source)

	refs := r.Referrers(context.Background(), uris["shared.rms"])
	require.Len(t, refs, 2, "root and mid refer to shared.rms; mid is reached transitively via sibling")

	assert.Equal(t, uris["mid.rms"], refs[0].Owner)
	assert.Equal(t, uris["root.rms"], refs[1].Owner)
	assert.Equal(t, uris["shared.rms"], refs[0].Target)
	assert.Equal(t, uris["shared.rms"], refs[1].Target)

	// A file nobody includes (or an outside URI) yields no hits.
	assert.Empty(t, r.Referrers(context.Background(), uris["root.rms"]))
	assert.Empty(t, r.Referrers(context.Background(), "file:///elsewhere/none.rms"))

	// Same directive spelled twice in one owner: two hits, one per range.
	uris2 := writeTree(t, t.TempDir(), map[string]string{
		"root.rms":   "<PLAYER_SETUP>\n#include \"shared.rms\"\n#include \"shared.rms\"\n",
		"shared.rms": "<PLAYER_SETUP>\n",
	})
	r2 := NewResolver(fakeSource{uris2["root.rms"]: "<PLAYER_SETUP>\n#include \"shared.rms\"\n#include \"shared.rms\"\n"})
	refs2 := r2.Referrers(context.Background(), uris2["shared.rms"])
	require.Len(t, refs2, 2, "each directive is its own edit site")
	assert.NotEqual(t, refs2[0].Inc.Range, refs2[1].Inc.Range)
}

// TestResolver_Referrers_DirectiveKinds pins that #include, #include_drs
// and #includeXS all surface in the index with their path-argument ranges.
func TestResolver_Referrers_DirectiveKinds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := "<PLAYER_SETUP>\n#include \"shared.rms\"\n#include_drs \"shared.rms\" 54000\n#includeXS \"shared.xs\"\n"
	uris := writeTree(t, dir, map[string]string{
		"root.rms":   src,
		"shared.rms": "<PLAYER_SETUP>\n",
		"shared.xs":  "void f() {}\n",
	})

	r := NewResolver(fakeSource{uris["root.rms"]: src})

	rmsRefs := r.Referrers(context.Background(), uris["shared.rms"])
	require.Len(t, rmsRefs, 2, "#include and #include_drs both refer to shared.rms")
	assert.Equal(t, "shared.rms", rmsRefs[0].Inc.Path)
	assert.Equal(t, "shared.rms", rmsRefs[1].Inc.Path)

	xsRefs := r.Referrers(context.Background(), uris["shared.xs"])
	require.Len(t, xsRefs, 1)
	assert.Equal(t, "shared.xs", xsRefs[0].Inc.Path)
}
