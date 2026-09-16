package store

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// BadgerStore implements Store on top of an embedded BadgerDB database.
//
// Key layout: <metric-name> 0x00 <8-byte big-endian unix-nano timestamp>.
// Big-endian timestamps make lexicographic key order equal chronological
// order, so prefix scans return points in ascending time order directly.
// Values are 1 byte of metric type ('c' counter, 'g' gauge) followed by
// 8-byte big-endian float64 — the key alone cannot recover the type.
type BadgerStore struct {
	db *badger.DB
}

var _ Store = (*BadgerStore)(nil)

// OpenBadger opens (creating if needed) a BadgerDB store in dir.
func OpenBadger(dir string) (*BadgerStore, error) {
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		return nil, fmt.Errorf("open storage at %q: %w", dir, err)
	}
	return &BadgerStore{db: db}, nil
}

// OpenBadgerInMemory returns a BadgerDB store that keeps data in memory only.
// Intended for tests.
func OpenBadgerInMemory() (*BadgerStore, error) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		return nil, fmt.Errorf("open in-memory storage: %w", err)
	}
	return &BadgerStore{db: db}, nil
}

func keyPrefix(name string) []byte {
	return []byte(name + "\x00")
}

func keyFor(name string, ts time.Time) []byte {
	k := make([]byte, 0, len(name)+1+8)
	k = append(k, keyPrefix(name)...)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(ts.UnixNano()))
	return append(k, buf[:]...)
}

// Write persists a single data point.
func (s *BadgerStore) Write(p Point) error {
	key := keyFor(p.Name, p.Timestamp)
	val := make([]byte, 0, 9)
	switch p.Type {
	case "counter":
		val = append(val, 'c')
	case "gauge":
		val = append(val, 'g')
	default:
		return fmt.Errorf("unknown metric type %q", p.Type)
	}
	var bits [8]byte
	binary.BigEndian.PutUint64(bits[:], math.Float64bits(p.Value))
	val = append(val, bits[:]...)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, val)
	})
}

// Query returns stored points for name within [from, to] inclusive,
// ordered by timestamp ascending.
func (s *BadgerStore) Query(name string, from, to time.Time) ([]Point, error) {
	points := []Point{}
	prefix := keyPrefix(name)
	start := keyFor(name, from)
	end := keyFor(name, to)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(start); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			if bytes.Compare(item.Key(), end) > 0 {
				break // past the inclusive end of the window
			}
			var tsNano uint64
			var value float64
			var typ string
			key := item.KeyCopy(nil)
			if len(key) != len(prefix)+8 {
				continue // defensive: skip malformed keys
			}
			tsNano = binary.BigEndian.Uint64(key[len(prefix):])
			err := item.Value(func(v []byte) error {
				if len(v) != 9 {
					return fmt.Errorf("corrupt value for key %q: %d bytes", key, len(v))
				}
				switch v[0] {
				case 'c':
					typ = "counter"
				case 'g':
					typ = "gauge"
				default:
					return fmt.Errorf("corrupt type tag for key %q: %q", key, v[0])
				}
				value = math.Float64frombits(binary.BigEndian.Uint64(v[1:]))
				return nil
			})
			if err != nil {
				return err
			}
			points = append(points, Point{
				Name:      name,
				Type:      typ,
				Value:     value,
				Timestamp: time.Unix(0, int64(tsNano)).UTC(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("query metric %q: %w", name, err)
	}
	return points, nil
}

// Close cleanly closes the underlying database, flushing pending writes.
func (s *BadgerStore) Close() error {
	return s.db.Close()
}
