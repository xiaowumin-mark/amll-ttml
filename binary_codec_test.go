package ttml

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEncodeDecodeBinaryRoundTrip(t *testing.T) {
	// 覆盖完整字段组合，验证结构化对象在二进制往返后保持一致。
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
	// 验证 TTML 文本桥接接口与底层二进制编解码结果一致。
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
	// 无效载荷应被严格拒绝，避免静默容错导致脏数据进入系统。
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
	// 兼容历史数据：行时间包络应自动覆盖所有单词。
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
						StartTime:    900, // 早于行起点（历史数据不一致场景）
						EndTime:      1100,
						Word:         "w1",
						Obscene:      true,
						RomanWord:    "rw1",
						RomanWarning: true,
						EmptyBeat:    0,
					},
					{
						StartTime: 1100,
						EndTime:   1500, // 晚于行终点
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

	// 为兼容旧数据，行时间会被归一化为覆盖全部单词的包络。
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
	// 非法 emptyBeat（NaN/Inf/<=0）应被编码层忽略。
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

func TestEncodeBinarySectionDiagnostics(t *testing.T) {
	/*diagnosticSample := TTMLLyric{
		Metadata: []TTMLMetadata{
			{
				Key:   "album",
				Value: []string{"1989", "Deluxe"},
			},
			{
				Key:   "source",
				Value: []string{"itunes"},
				Error: true,
			},
		},
		LyricLines: []LyricLine{
			{
				StartTime:       1000,
				EndTime:         2200,
				IsDuet:          true,
				IgnoreSync:      true,
				TranslatedLyric: "welcome-cn",
				RomanLyric:      "huan ying lai dao niu yue",
				Words: []LyricWord{
					{
						StartTime:    1000,
						EndTime:      1400,
						Word:         "Wel",
						Obscene:      true,
						RomanWord:    "wel",
						RomanWarning: true,
					},
					{
						StartTime: 1400,
						EndTime:   2200,
						Word:      "come",
						EmptyBeat: 120,
					},
				},
			},
			{
				StartTime: 2300,
				EndTime:   2600,
				IsBG:      true,
				Words: []LyricWord{
					{
						StartTime: 2300,
						EndTime:   2600,
						Word:      "(New York)",
					},
				},
			},
		},
	}*/
	// 解析/test/raw-ttml/1689089845000-39523898-31c2fa0c.ttml
	file, err := os.Open("./test/raw-ttml/1689089845000-39523898-31c2fa0c.ttml")
	if err != nil {
		t.Fatalf("open file failed: %v", err)
	}
	text, err := ioutil.ReadAll(file)
	if err != nil {

		t.Fatalf("read file failed: %v", err)
	}

	diagnosticSample, err := ParseLyric(string(text))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	encoded, err := EncodeBinary(diagnosticSample)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	reader := bytes.NewReader(encoded)
	magic, err := readBytes(reader, uint64(len(amlxMagic)), "magic")
	if err != nil {
		t.Fatalf("read magic failed: %v", err)
	}
	version, _, err := readTestByteWithSize(reader, "version")
	if err != nil {
		t.Fatalf("read version failed: %v", err)
	}
	globalFlags, _, err := readTestByteWithSize(reader, "global_flags")
	if err != nil {
		t.Fatalf("read global_flags failed: %v", err)
	}

	headerSize, headerSizeVarintBytes, err := readTestUvarintWithSize(reader, "header_size")
	if err != nil {
		t.Fatalf("read header_size failed: %v", err)
	}
	headerBytes, err := readBytes(reader, headerSize, "header_section")
	if err != nil {
		t.Fatalf("read header_section failed: %v", err)
	}

	t.Logf("container: total=%dB magic=%q version=0x%02x global_flags=0x%02x", len(encoded), string(magic), version, globalFlags)

	headerReader := bytes.NewReader(headerBytes)
	metadataCount, metadataCountVarintBytes, err := readTestUvarintWithSize(headerReader, "metadata_count")
	if err != nil {
		t.Fatalf("read metadata_count failed: %v", err)
	}
	t.Logf("header section: size=%dB metadata_count=%d(%dB)", len(headerBytes), metadataCount, metadataCountVarintBytes)

	for metaIndex := uint64(0); metaIndex < metadataCount; metaIndex++ {
		entryStart := headerReader.Len()

		keyID, keyIDBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].key_id", metaIndex))
		if err != nil {
			t.Fatalf("read metadata[%d].key_id failed: %v", metaIndex, err)
		}
		valueCount, valueCountBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].value_count", metaIndex))
		if err != nil {
			t.Fatalf("read metadata[%d].value_count failed: %v", metaIndex, err)
		}

		valueIDs := make([]uint64, 0, valueCount)
		valueIDVarintBytes := make([]int, 0, valueCount)
		for valueIndex := uint64(0); valueIndex < valueCount; valueIndex++ {
			valueID, valueBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].value[%d]", metaIndex, valueIndex))
			if err != nil {
				t.Fatalf("read metadata[%d].value[%d] failed: %v", metaIndex, valueIndex, err)
			}
			valueIDs = append(valueIDs, valueID)
			valueIDVarintBytes = append(valueIDVarintBytes, valueBytes)
		}

		errorFlag, errorFlagBytes, err := readTestByteWithSize(headerReader, fmt.Sprintf("metadata[%d].error_flag", metaIndex))
		if err != nil {
			t.Fatalf("read metadata[%d].error_flag failed: %v", metaIndex, err)
		}

		entryBytes := entryStart - headerReader.Len()
		t.Logf(
			"  metadata[%d]: size=%dB key_id=%d(%dB) value_count=%d(%dB) value_ids=%v(value_varint_bytes=%v) error=%t(%dB)",
			metaIndex,
			entryBytes,
			keyID,
			keyIDBytes,
			valueCount,
			valueCountBytes,
			valueIDs,
			valueIDVarintBytes,
			errorFlag != 0,
			errorFlagBytes,
		)
	}
	if headerReader.Len() != 0 {
		t.Fatalf("header section has unexpected trailing bytes: %d", headerReader.Len())
	}

	stringPoolSectionStart := reader.Len()
	stringCount, stringCountVarintBytes, err := readTestUvarintWithSize(reader, "string_count")
	if err != nil {
		t.Fatalf("read string_count failed: %v", err)
	}
	t.Logf("string_pool: string_count=%d(%dB)", stringCount, stringCountVarintBytes)

	for stringIndex := uint64(0); stringIndex < stringCount; stringIndex++ {
		entryStart := reader.Len()
		stringLen, stringLenVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("string[%d].length", stringIndex))
		if err != nil {
			t.Fatalf("read string[%d].length failed: %v", stringIndex, err)
		}
		raw, err := readBytes(reader, stringLen, fmt.Sprintf("string[%d].bytes", stringIndex))
		if err != nil {
			t.Fatalf("read string[%d].bytes failed: %v", stringIndex, err)
		}
		entryBytes := entryStart - reader.Len()
		t.Logf(
			"  string[%d]: size=%dB len=%d(%dB) value=%q",
			stringIndex,
			entryBytes,
			stringLen,
			stringLenVarintBytes,
			string(raw),
		)
	}
	stringPoolSectionBytes := stringPoolSectionStart - reader.Len()
	t.Logf("string_pool section size=%dB", stringPoolSectionBytes)

	lyricDataSectionStart := reader.Len()
	lineCount, lineCountVarintBytes, err := readTestUvarintWithSize(reader, "line_count")
	if err != nil {
		t.Fatalf("read line_count failed: %v", err)
	}
	t.Logf("lyric_data: line_count=%d(%dB)", lineCount, lineCountVarintBytes)

	for lineIndex := uint64(0); lineIndex < lineCount; lineIndex++ {
		lineStart := reader.Len()
		lineStartMS, lineStartVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].start_time", lineIndex))
		if err != nil {
			t.Fatalf("read line[%d].start_time failed: %v", lineIndex, err)
		}
		lineEndMS, lineEndVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].end_time", lineIndex))
		if err != nil {
			t.Fatalf("read line[%d].end_time failed: %v", lineIndex, err)
		}
		lineFlags, lineFlagsBytes, err := readTestByteWithSize(reader, fmt.Sprintf("line[%d].flags", lineIndex))
		if err != nil {
			t.Fatalf("read line[%d].flags failed: %v", lineIndex, err)
		}
		wordCount, wordCountVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word_count", lineIndex))
		if err != nil {
			t.Fatalf("read line[%d].word_count failed: %v", lineIndex, err)
		}

		optionalLineFields := []string{}
		if lineFlags&lineFlagHasTranslatedLyric != 0 {
			translatedID, translatedBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].translated_id", lineIndex))
			if err != nil {
				t.Fatalf("read line[%d].translated_id failed: %v", lineIndex, err)
			}
			optionalLineFields = append(optionalLineFields, fmt.Sprintf("translated_id=%d(%dB)", translatedID, translatedBytes))
		}
		if lineFlags&lineFlagHasRomanLyric != 0 {
			romanID, romanBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].roman_id", lineIndex))
			if err != nil {
				t.Fatalf("read line[%d].roman_id failed: %v", lineIndex, err)
			}
			optionalLineFields = append(optionalLineFields, fmt.Sprintf("roman_id=%d(%dB)", romanID, romanBytes))
		}
		if len(optionalLineFields) == 0 {
			optionalLineFields = append(optionalLineFields, "none")
		}

		t.Logf(
			"  line[%d]: start=%d(%dB) end=%d(%dB) flags=0x%02x[%s](%dB) word_count=%d(%dB) optional=%s",
			lineIndex,
			lineStartMS,
			lineStartVarintBytes,
			lineEndMS,
			lineEndVarintBytes,
			lineFlags,
			formatLineFlagsForTest(lineFlags),
			lineFlagsBytes,
			wordCount,
			wordCountVarintBytes,
			strings.Join(optionalLineFields, ", "),
		)

		for wordIndex := uint64(0); wordIndex < wordCount; wordIndex++ {
			wordStart := reader.Len()
			deltaStart, deltaStartBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].delta_start", lineIndex, wordIndex))
			if err != nil {
				t.Fatalf("read line[%d].word[%d].delta_start failed: %v", lineIndex, wordIndex, err)
			}
			duration, durationBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].duration", lineIndex, wordIndex))
			if err != nil {
				t.Fatalf("read line[%d].word[%d].duration failed: %v", lineIndex, wordIndex, err)
			}
			textID, textIDBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].text_id", lineIndex, wordIndex))
			if err != nil {
				t.Fatalf("read line[%d].word[%d].text_id failed: %v", lineIndex, wordIndex, err)
			}
			wordFlags, wordFlagsBytes, err := readTestByteWithSize(reader, fmt.Sprintf("line[%d].word[%d].flags", lineIndex, wordIndex))
			if err != nil {
				t.Fatalf("read line[%d].word[%d].flags failed: %v", lineIndex, wordIndex, err)
			}

			optionalWordFields := []string{}
			if wordFlags&wordFlagHasRomanWord != 0 {
				romanID, romanBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].roman_id", lineIndex, wordIndex))
				if err != nil {
					t.Fatalf("read line[%d].word[%d].roman_id failed: %v", lineIndex, wordIndex, err)
				}
				optionalWordFields = append(optionalWordFields, fmt.Sprintf("roman_id=%d(%dB)", romanID, romanBytes))
			}
			if wordFlags&wordFlagHasEmptyBeat != 0 {
				emptyBeatMS, emptyBeatBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].empty_beat", lineIndex, wordIndex))
				if err != nil {
					t.Fatalf("read line[%d].word[%d].empty_beat failed: %v", lineIndex, wordIndex, err)
				}
				optionalWordFields = append(optionalWordFields, fmt.Sprintf("empty_beat_ms=%d(%dB)", emptyBeatMS, emptyBeatBytes))
			}
			if len(optionalWordFields) == 0 {
				optionalWordFields = append(optionalWordFields, "none")
			}

			wordBytes := wordStart - reader.Len()
			t.Logf(
				"    word[%d]: size=%dB delta_start=%d(%dB) duration=%d(%dB) text_id=%d(%dB) flags=0x%02x[%s](%dB) optional=%s",
				wordIndex,
				wordBytes,
				deltaStart,
				deltaStartBytes,
				duration,
				durationBytes,
				textID,
				textIDBytes,
				wordFlags,
				formatWordFlagsForTest(wordFlags),
				wordFlagsBytes,
				strings.Join(optionalWordFields, ", "),
			)
		}

		lineBytes := lineStart - reader.Len()
		t.Logf("  line[%d] total size=%dB", lineIndex, lineBytes)
	}

	lyricDataSectionBytes := lyricDataSectionStart - reader.Len()
	if reader.Len() != 0 {
		t.Fatalf("payload has unexpected trailing bytes: %d", reader.Len())
	}

	fixedHeaderBytes := len(amlxMagic) + 1 + 1
	totalFromSections := fixedHeaderBytes + headerSizeVarintBytes + len(headerBytes) + stringPoolSectionBytes + lyricDataSectionBytes
	if totalFromSections != len(encoded) {
		t.Fatalf(
			"section size mismatch: total=%d computed=%d (fixed=%d header_size_varint=%d header=%d string_pool=%d lyric=%d)",
			len(encoded),
			totalFromSections,
			fixedHeaderBytes,
			headerSizeVarintBytes,
			len(headerBytes),
			stringPoolSectionBytes,
			lyricDataSectionBytes,
		)
	}

	totalFloat := float64(len(encoded))
	t.Logf(
		"size summary: total=%dB fixed=%dB header_size_varint=%dB header=%dB string_pool=%dB lyric_data=%dB",
		len(encoded),
		fixedHeaderBytes,
		headerSizeVarintBytes,
		len(headerBytes),
		stringPoolSectionBytes,
		lyricDataSectionBytes,
	)
	t.Logf(
		"size ratio: header=%.2f%% string_pool=%.2f%% lyric_data=%.2f%%",
		float64(len(headerBytes))*100/totalFloat,
		float64(stringPoolSectionBytes)*100/totalFloat,
		float64(lyricDataSectionBytes)*100/totalFloat,
	)
}

func normalizeLyricForCompare(lyric TTMLLyric) TTMLLyric {
	// 比较时忽略运行期生成 ID，避免非功能差异导致误报。
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
	writeTestUvarint(&header, 1) // key_string_id（越界）
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
	payload.WriteByte(0x20)       // line_flags（保留位 bit 5）
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
	payload.WriteByte(0x10)       // word_flags（保留位 bit 4）

	return payload.Bytes()
}

func writeTestUvarint(buf *bytes.Buffer, value uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], value)
	buf.Write(tmp[:n])
}

func readTestUvarintWithSize(reader *bytes.Reader, field string) (uint64, int, error) {
	before := reader.Len()
	value, err := readUvarint(reader)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", field, err)
	}
	return value, before - reader.Len(), nil
}

func readTestByteWithSize(reader *bytes.Reader, field string) (byte, int, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", field, err)
	}
	return value, 1, nil
}

func formatLineFlagsForTest(flags uint8) string {
	names := make([]string, 0, 5)
	if flags&lineFlagIsBG != 0 {
		names = append(names, "is_bg")
	}
	if flags&lineFlagIsDuet != 0 {
		names = append(names, "is_duet")
	}
	if flags&lineFlagIgnoreSync != 0 {
		names = append(names, "ignore_sync")
	}
	if flags&lineFlagHasTranslatedLyric != 0 {
		names = append(names, "has_translated")
	}
	if flags&lineFlagHasRomanLyric != 0 {
		names = append(names, "has_roman")
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "|")
}

func formatWordFlagsForTest(flags uint8) string {
	names := make([]string, 0, 4)
	if flags&wordFlagObscene != 0 {
		names = append(names, "obscene")
	}
	if flags&wordFlagHasEmptyBeat != 0 {
		names = append(names, "has_empty_beat")
	}
	if flags&wordFlagHasRomanWord != 0 {
		names = append(names, "has_roman")
	}
	if flags&wordFlagRomanWarning != 0 {
		names = append(names, "roman_warning")
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "|")
}
