package ttml

import (
	"math"
	"strconv"
	"strings"
)

// ExportTTMLText converts a TTMLLyric into TTML XML text.
// The output mirrors the TS writer behavior.
func ExportTTMLText(ttmlLyric TTMLLyric, pretty bool) string {
	params := make([][]LyricLine, 0)
	lyric := ttmlLyric.LyricLines

	var tmp []LyricLine
	for _, line := range lyric {
		if len(line.Words) == 0 && len(tmp) > 0 {
			params = append(params, tmp)
			tmp = []LyricLine{}
		} else {
			tmp = append(tmp, line)
		}
	}
	if len(tmp) > 0 {
		params = append(params, tmp)
	}

	doc := &xmlNode{Type: nodeDocument}

	createWordElement := func(word LyricWord) *xmlNode {
		span := newElement("span")
		span.setAttr("begin", MsToTimestamp(word.StartTime))
		span.setAttr("end", MsToTimestamp(word.EndTime))
		if word.Obscene {
			span.setAttr("amll:obscene", "true")
		}
		if word.EmptyBeat != 0 && !math.IsNaN(word.EmptyBeat) {
			span.setAttr("amll:empty-beat", formatNumber(word.EmptyBeat))
		}
		span.appendChild(newText(word.Word))
		return span
	}

	createRomanizationSpan := func(word LyricWord) *xmlNode {
		span := newElement("span")
		span.setAttr("begin", MsToTimestamp(word.StartTime))
		span.setAttr("end", MsToTimestamp(word.EndTime))
		span.appendChild(newText(word.RomanWord))
		return span
	}

	ttRoot := newElement("tt")
	ttRoot.setAttr("xmlns", nsTTML)
	ttRoot.setAttr("xmlns:ttm", nsTTM)
	ttRoot.setAttr("xmlns:amll", nsAMLL)
	ttRoot.setAttr("xmlns:itunes", nsItunes)

	nonBlankWordCounts := make([]int, 0, len(lyric))
	totalNonBlankWords := 0
	hasAnyTiming := false
	for _, line := range lyric {
		count := 0
		for _, word := range line.Words {
			if strings.TrimSpace(word.Word) != "" {
				count++
				if word.EndTime > word.StartTime {
					hasAnyTiming = true
				}
			}
		}
		nonBlankWordCounts = append(nonBlankWordCounts, count)
		totalNonBlankWords += count
	}

	timingMode := "None"
	if totalNonBlankWords != 0 && hasAnyTiming {
		timingMode = "Line"
		for _, count := range nonBlankWordCounts {
			if count > 1 {
				timingMode = "Word"
				break
			}
		}
	}
	ttRoot.setAttr("itunes:timing", timingMode)

	doc.appendChild(ttRoot)

	head := newElement("head")
	ttRoot.appendChild(head)

	body := newElement("body")

	hasOtherPerson := false
	for _, line := range lyric {
		if line.IsDuet {
			hasOtherPerson = true
			break
		}
	}

	metadataEl := newElement("metadata")
	mainPersonAgent := newElement("ttm:agent")
	mainPersonAgent.setAttr("type", "person")
	mainPersonAgent.setAttr("xml:id", "v1")
	metadataEl.appendChild(mainPersonAgent)

	if hasOtherPerson {
		otherPersonAgent := newElement("ttm:agent")
		otherPersonAgent.setAttr("type", "other")
		otherPersonAgent.setAttr("xml:id", "v2")
		metadataEl.appendChild(otherPersonAgent)
	}

	// Songwriter metadata (iTunes format)
	var songwriterMeta *TTMLMetadata
	for i := range ttmlLyric.Metadata {
		meta := &ttmlLyric.Metadata[i]
		if meta.Key == "songwriter" {
			for _, v := range meta.Value {
				if strings.TrimSpace(v) != "" {
					songwriterMeta = meta
					break
				}
			}
			break
		}
	}
	if songwriterMeta != nil {
		iTunesMetadata := newElement("iTunesMetadata")
		iTunesMetadata.setAttr("xmlns", nsItunes)
		songwritersEl := newElement("songwriters")
		for _, name := range songwriterMeta.Value {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			swEl := newElement("songwriter")
			swEl.appendChild(newText(trimmed))
			songwritersEl.appendChild(swEl)
		}
		if len(songwritersEl.Children) > 0 {
			iTunesMetadata.appendChild(songwritersEl)
			metadataEl.appendChild(iTunesMetadata)
		}
	}

	// Remaining metadata (AMLL format)
	for _, meta := range ttmlLyric.Metadata {
		if meta.Key == "songwriter" {
			continue
		}
		for _, value := range meta.Value {
			metaEl := newElement("amll:meta")
			metaEl.setAttr("key", meta.Key)
			metaEl.setAttr("value", value)
			metadataEl.appendChild(metaEl)
		}
	}

	head.appendChild(metadataEl)

	i := 0
	type romanizationEntry struct {
		key  string
		main []LyricWord
		bg   []LyricWord
	}
	var romanizationEntries []romanizationEntry

	guessDuration := float64(0)
	if len(lyric) > 0 {
		guessDuration = lyric[len(lyric)-1].EndTime
	}
	body.setAttr("dur", MsToTimestamp(guessDuration))

	isDynamicLyric := false
	for _, line := range lyric {
		count := 0
		for _, word := range line.Words {
			if strings.TrimSpace(word.Word) != "" {
				count++
			}
		}
		if count > 1 {
			isDynamicLyric = true
			break
		}
	}

	for _, param := range params {
		paramDiv := newElement("div")
		beginTime := float64(0)
		endTime := float64(0)
		if len(param) > 0 {
			beginTime = param[0].StartTime
			endTime = param[len(param)-1].EndTime
		}
		paramDiv.setAttr("begin", MsToTimestamp(beginTime))
		paramDiv.setAttr("end", MsToTimestamp(endTime))

		for lineIndex := 0; lineIndex < len(param); lineIndex++ {
			line := param[lineIndex]
			lineP := newElement("p")
			beginTime := line.StartTime
			endTime := line.EndTime

			lineP.setAttr("begin", MsToTimestamp(beginTime))
			lineP.setAttr("end", MsToTimestamp(endTime))
			if line.IsDuet {
				lineP.setAttr("ttm:agent", "v2")
			} else {
				lineP.setAttr("ttm:agent", "v1")
			}

			i++
			itunesKey := "L" + strconv.Itoa(i)
			lineP.setAttr("itunes:key", itunesKey)

			mainWords := line.Words
			var bgWords []LyricWord

			if isDynamicLyric {
				for _, word := range line.Words {
					if strings.TrimSpace(word.Word) == "" {
						lineP.appendChild(newText(word.Word))
					} else {
						lineP.appendChild(createWordElement(word))
					}
				}
				lineP.setAttr("begin", MsToTimestamp(line.StartTime))
				lineP.setAttr("end", MsToTimestamp(line.EndTime))
			} else {
				word := line.Words[0]
				lineP.appendChild(newText(word.Word))
				lineP.setAttr("begin", MsToTimestamp(word.StartTime))
				lineP.setAttr("end", MsToTimestamp(word.EndTime))
			}

			var nextLine *LyricLine
			if lineIndex+1 < len(param) {
				nextLine = &param[lineIndex+1]
			}

			if nextLine != nil && nextLine.IsBG {
				lineIndex++
				bgLine := *nextLine
				bgWords = bgLine.Words

				bgLineSpan := newElement("span")
				bgLineSpan.setAttr("ttm:role", "x-bg")

				if isDynamicLyric {
					beginTime := math.Inf(1)
					endTime := float64(0)

					firstWordIndex := -1
					lastWordIndex := -1
					for idx, word := range bgLine.Words {
						if strings.TrimSpace(word.Word) != "" {
							if firstWordIndex == -1 {
								firstWordIndex = idx
							}
							lastWordIndex = idx
						}
					}

					for wordIndex, word := range bgLine.Words {
						if strings.TrimSpace(word.Word) == "" {
							bgLineSpan.appendChild(newText(word.Word))
						} else {
							span := createWordElement(word)
							if wordIndex == firstWordIndex && len(span.Children) > 0 && span.Children[0].Type == nodeText {
								span.Children[0].Text = "(" + span.Children[0].Text
							}
							if wordIndex == lastWordIndex && len(span.Children) > 0 && span.Children[0].Type == nodeText {
								span.Children[0].Text = span.Children[0].Text + ")"
							}
							bgLineSpan.appendChild(span)
							beginTime = math.Min(beginTime, word.StartTime)
							endTime = math.Max(endTime, word.EndTime)
						}
					}
					bgLineSpan.setAttr("begin", MsToTimestamp(beginTime))
					bgLineSpan.setAttr("end", MsToTimestamp(endTime))
				} else {
					word := bgLine.Words[0]
					bgLineSpan.appendChild(newText("(" + word.Word + ")"))
					bgLineSpan.setAttr("begin", MsToTimestamp(word.StartTime))
					bgLineSpan.setAttr("end", MsToTimestamp(word.EndTime))
				}

				if bgLine.TranslatedLyric != "" {
					span := newElement("span")
					span.setAttr("ttm:role", "x-translation")
					span.setAttr("xml:lang", "zh-CN")
					span.appendChild(newText(bgLine.TranslatedLyric))
					bgLineSpan.appendChild(span)
				}

				if bgLine.RomanLyric != "" {
					span := newElement("span")
					span.setAttr("ttm:role", "x-roman")
					span.appendChild(newText(bgLine.RomanLyric))
					bgLineSpan.appendChild(span)
				}

				lineP.appendChild(bgLineSpan)
			}

			if line.TranslatedLyric != "" {
				span := newElement("span")
				span.setAttr("ttm:role", "x-translation")
				span.setAttr("xml:lang", "zh-CN")
				span.appendChild(newText(line.TranslatedLyric))
				lineP.appendChild(span)
			}

			if line.RomanLyric != "" {
				span := newElement("span")
				span.setAttr("ttm:role", "x-roman")
				span.appendChild(newText(line.RomanLyric))
				lineP.appendChild(span)
			}

			hasRoman := false
			for _, word := range mainWords {
				if strings.TrimSpace(word.RomanWord) != "" {
					hasRoman = true
					break
				}
			}
			if !hasRoman {
				for _, word := range bgWords {
					if strings.TrimSpace(word.RomanWord) != "" {
						hasRoman = true
						break
					}
				}
			}

			if hasRoman {
				romanizationEntries = append(romanizationEntries, romanizationEntry{
					key:  itunesKey,
					main: mainWords,
					bg:   bgWords,
				})
			}

			paramDiv.appendChild(lineP)
		}

		body.appendChild(paramDiv)
	}

	if len(romanizationEntries) > 0 {
		itunesMeta := newElement("iTunesMetadata")
		itunesMeta.setAttr("xmlns", nsItunes)

		transliterations := newElement("transliterations")
		transliteration := newElement("transliteration")

		for _, entry := range romanizationEntries {
			key := entry.key
			words := struct {
				main []LyricWord
				bg   []LyricWord
			}{main: entry.main, bg: entry.bg}
			textEl := newElement("text")
			textEl.setAttr("for", key)

			for _, word := range words.main {
				if strings.TrimSpace(word.RomanWord) != "" {
					textEl.appendChild(createRomanizationSpan(word))
				} else if strings.TrimSpace(word.Word) == "" && len(textEl.Children) > 0 {
					textEl.appendChild(newText(word.Word))
				}
			}

			hasBgRoman := false
			for _, word := range words.bg {
				if strings.TrimSpace(word.RomanWord) != "" {
					hasBgRoman = true
					break
				}
			}
			if hasBgRoman {
				bgSpan := newElement("span")
				bgSpan.setAttr("ttm:role", "x-bg")

				type indexedWord struct {
					word  LyricWord
					index int
				}
				var romanBgWords []indexedWord
				for idx, word := range words.bg {
					if strings.TrimSpace(word.RomanWord) != "" {
						romanBgWords = append(romanBgWords, indexedWord{word: word, index: idx})
					}
				}

				for wordIndex, iw := range romanBgWords {
					span := createRomanizationSpan(iw.word)
					if wordIndex == 0 && len(span.Children) > 0 && span.Children[0].Type == nodeText {
						span.Children[0].Text = "(" + span.Children[0].Text
					}
					if wordIndex == len(romanBgWords)-1 && len(span.Children) > 0 && span.Children[0].Type == nodeText {
						span.Children[0].Text = span.Children[0].Text + ")"
					}
					bgSpan.appendChild(span)

					if iw.index > -1 && iw.index < len(words.bg)-1 {
						nextWord := words.bg[iw.index+1]
						if strings.TrimSpace(nextWord.Word) == "" {
							bgSpan.appendChild(newText(nextWord.Word))
						}
					}
				}

				textEl.appendChild(bgSpan)
			}

			transliteration.appendChild(textEl)
		}

		transliterations.appendChild(transliteration)
		itunesMeta.appendChild(transliterations)
		metadataEl.appendChild(itunesMeta)
	}

	ttRoot.appendChild(body)

	return serializeDocument(doc, pretty)
}

func serializeDocument(doc *xmlNode, pretty bool) string {
	var sb strings.Builder
	serializeNode(&sb, doc, pretty, 0)
	return sb.String()
}

func formatNumber(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
