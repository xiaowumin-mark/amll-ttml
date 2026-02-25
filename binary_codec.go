package ttml

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	// AMLX 二进制头与版本号。
	amlxMagic        = "AMLX"
	amlxVersion byte = 0x01
	// 以 uint64 正数区间作为时间上限，避免后续运算溢出。
	maxBinaryTimeMS = uint64(^uint64(0) >> 1)
)

const (
	// 行级标记位（bit flags）。
	lineFlagIsBG uint8 = 1 << iota
	lineFlagIsDuet
	lineFlagIgnoreSync
	lineFlagHasTranslatedLyric
	lineFlagHasRomanLyric
	// 已定义的合法行标记掩码。
	lineFlagMask = lineFlagIsBG | lineFlagIsDuet | lineFlagIgnoreSync | lineFlagHasTranslatedLyric | lineFlagHasRomanLyric
)

const (
	// 词级标记位（bit flags）。
	wordFlagObscene uint8 = 1 << iota
	wordFlagHasEmptyBeat
	wordFlagHasRomanWord
	wordFlagRomanWarning
	// 已定义的合法词标记掩码。
	wordFlagMask = wordFlagObscene | wordFlagHasEmptyBeat | wordFlagHasRomanWord | wordFlagRomanWarning
)

// stringPoolBuilder 用于构建字符串池，并为字符串分配稳定 ID。
type stringPoolBuilder struct {
	values []string
	index  map[string]uint64
}

func newStringPoolBuilder() *stringPoolBuilder {
	return &stringPoolBuilder{
		values: []string{},
		index:  map[string]uint64{},
	}
}

func (sp *stringPoolBuilder) add(value string) uint64 {
	// 已存在则复用 ID，保证字符串去重。
	if idx, ok := sp.index[value]; ok {
		return idx
	}
	idx := uint64(len(sp.values))
	sp.values = append(sp.values, value)
	sp.index[value] = idx
	return idx
}

func (sp *stringPoolBuilder) get(value string) (uint64, bool) {
	idx, ok := sp.index[value]
	return idx, ok
}

// TTMLToBinary 将 TTML XML 文本转换为 AMLX 二进制。
func TTMLToBinary(ttmlText string) ([]byte, error) {
	lyric, err := ParseLyric(ttmlText)
	if err != nil {
		return nil, err
	}
	return EncodeBinary(lyric)
}

// BinaryToTTML 将 AMLX 二进制转换为 TTML XML 文本。
func BinaryToTTML(binaryData []byte, pretty bool) (string, error) {
	lyric, err := DecodeBinary(binaryData)
	if err != nil {
		return "", err
	}
	return ExportTTMLText(lyric, pretty), nil
}

// EncodeBinary 将结构化歌词编码为 AMLX 二进制。
func EncodeBinary(ttmlLyric TTMLLyric) ([]byte, error) {
	// 先构建全局字符串池，后续段落通过 ID 引用字符串，减少体积。
	stringPool := buildStringPool(ttmlLyric)

	headerSection, err := encodeHeaderSection(ttmlLyric.Metadata, stringPool)
	if err != nil {
		return nil, err
	}

	stringPoolSection := encodeStringPoolSection(stringPool.values)

	lyricDataSection, err := encodeLyricDataSection(ttmlLyric.LyricLines, stringPool)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(amlxMagic)
	out.WriteByte(amlxVersion)
	out.WriteByte(0) // GlobalFlags（v1 暂未使用）
	writeUvarint(&out, uint64(headerSection.Len()))
	out.Write(headerSection.Bytes())
	out.Write(stringPoolSection.Bytes())
	out.Write(lyricDataSection.Bytes())

	return out.Bytes(), nil
}

// DecodeBinary 将 AMLX 二进制解码为结构化歌词。
func DecodeBinary(binaryData []byte) (TTMLLyric, error) {
	reader := bytes.NewReader(binaryData)

	// 读取并校验 magic，防止误解码非 AMLX 数据。
	magic := make([]byte, len(amlxMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return TTMLLyric{}, fmt.Errorf("read magic: %w", err)
	}
	if string(magic) != amlxMagic {
		return TTMLLyric{}, fmt.Errorf("invalid magic: %q", string(magic))
	}

	version, err := reader.ReadByte()
	if err != nil {
		return TTMLLyric{}, fmt.Errorf("read version: %w", err)
	}
	if version != amlxVersion {
		return TTMLLyric{}, fmt.Errorf("unsupported version: %d", version)
	}

	if _, err := reader.ReadByte(); err != nil {
		return TTMLLyric{}, fmt.Errorf("read global flags: %w", err)
	}

	// header 长度在主流中紧随固定头，先读出再单独解析。
	headerSize, err := readUvarint(reader)
	if err != nil {
		return TTMLLyric{}, fmt.Errorf("read header size: %w", err)
	}
	headerBytes, err := readBytes(reader, headerSize, "header section")
	if err != nil {
		return TTMLLyric{}, err
	}

	stringPool, err := decodeStringPoolSection(reader)
	if err != nil {
		return TTMLLyric{}, err
	}

	metadata, err := decodeHeaderSection(headerBytes, stringPool)
	if err != nil {
		return TTMLLyric{}, err
	}

	lines, err := decodeLyricDataSection(reader, stringPool)
	if err != nil {
		return TTMLLyric{}, err
	}

	return TTMLLyric{
		Metadata:   metadata,
		LyricLines: lines,
	}, nil
}

// EncodeAMLX 是 EncodeBinary 的别名。
func EncodeAMLX(ttmlLyric TTMLLyric) ([]byte, error) {
	return EncodeBinary(ttmlLyric)
}

// DecodeAMLX 是 DecodeBinary 的别名。
func DecodeAMLX(binaryData []byte) (TTMLLyric, error) {
	return DecodeBinary(binaryData)
}

// buildStringPool 遍历元数据与歌词正文，收集所有可复用字符串。
func buildStringPool(ttmlLyric TTMLLyric) *stringPoolBuilder {
	pool := newStringPoolBuilder() // 字符串池

	for _, meta := range ttmlLyric.Metadata {
		pool.add(meta.Key)
		for _, value := range meta.Value {
			pool.add(value)
		}
	}

	for _, line := range ttmlLyric.LyricLines {
		if line.TranslatedLyric != "" {
			pool.add(line.TranslatedLyric)
		}
		if line.RomanLyric != "" {
			pool.add(line.RomanLyric)
		}
		for _, word := range line.Words {
			pool.add(word.Word)
			if word.RomanWord != "" {
				pool.add(word.RomanWord)
			}
		}
	}

	return pool
}

// encodeHeaderSection 编码元数据段：key/value 均写入字符串池 ID。
func encodeHeaderSection(metadata []TTMLMetadata, stringPool *stringPoolBuilder) (*bytes.Buffer, error) {
	var section bytes.Buffer
	writeUvarint(&section, uint64(len(metadata)))

	for metaIndex, meta := range metadata {
		keyID, ok := stringPool.get(meta.Key)
		if !ok {
			return nil, fmt.Errorf("metadata[%d].key missing from string pool", metaIndex)
		}
		writeUvarint(&section, keyID)

		writeUvarint(&section, uint64(len(meta.Value)))
		for valueIndex, value := range meta.Value {
			valueID, ok := stringPool.get(value)
			if !ok {
				return nil, fmt.Errorf("metadata[%d].value[%d] missing from string pool", metaIndex, valueIndex)
			}
			writeUvarint(&section, valueID)
		}

		if meta.Error {
			section.WriteByte(1)
		} else {
			section.WriteByte(0)
		}
	}

	return &section, nil
}

// encodeStringPoolSection 编码字符串池段：先数量，再逐条写入长度与字节。
func encodeStringPoolSection(values []string) *bytes.Buffer {
	var section bytes.Buffer
	writeUvarint(&section, uint64(len(values)))
	for _, value := range values {
		raw := []byte(value)
		writeUvarint(&section, uint64(len(raw)))
		section.Write(raw)
	}
	return &section
}

// encodeLyricDataSection 编码歌词段，包含行信息与逐词时间/文本信息。
func encodeLyricDataSection(lines []LyricLine, stringPool *stringPoolBuilder) (*bytes.Buffer, error) {
	var section bytes.Buffer
	writeUvarint(&section, uint64(len(lines)))

	for lineIndex, line := range lines {
		lineStartMS, err := toMilliseconds(line.StartTime, fmt.Sprintf("line[%d].start_time", lineIndex))
		if err != nil {
			return nil, err
		}
		lineEndMS, err := toMilliseconds(line.EndTime, fmt.Sprintf("line[%d].end_time", lineIndex))
		if err != nil {
			return nil, err
		}

		type encodedWord struct {
			startMS      uint64
			endMS        uint64
			hasEmptyBeat bool
			emptyBeatMS  uint64
			hasRomanWord bool
			textID       uint64
			romanID      uint64
			wordFlags    uint8
		}
		encodedWords := make([]encodedWord, 0, len(line.Words))

		for wordIndex, word := range line.Words {
			wordStartMS, err := toMilliseconds(word.StartTime, fmt.Sprintf("line[%d].word[%d].start_time", lineIndex, wordIndex))
			if err != nil {
				return nil, err
			}
			wordEndMS, err := toMilliseconds(word.EndTime, fmt.Sprintf("line[%d].word[%d].end_time", lineIndex, wordIndex))
			if err != nil {
				return nil, err
			}
			if wordEndMS < wordStartMS {
				// 兼容旧数据：当词结束时间小于开始时间时，保留该词并将时长钳制为 0。
				wordEndMS = wordStartMS
			}

			if wordStartMS < lineStartMS {
				// 兼容旧数据：如果词比行更早开始，则向前扩展行起点。
				lineStartMS = wordStartMS
			}
			if wordEndMS > lineEndMS {
				// 词尾超出行尾时，向后扩展行终点。
				lineEndMS = wordEndMS
			}

			textID, ok := stringPool.get(word.Word)
			if !ok {
				return nil, fmt.Errorf("line[%d].word[%d].word missing from string pool", lineIndex, wordIndex)
			}

			hasRomanWord := word.RomanWord != ""

			var romanID uint64
			if hasRomanWord {
				var ok bool
				romanID, ok = stringPool.get(word.RomanWord)
				if !ok {
					return nil, fmt.Errorf("line[%d].word[%d].roman_word missing from string pool", lineIndex, wordIndex)
				}
			}

			hasEmptyBeat := false
			emptyBeatMS := uint64(0)
			// 仅接受有限且大于 0 的 emptyBeat。
			if !math.IsNaN(word.EmptyBeat) && !math.IsInf(word.EmptyBeat, 0) && word.EmptyBeat > 0 {
				parsedEmptyBeatMS, err := toMilliseconds(word.EmptyBeat, fmt.Sprintf("line[%d].word[%d].empty_beat", lineIndex, wordIndex))
				if err != nil {
					return nil, err
				}
				if parsedEmptyBeatMS > 0 {
					hasEmptyBeat = true
					emptyBeatMS = parsedEmptyBeatMS
				}
			}

			var wordFlags uint8
			if word.Obscene {
				wordFlags |= wordFlagObscene
			}
			if hasEmptyBeat {
				wordFlags |= wordFlagHasEmptyBeat
			}
			if hasRomanWord {
				wordFlags |= wordFlagHasRomanWord
			}
			if word.RomanWarning {
				wordFlags |= wordFlagRomanWarning
			}

			encodedWords = append(encodedWords, encodedWord{
				startMS:      wordStartMS,
				endMS:        wordEndMS,
				hasEmptyBeat: hasEmptyBeat,
				emptyBeatMS:  emptyBeatMS,
				hasRomanWord: hasRomanWord,
				textID:       textID,
				romanID:      romanID,
				wordFlags:    wordFlags,
			})
		}
		if lineEndMS < lineStartMS {
			lineEndMS = lineStartMS
		}

		writeUvarint(&section, lineStartMS)
		writeUvarint(&section, lineEndMS)

		hasTranslatedLyric := line.TranslatedLyric != ""
		hasRomanLyric := line.RomanLyric != ""

		var lineFlags uint8
		if line.IsBG {
			lineFlags |= lineFlagIsBG
		}
		if line.IsDuet {
			lineFlags |= lineFlagIsDuet
		}
		if line.IgnoreSync {
			lineFlags |= lineFlagIgnoreSync
		}
		if hasTranslatedLyric {
			lineFlags |= lineFlagHasTranslatedLyric
		}
		if hasRomanLyric {
			lineFlags |= lineFlagHasRomanLyric
		}
		section.WriteByte(lineFlags)

		writeUvarint(&section, uint64(len(line.Words)))

		if hasTranslatedLyric {
			translatedID, ok := stringPool.get(line.TranslatedLyric)
			if !ok {
				return nil, fmt.Errorf("line[%d].translated_lyric missing from string pool", lineIndex)
			}
			writeUvarint(&section, translatedID)
		}

		if hasRomanLyric {
			romanID, ok := stringPool.get(line.RomanLyric)
			if !ok {
				return nil, fmt.Errorf("line[%d].roman_lyric missing from string pool", lineIndex)
			}
			writeUvarint(&section, romanID)
		}

		for wordIndex := range encodedWords {
			word := encodedWords[wordIndex]
			// 单词起点按“相对行起点”的增量编码，减小 varint 体积。
			deltaStart := word.startMS - lineStartMS
			duration := word.endMS - word.startMS

			writeUvarint(&section, deltaStart)
			writeUvarint(&section, duration)
			writeUvarint(&section, word.textID)
			section.WriteByte(word.wordFlags)

			if word.hasRomanWord {
				writeUvarint(&section, word.romanID)
			}

			if word.hasEmptyBeat {
				writeUvarint(&section, word.emptyBeatMS)
			}
		}
	}

	return &section, nil
}

// decodeStringPoolSection 解码字符串池段。
func decodeStringPoolSection(reader *bytes.Reader) ([]string, error) {
	stringCountU64, err := readUvarint(reader)
	if err != nil {
		return nil, fmt.Errorf("read string_count: %w", err)
	}
	stringCount, err := toInt(stringCountU64, "string_count")
	if err != nil {
		return nil, err
	}

	stringPool := make([]string, 0, stringCount)
	for i := 0; i < stringCount; i++ {
		lengthU64, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read string[%d].length: %w", i, err)
		}
		raw, err := readBytes(reader, lengthU64, fmt.Sprintf("string[%d].bytes", i))
		if err != nil {
			return nil, err
		}
		stringPool = append(stringPool, string(raw))
	}

	return stringPool, nil
}

// decodeHeaderSection 解码头部段，并检查是否存在尾随垃圾字节。
func decodeHeaderSection(header []byte, stringPool []string) ([]TTMLMetadata, error) {
	reader := bytes.NewReader(header)

	metadataCountU64, err := readUvarint(reader)
	if err != nil {
		return nil, fmt.Errorf("read metadata_count: %w", err)
	}
	metadataCount, err := toInt(metadataCountU64, "metadata_count")
	if err != nil {
		return nil, err
	}

	metadata := make([]TTMLMetadata, 0, metadataCount)
	for metaIndex := 0; metaIndex < metadataCount; metaIndex++ {
		keyID, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read metadata[%d].key_string_id: %w", metaIndex, err)
		}
		key, err := stringByID(stringPool, keyID, fmt.Sprintf("metadata[%d].key_string_id", metaIndex))
		if err != nil {
			return nil, err
		}

		valueCountU64, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read metadata[%d].value_count: %w", metaIndex, err)
		}
		valueCount, err := toInt(valueCountU64, fmt.Sprintf("metadata[%d].value_count", metaIndex))
		if err != nil {
			return nil, err
		}

		values := make([]string, 0, valueCount)
		for valueIndex := 0; valueIndex < valueCount; valueIndex++ {
			valueID, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read metadata[%d].value[%d]_string_id: %w", metaIndex, valueIndex, err)
			}
			value, err := stringByID(stringPool, valueID, fmt.Sprintf("metadata[%d].value[%d]_string_id", metaIndex, valueIndex))
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}

		errorFlag, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read metadata[%d].error_flag: %w", metaIndex, err)
		}

		metadata = append(metadata, TTMLMetadata{
			Key:   key,
			Value: values,
			Error: errorFlag != 0,
		})
	}

	if reader.Len() != 0 {
		return nil, fmt.Errorf("header section has %d unexpected trailing bytes", reader.Len())
	}

	return metadata, nil
}

// decodeLyricDataSection 解码歌词段，并按标记位恢复可选字段。
func decodeLyricDataSection(reader *bytes.Reader, stringPool []string) ([]LyricLine, error) {
	lineCountU64, err := readUvarint(reader)
	if err != nil {
		return nil, fmt.Errorf("read line_count: %w", err)
	}
	lineCount, err := toInt(lineCountU64, "line_count")
	if err != nil {
		return nil, err
	}

	lines := make([]LyricLine, 0, lineCount)
	for lineIndex := 0; lineIndex < lineCount; lineIndex++ {
		lineStartMS, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read line[%d].start_time: %w", lineIndex, err)
		}
		if lineStartMS > maxBinaryTimeMS {
			return nil, fmt.Errorf("line[%d].start_time overflow", lineIndex)
		}

		lineEndMS, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read line[%d].end_time: %w", lineIndex, err)
		}
		if lineEndMS > maxBinaryTimeMS {
			return nil, fmt.Errorf("line[%d].end_time overflow", lineIndex)
		}
		if lineEndMS < lineStartMS {
			return nil, fmt.Errorf("line[%d] end_time < start_time", lineIndex)
		}

		lineFlags, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read line[%d].line_flags: %w", lineIndex, err)
		}
		if lineFlags&^lineFlagMask != 0 {
			// 显式拒绝未知保留位，防止把未来版本数据静默当作当前格式解析。
			return nil, fmt.Errorf("line[%d] reserved line flags are set: 0x%02x", lineIndex, lineFlags&^lineFlagMask)
		}

		wordCountU64, err := readUvarint(reader)
		if err != nil {
			return nil, fmt.Errorf("read line[%d].word_count: %w", lineIndex, err)
		}
		wordCount, err := toInt(wordCountU64, fmt.Sprintf("line[%d].word_count", lineIndex))
		if err != nil {
			return nil, err
		}

		line := NewLyricLine()
		line.StartTime = float64(lineStartMS)
		line.EndTime = float64(lineEndMS)
		line.IsBG = lineFlags&lineFlagIsBG != 0
		line.IsDuet = lineFlags&lineFlagIsDuet != 0
		line.IgnoreSync = lineFlags&lineFlagIgnoreSync != 0
		line.Words = make([]LyricWord, 0, wordCount)

		if lineFlags&lineFlagHasTranslatedLyric != 0 {
			translatedID, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read line[%d].translated_string_id: %w", lineIndex, err)
			}
			translated, err := stringByID(stringPool, translatedID, fmt.Sprintf("line[%d].translated_string_id", lineIndex))
			if err != nil {
				return nil, err
			}
			line.TranslatedLyric = translated
		}

		if lineFlags&lineFlagHasRomanLyric != 0 {
			romanID, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read line[%d].roman_string_id: %w", lineIndex, err)
			}
			roman, err := stringByID(stringPool, romanID, fmt.Sprintf("line[%d].roman_string_id", lineIndex))
			if err != nil {
				return nil, err
			}
			line.RomanLyric = roman
		}

		for wordIndex := 0; wordIndex < wordCount; wordIndex++ {
			deltaStart, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read line[%d].word[%d].delta_start_time: %w", lineIndex, wordIndex, err)
			}
			duration, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read line[%d].word[%d].duration: %w", lineIndex, wordIndex, err)
			}
			textID, err := readUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("read line[%d].word[%d].text_string_id: %w", lineIndex, wordIndex, err)
			}

			wordFlags, err := reader.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("read line[%d].word[%d].word_flags: %w", lineIndex, wordIndex, err)
			}
			if wordFlags&^wordFlagMask != 0 {
				// 词级保留位同样严格校验。
				return nil, fmt.Errorf("line[%d].word[%d] reserved word flags are set: 0x%02x", lineIndex, wordIndex, wordFlags&^wordFlagMask)
			}

			wordStartMS, err := safeAddMillis(lineStartMS, deltaStart, fmt.Sprintf("line[%d].word[%d].start_time", lineIndex, wordIndex))
			if err != nil {
				return nil, err
			}
			wordEndMS, err := safeAddMillis(wordStartMS, duration, fmt.Sprintf("line[%d].word[%d].end_time", lineIndex, wordIndex))
			if err != nil {
				return nil, err
			}

			wordText, err := stringByID(stringPool, textID, fmt.Sprintf("line[%d].word[%d].text_string_id", lineIndex, wordIndex))
			if err != nil {
				return nil, err
			}

			word := NewLyricWord()
			word.StartTime = float64(wordStartMS)
			word.EndTime = float64(wordEndMS)
			word.Word = wordText
			word.Obscene = wordFlags&wordFlagObscene != 0
			word.RomanWarning = wordFlags&wordFlagRomanWarning != 0

			if wordFlags&wordFlagHasRomanWord != 0 {
				romanID, err := readUvarint(reader)
				if err != nil {
					return nil, fmt.Errorf("read line[%d].word[%d].roman_string_id: %w", lineIndex, wordIndex, err)
				}
				romanWord, err := stringByID(stringPool, romanID, fmt.Sprintf("line[%d].word[%d].roman_string_id", lineIndex, wordIndex))
				if err != nil {
					return nil, err
				}
				word.RomanWord = romanWord
			}

			if wordFlags&wordFlagHasEmptyBeat != 0 {
				emptyBeatMS, err := readUvarint(reader)
				if err != nil {
					return nil, fmt.Errorf("read line[%d].word[%d].empty_beat_ms: %w", lineIndex, wordIndex, err)
				}
				if emptyBeatMS > maxBinaryTimeMS {
					return nil, fmt.Errorf("line[%d].word[%d].empty_beat_ms overflow", lineIndex, wordIndex)
				}
				word.EmptyBeat = float64(emptyBeatMS)
			}

			line.Words = append(line.Words, word)
		}

		lines = append(lines, line)
	}

	return lines, nil
}

// safeAddMillis 安全执行时间加法，避免无符号整数溢出。
func safeAddMillis(base uint64, delta uint64, field string) (uint64, error) {
	if base > maxBinaryTimeMS || delta > maxBinaryTimeMS {
		return 0, fmt.Errorf("%s overflow", field)
	}
	if base > maxBinaryTimeMS-delta {
		return 0, fmt.Errorf("%s overflow", field)
	}
	return base + delta, nil
}

// toMilliseconds 将浮点毫秒值规整为 uint64（四舍五入）。
func toMilliseconds(value float64, field string) (uint64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s must be a finite number", field)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be >= 0", field)
	}
	rounded := math.Round(value)
	if rounded > float64(maxBinaryTimeMS) {
		return 0, fmt.Errorf("%s overflow", field)
	}
	return uint64(rounded), nil
}

// stringByID 从字符串池按 ID 读取字符串并做越界检查。
func stringByID(stringPool []string, id uint64, field string) (string, error) {
	if id >= uint64(len(stringPool)) {
		return "", fmt.Errorf("%s out of bounds: %d (pool size %d)", field, id, len(stringPool))
	}
	return stringPool[id], nil
}

// writeUvarint 以无符号 varint 写入整数。
func writeUvarint(buf *bytes.Buffer, value uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], value)
	buf.Write(tmp[:n])
}

// readUvarint 读取无符号 varint，并把 EOF 统一为 UnexpectedEOF。
func readUvarint(reader *bytes.Reader) (uint64, error) {
	value, err := binary.ReadUvarint(reader)
	if err == nil {
		return value, nil
	}
	if errors.Is(err, io.EOF) {
		return 0, io.ErrUnexpectedEOF
	}
	return 0, err
}

// readBytes 从 reader 读取定长字节切片，并保证不会超过剩余长度。
func readBytes(reader *bytes.Reader, length uint64, field string) ([]byte, error) {
	if length > uint64(reader.Len()) {
		return nil, fmt.Errorf("%s exceeds remaining bytes", field)
	}
	n, err := toInt(length, field)
	if err != nil {
		return nil, err
	}
	raw := make([]byte, n)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return nil, fmt.Errorf("read %s: %w", field, err)
	}
	return raw, nil
}

// toInt 将 uint64 安全转换为 int，防止平台相关溢出。
func toInt(value uint64, field string) (int, error) {
	maxInt := uint64(^uint(0) >> 1)
	if value > maxInt {
		return 0, fmt.Errorf("%s is too large", field)
	}
	return int(value), nil
}
