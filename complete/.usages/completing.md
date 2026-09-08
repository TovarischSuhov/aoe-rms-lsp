# Complete Computing — consuming the complete cell

Domain: computing completion candidates for RMS and XS positions.
Target audience: implementers of the server cell (the LSP handler).

## Construct once, share everywhere

completer := complete.NewCompleter(store) // kb.Store via constructor DI

## RMS candidates at a position

// Context matrix: command-name/tail → section commands + owner
// attributes; attribute name → owner attributes; argument/attribute
// value → constants; no section → empty slice.
items := completer.RmsAt(rmsFile, pos)

## XS candidates at a position

// visible, found := xsFile.VisibleAt(pos); !found → inside string or
// comment, render nothing. external decls from Closure.ExternalDecls(uri);
// for inline RMS blocks translate the cursor into block-local coordinates
// first (XsBlock.Range offset) — same as signature help.
items := completer.XsAt(xsFile, pos, externalDecls)

Preconditions:
- Parse the document first; candidates are valid for that parse only.
- Empty slice is the designed silence — never an error.
- Source declarations win over same-name kb entries (truth model);
  same-name source symbols of different kinds both come back.
- The cell never imports go.lsp.dev — mapping Candidate to
  protocol.CompletionItem (Label/Kind/Detail/SortText) belongs to the
  server cell.
