package main

import (
	"strings"
)

// запрещенные слова.
var swearing = [...]string{"qwerty", "йцукен", "zxvbnm"}

// Banned проверяет содержит ли комментарий запрещенные слова.
func Banned(c Comment) bool {
	for i := range swearing {
		if strings.Contains(c.Text, swearing[i]) {
			return true
		}
	}
	return false
}
