package ttml

import (
	"math"
	"strconv"
	"strings"
)

type romanWord struct {
	StartTime float64
	EndTime   float64
	Text      string
}

type lineMetadata struct {
	Main string
	Bg   string
}

type wordRomanMetadata struct {
	Main []romanWord
	Bg   []romanWord
}

const (
	fullwidthLeftParen  = "\uFF08"
	fullwidthRightParen = "\uFF09"
)

// ParseLyric parses TTML text into a TTMLLyric structure.
// It mirrors the TS parser behavior, including edge cases.
func ParseLyric(ttmlText string) (TTMLLyric, error) {
	doc, err := parseXMLDocument(ttmlText)
	if err != nil {
		return TTMLLyric{}, err
	}

	itunesTranslations := map[string]lineMetadata{}
	translationTextElements := findElementsByPath(doc, []string{
		"iTunesMetadata", "translations", "translation", "text",
	})

	for _, textEl := range translationTextElements {
		key, ok := textEl.attrValueLocal("for")
		if !ok || key == "" {
			continue
		}

		main, bg := extractLineMetadata(textEl)
		if main != "" || bg != "" {
			itunesTranslations[key] = lineMetadata{Main: main, Bg: bg}
		}
	}

	itunesLineRomanizations := map[string]lineMetadata{}
	itunesWordRomanizations := map[string]wordRomanMetadata{}

	romanizationTextElements := findElementsByPath(doc, []string{
		"iTunesMetadata", "transliterations", "transliteration", "text",
	})

	for _, textEl := range romanizationTextElements {
		key, ok := textEl.attrValueLocal("for")
		if !ok || key == "" {
			continue
		}

		var mainWords []romanWord
		var bgWords []romanWord
		lineRomanMain := ""
		lineRomanBg := ""
		isWordByWord := false

		for _, node := range textEl.Children {
			if node.Type == nodeText {
				lineRomanMain += node.Text
				continue
			}
			if node.Type != nodeElement {
				continue
			}
			role, _ := node.attrValueNS(nsTTM, "role", "ttm:role")
			if role == "x-bg" {
				nestedSpans := findDescendantElements(node, func(n *xmlNode) bool {
					return nameMatches(n, "span") && n.hasAttrLocal("begin") && n.hasAttrLocal("end")
				})
				if len(nestedSpans) > 0 {
					isWordByWord = true
					for _, span := range nestedSpans {
						bgWordText := strings.TrimSpace(span.textContent())
						bgWordText = trimParens(bgWordText)

						beginStr, _ := span.attrValueLocal("begin")
						endStr, _ := span.attrValueLocal("end")
						begin, err := ParseTimespan(beginStr)
						if err != nil {
							return TTMLLyric{}, err
						}
						end, err := ParseTimespan(endStr)
						if err != nil {
							return TTMLLyric{}, err
						}
						bgWords = append(bgWords, romanWord{
							StartTime: begin,
							EndTime:   end,
							Text:      bgWordText,
						})
					}
				} else {
					lineRomanBg += node.textContent()
				}
			} else if node.hasAttrLocal("begin") && node.hasAttrLocal("end") {
				isWordByWord = true
				beginStr, _ := node.attrValueLocal("begin")
				endStr, _ := node.attrValueLocal("end")
				begin, err := ParseTimespan(beginStr)
				if err != nil {
					return TTMLLyric{}, err
				}
				end, err := ParseTimespan(endStr)
				if err != nil {
					return TTMLLyric{}, err
				}
				mainWords = append(mainWords, romanWord{
					StartTime: begin,
					EndTime:   end,
					Text:      node.textContent(),
				})
			}
		}

		if isWordByWord {
			itunesWordRomanizations[key] = wordRomanMetadata{
				Main: mainWords,
				Bg:   bgWords,
			}
		}

		lineRomanMain = strings.TrimSpace(lineRomanMain)
		lineRomanBg = trimParens(strings.TrimSpace(lineRomanBg))

		if lineRomanMain != "" || lineRomanBg != "" {
			itunesLineRomanizations[key] = lineMetadata{
				Main: lineRomanMain,
				Bg:   lineRomanBg,
			}
		}
	}

	itunesTimedTranslations := map[string]lineMetadata{}
	timedTranslationTextElements := findElementsByPath(doc, []string{
		"iTunesMetadata", "translations", "translation", "text",
	})

	for _, textEl := range timedTranslationTextElements {
		key, ok := textEl.attrValueLocal("for")
		if !ok || key == "" {
			continue
		}

		main, bg := extractLineMetadata(textEl)
		if (main != "" || bg != "") && hasDescendantTag(textEl, "span") {
			itunesTimedTranslations[key] = lineMetadata{Main: main, Bg: bg}
			delete(itunesTranslations, key)
		}
	}

	mainAgentID := "v1"

	metadata := []TTMLMetadata{}
	for _, meta := range findAllElements(doc) {
		if meta.Local != "meta" {
			continue
		}
		if meta.Name != "amll:meta" && meta.Namespace != nsAMLL {
			continue
		}
		key, ok := meta.attrValueLocal("key")
		if !ok || key == "" {
			continue
		}
		value, ok := meta.attrValueLocal("value")
		if !ok || value == "" {
			continue
		}
		found := false
		for i := range metadata {
			if metadata[i].Key == key {
				metadata[i].Value = append(metadata[i].Value, value)
				found = true
				break
			}
		}
		if !found {
			metadata = append(metadata, TTMLMetadata{
				Key:   key,
				Value: []string{value},
			})
		}
	}

	songwriterElements := findElementsByPath(doc, []string{
		"iTunesMetadata", "songwriters", "songwriter",
	})
	if len(songwriterElements) > 0 {
		var songwriterValues []string
		for _, el := range songwriterElements {
			name := strings.TrimSpace(el.textContent())
			if name != "" {
				songwriterValues = append(songwriterValues, name)
			}
		}
		if len(songwriterValues) > 0 {
			metadata = append(metadata, TTMLMetadata{
				Key:   "songwriter",
				Value: songwriterValues,
			})
		}
	}

	for _, agent := range findAllElements(doc) {
		if agent.Local != "agent" {
			continue
		}
		if agent.Name != "ttm:agent" && agent.Namespace != nsTTM {
			continue
		}
		agentType, _ := agent.attrValueLocal("type")
		if agentType == "person" {
			if id, ok := agent.attrValueNS(nsXML, "id", "xml:id"); ok && id != "" {
				mainAgentID = id
				break
			}
		}
	}

	var lyricLines []LyricLine

	var parseLineElement func(lineEl *xmlNode, isBG bool, isDuet bool, parentItunesKey *string) error
	parseLineElement = func(lineEl *xmlNode, isBG bool, isDuet bool, parentItunesKey *string) error {
		startTimeAttr, startOk := lineEl.attrValueLocal("begin")
		endTimeAttr, endOk := lineEl.attrValueLocal("end")
		if startOk && startTimeAttr == "" {
			startOk = false
		}
		if endOk && endTimeAttr == "" {
			endOk = false
		}

		parsedStartTime := float64(0)
		parsedEndTime := float64(0)

		if startOk && endOk {
			start, err := ParseTimespan(startTimeAttr)
			if err != nil {
				return err
			}
			end, err := ParseTimespan(endTimeAttr)
			if err != nil {
				return err
			}
			parsedStartTime = start
			parsedEndTime = end
		}

		line := LyricLine{
			ID:              newUID(),
			Words:           []LyricWord{},
			TranslatedLyric: "",
			RomanLyric:      "",
			IsBG:            isBG,
			IsDuet:          false,
			StartTime:       parsedStartTime,
			EndTime:         parsedEndTime,
			IgnoreSync:      false,
		}

		if isBG {
			line.IsDuet = isDuet
		} else {
			if agent, ok := lineEl.attrValueNS(nsTTM, "agent", "ttm:agent"); ok && agent != "" && agent != mainAgentID {
				line.IsDuet = true
			}
		}

		var itunesKey string
		if isBG {
			if parentItunesKey != nil {
				itunesKey = *parentItunesKey
			}
		} else {
			if key, ok := lineEl.attrValueNS(nsItunes, "key", "itunes:key"); ok && key != "" {
				itunesKey = key
			}
		}

		var availableRomanWords []romanWord
		if itunesKey != "" {
			if romanData, ok := itunesWordRomanizations[itunesKey]; ok {
				if isBG {
					availableRomanWords = append([]romanWord(nil), romanData.Bg...)
				} else {
					availableRomanWords = append([]romanWord(nil), romanData.Main...)
				}
			}
		}

		if itunesKey != "" {
			if timed, ok := itunesTimedTranslations[itunesKey]; ok {
				if isBG {
					line.TranslatedLyric = timed.Bg
				} else {
					line.TranslatedLyric = timed.Main
				}
			} else if trans, ok := itunesTranslations[itunesKey]; ok {
				if isBG {
					line.TranslatedLyric = trans.Bg
				} else {
					line.TranslatedLyric = trans.Main
				}
			}

			if roman, ok := itunesLineRomanizations[itunesKey]; ok {
				if isBG {
					line.RomanLyric = roman.Bg
				} else {
					line.RomanLyric = roman.Main
				}
			}
		}

		haveBG := false

		for _, wordNode := range lineEl.Children {
			switch wordNode.Type {
			case nodeText:
				wordText := wordNode.Text
				trimmed := strings.TrimSpace(wordText)
				start := float64(0)
				end := float64(0)
				if trimmed != "" {
					start = line.StartTime
					end = line.EndTime
				}
				line.Words = append(line.Words, LyricWord{
					ID:        newUID(),
					Word:      wordText,
					StartTime: start,
					EndTime:   end,
					Obscene:   false,
					EmptyBeat: 0,
					RomanWord: "",
				})
			case nodeElement:
				role, _ := wordNode.attrValueNS(nsTTM, "role", "ttm:role")
				if nameMatches(wordNode, "span") && role != "" {
					if role == "x-bg" {
						if err := parseLineElement(wordNode, true, line.IsDuet, &itunesKey); err != nil {
							return err
						}
						haveBG = true
					} else if role == "x-translation" {
						if line.TranslatedLyric == "" {
							line.TranslatedLyric = wordNode.innerXML()
						}
					} else if role == "x-roman" {
						if line.RomanLyric == "" {
							line.RomanLyric = wordNode.innerXML()
						}
					}
				} else if wordNode.hasAttrLocal("begin") && wordNode.hasAttrLocal("end") {
					wordStartStr, _ := wordNode.attrValueLocal("begin")
					wordEndStr, _ := wordNode.attrValueLocal("end")
					wordStartTime, err := ParseTimespan(wordStartStr)
					if err != nil {
						return err
					}
					wordEndTime, err := ParseTimespan(wordEndStr)
					if err != nil {
						return err
					}

					word := LyricWord{
						ID:        newUID(),
						Word:      wordNode.textContent(),
						StartTime: wordStartTime,
						EndTime:   wordEndTime,
						Obscene:   false,
						EmptyBeat: 0,
						RomanWord: "",
					}

					if emptyBeat, ok := wordNode.attrValueNS(nsAMLL, "empty-beat", "amll:empty-beat"); ok && emptyBeat != "" {
						if parsed, err := parseFloatNumber(emptyBeat); err == nil {
							word.EmptyBeat = parsed
						}
					}
					if obscene, ok := wordNode.attrValueNS(nsAMLL, "obscene", "amll:obscene"); ok && obscene == "true" {
						word.Obscene = true
					}

					if len(availableRomanWords) > 0 {
						matchIndex := -1
						for i, roman := range availableRomanWords {
							if roman.StartTime == wordStartTime && roman.EndTime == wordEndTime {
								matchIndex = i
								break
							}
						}
						if matchIndex != -1 {
							word.RomanWord = availableRomanWords[matchIndex].Text
							availableRomanWords = append(availableRomanWords[:matchIndex], availableRomanWords[matchIndex+1:]...)
						}
					}

					line.Words = append(line.Words, word)
				}
			}
		}

		if !startOk || !endOk {
			minStart := math.Inf(1)
			maxEnd := float64(0)
			for _, w := range line.Words {
				if strings.TrimSpace(w.Word) == "" {
					continue
				}
				if w.StartTime < minStart {
					minStart = w.StartTime
				}
				if w.EndTime > maxEnd {
					maxEnd = w.EndTime
				}
			}
			line.StartTime = minStart
			line.EndTime = maxEnd
		}

		if line.IsBG {
			if len(line.Words) > 0 {
				firstWord := line.Words[0].Word
				if strings.HasPrefix(firstWord, fullwidthLeftParen) || strings.HasPrefix(firstWord, "(") {
					if strings.HasPrefix(firstWord, fullwidthLeftParen) {
						firstWord = strings.TrimPrefix(firstWord, fullwidthLeftParen)
					} else {
						firstWord = strings.TrimPrefix(firstWord, "(")
					}
					if firstWord == "" {
						line.Words = line.Words[1:]
					} else {
						line.Words[0].Word = firstWord
					}
				}
			}
			if len(line.Words) > 0 {
				lastIdx := len(line.Words) - 1
				lastWord := line.Words[lastIdx].Word
				if strings.HasSuffix(lastWord, fullwidthRightParen) || strings.HasSuffix(lastWord, ")") {
					if strings.HasSuffix(lastWord, fullwidthRightParen) {
						lastWord = strings.TrimSuffix(lastWord, fullwidthRightParen)
					} else {
						lastWord = strings.TrimSuffix(lastWord, ")")
					}
					if lastWord == "" {
						line.Words = line.Words[:lastIdx]
					} else {
						line.Words[lastIdx].Word = lastWord
					}
				}
			}
		}

		if haveBG {
			var bgLine *LyricLine
			if len(lyricLines) > 0 {
				last := lyricLines[len(lyricLines)-1]
				bgLine = &last
				lyricLines = lyricLines[:len(lyricLines)-1]
			}
			lyricLines = append(lyricLines, line)
			if bgLine != nil {
				lyricLines = append(lyricLines, *bgLine)
			}
		} else {
			lyricLines = append(lyricLines, line)
		}
		return nil
	}

	for _, lineEl := range findBodyParagraphs(doc) {
		if err := parseLineElement(lineEl, false, false, nil); err != nil {
			return TTMLLyric{}, err
		}
	}

	return TTMLLyric{
		Metadata:   metadata,
		LyricLines: lyricLines,
	}, nil
}

func extractLineMetadata(textEl *xmlNode) (string, string) {
	var mainSB strings.Builder
	var bgSB strings.Builder

	for _, node := range textEl.Children {
		if node.Type == nodeText {
			mainSB.WriteString(node.Text)
		} else if node.Type == nodeElement {
			role, _ := node.attrValueNS(nsTTM, "role", "ttm:role")
			if role == "x-bg" {
				bgSB.WriteString(node.textContent())
			}
		}
	}

	main := strings.TrimSpace(mainSB.String())
	bg := trimParens(strings.TrimSpace(bgSB.String()))
	return main, bg
}

func trimParens(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, fullwidthLeftParen) || strings.HasPrefix(text, "(") {
		if strings.HasPrefix(text, fullwidthLeftParen) {
			text = strings.TrimPrefix(text, fullwidthLeftParen)
		} else {
			text = strings.TrimPrefix(text, "(")
		}
	}
	if strings.HasSuffix(text, fullwidthRightParen) || strings.HasSuffix(text, ")") {
		if strings.HasSuffix(text, fullwidthRightParen) {
			text = strings.TrimSuffix(text, fullwidthRightParen)
		} else {
			text = strings.TrimSuffix(text, ")")
		}
	}
	return strings.TrimSpace(text)
}

func findBodyParagraphs(doc *xmlNode) []*xmlNode {
	var result []*xmlNode
	var walk func(node *xmlNode, inBody bool)
	walk = func(node *xmlNode, inBody bool) {
		if node.Type == nodeDocument {
			for _, child := range node.Children {
				walk(child, inBody)
			}
			return
		}
		if node.Type != nodeElement {
			return
		}
		if node.Local == "body" {
			inBody = true
		}
		if inBody && node.Local == "p" {
			if node.hasAttrLocal("begin") && node.hasAttrLocal("end") {
				result = append(result, node)
			}
		}
		for _, child := range node.Children {
			walk(child, inBody)
		}
	}
	walk(doc, false)
	return result
}

func parseFloatNumber(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return math.NaN(), nil
	}
	return parsed, nil
}
