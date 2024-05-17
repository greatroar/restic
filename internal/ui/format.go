package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"strconv"
	"time"
)

func FormatBytes(c uint64) string {
	s := make([]byte, 0, 16)

	shift, unit := 0, ""
	switch {
	case c >= 1<<40:
		shift, unit = 40, " TiB"
	case c >= 1<<30:
		shift, unit = 30, " GiB"
	case c >= 1<<20:
		shift, unit = 20, " MiB"
	case c >= 1<<10:
		shift, unit = 10, " KiB"
	default:
		s = strconv.AppendUint(s, c, 10)
		s = append(s, " B"...)
		return string(s)
	}

	b := float64(c) / float64(int(1)<<shift)
	s = strconv.AppendFloat(s, b, 'f', 3, 64)
	s = append(s, unit...)
	return string(s)
}

// FormatPercent formats numerator/denominator as a percentage.
func FormatPercent(numerator uint64, denominator uint64) string {
	if denominator == 0 {
		return ""
	}

	percent := 100.0 * float64(numerator) / float64(denominator)
	if percent > 100 {
		percent = 100
	}

	return fmt.Sprintf("%3.2f%%", percent)
}

// FormatDuration formats d as FormatSeconds would.
func FormatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return FormatSeconds(sec)
}

// FormatSeconds formats sec as MM:SS, or HH:MM:SS if sec seconds
// is at least an hour.
func FormatSeconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}

// ParseBytes parses a size in bytes from s. It understands the suffixes
// B, K, M, G and T for powers of 1024.
func ParseBytes(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("expected size, got empty string")
	}

	numStr := s[:len(s)-1]
	var unit uint64 = 1

	switch s[len(s)-1] {
	case 'b', 'B':
		// use initialized values, do nothing here
	case 'k', 'K':
		unit = 1024
	case 'm', 'M':
		unit = 1024 * 1024
	case 'g', 'G':
		unit = 1024 * 1024 * 1024
	case 't', 'T':
		unit = 1024 * 1024 * 1024 * 1024
	default:
		numStr = s
	}
	value, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, err
	}

	hi, lo := bits.Mul64(uint64(value), unit)
	value = int64(lo)
	if hi != 0 || value < 0 {
		return 0, fmt.Errorf("ParseSize: %q: %w", numStr, strconv.ErrRange)
	}

	return value, nil
}

func ToJSONString(status interface{}) string {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(status)
	if err != nil {
		panic(err)
	}
	return buf.String()
}
