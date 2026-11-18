# Patched copy of github.com/tdewolff/font

reel uses this copy through a `replace` line in go.mod. It is the upstream
module at the version in go.sum with one change in `sfnt_cff.go`:

- `trimOperands` drops surplus leading operands that some CFF fonts leave on
  the stack before a path operator. Upstream rejects the whole glyph instead.
  In Latin Modern Roman 10pt this made the letter g, the dollar sign and ß
  disappear from typeset maths (`\log` showed as "lo").

Tests, test data and examples were left out to keep the copy small. The
licence is unchanged (LICENSE.md). Drop this copy once the fix is upstream.
