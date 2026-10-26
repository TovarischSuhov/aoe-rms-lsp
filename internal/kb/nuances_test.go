package kb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// nuanceHit bundles a deprecated/legacy lookup for equality checks.
type nuanceHit struct {
	code    string
	message string
	found   bool
}

// aliasHit bundles an alias lookup for equality checks.
type aliasHit struct {
	right string
	found bool
}

func deprecatedHit(s *Store, name string) nuanceHit {
	code, message, found := s.Deprecated(name)

	return nuanceHit{code, message, found}
}

func legacyHit(s *Store, name string) nuanceHit {
	code, message, found := s.Legacy(name)

	return nuanceHit{code, message, found}
}

// TestStore_NuancesEmbedded checks the shipped nuances.json: effect_percent
// keeps its stable public code and verbatim message; the object-scaling
// pairs are no-ops scoped to create_object; the remaining nuance classes
// carry no data yet.
func TestStore_NuancesEmbedded(t *testing.T) {
	t.Parallel()

	s, err := NewStore()
	require.NoError(t, err)

	t.Run("effect_percent is deprecated", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			nuanceHit{"deprecated-effect-percent", "effect_percent is deprecated; use effect_amount instead", true},
			deprecatedHit(s, "effect_percent"))
	})

	t.Run("object scaling pairs are no-ops on create_object", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			map[string]bool{
				"create_object/set_scale_by_size":   true,
				"create_object/set_scale_by_groups": true,
				"create_terrain/set_scale_by_size":  false,
			},
			map[string]bool{
				"create_object/set_scale_by_size":   s.NoopAttribute("create_object", "set_scale_by_size"),
				"create_object/set_scale_by_groups": s.NoopAttribute("create_object", "set_scale_by_groups"),
				"create_terrain/set_scale_by_size":  s.NoopAttribute("create_terrain", "set_scale_by_size"),
			})
	})

	t.Run("no typo aliases ship", func(t *testing.T) {
		t.Parallel()

		_, found := s.Alias("group_varience")

		require.False(t, found)
	})

	t.Run("no legacy markers ship", func(t *testing.T) {
		t.Parallel()

		_, _, found := s.Legacy("#undefine")

		require.False(t, found)
	})
}

// TestIndexNuances_Synthetic loads one payload with all four nuance classes
// and checks the lookups, including the scoping of no-op pairs to their
// command.
func TestIndexNuances_Synthetic(t *testing.T) {
	t.Parallel()

	s := newStore()
	require.NoError(t, s.indexNuances([]byte(`{
		"noop_attributes": [{"command": "create_terrain", "attribute": "border_fuzziness"}],
		"aliases": [{"wrong": "group_varience", "right": "group_variance"}],
		"deprecated": [{"name": "old_cmd", "code": "deprecated-old-cmd", "message": "old_cmd is deprecated"}],
		"legacy": [{"name": "#undefine", "code": "legacy-undefine", "message": "#undefine is a legacy form"}]
	}`)))

	t.Run("noop pair is scoped to its command", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			map[string]bool{
				"create_terrain/border_fuzziness": true,
				"create_object/border_fuzziness":  false,
			},
			map[string]bool{
				"create_terrain/border_fuzziness": s.NoopAttribute("create_terrain", "border_fuzziness"),
				"create_object/border_fuzziness":  s.NoopAttribute("create_object", "border_fuzziness"),
			})
	})

	t.Run("alias lookups", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name  string
			probe string
			want  aliasHit
		}{
			{name: "typo resolves to the right spelling", probe: "group_varience", want: aliasHit{"group_variance", true}},
			{name: "correct spelling is not an alias", probe: "group_variance", want: aliasHit{"", false}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				right, found := s.Alias(tt.probe)

				require.Equal(t, tt.want, aliasHit{right, found})
			})
		}
	})

	t.Run("deprecated lookup", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			nuanceHit{"deprecated-old-cmd", "old_cmd is deprecated", true},
			deprecatedHit(s, "old_cmd"))
	})

	t.Run("legacy lookup", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			nuanceHit{"legacy-undefine", "#undefine is a legacy form", true},
			legacyHit(s, "#undefine"))
	})
}

// TestIndexNuances_InvalidPayload rejects malformed nuances.json content.
func TestIndexNuances_InvalidPayload(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "unknown key", payload: `{"bogus": []}`, wantErr: "unknown field"},
		{name: "empty noop attribute", payload: `{"noop_attributes": [{"command": "", "attribute": "x"}]}`, wantErr: "empty noop"},
		{
			name:    "duplicate noop pair",
			payload: `{"noop_attributes": [{"command": "c", "attribute": "a"}, {"command": "c", "attribute": "a"}]}`,
			wantErr: `duplicate noop attribute "a" of "c"`,
		},
		{name: "empty alias", payload: `{"aliases": [{"wrong": "w", "right": ""}]}`, wantErr: "empty alias"},
		{
			name:    "duplicate alias",
			payload: `{"aliases": [{"wrong": "w", "right": "a"}, {"wrong": "w", "right": "b"}]}`,
			wantErr: `duplicate alias "w"`,
		},
		{name: "deprecated without code", payload: `{"deprecated": [{"name": "x", "message": "m"}]}`, wantErr: "empty deprecated"},
		{
			name: "duplicate deprecated",
			payload: `{"deprecated": [{"name": "x", "code": "c", "message": "m"},` +
				` {"name": "x", "code": "c2", "message": "m2"}]}`,
			wantErr: `duplicate deprecated name "x"`,
		},
		{name: "legacy without message", payload: `{"legacy": [{"name": "x", "code": "c"}]}`, wantErr: "empty legacy"},
		{
			name: "duplicate legacy",
			payload: `{"legacy": [{"name": "x", "code": "c", "message": "m"},` +
				` {"name": "x", "code": "c2", "message": "m2"}]}`,
			wantErr: `duplicate legacy name "x"`,
		},
		{
			name: "deprecated and legacy clash",
			payload: `{"deprecated": [{"name": "x", "code": "c", "message": "m"}],` +
				` "legacy": [{"name": "x", "code": "c2", "message": "m2"}]}`,
			wantErr: "both deprecated and legacy",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newStore()

			require.ErrorContains(t, s.indexNuances([]byte(tt.payload)), tt.wantErr)
		})
	}
}

// TestNewStoreFromDir loads the synthetic fixture directory: all four data
// files go through the same indexers as the embedded set.
func TestNewStoreFromDir(t *testing.T) {
	t.Parallel()

	s, err := NewStoreFromDir("testdata/nuance-dir")
	require.NoError(t, err)

	t.Run("deprecated", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			nuanceHit{"deprecated-old-cmd", "old_cmd is deprecated", true},
			deprecatedHit(s, "old_cmd"))
	})

	t.Run("legacy", func(t *testing.T) {
		t.Parallel()

		require.Equal(t,
			nuanceHit{"legacy-include-old", "#include_old is legacy", true},
			legacyHit(s, "#include_old"))
	})

	t.Run("noop", func(t *testing.T) {
		t.Parallel()

		require.True(t, s.NoopAttribute("create_terrain", "border_fuzziness"))
	})

	t.Run("alias", func(t *testing.T) {
		t.Parallel()

		right, found := s.Alias("group_varience")

		require.Equal(t, aliasHit{"group_variance", true}, aliasHit{right, found})
	})
}

// TestNewStoreFromDir_MissingDir fails on an absent directory.
func TestNewStoreFromDir_MissingDir(t *testing.T) {
	t.Parallel()

	_, err := NewStoreFromDir("testdata/absent-nuance-dir")

	require.Error(t, err)
}
