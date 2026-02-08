package ttml

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var timeRegexp = regexp.MustCompile(`^(((\d+):)?(\d+):)?((\d+)([.:](\d{1,3}))?)$`)

// ParseTimespan parses a TTML time string into milliseconds.
// It mirrors the TS parseTimespan behavior.
func ParseTimespan(timeSpan string) (float64, error) {
	matches := timeRegexp.FindStringSubmatch(timeSpan)
	if matches == nil {
		return 0, fmt.Errorf("时间戳字符串解析失败：%s", timeSpan)
	}

	getInt := func(idx int) int64 {
		if idx >= len(matches) || matches[idx] == "" {
			return 0
		}
		v, _ := strconv.ParseInt(matches[idx], 10, 64)
		return v
	}

	hour := getInt(3)
	min := getInt(4)
	sec := getInt(6)

	fracStr := ""
	if len(matches) > 8 {
		fracStr = matches[8]
	}
	if fracStr == "" {
		fracStr = "0"
	}
	if len(fracStr) < 3 {
		fracStr = fracStr + strings.Repeat("0", 3-len(fracStr))
	}
	frac, _ := strconv.ParseInt(fracStr, 10, 64)

	total := (hour*3600 + min*60 + sec) * 1000
	return float64(total + frac), nil
}

// MsToTimestamp converts milliseconds to a TTML time string.
// If ms is omitted, milliseconds are included by default.
func MsToTimestamp(timeMS float64, ms ...bool) string {
	if math.IsInf(timeMS, 1) {
		return "99:99.999"
	}
	if timeMS < 0 || math.IsNaN(timeMS) {
		timeMS = 0
	}

	timeMS = math.Round(timeMS)

	t := timeMS / 1000
	secs := math.Mod(t, 60)
	t = (t - secs) / 60
	mins := math.Mod(t, 60)
	hrs := (t - mins) / 60

	h := fmt.Sprintf("%02d", int64(hrs))
	m := fmt.Sprintf("%02d", int64(mins))
	s := fmt.Sprintf("%06.3f", secs)
	sNoMS := fmt.Sprintf("%02d", int64(math.Floor(secs)))

	withMS := true
	if len(ms) > 0 && !ms[0] {
		withMS = false
	}

	if !withMS {
		if hrs > 0 {
			return fmt.Sprintf("%s:%s:%s", h, m, sNoMS)
		}
		return fmt.Sprintf("%s:%s", m, sNoMS)
	}

	if hrs > 0 {
		return fmt.Sprintf("%s:%s:%s", h, m, s)
	}
	return fmt.Sprintf("%s:%s", m, s)
}
