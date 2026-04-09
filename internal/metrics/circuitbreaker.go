package metrics

import (
	"log"
	"time"

	"github.com/sony/gobreaker"
)

type CircuitBreaker struct {
	cb     *gobreaker.CircuitBreaker
	name   string
	onTrip func(name string, reason error)
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func NewCircuitBreaker(name string, threshold uint32, timeout, halfOpenTimeout time.Duration, onTrip func(string, error), onReset func(string)) *CircuitBreaker {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return uint32(counts.ConsecutiveFailures) >= threshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			if to == gobreaker.StateOpen {
				onTrip(name, nil)
			} else if to == gobreaker.StateClosed {
				onReset(name)
			}
		},
	})
	return &CircuitBreaker{cb: cb, name: name, onTrip: onTrip}
}

func (c *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	return c.cb.Execute(fn)
}

func (c *CircuitBreaker) State() CircuitState {
	switch c.cb.State() {
	case gobreaker.StateClosed:
		return CircuitClosed
	case gobreaker.StateOpen:
		return CircuitOpen
	case gobreaker.StateHalfOpen:
		return CircuitHalfOpen
	}
	return CircuitClosed
}

func (c *CircuitBreaker) HalfOpen() bool {
	return c.cb.State() == gobreaker.StateHalfOpen
}

func DefaultOnTrip(name string, reason error) {
	if reason != nil {
		log.Printf("[WARN] Circuit breaker tripped: %s: %v", name, reason)
	} else {
		log.Printf("[WARN] Circuit breaker tripped: %s", name)
	}
}

func DefaultOnReset(name string) {
	log.Printf("[INFO] Circuit breaker reset: %s", name)
}
