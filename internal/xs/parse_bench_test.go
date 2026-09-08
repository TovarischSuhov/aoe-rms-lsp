package xs

import (
	"aoe2-lsp/internal/common"
	"fmt"
	"strings"
	"testing"
)

// benchXs generates a deterministic XS document: functions with bodies of
// calls and locals, plus top-level rules/events.
func benchXs(functions int) string {
	var b strings.Builder

	b.WriteString("/* generated benchmark xs */\n\n")

	for i := range functions {
		fmt.Fprintf(&b, "void fn_%d(int a, float b) {\n", i)
		b.WriteString("  int local = a + 1;\n")
		b.WriteString("  float acc = 0.0;\n")

		for j := range 6 {
			fmt.Fprintf(&b, "  acc += xsGetGoal(%d) * b + local;\n  xsSetGoal(%d, acc);\n", j, j)
		}

		fmt.Fprintf(&b, "  if (acc > %d.0) { xsSetRiverHeight(acc); }\n", i)
		b.WriteString("}\n\n")
	}

	for i := range functions {
		fmt.Fprintf(&b, "rule bench_rule_%d {\n  fn_%d(%d, 1.5);\n}\n\n", i, i, i)
	}

	return b.String()
}

// BenchmarkXsParse measures the XS hot path behind .xs requests and every
// inline block inside .rms documents.
func BenchmarkXsParse(b *testing.B) {
	for _, size := range []struct {
		name      string
		functions int
	}{
		{"small_20", 20},
		{"medium_200", 200},
		{"large_1000", 1000},
	} {
		src := benchXs(size.functions)

		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				_, _ = XsParse(src, "bench.xs")
			}
		})
	}
}

// BenchmarkXsSymbolAt measures symbol lookup at a position (hover path).
func BenchmarkXsSymbolAt(b *testing.B) {
	src := benchXs(500)
	file, _ := XsParse(src, "bench.xs")

	// line 5 (0-based) is the first "acc += xsGetGoal(0)" call of fn_0
	pos := common.Pos{Line: 5, Column: 7}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = file.SymbolAt(pos)
	}
}
