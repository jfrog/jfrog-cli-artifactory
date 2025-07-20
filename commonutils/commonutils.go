package commonutils

import "strconv"

func IsFlagPositiveNumber(flag string) bool {
	num, err := strconv.Atoi(flag)
	if err != nil {
		return false
	}
	return num > 0
}
