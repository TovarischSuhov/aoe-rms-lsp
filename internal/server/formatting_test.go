package server

import (
	"aoe2-lsp/internal/format"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestServe_InitializeAdvertisesFormatting pins the capability contract:
// documentFormatting is advertised like its document-feature siblings, so
// editors learn the feature exists without probing it.
func TestServe_InitializeAdvertisesFormatting(t *testing.T) {
	h := startHarness(t)

	res, err := h.disp.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.Equal(t, protocol.Boolean(true), res.Capabilities.DocumentFormattingProvider,
		"documentFormatting must be advertised like the other document features")
}

// TestServe_Formatting drives textDocument/formatting through the real
// stdio harness: the answer is one full-document edit whose text is the
// cell's canonical print; refusals and non-RMS documents answer an empty
// slice, and positions follow the negotiated encoding.
func TestServe_Formatting(t *testing.T) {
	cleanRMS := "<LAND_GENERATION>\ncreate_land {\nland_percent 50\n}\n</LAND_GENERATION>\n"

	// the parse-level refusal fixture of internal/format (TestRMS_RefusalOnError):
	// error-severity parse diagnostics, not an unknown-command analyzer note
	parseErrorRMS := "<LAND_GENERATION>\n}\ncreate_object @@@\n</LAND_GENERATION>\n"

	// no trailing newline, non-ASCII on the last line: the end position is
	// what the encoding negotiation is about
	cyrillicRMS := "<LAND_GENERATION>\n/* Поколение */ base_terrain GRASS"

	tests := []struct {
		name          string
		docURI        uri.URI
		languageID    string
		text          string
		negotiateUTF8 bool
		options       protocol.FormattingOptions
		formatOpts    format.Options // the oracle options the request maps to
		wantTabs      bool           // the print must indent with tabs
		wantEmpty     bool           // empty non-nil answer: format not invoked
	}{
		{
			name:          "clean rms",
			docURI:        uri.URI("file:///work/fmt.rms"),
			languageID:    "aoe2rms",
			text:          cleanRMS,
			negotiateUTF8: true,
			options:       protocol.FormattingOptions{TabSize: 2, InsertSpaces: true},
			formatOpts:    format.Options{TabSize: 2},
		},
		{
			name:          "insert spaces false indents with tabs",
			docURI:        uri.URI("file:///work/tabs.rms"),
			languageID:    "aoe2rms",
			text:          cleanRMS,
			negotiateUTF8: true,
			options:       protocol.FormattingOptions{TabSize: 4},
			formatOpts:    format.Options{TabSize: 4, IndentTabs: true},
			wantTabs:      true,
		},
		{
			name:          "tab size 0 falls back to the default 4",
			docURI:        uri.URI("file:///work/zero.rms"),
			languageID:    "aoe2rms",
			text:          cleanRMS,
			negotiateUTF8: true,
			options:       protocol.FormattingOptions{InsertSpaces: true},
			formatOpts:    format.Options{},
		},
		{
			name:          "parse errors refuse the format",
			docURI:        uri.URI("file:///work/broken.rms"),
			languageID:    "aoe2rms",
			text:          parseErrorRMS,
			negotiateUTF8: true,
			options:       protocol.FormattingOptions{TabSize: 4, InsertSpaces: true},
			wantEmpty:     true,
		},
		{
			name:          "xs documents are not formatted",
			docURI:        uri.URI("file:///work/script.xs"),
			languageID:    "aoe2xs",
			text:          "<LAND_GENERATION>\ncreate_land\n</LAND_GENERATION>\n",
			negotiateUTF8: true,
			options:       protocol.FormattingOptions{TabSize: 2, InsertSpaces: true},
			wantEmpty:     true,
		},
		{
			name:       "default utf-16 end position",
			docURI:     uri.URI("file:///work/ru.rms"),
			languageID: "aoe2rms",
			text:       cyrillicRMS,
			options:    protocol.FormattingOptions{TabSize: 2, InsertSpaces: true},
			formatOpts: format.Options{TabSize: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startFormattingHarness(t, tt.negotiateUTF8)
			openForFormatting(t, h, tt.docURI, tt.languageID, tt.text)

			edits, err := h.disp.Formatting(context.Background(), &protocol.DocumentFormattingParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: tt.docURI},
				Options:      tt.options,
			})
			require.NoError(t, err)

			// the one decisive comparison: DeepEqual pins the single edit's
			// range and bytes, and for the empty cases non-nil emptiness
			require.Equal(t, wantFormattingEdits(t, tt.text, tt.formatOpts, tt.wantEmpty, tt.negotiateUTF8), edits)

			if tt.wantTabs {
				require.Contains(t, edits[0].NewText, "\t", "the print must indent with tabs")
			}
		})
	}
}

// wantFormattingEdits builds the expected answer from the cell's format
// oracle: one edit covering the whole document whose text is the canonical
// print — or the empty non-nil slice where no format may happen at all.
func wantFormattingEdits(
	t *testing.T,
	text string,
	opts format.Options,
	wantEmpty bool,
	utf8Encoding bool,
) []protocol.TextEdit {
	t.Helper()

	if wantEmpty {
		return []protocol.TextEdit{}
	}

	want, err := format.RMS(text, opts)
	require.NoError(t, err, "the oracle must accept the fixture")

	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   endOfText(text, utf8Encoding),
		},
		NewText: want,
	}}
}

// endOfText is the position just past the document's last byte: the range
// a full-document replacement covers. Columns follow the negotiated
// encoding — bytes for utf-8, UTF-16 code units otherwise.
func endOfText(text string, utf8Encoding bool) protocol.Position {
	last := text[strings.LastIndexByte(text, '\n')+1:]
	line := uint32(strings.Count(text, "\n"))

	if utf8Encoding {
		return protocol.Position{Line: line, Character: uint32(len(last))}
	}

	return protocol.Position{Line: line, Character: byteToUTF16(last, uint32(len(last)))}
}

// startFormattingHarness starts the stdio harness and negotiates utf-8
// when wanted; left out, the protocol's utf-16 default stays.
func startFormattingHarness(t *testing.T, negotiateUTF8 bool) *lspHarness {
	t.Helper()

	h := startHarness(t)

	params := &protocol.InitializeParams{}
	if negotiateUTF8 {
		params.Capabilities.General = &protocol.GeneralClientCapabilities{
			PositionEncodings: []protocol.PositionEncodingKind{protocol.PositionEncodingKindUTF8},
		}
	}

	_, err := h.disp.Initialize(context.Background(), params)
	require.NoError(t, err)

	return h
}

// openForFormatting opens the document with the given language and waits
// for its first diagnostics batch (the formatting test prologue).
func openForFormatting(t *testing.T, h *lspHarness, docURI uri.URI, languageID, text string) {
	t.Helper()

	require.NoError(t, h.disp.DidOpen(context.Background(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: protocol.LanguageKind(languageID), Version: 1, Text: text,
		},
	}))
	h.waitDiagnostics(docURI)
}
