// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

// Concurrency is the default concurrent goroutines number
var Concurrency = int(8)

// RateLimiter is the interface for rate limiting
type RateLimiter interface {
	//Acquire acquires a token (blocks if none accessible)
	Acquire() Token
	//Release releases the token
	Release(Token)
}

// Token is a token
type Token struct{}

// NewRateLimiter returns a RateLimiter
func NewRateLimiter(n int) RateLimiter {
	rl := &rateLimiter{tokens: make(chan Token, n)}
	var t Token
	for i := 0; i < n; i++ {
		rl.tokens <- t
	}
	return rl
}

type rateLimiter struct {
	tokens chan Token
}

// Acquire pulls a token
func (rl *rateLimiter) Acquire() Token {
	return <-rl.tokens
}

// Release pushes back the token
func (rl *rateLimiter) Release(t Token) {
	select {
	case rl.tokens <- t:
	default:
	}
}
