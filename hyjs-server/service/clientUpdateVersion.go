package service

import (
	"strconv"
	"strings"
)

// ClientUpdateVersionValid 接受规范的三段十进制版本，每段为 0 至 999。
func ClientUpdateVersionValid(version string) bool {
	_, ok := clientUpdateVersionParts(version)
	return ok
}

func clientUpdateVersionParts(version string) ([3]int, bool) {
	var values [3]int
	parts := strings.Split(version, ".")
	if len(parts) != len(values) {
		return values, false
	}
	for i, part := range parts {
		if len(part) == 0 || len(part) > 3 || (len(part) > 1 && part[0] == '0') {
			return values, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return values, false
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil || n > 999 {
			return values, false
		}
		values[i] = n
	}
	return values, true
}

// clientUpdateKnownVersion 仅对已核实的正式包构建号补充版本。
func clientUpdateKnownVersion(build int64, version string) string {
	if ClientUpdateVersionValid(version) {
		return version
	}
	if version == "" && build == 2026110101 {
		return "1.1.0"
	}
	return ""
}

// clientUpdateVersionBelow 对有门槛的未知或无效版本采取保守判定。
func clientUpdateVersionBelow(version, minimum string) bool {
	if minimum == "" {
		return false
	}
	a, okA := clientUpdateVersionParts(version)
	b, okB := clientUpdateVersionParts(minimum)
	if !okA || !okB {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
