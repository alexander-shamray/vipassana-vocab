package search

// within is the plain DP edit distance: the fallback for long inputs and
// the test oracle for Myers.
// It returns the edit distance between a and b when it is at most max,
// and -1 otherwise. It keeps one row of the DP table and gives up as soon as
// every cell in a row exceeds max.
func within(a, b []rune, max int) int {
	if d := len(a) - len(b); d > max || -d > max {
		return -1
	}
	var buf [2 * 48]int
	var prev, cur []int
	if n := len(b) + 1; n <= 48 {
		prev, cur = buf[:n], buf[48:48+n]
	} else {
		prev, cur = make([]int, n), make([]int, n)
	}
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > max {
			return -1
		}
		prev, cur = cur, prev
	}
	if prev[len(b)] > max {
		return -1
	}
	return prev[len(b)]
}
