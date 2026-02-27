// 一个简单的ttml解析，生成，生成二进制，解码二进制的tool
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	ttml "github.com/xiaowumin-mark/amll-ttml"
)

const (
	// AMLX 二进制头与版本号。
	amlxMagic = "AMLX"
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

var fileData []byte
var filetype string
var isDetail bool // 详情
var fp string
var outputType string

func main() {
	var rootCmd = &cobra.Command{
		Use:   "app",
		Short: "为amll-ttml提供的工具，支持ttml解析，生成，生成二进制，解码二进制，查看ttml结构和二进制结构",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("请选择命令")
			if fp == "" {
				fmt.Println("请输入ttml文件或者二进制文件路径")
				return
			}
			var err error
			// 判断文件类型
			if filepathExt := strings.ToLower(filepath.Ext(fp)); filepathExt == ".ttml" {
				// ttml文件
				fmt.Println("输入的ttml文件")
				filetype = "ttml"
				fileData, err = os.ReadFile(fp)
				if err != nil {
					fmt.Println("读取文件失败")
					return
				}
				tm, err := ttml.ParseLyric(string(fileData))
				if err != nil {
					fmt.Println("解析ttml文件失败")
					return
				}
				if isDetail {
					detailTTML(tm)
				}

			} else if filepathExt == ".amlx" {
				// 二进制文件
				fmt.Println("输入的二进制文件")
				filetype = "binary"
				fileData, err = os.ReadFile(fp)
				if err != nil {
					fmt.Println("读取文件失败")
					return
				}
				tm, err := ttml.DecodeBinary(fileData)
				if err != nil {
					fmt.Println("解析二进制文件失败")
					return
				}
				if isDetail {
					detailTTMLBinary(tm)
				}
			} else {
				fmt.Println("请输入ttml文件或者二进制文件路径")
				return
			}

			if outputType != "" {
				//去除后缀名
				filepathExt := filepath.Ext(fp)
				fileName := strings.TrimSuffix(fp, filepathExt)
				if outputType == "ttml" || outputType == "t" {
					fmt.Println("输出ttml文件")
					if filetype == "ttml" {
						fmt.Println("当前文件不需要转换，因为已经是ttml")

					} else {
						tm, err := ttml.DecodeAMLX(fileData)
						if err != nil {
							fmt.Println("解析amlx文件失败")
							return
						}
						exported := ttml.ExportTTMLText(tm, false)
						err = os.WriteFile(fileName+".ttml", []byte(exported), 0644)
						if err != nil {
							fmt.Println("写入文件失败")
							return
						}
						fmt.Println("输出成功")

					}
				} else if outputType == "amlx" || outputType == "a" {
					fmt.Println("输出二进制文件")
					if filetype == "binary" {
						fmt.Println("当前文件不需要转换，因为已经是二进制")

					} else {
						tm, err := ttml.ParseLyric(string(fileData))
						if err != nil {
							fmt.Println("解析ttml文件失败")
							return
						}
						encoded, err := ttml.EncodeBinary(tm)
						if err != nil {
							fmt.Println("编码失败")
							return
						}
						err = os.WriteFile(fileName+".amlx", encoded, 0644)
						if err != nil {
							fmt.Println("写入文件失败")
							return
						}
						fmt.Println("输出成功")
					}
				} else if outputType == "json" || outputType == "j" {
					fmt.Println("输出json文件")
					var err error
					var tm ttml.TTMLLyric
					if filetype == "ttml" {
						tm, err = ttml.ParseLyric(string(fileData))
						if err != nil {
							fmt.Println("解析ttml文件失败")
							return
						}

					} else {
						tm, err = ttml.DecodeBinary(fileData)
						if err != nil {
							fmt.Println("解析amlx文件失败")
							return
						}
					}
					// 转换为json
					j, err := json.MarshalIndent(tm, "", "  ")
					if err != nil {
						fmt.Println("转换json失败")
						return
					}
					err = os.WriteFile(fileName+".json", j, 0644)

				}
			}
		},
	}

	rootCmd.Flags().StringVarP(&fp, "input", "i", "", "输入文件")
	rootCmd.Flags().StringVarP(&outputType, "to", "t", "", "输出类型")
	rootCmd.Flags().BoolVarP(&isDetail, "detail", "d", false, "输出详细信息")

	rootCmd.Execute()
}

func detailTTML(tm ttml.TTMLLyric) {
	// 输出metadata
	fmt.Println("---------- METADATA ----------")
	for _, metadata := range tm.Metadata {
		fmt.Printf("|%s: %s\n", metadata.Key, metadata.Value)
	}
	// 输出lines
	fmt.Println("---------- LINES ----------")
	for _, line := range tm.LyricLines {
		fmt.Printf("|%s: %s\n", "LineID", line.ID)
		fmt.Printf("|%s: %s\n", "Translated", line.TranslatedLyric)
		fmt.Printf("|%s: %s\n", "Roman", line.RomanLyric)
		fmt.Printf("|%s: %s\n", "StartTime", colorText(ttml.MsToTimestamp(line.StartTime), color.FgYellow))
		fmt.Printf("|%s: %s\n", "EndTime", colorText(ttml.MsToTimestamp(line.EndTime), color.FgYellow))
		fmt.Printf("|%s: %t\n", "ISBG", line.IsBG)
		fmt.Printf("|%s: %t\n", "IsDuet", line.IsDuet)
		fmt.Println("|  |---------- WORDS ----------")
		for _, word := range line.Words {
			fmt.Printf("|  |%s: %s\n", "Word", colorText(word.Word, color.FgHiGreen))
			fmt.Printf("|  |%s: %s\n", "RomanWord", word.RomanWord)
			fmt.Printf("|  |%s: %s\n", "StartTime", colorText(ttml.MsToTimestamp(word.StartTime), color.FgYellow))
			fmt.Printf("|  |%s: %s\n", "EndTime", colorText(ttml.MsToTimestamp(word.EndTime), color.FgYellow))

			fmt.Println("|  |----------")
		}
		fmt.Println("|---------- END ----------")
	}
}
func detailTTMLBinary(tm ttml.TTMLLyric) {
	encoded, err := ttml.EncodeBinary(tm)
	if err != nil {
		fmt.Printf("encode failed: %v\n", err)
	}

	reader := bytes.NewReader(encoded)
	magic, err := readBytes(reader, uint64(len(amlxMagic)), "magic")
	if err != nil {
		fmt.Printf("read magic failed: %v\n", err)
	}
	version, _, err := readTestByteWithSize(reader, "version")
	if err != nil {
		fmt.Printf("read version failed: %v\n", err)
	}
	globalFlags, _, err := readTestByteWithSize(reader, "global_flags")
	if err != nil {
		fmt.Printf("read global_flags failed: %v\n", err)
	}

	headerSize, headerSizeVarintBytes, err := readTestUvarintWithSize(reader, "header_size")
	if err != nil {
		fmt.Printf("read header_size failed: %v\n", err)
	}
	headerBytes, err := readBytes(reader, headerSize, "header_section")
	if err != nil {
		fmt.Printf("read header_section failed: %v\n", err)
	}

	fmt.Printf("container: total=%dB magic=%q version=0x%02x global_flags=0x%02x\n", len(encoded), string(magic), version, globalFlags)

	headerReader := bytes.NewReader(headerBytes)
	metadataCount, metadataCountVarintBytes, err := readTestUvarintWithSize(headerReader, "metadata_count")
	if err != nil {
		fmt.Printf("read metadata_count failed: %v\n", err)
	}
	fmt.Printf("header section: size=%dB metadata_count=%d(%dB)\n", len(headerBytes), metadataCount, metadataCountVarintBytes)

	for metaIndex := uint64(0); metaIndex < metadataCount; metaIndex++ {
		entryStart := headerReader.Len()

		keyID, keyIDBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].key_id", metaIndex))
		if err != nil {
			fmt.Printf("read metadata[%d].key_id failed: %v\n", metaIndex, err)
		}
		valueCount, valueCountBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].value_count", metaIndex))
		if err != nil {
			fmt.Printf("read metadata[%d].value_count failed: %v\n", metaIndex, err)
		}

		valueIDs := make([]uint64, 0, valueCount)
		valueIDVarintBytes := make([]int, 0, valueCount)
		for valueIndex := uint64(0); valueIndex < valueCount; valueIndex++ {
			valueID, valueBytes, err := readTestUvarintWithSize(headerReader, fmt.Sprintf("metadata[%d].value[%d]", metaIndex, valueIndex))
			if err != nil {
				fmt.Printf("read metadata[%d].value[%d] failed: %v\n", metaIndex, valueIndex, err)
			}
			valueIDs = append(valueIDs, valueID)
			valueIDVarintBytes = append(valueIDVarintBytes, valueBytes)
		}

		errorFlag, errorFlagBytes, err := readTestByteWithSize(headerReader, fmt.Sprintf("metadata[%d].error_flag", metaIndex))
		if err != nil {
			fmt.Printf("read metadata[%d].error_flag failed: %v\n", metaIndex, err)
		}

		entryBytes := entryStart - headerReader.Len()
		fmt.Printf(
			"  metadata[%d]: size=%dB key_id=%d(%dB) value_count=%d(%dB) value_ids=%v(value_varint_bytes=%v) error=%t(%dB)\n",
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
		fmt.Printf("header section has unexpected trailing bytes: %d\n", headerReader.Len())
	}

	stringPoolSectionStart := reader.Len()
	stringCount, stringCountVarintBytes, err := readTestUvarintWithSize(reader, "string_count")
	if err != nil {
		fmt.Printf("read string_count failed: %v\n", err)
	}
	fmt.Printf("string_pool: string_count=%d(%dB)\n", stringCount, stringCountVarintBytes)

	for stringIndex := uint64(0); stringIndex < stringCount; stringIndex++ {
		entryStart := reader.Len()
		stringLen, stringLenVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("string[%d].length", stringIndex))
		if err != nil {
			fmt.Printf("read string[%d].length failed: %v\n", stringIndex, err)
		}
		raw, err := readBytes(reader, stringLen, fmt.Sprintf("string[%d].bytes", stringIndex))
		if err != nil {
			fmt.Printf("read string[%d].bytes failed: %v\n", stringIndex, err)
		}
		entryBytes := entryStart - reader.Len()
		fmt.Printf(
			"  string[%d]: size=%dB len=%d(%dB) value=%q\n",
			stringIndex,
			entryBytes,
			stringLen,
			stringLenVarintBytes,
			string(raw),
		)
	}
	stringPoolSectionBytes := stringPoolSectionStart - reader.Len()
	fmt.Printf("string_pool section size=%dB\n", stringPoolSectionBytes)

	lyricDataSectionStart := reader.Len()
	lineCount, lineCountVarintBytes, err := readTestUvarintWithSize(reader, "line_count")
	if err != nil {
		fmt.Printf("read line_count failed: %v\n", err)
	}
	fmt.Printf("lyric_data: line_count=%d(%dB)\n", lineCount, lineCountVarintBytes)

	for lineIndex := uint64(0); lineIndex < lineCount; lineIndex++ {
		lineStart := reader.Len()
		lineStartMS, lineStartVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].start_time", lineIndex))
		if err != nil {
			fmt.Printf("read line[%d].start_time failed: %v\n", lineIndex, err)
		}
		lineEndMS, lineEndVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].end_time", lineIndex))
		if err != nil {
			fmt.Printf("read line[%d].end_time failed: %v\n", lineIndex, err)
		}
		lineFlags, lineFlagsBytes, err := readTestByteWithSize(reader, fmt.Sprintf("line[%d].flags", lineIndex))
		if err != nil {
			fmt.Printf("read line[%d].flags failed: %v\n", lineIndex, err)
		}
		wordCount, wordCountVarintBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word_count", lineIndex))
		if err != nil {
			fmt.Printf("read line[%d].word_count failed: %v\n", lineIndex, err)
		}

		optionalLineFields := []string{}
		if lineFlags&lineFlagHasTranslatedLyric != 0 {
			translatedID, translatedBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].translated_id", lineIndex))
			if err != nil {
				fmt.Printf("read line[%d].translated_id failed: %v\n", lineIndex, err)
			}
			optionalLineFields = append(optionalLineFields, fmt.Sprintf("translated_id=%d(%dB)", translatedID, translatedBytes))
		}
		if lineFlags&lineFlagHasRomanLyric != 0 {
			romanID, romanBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].roman_id", lineIndex))
			if err != nil {
				fmt.Printf("read line[%d].roman_id failed: %v\n", lineIndex, err)
			}
			optionalLineFields = append(optionalLineFields, fmt.Sprintf("roman_id=%d(%dB)", romanID, romanBytes))
		}
		if len(optionalLineFields) == 0 {
			optionalLineFields = append(optionalLineFields, "none")
		}

		fmt.Printf(
			"  line[%d]: start=%d(%dB) end=%d(%dB) flags=0x%02x[%s](%dB) word_count=%d(%dB) optional=%s\n",
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
				fmt.Printf("read line[%d].word[%d].delta_start failed: %v\n", lineIndex, wordIndex, err)
			}
			duration, durationBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].duration", lineIndex, wordIndex))
			if err != nil {
				fmt.Printf("read line[%d].word[%d].duration failed: %v\n", lineIndex, wordIndex, err)
			}
			textID, textIDBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].text_id", lineIndex, wordIndex))
			if err != nil {
				fmt.Printf("read line[%d].word[%d].text_id failed: %v\n", lineIndex, wordIndex, err)
			}
			wordFlags, wordFlagsBytes, err := readTestByteWithSize(reader, fmt.Sprintf("line[%d].word[%d].flags", lineIndex, wordIndex))
			if err != nil {
				fmt.Printf("read line[%d].word[%d].flags failed: %v\n", lineIndex, wordIndex, err)
			}

			optionalWordFields := []string{}
			if wordFlags&wordFlagHasRomanWord != 0 {
				romanID, romanBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].roman_id", lineIndex, wordIndex))
				if err != nil {
					fmt.Printf("read line[%d].word[%d].roman_id failed: %v\n", lineIndex, wordIndex, err)
				}
				optionalWordFields = append(optionalWordFields, fmt.Sprintf("roman_id=%d(%dB)", romanID, romanBytes))
			}
			if wordFlags&wordFlagHasEmptyBeat != 0 {
				emptyBeatMS, emptyBeatBytes, err := readTestUvarintWithSize(reader, fmt.Sprintf("line[%d].word[%d].empty_beat", lineIndex, wordIndex))
				if err != nil {
					fmt.Printf("read line[%d].word[%d].empty_beat failed: %v\n", lineIndex, wordIndex, err)
				}
				optionalWordFields = append(optionalWordFields, fmt.Sprintf("empty_beat_ms=%d(%dB)", emptyBeatMS, emptyBeatBytes))
			}
			if len(optionalWordFields) == 0 {
				optionalWordFields = append(optionalWordFields, "none")
			}

			wordBytes := wordStart - reader.Len()
			fmt.Printf(
				"    word[%d]: size=%dB delta_start=%d(%dB) duration=%d(%dB) text_id=%d(%dB) flags=0x%02x[%s](%dB) optional=%s\n",
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
		fmt.Printf("  line[%d] total size=%dB\n", lineIndex, lineBytes)
	}

	lyricDataSectionBytes := lyricDataSectionStart - reader.Len()
	if reader.Len() != 0 {
		fmt.Printf("payload has unexpected trailing bytes: %d\n", reader.Len())
	}

	fixedHeaderBytes := len(amlxMagic) + 1 + 1
	totalFromSections := fixedHeaderBytes + headerSizeVarintBytes + len(headerBytes) + stringPoolSectionBytes + lyricDataSectionBytes
	if totalFromSections != len(encoded) {
		fmt.Printf(
			"section size mismatch: total=%d computed=%d (fixed=%d header_size_varint=%d header=%d string_pool=%d lyric=%d)\n",
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
	fmt.Printf(
		"size summary: total=%dB fixed=%dB header_size_varint=%dB header=%dB string_pool=%dB lyric_data=%dB\n",
		len(encoded),
		fixedHeaderBytes,
		headerSizeVarintBytes,
		len(headerBytes),
		stringPoolSectionBytes,
		lyricDataSectionBytes,
	)
	fmt.Printf(
		"size ratio: header=%.2f%% string_pool=%.2f%% lyric_data=%.2f%%\n",
		float64(len(headerBytes))*100/totalFloat,
		float64(stringPoolSectionBytes)*100/totalFloat,
		float64(lyricDataSectionBytes)*100/totalFloat,
	)
}
func colorText(text string, c color.Attribute) string { // 返回带有颜色的文本
	return color.New(c).SprintFunc()(text)
}

func readTestUvarintWithSize(reader *bytes.Reader, field string) (uint64, int, error) {
	before := reader.Len()
	value, err := readUvarint(reader)
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", field, err)
	}
	return value, before - reader.Len(), nil
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
