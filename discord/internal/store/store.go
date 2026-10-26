// Package store is the verify bot's local record of who it has verified.
//
// It is the verify bot's OWN state (used to render the bonded-members list, to
// survive a restart, and to rate-limit). It is NOT a shared source of truth:
// the sentinel bot resolves bonded members from the chain on demand
// (getBondByDiscord), so the two bots are not coupled through this file.
//
// The chain is always authoritative. This cache can lag (e.g. a bond refunded
// or liquidated on-chain) and is refreshed opportunistically; never gate a
// money decision on it.
//
// Standard-library only. Writes are atomic (temp file + rename) so a concurrent
// reader always sees a whole, valid file.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Record is one verified bond membership (all fields are public on-chain data).
type Record struct {
	DiscordID  string `json:"discord_id"`
	Username   string `json:"username"`
	Wallet     string `json:"wallet"` // N-address
	BondID     string `json:"bond_id"`
	TxID       string `json:"tx_id"`
	VerifiedAt int64  `json:"verified_at_ms"`
}

// Store is a concurrency-safe map of Discord ID -> Record backed by a JSON file.
type Store struct {
	mu      sync.RWMutex
	path    string
	records map[string]Record
}

type fileModel struct {
	Version int               `json:"version"`
	Records map[string]Record `json:"records"`
}

// Open loads the store from path, creating an empty one if the file is absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, records: map[string]Record{}}
	raw, err := os.ReadFile(path) // #nosec G304 G703 -- operator-configured store path, not attacker input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("open store %q: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	var fm fileModel
	if err := json.Unmarshal(raw, &fm); err != nil {
		return nil, fmt.Errorf("parse store %q: %w", path, err)
	}
	if fm.Records != nil {
		s.records = fm.Records
	}
	return s, nil
}

// Get returns the record for a Discord ID.
func (s *Store) Get(discordID string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[discordID]
	return r, ok
}

// Put upserts a record and persists the store atomically.
func (s *Store) Put(r Record) error {
	s.mu.Lock()
	s.records[r.DiscordID] = r
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// Delete removes a record (e.g. after an on-chain refund/liquidation is
// observed) and persists.
func (s *Store) Delete(discordID string) error {
	s.mu.Lock()
	delete(s.records, discordID)
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// All returns every record, sorted by verification time (oldest first).
func (s *Store) All() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VerifiedAt < out[j].VerifiedAt })
	return out
}

// Count returns the number of records.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// saveLocked writes the store atomically. Caller holds s.mu.
func (s *Store) saveLocked() error {
	fm := fileModel{Version: 1, Records: s.records}
	data, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".ftb-bonds-*.tmp")
	if err != nil {
		return fmt.Errorf("store temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("store rename: %w", err)
	}
	return nil
}

// NowMillis is a small helper for VerifiedAt.
func NowMillis() int64 { return time.Now().UnixMilli() }
