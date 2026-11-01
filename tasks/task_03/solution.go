package main

import (
	"errors"
	"strconv"
)

var ErrZero = errors.New("zero is not allowed")

func fizzBuzz(n int) (string, error) {
	if n == 0 {
		return "", ErrZero
	}
	if n%15 == 0 {
		return "FizzBuzz", nil
	}
	if n%3 == 0 {
		return "Fizz", nil
	}
	if n%5 == 0 {
		return "Buzz", nil
	}
	return strconv.Itoa(n), nil
}
