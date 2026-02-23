# amll-ttml

[中文文档](README.zh-CN.md)

`amll-ttml` is a Go library for:

- Parsing TTML lyrics into structured data
- Exporting structured lyrics back to TTML
- Converting `TTML -> AMLX binary`
- Converting `AMLX binary -> TTML`

The AMLX binary format implementation follows the project spec in `SPEC.md`.

## Install

```bash
go get github.com/xiaowumin-mark/amll-ttml
```

## Core APIs

### TTML parser/writer

- `ParseLyric(ttmlText string) (TTMLLyric, error)`
- `ExportTTMLText(ttmlLyric TTMLLyric, pretty bool) string`

### AMLX binary codec

- `TTMLToBinary(ttmlText string) ([]byte, error)`
- `BinaryToTTML(binaryData []byte, pretty bool) (string, error)`
- `EncodeBinary(ttmlLyric TTMLLyric) ([]byte, error)`
- `DecodeBinary(binaryData []byte) (TTMLLyric, error)`
- Aliases: `EncodeAMLX`, `DecodeAMLX`

## Quick Example

```go
package main

import (
	"fmt"
	"os"

	ttml "github.com/xiaowumin-mark/amll-ttml"
)

func main() {
	rawTTML, err := os.ReadFile("input.ttml")
	if err != nil {
		panic(err)
	}

	// TTML -> binary
	bin, err := ttml.TTMLToBinary(string(rawTTML))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("output.amlx", bin, 0o644); err != nil {
		panic(err)
	}

	// binary -> TTML
	roundTripTTML, err := ttml.BinaryToTTML(bin, false)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("roundtrip.ttml", []byte(roundTripTTML), 0o644); err != nil {
		panic(err)
	}

	fmt.Println("done")
}
```

## Compatibility Behavior (Legacy TTML)

To support historical TTML variants in production:

- If line timing does not fully cover word timing, line timing is normalized to the word envelope.
- Timing values are rounded to integer milliseconds.
- Invalid/non-finite `empty-beat` values are ignored during encoding.

This keeps conversion robust while preserving lyric content and supported attributes.

## Test

### Normal tests

```bash
go test ./...
```

### Extreme batch test

This test:

- Reads all `.ttml` files under `test/raw-ttml`
- Writes AMLX files to `test/binary`
- Converts those AMLX files back to TTML under `test/binary-to-ttml`
- Records per-file timing and average timing
- Writes logs to:
  - `test/extreme-conversion.log`
  - `test/extreme-conversion.json`

Run it with:

```bash
RUN_EXTREME_TEST=1 go test -run TestExtremeTTMLBinaryPipeline -count=1 ./...
```

PowerShell example:

```powershell
$env:RUN_EXTREME_TEST='1'
go test -run TestExtremeTTMLBinaryPipeline -count=1 ./...
```

## License

MIT. See `LICENSE`.
