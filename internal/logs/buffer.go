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

package logs

import (
	"bytes"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxLines = 10000
	DefaultMaxBytes = 4 * 1024 * 1024
)

type storedEntry struct {
	seq       uint64
	timestamp time.Time
	message   string
	level     string
	fields    map[string]string
	bytes     int
}

// Buffer is a fixed-capacity in-memory ring buffer. When full, oldest entries are dropped.
type Buffer struct {
	mu       sync.RWMutex
	maxLines int
	maxBytes int
	entries  []storedEntry
	total    int
	nextSeq  uint64
}

func NewBuffer(maxLines, maxBytes int) *Buffer {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Buffer{maxLines: maxLines, maxBytes: maxBytes}
}

func (b *Buffer) Append(level, message string, fields map[string]string) {
	if message == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	fieldsCopy := cloneFields(fields)
	e := storedEntry{
		seq:       b.nextSeq,
		timestamp: time.Now().UTC(),
		message:   message,
		level:     level,
		fields:    fieldsCopy,
		bytes:     entryBytes(message, fieldsCopy),
	}
	b.nextSeq++
	b.entries = append(b.entries, e)
	b.total += e.bytes
	b.evict()
}

// AppendOutput splits process stdout/stderr into lines and appends them.
func (b *Buffer) AppendOutput(stream string, data []byte) {
	level := "info"
	if stream == "stderr" {
		level = "error"
	}
	fields := map[string]string{"stream": stream}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		b.Append(level, string(line), fields)
	}
}

func (b *Buffer) Query(q Query) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	matched := make([]storedEntry, 0, len(b.entries))
	for _, e := range b.entries {
		if q.Start != nil && e.timestamp.UnixMilli() < *q.Start {
			continue
		}
		if q.Level != "" && e.level != q.Level {
			continue
		}
		if q.Search != "" && !strings.Contains(e.message, q.Search) {
			continue
		}
		if q.Cursor != nil {
			cursor := uint64(*q.Cursor)
			dir := q.Direction
			if dir == "" {
				dir = "forward"
			}
			switch dir {
			case "forward":
				if e.seq < cursor {
					continue
				}
			case "backward":
				if e.seq > cursor {
					continue
				}
			}
		}
		matched = append(matched, e)
	}

	dir := q.Direction
	if dir == "" {
		dir = "forward"
	}
	sort.Slice(matched, func(i, j int) bool {
		if dir == "backward" {
			return matched[i].seq > matched[j].seq
		}
		return matched[i].seq < matched[j].seq
	})

	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}
	return toEntries(matched)
}

func (b *Buffer) evict() {
	for len(b.entries) > b.maxLines || b.total > b.maxBytes {
		if len(b.entries) == 0 {
			b.total = 0
			return
		}
		removed := b.entries[0]
		b.entries = b.entries[1:]
		b.total -= removed.bytes
	}
}

func entryBytes(message string, fields map[string]string) int {
	n := len(message)
	for k, v := range fields {
		n += len(k) + len(v)
	}
	return n
}

func cloneFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func toEntries(in []storedEntry) []Entry {
	out := make([]Entry, len(in))
	for i, e := range in {
		out[i] = Entry{
			Timestamp: e.timestamp.Format(time.RFC3339Nano),
			Message:   e.message,
			Level:     e.level,
			Fields:    e.fields,
		}
	}
	return out
}
