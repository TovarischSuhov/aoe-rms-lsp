package include

import (
	"context"
	"slices"
	"strings"
)

// Referrers is the reverse include index for targetURI: every resolved
// directive across the closures of the open documents (Source.URIs())
// whose target is the given file — one hit per (owner, directive range),
// sorted by owner URI and directive position. It answers "who includes
// me" for workspace/willRenameFiles: unopened referring files are
// covered when an open document's closure reaches them; files outside
// every open closure are invisible (the same editor-state model as
// References). The target's own closure is not a root — a closure
// answers "what the root includes", never who includes the root.
func (r *Resolver) Referrers(ctx context.Context, targetURI string) []ResolvedInclude {
	tgt := canonicalPath(targetURI)
	if tgt == "" {
		return nil
	}

	seen := make(map[targetKey]bool)
	out := make([]ResolvedInclude, 0, 1)

	for _, u := range r.source.URIs() {
		for _, res := range r.Closure(ctx, u).Resolved {
			if canonicalPath(res.Target) != tgt {
				continue
			}

			key := targetKey{uri: res.Owner, r: res.Inc.Range}
			if seen[key] {
				continue
			}

			seen[key] = true
			out = append(out, res)
		}
	}

	slices.SortFunc(out, func(a, b ResolvedInclude) int {
		if c := strings.Compare(a.Owner, b.Owner); c != 0 {
			return c
		}

		if a.Inc.Range.Start.Line != b.Inc.Range.Start.Line {
			return int(a.Inc.Range.Start.Line) - int(b.Inc.Range.Start.Line)
		}

		return int(a.Inc.Range.Start.Column) - int(b.Inc.Range.Start.Column)
	})

	return out
}
