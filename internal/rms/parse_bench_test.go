package rms

import (
	"fmt"
	"strings"
	"testing"
)

// benchMap generates a deterministic RMS document shaped like a real map:
// sections, brace-less and braced attribute blocks, conditionals and
// comments. commands controls the create_* statement count.
func benchMap(commands int) string {
	var b strings.Builder

	b.WriteString("/* generated benchmark map */\n")
	b.WriteString("#const BENCH_CONST 42\n")
	b.WriteString("\n<LAND_GENERATION>\n")
	b.WriteString("base_terrain GRASS\n")
	b.WriteString("create_player_lands {\n  terrain_type GRASS\n  land_percent 70\n}\n")

	for i := range commands {
		switch i % 4 {
		case 0:
			fmt.Fprintf(&b, "create_land\n  terrain_type DESERT\n  land_percent %d\n  clumping_factor 8\n}\n", i%90+5)
		case 1:
			fmt.Fprintf(&b, "create_elevation %d\n{\n  number_of_clumps %d\n  set_scale_by_size\n}\n", i%7, i%9+1)
		case 2:
			b.WriteString("if TINY_MAP\n  land_percent 30\nelseif MEDIUM_MAP\n  land_percent 50\nelse\n  land_percent 70\nendif\n")
		case 3:
			fmt.Fprintf(&b, "start_random\n  percent_chance 35\n    land_percent %d\n  percent_chance 65\n    land_percent %d\nend_random\n", i%80, i%70)
		}
	}

	b.WriteString("\n<OBJECTS_GENERATION>\n")

	for i := range commands {
		// alternate brace-less and braced attribute blocks
		if i%2 == 0 {
			fmt.Fprintf(&b, "create_object SHEEP_%d\n  number_of_objects %d\n  set_gaia_object_only\n}\n", i, i%20+1)
		} else {
			fmt.Fprintf(&b, "create_object DEER_%d {\n  number_of_objects %d\n  set_scaling_to_map_size\n}\n", i, i%15+1)
		}
	}

	b.WriteString("\n<TERRAIN_GENERATION>\n")

	for i := range commands {
		fmt.Fprintf(&b, "create_terrain FOREST_%d\n  base_terrain GRASS\n  land_percent %d\n  number_of_clumps %d\n}\n", i, i%25+5, i%40+10)
	}

	return b.String()
}

// BenchmarkParse measures the hot path behind every LSP request: rms.Parse
// over map documents of growing size.
func BenchmarkParse(b *testing.B) {
	for _, size := range []struct {
		name     string
		commands int
	}{
		{"small_200", 50},
		{"medium_2000", 500},
		{"large_10000", 2500},
	} {
		src := benchMap(size.commands)

		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				_, _ = Parse(src, "bench.rms")
			}
		})
	}
}

// BenchmarkParseCorpusStyle measures a comment-heavy document (the shape
// of published maps): comments blank per line before parsing.
func BenchmarkParseCorpusStyle(b *testing.B) {
	var sb strings.Builder

	for i := range 2000 {
		fmt.Fprintf(&sb, "/* section comment %d */\ncreate_object SHEEP\nnumber_of_objects %d // trailing\n", i, i%25)
	}

	src := sb.String()

	b.SetBytes(int64(len(src)))
	b.ReportAllocs()

	for b.Loop() {
		_, _ = Parse(src, "comments.rms")
	}
}
