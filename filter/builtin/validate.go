package builtin

import "strings"

func digitsOf(s string) []int {
	var d []int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d = append(d, int(r-'0'))
		}
	}
	return d
}

func allEqual(d []int) bool {
	for _, x := range d[1:] {
		if x != d[0] {
			return false
		}
	}
	return true
}

func checkDigit(d []int, weights []int) int {
	sum := 0
	for i, w := range weights {
		sum += d[i] * w
	}
	if r := sum % 11; r >= 2 {
		return 11 - r
	}
	return 0
}

func validCPF(s string) bool {
	d := digitsOf(s)
	if len(d) != 11 || allEqual(d) {
		return false
	}
	return checkDigit(d, []int{10, 9, 8, 7, 6, 5, 4, 3, 2}) == d[9] && checkDigit(d, []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2}) == d[10]
}

func validCNPJ(s string) bool {
	d := digitsOf(s)
	if len(d) != 14 || allEqual(d) {
		return false
	}
	return checkDigit(d, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == d[12] && checkDigit(d, []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == d[13]
}

func validCard(s string) bool {
	d := digitsOf(s)
	if len(d) < 13 || len(d) > 19 || d[0] < 2 || d[0] > 6 || strings.Trim(s, "0123456789 -") != "" {
		return false
	}
	sum := 0
	for i := range d {
		x := d[len(d)-1-i]
		if i%2 == 1 {
			if x *= 2; x > 9 {
				x -= 9
			}
		}
		sum += x
	}
	return sum%10 == 0
}
