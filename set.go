package main

import (
	"strings"
	"sync"
)

type Set struct {
	list map[string]struct{} //empty structs occupy 0 memory
	sync.RWMutex
}

func (s *Set) Contains(v string) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.list[v]
	return ok
}

func (s *Set) Add(v string) {
	s.Lock()
	defer s.Unlock()
	s.list[v] = struct{}{}
}

func (s *Set) Remove(v string) {
	s.Lock()
	defer s.Unlock()
	delete(s.list, v)
}

func (s *Set) Clear() {
	s.Lock()
	defer s.Unlock()
	s.list = make(map[string]struct{})
}

func (s *Set) Size() int {
	return len(s.list)
}

func NewSet() *Set {
	s := &Set{}
	s.list = make(map[string]struct{})
	return s
}

func (s *Set) String() string {
	s.RLock()
	defer s.RUnlock()
	keys := make([]string, 0, s.Size())
	for key := range s.list {
		keys = append(keys, key)
	}
	return strings.Join(keys, ", ")
}
