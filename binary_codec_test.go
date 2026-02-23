package ttml

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

func TestEncodeDecodeBinaryRoundTrip(t *testing.T) {
	original := TTMLLyric{
		Metadata: []TTMLMetadata{
			{
				Key:   "album",
				Value: []string{"1989", "Deluxe"},
				Error: false,
			},
			{
				Key:   "source",
				Value: []string{"itunes"},
				Error: true,
			},
		},
		LyricLines: []LyricLine{
			{
				ID:              "line-main",
				StartTime:       1000,
				EndTime:         2200,
				IsDuet:          true,
				IgnoreSync:      true,
				TranslatedLyric: "welcome-cn",
				RomanLyric:      "huan ying lai dao niu yue",
				Words: []LyricWord{
					{
						ID:           "w1",
						StartTime:    1000,
						EndTime:      1400,
						Word:         "Wel",
						Obscene:      true,
						RomanWord:    "wel",
						RomanWarning: true,
					},
					{
						ID:        "w2",
						StartTime: 1400,
						EndTime:   2200,
						Word:      "come",
						EmptyBeat: 120,
					},
				},
			},
			{
				ID:        "line-bg",
				StartTime: 2300,
				EndTime:   2600,
				IsBG:      true,
				Words: []LyricWord{
					{
						ID:        "w3",
						StartTime: 2300,
						EndTime:   2600,
						Word:      "(New York)",
					},
				},
			},
		},
	}

	encoded, err := EncodeBinary(original)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := DecodeBinary(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if !reflect.DeepEqual(normalizeLyricForCompare(original), normalizeLyricForCompare(decoded)) {
		t.Fatalf("decoded lyric mismatch\nexpected: %#v\nactual: %#v", normalizeLyricForCompare(original), normalizeLyricForCompare(decoded))
	}
}

func TestTTMLBinaryBridges(t *testing.T) {
	original := TTMLLyric{
		Metadata: []TTMLMetadata{
			{
				Key:   "artist",
				Value: []string{"Taylor Swift"},
			},
		},
		LyricLines: []LyricLine{
			{
				StartTime:       500,
				EndTime:         1200,
				TranslatedLyric: "hello-cn",
				Words: []LyricWord{
					{
						StartTime: 500,
						EndTime:   900,
						Word:      "Hel",
					},
					{
						StartTime: 900,
						EndTime:   1200,
						Word:      "lo",
					},
				},
			},
		},
	}

	ttmlText := ExportTTMLText(original, false)

	binaryData, err := TTMLToBinary(ttmlText)
	if err != nil {
		t.Fatalf("TTMLToBinary failed: %v", err)
	}

	decodedLyric, err := DecodeBinary(binaryData)
	if err != nil {
		t.Fatalf("DecodeBinary failed: %v", err)
	}

	recoveredTTML, err := BinaryToTTML(binaryData, false)
	if err != nil {
		t.Fatalf("BinaryToTTML failed: %v", err)
	}

	parsedRecovered, err := ParseLyric(recoveredTTML)
	if err != nil {
		t.Fatalf("ParseLyric(recovered) failed: %v", err)
	}

	if !reflect.DeepEqual(normalizeLyricForCompare(decodedLyric), normalizeLyricForCompare(parsedRecovered)) {
		t.Fatalf("bridge round-trip mismatch\nfrom binary: %#v\nfrom ttml: %#v", normalizeLyricForCompare(decodedLyric), normalizeLyricForCompare(parsedRecovered))
	}
}

func TestDecodeBinaryRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "invalid magic",
			payload: []byte("BMLX"),
		},
		{
			name:    "string index out of bounds",
			payload: buildOutOfBoundsStringIDPayload(),
		},
		{
			name:    "reserved line flags",
			payload: buildReservedLineFlagPayload(),
		},
		{
			name:    "reserved word flags",
			payload: buildReservedWordFlagPayload(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeBinary(tc.payload); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestEncodeBinaryLegacyLineTimingCompatibility(t *testing.T) {
	input := TTMLLyric{
		Metadata: []TTMLMetadata{
			{
				Key:   "k",
				Value: []string{"v1", "v2"},
			},
		},
		LyricLines: []LyricLine{
			{
				StartTime:       1000,
				EndTime:         1300,
				TranslatedLyric: "tr",
				RomanLyric:      "rm",
				IsBG:            true,
				IsDuet:          true,
				IgnoreSync:      true,
				Words: []LyricWord{
					{
						StartTime:    900, // earlier than line start (legacy inconsistent case)
						EndTime:      1100,
						Word:         "w1",
						Obscene:      true,
						RomanWord:    "rw1",
						RomanWarning: true,
						EmptyBeat:    0,
					},
					{
						StartTime: 1100,
						EndTime:   1500, // later than line end
						Word:      "w2",
						EmptyBeat: 200,
					},
				},
			},
		},
	}

	b, err := EncodeBinary(input)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	got, err := DecodeBinary(b)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(got.LyricLines) != 1 {
		t.Fatalf("unexpected line count: %d", len(got.LyricLines))
	}
	line := got.LyricLines[0]

	// Line envelope is normalized to cover all words for legacy compatibility.
	if line.StartTime != 900 {
		t.Fatalf("expected normalized line start 900, got %.3f", line.StartTime)
	}
	if line.EndTime != 1500 {
		t.Fatalf("expected normalized line end 1500, got %.3f", line.EndTime)
	}

	if line.TranslatedLyric != "tr" || line.RomanLyric != "rm" || !line.IsBG || !line.IsDuet || !line.IgnoreSync {
		t.Fatalf("line properties not preserved: %#v", line)
	}

	if len(line.Words) != 2 {
		t.Fatalf("unexpected word count: %d", len(line.Words))
	}
	if line.Words[0].Word != "w1" || line.Words[0].RomanWord != "rw1" || !line.Words[0].Obscene || !line.Words[0].RomanWarning {
		t.Fatalf("word[0] properties not preserved: %#v", line.Words[0])
	}
	if line.Words[1].Word != "w2" || line.Words[1].EmptyBeat != 200 {
		t.Fatalf("word[1] properties not preserved: %#v", line.Words[1])
	}
}

func TestEncodeBinaryIgnoresInvalidEmptyBeat(t *testing.T) {
	input := TTMLLyric{
		LyricLines: []LyricLine{
			{
				StartTime: 1000,
				EndTime:   1200,
				Words: []LyricWord{
					{
						StartTime: 1000,
						EndTime:   1200,
						Word:      "x",
						EmptyBeat: math.NaN(),
					},
				},
			},
		},
	}

	b, err := EncodeBinary(input)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	got, err := DecodeBinary(b)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(got.LyricLines) != 1 || len(got.LyricLines[0].Words) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", got)
	}
	if got.LyricLines[0].Words[0].EmptyBeat != 0 {
		t.Fatalf("invalid empty beat should be omitted, got %.3f", got.LyricLines[0].Words[0].EmptyBeat)
	}
}

func normalizeLyricForCompare(lyric TTMLLyric) TTMLLyric {
	out := TTMLLyric{
		Metadata:   make([]TTMLMetadata, 0, len(lyric.Metadata)),
		LyricLines: make([]LyricLine, 0, len(lyric.LyricLines)),
	}

	for _, meta := range lyric.Metadata {
		values := append([]string(nil), meta.Value...)
		out.Metadata = append(out.Metadata, TTMLMetadata{
			Key:   meta.Key,
			Value: values,
			Error: meta.Error,
		})
	}

	for _, line := range lyric.LyricLines {
		cleanLine := line
		cleanLine.ID = ""
		cleanLine.Words = make([]LyricWord, 0, len(line.Words))
		for _, word := range line.Words {
			cleanWord := word
			cleanWord.ID = ""
			cleanLine.Words = append(cleanLine.Words, cleanWord)
		}
		out.LyricLines = append(out.LyricLines, cleanLine)
	}

	return out
}

func buildOutOfBoundsStringIDPayload() []byte {
	var header bytes.Buffer
	writeTestUvarint(&header, 1) // metadata_count
	writeTestUvarint(&header, 1) // key_string_id (out of bounds)
	writeTestUvarint(&header, 0) // value_count
	header.WriteByte(0)          // error_flag

	var payload bytes.Buffer
	payload.WriteString(amlxMagic)
	payload.WriteByte(amlxVersion)
	payload.WriteByte(0) // global_flags
	writeTestUvarint(&payload, uint64(header.Len()))
	payload.Write(header.Bytes())

	writeTestUvarint(&payload, 1) // string_count
	writeTestUvarint(&payload, 1) // string[0].byte_length
	payload.WriteByte('a')

	writeTestUvarint(&payload, 0) // line_count
	return payload.Bytes()
}

func buildReservedLineFlagPayload() []byte {
	var header bytes.Buffer
	writeTestUvarint(&header, 0) // metadata_count

	var payload bytes.Buffer
	payload.WriteString(amlxMagic)
	payload.WriteByte(amlxVersion)
	payload.WriteByte(0) // global_flags
	writeTestUvarint(&payload, uint64(header.Len()))
	payload.Write(header.Bytes())

	writeTestUvarint(&payload, 0) // string_count

	writeTestUvarint(&payload, 1) // line_count
	writeTestUvarint(&payload, 0) // line_start_time
	writeTestUvarint(&payload, 1) // line_end_time
	payload.WriteByte(0x20)       // line_flags (reserved bit 5)
	writeTestUvarint(&payload, 0) // word_count

	return payload.Bytes()
}

func buildReservedWordFlagPayload() []byte {
	var header bytes.Buffer
	writeTestUvarint(&header, 0) // metadata_count

	var payload bytes.Buffer
	payload.WriteString(amlxMagic)
	payload.WriteByte(amlxVersion)
	payload.WriteByte(0) // global_flags
	writeTestUvarint(&payload, uint64(header.Len()))
	payload.Write(header.Bytes())

	writeTestUvarint(&payload, 1) // string_count
	writeTestUvarint(&payload, 1) // string[0].byte_length
	payload.WriteByte('x')

	writeTestUvarint(&payload, 1) // line_count
	writeTestUvarint(&payload, 0) // line_start_time
	writeTestUvarint(&payload, 1) // line_end_time
	payload.WriteByte(0x00)       // line_flags
	writeTestUvarint(&payload, 1) // word_count

	writeTestUvarint(&payload, 0) // delta_start_time
	writeTestUvarint(&payload, 1) // duration
	writeTestUvarint(&payload, 0) // text_string_id
	payload.WriteByte(0x10)       // word_flags (reserved bit 4)

	return payload.Bytes()
}

func writeTestUvarint(buf *bytes.Buffer, value uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], value)
	buf.Write(tmp[:n])
}
