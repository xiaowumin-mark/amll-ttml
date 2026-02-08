package ttml

import (
	"strconv"
	"sync/atomic"
)

// TTMLMetadata matches the metadata structure used by the TS implementation.
type TTMLMetadata struct {
	Key   string
	Value []string
	Error bool
}

// TTMLLyric is the container for parsed lyrics and metadata.
type TTMLLyric struct {
	Metadata   []TTMLMetadata
	LyricLines []LyricLine
}

// LyricWord represents a single word (or whitespace token) in a lyric line.
// Times are in milliseconds.
type LyricWord struct {
	ID           string
	StartTime    float64
	EndTime      float64
	Word         string
	Obscene      bool
	EmptyBeat    float64
	RomanWord    string
	RomanWarning bool
}

// LyricLine represents a single lyric line.
// Times are in milliseconds.
type LyricLine struct {
	ID              string
	Words           []LyricWord
	TranslatedLyric string
	RomanLyric      string
	IsBG            bool
	IsDuet          bool
	StartTime       float64
	EndTime         float64
	IgnoreSync      bool
}

var uidCounter uint64

func newUID() string {
	return strconv.FormatUint(atomic.AddUint64(&uidCounter, 1), 10)
}

// NewLyricWord creates a LyricWord with default values.
func NewLyricWord() LyricWord {
	return LyricWord{
		ID:        newUID(),
		StartTime: 0,
		EndTime:   0,
		Word:      "",
		Obscene:   false,
		EmptyBeat: 0,
		RomanWord: "",
	}
}

// NewLyricLine creates a LyricLine with default values.
func NewLyricLine() LyricLine {
	return LyricLine{
		ID:              newUID(),
		Words:           []LyricWord{},
		TranslatedLyric: "",
		RomanLyric:      "",
		IsBG:            false,
		IsDuet:          false,
		StartTime:       0,
		EndTime:         0,
		IgnoreSync:      false,
	}
}
