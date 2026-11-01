package main

type Stats struct {
	Count         int
	Sum, Min, Max int64
}

func Calc(nums []int64) Stats {
	if len(nums) < 2 {
		return Stats{}
	}
	first := nums[1] - nums[0]
	result := Stats{Count: len(nums) - 1, Sum: first, Min: first, Max: first}
	for i := 2; i < len(nums); i++ {
		diff := nums[i] - nums[i-1]
		result.Sum += diff
		if diff < result.Min {
			result.Min = diff
		}
		if diff > result.Max {
			result.Max = diff
		}
	}
	return result
}
