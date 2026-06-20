// Copyright 2026 The Actordock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envd

import (
	"slices"
	"sync"
	"sync/atomic"
)

// MultiplexedChannel fans out values written to Source to every subscriber from Fork.
type MultiplexedChannel[T any] struct {
	Source chan T

	mu       sync.RWMutex
	channels []*subscriber[T]
	exited   atomic.Bool
}

type subscriber[T any] struct {
	ch   chan T
	done chan struct{}
	once sync.Once
}

func (s *subscriber[T]) cancel() {
	s.once.Do(func() { close(s.done) })
}

func (s *subscriber[T]) isCancelled() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func NewMultiplexedChannel[T any](buffer int) *MultiplexedChannel[T] {
	c := &MultiplexedChannel[T]{
		Source: make(chan T, buffer),
	}
	go c.run()
	return c
}

func (m *MultiplexedChannel[T]) run() {
	for v := range m.Source {
		m.mu.RLock()
		subs := m.channels
		m.mu.RUnlock()

		for _, s := range subs {
			if s.isCancelled() {
				continue
			}
			select {
			case s.ch <- v:
			case <-s.done:
			}
		}
	}

	m.exited.Store(true)

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.channels {
		s.cancel()
		close(s.ch)
	}
	m.channels = nil
}

func (m *MultiplexedChannel[T]) HasSubscribers() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.channels {
		if !s.isCancelled() {
			return true
		}
	}
	return false
}

func (m *MultiplexedChannel[T]) Fork() (chan T, func()) {
	if m.exited.Load() {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.exited.Load() {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	s := &subscriber[T]{
		ch:   make(chan T),
		done: make(chan struct{}),
	}
	m.channels = append(m.channels, s)

	return s.ch, func() { m.remove(s) }
}

func (m *MultiplexedChannel[T]) remove(s *subscriber[T]) {
	s.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	for i, sub := range m.channels {
		if sub == s {
			m.channels = slices.Concat(m.channels[:i], m.channels[i+1:])
			return
		}
	}
}
