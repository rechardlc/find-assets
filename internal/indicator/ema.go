package indicator

// EMA 按递推公式计算指数移动平均：
//
//	alpha = 2 / (n + 1)
//	EMA[i] = alpha * Close[i] + (1 - alpha) * EMA[i-1]
//
// 冷启动用 Close[0] 作为初值，前 n-1 根尚未稳定。
// 返回切片长度与 closes 一致；空输入返回空切片。
func EMA(closes []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, len(closes))
	if len(closes) == 0 {
		return out
	}
	alpha := 2.0 / float64(n+1)
	out[0] = closes[0]
	for i := 1; i < len(closes); i++ {
		out[i] = alpha*closes[i] + (1-alpha)*out[i-1]
	}
	return out
}

// Cross 判定 a、b 两条均线在 [from, to] 闭区间内是否发生过金叉或死叉。
// 任意一种方向均算命中，符合"交织 / 缠绕"语义。
// 要求 from >= 1 且 to < len(a) == len(b)。
func Cross(a, b []float64, from, to int) bool {
	if len(a) != len(b) {
		return false
	}
	if from < 1 {
		from = 1
	}
	if to >= len(a) {
		to = len(a) - 1
	}
	for i := from; i <= to; i++ {
		prev := a[i-1] - b[i-1]
		curr := a[i] - b[i]
		if prev == 0 || curr == 0 {
			// 触碰也视作一次穿越
			return true
		}
		if (prev < 0) != (curr < 0) {
			return true
		}
	}
	return false
}

// DeadCrossAt 判定在索引 i 处 fast 是否「下穿」slow（死叉）：
// 前一根 fast 在 slow 之上或相等，当根 fast 跌到 slow 之下。
// 要求 1 <= i < len(fast) == len(slow)，否则返回 false。
func DeadCrossAt(fast, slow []float64, i int) bool {
	if len(fast) != len(slow) || i < 1 || i >= len(fast) {
		return false
	}
	return fast[i-1]-slow[i-1] >= 0 && fast[i]-slow[i] < 0
}
