// RMS argument and attribute value checks against knowledge base specs.

package analysis

import (
	"fmt"
	"strconv"
	"strings"

	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
)

// CheckRmsValue checks a single RMS argument/attribute value against its
// Kind specification. kind and value mirror the fields of the leaf
// rms.Expr (the caller skips expressions with operands); r is the value's
// source range. Only percent specs are checked: RMS-level constant names
// (terrain/effect types) are not modelled by the knowledge base.
func CheckRmsValue(spec kb.CommandArg, kind, value string, r common.Range) (common.Diagnostic, bool) {
	if spec.Kind != "percent" {
		return common.Diagnostic{}, false
	}

	if kind != rms.KindNumber && kind != rms.KindPercent {
		return common.Diagnostic{}, false
	}

	n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	if err != nil {
		return common.Diagnostic{}, false
	}

	if n < 0 || n > 100 {
		return common.Diagnostic{
			Range:    r,
			Severity: common.SeverityError,
			Message:  fmt.Sprintf("percent value %s is outside the 0..100 range", value),
			Code:     CodeBadArgumentValue,
		}, true
	}

	return common.Diagnostic{}, false
}
