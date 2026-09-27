package index

import (
	"sort"
	"sync"
)

// SecondaryIndex maintains inverted token-to-primary-key postings lists.
type SecondaryIndex struct {
	mu        sync.RWMutex
	postings  map[uint64][]uint64
	docTokens map[uint64][]uint64
}

// NewSecondaryIndex initializes an empty secondary inverted index.
func NewSecondaryIndex() *SecondaryIndex {
	return &SecondaryIndex{
		postings:  make(map[uint64][]uint64),
		docTokens: make(map[uint64][]uint64),
	}
}

// Index adds or updates the token associations for a primary key.
func (idx *SecondaryIndex) Index(primaryKey uint64, tokens []uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old postings if this key was already indexed
	if oldTokens, exists := idx.docTokens[primaryKey]; exists {
		for _, token := range oldTokens {
			idx.removePostingLocked(token, primaryKey)
		}
	}

	if len(tokens) == 0 {
		delete(idx.docTokens, primaryKey)
		return
	}

	tokenCopy := make([]uint64, len(tokens))
	copy(tokenCopy, tokens)
	idx.docTokens[primaryKey] = tokenCopy

	for _, token := range tokens {
		list := idx.postings[token]
		pos := sort.Search(len(list), func(i int) bool {
			return list[i] >= primaryKey
		})
		if pos < len(list) && list[pos] == primaryKey {
			continue
		}
		list = append(list, 0)
		copy(list[pos+1:], list[pos:])
		list[pos] = primaryKey
		idx.postings[token] = list
	}
}

// Unindex removes all secondary index postings for a primary key.
func (idx *SecondaryIndex) Unindex(primaryKey uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	oldTokens, exists := idx.docTokens[primaryKey]
	if !exists {
		return
	}

	for _, token := range oldTokens {
		idx.removePostingLocked(token, primaryKey)
	}
	delete(idx.docTokens, primaryKey)
}

func (idx *SecondaryIndex) removePostingLocked(token, primaryKey uint64) {
	list := idx.postings[token]
	pos := sort.Search(len(list), func(i int) bool {
		return list[i] >= primaryKey
	})
	if pos < len(list) && list[pos] == primaryKey {
		copy(list[pos:], list[pos+1:])
		idx.postings[token] = list[:len(list)-1]
		if len(idx.postings[token]) == 0 {
			delete(idx.postings, token)
		}
	}
}

// Lookup returns all primary keys matching a single token hash.
func (idx *SecondaryIndex) Lookup(token uint64) []uint64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	list := idx.postings[token]
	if len(list) == 0 {
		return nil
	}
	res := make([]uint64, len(list))
	copy(res, list)
	return res
}

// LookupIntersect returns primary keys that match ALL provided token hashes.
func (idx *SecondaryIndex) LookupIntersect(tokens []uint64) []uint64 {
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) == 1 {
		return idx.Lookup(tokens[0])
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lists := make([][]uint64, 0, len(tokens))
	for _, t := range tokens {
		l := idx.postings[t]
		if len(l) == 0 {
			return nil
		}
		lists = append(lists, l)
	}

	// Sort lists by size ascending for minimal intersection comparisons
	sort.Slice(lists, func(i, j int) bool {
		return len(lists[i]) < len(lists[j])
	})

	base := lists[0]
	result := make([]uint64, len(base))
	copy(result, base)

	for _, nextList := range lists[1:] {
		result = intersectSorted(result, nextList)
		if len(result) == 0 {
			break
		}
	}
	return result
}

func intersectSorted(a, b []uint64) []uint64 {
	var res []uint64
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			res = append(res, a[i])
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return res
}
