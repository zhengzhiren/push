package safemap

import (
	"sync"
)

type SafeMap struct {
	lock *sync.RWMutex
	mp   map[interface{}]interface{}
}

// NewSafeMap return new safemap.
func NewSafeMap() *SafeMap {
	return &SafeMap{
		lock: new(sync.RWMutex),
		mp:   make(map[interface{}]interface{}),
	}
}

// Get from maps return the k's value.
func (this *SafeMap) Get(k interface{}) interface{} {
	this.lock.RLock()
	r, ok := this.mp[k]
	if !ok {
		r = nil
	}
	this.lock.RUnlock()
	return r
}

// Maps the given key and value.
// Returns false if the key is already in the map and changes nothing.
func (this *SafeMap) Set(k interface{}, v interface{}) bool {
	this.lock.Lock()
	r := true
	if val, ok := this.mp[k]; !ok {
		this.mp[k] = v
	} else if val != v {
		this.mp[k] = v
	} else {
		r = false
	}
	this.lock.Unlock()
	return r
}

// Returns true if k is exist in the map.
func (this *SafeMap) Check(k interface{}) bool {
	this.lock.RLock()
	r := true
	if _, ok := this.mp[k]; !ok {
		r = false
	}
	this.lock.RUnlock()
	return r
}

// Delete the given key and value.
func (this *SafeMap) Delete(k interface{}) {
	this.lock.Lock()
	delete(this.mp, k)
	this.lock.Unlock()
}

// Items returns all items in safemap
func (this *SafeMap) Items() map[interface{}]interface{} {
	this.lock.RLock()
	r := this.mp
	this.lock.RUnlock()
	return r
}

// Size returns the size of the safemap
func (this *SafeMap) Size() int {
	this.lock.RLock()
	r := len(this.mp)
	this.lock.RUnlock()
	return r
}
