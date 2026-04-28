package drivers

import (
	"errors"
	"github.com/bradfitz/gomemcache/memcache"
	"time"
)

type Memcached struct {
	Addr   string           `json:"addr"`
	Handle *memcache.Client `json:"-"`
}

func (m *Memcached) Init() error {
	m.Handle = memcache.New(m.Addr)
	return m.Handle.Ping()
}

func (m *Memcached) Close() error {
	return nil
}

func (m *Memcached) Get(bucket, key string) ([]byte, error) {
	if err := m.undefined(); err != nil {
		return nil, err
	}
	item, err := m.Handle.Get(m.prefix(bucket) + key)
	if err != nil {
		return nil, err
	}
	return item.Value, nil
}

func (m *Memcached) Set(bucket, key string, val []byte, ttl time.Duration) error {
	if err := m.undefined(); err != nil {
		return err
	}
	return m.Handle.Set(&memcache.Item{Key: m.prefix(bucket) + key, Value: val, Expiration: int32(ttl.Seconds())})
}

func (m *Memcached) Delete(bucket, key string) error {
	if err := m.undefined(); err != nil {
		return err
	}
	return m.Handle.Delete(m.prefix(bucket) + key)
}

// DeleteByPrefix deletes all keys with the given prefix.
// Note: Memcached doesn't support prefix-based deletion natively,
// so this implementation uses DeleteAll which is more aggressive.
// For production use, consider maintaining a key registry or using a different cache driver.
func (m *Memcached) DeleteByPrefix(bucket, prefix string) error {
	if err := m.undefined(); err != nil {
		return err
	}
	// Memcached doesn't support prefix-based deletion, so we use DeleteAll
	// This is a limitation of Memcached - it doesn't support pattern matching
	return m.Handle.DeleteAll()
}

func (m *Memcached) ClearBucket(bucket string) error {
	if err := m.undefined(); err != nil {
		return err
	}
	return m.Handle.DeleteAll()
}

func (m *Memcached) prefix(bucket string) string {
	return bucket + ":"
}

func (m *Memcached) undefined() error {
	if m.Handle == nil {
		return errors.New("client uninitialized or is closed")
	}
	return nil
}

func (m *Memcached) Size() (int64, error) {
	return -1, nil
}
