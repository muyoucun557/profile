# sync.Map

内置的map是并发不安全的，标准库sync提供了一个并发安全的Map。
如果要实现并发安全，那么得加锁。与LockFree相比，加锁的性能开销比较大，sync.Map为次做了一些优化。

## sync.Map结构体

```Golang
type Map struct {
	mu Mutex
	read atomic.Value // readOnly
	dirty map[interface{}]*entry
	misses int
}

// An entry is a slot in the map corresponding to a particular key.
type entry struct {
	p unsafe.Pointer // *interface{}
}

// readOnly is an immutable struct stored atomically in the Map.read field.
type readOnly struct {
	m       map[interface{}]*entry
	amended bool // true if the dirty map contains some key not in m.
}
```
上面是sync.Map中最重要的三个结构体。在sync.Map中，dirty用于存储数据，如果一旦读比较多，那么从read中读取数据，对read进行读写不需要加锁，dirty读写需要加锁。
同时readOnly中的m与dirty是相同类型的。


## 如何从sync.Map中查询数据
```Golang
func (m *Map) Load(key interface{}) (value interface{}, ok bool) {
    // 1. 判断read中有没有
	read, _ := m.read.Load().(readOnly)
	e, ok := read.m[key]        // NOTICE: Load OPERATION 1
    // 2. 如果read中没有，且dirty中存在read中不存在的k-v。read.amended用于标识dirty是有read中不存在的k-v。
    // 什么情况下read.amended会发生变更？  NOTICE:疑问一
	if !ok && read.amended {
    // 3. dirty是一个map，并发不安全，需要加锁
		m.mu.Lock()				//NOTICE: Load OPERATION 2
		// Avoid reporting a spurious miss if m.dirty got promoted while we were
		// blocked on m.mu. (If further loads of the same key will not miss, it's
		// not worth copying the dirty map for this key.)
    // 4. 再次判断read中是否存在（为什么要再次判断？NOTICE:疑问二） 
		read, _ = m.read.Load().(readOnly)      
		e, ok = read.m[key]
    // 5. 如果read中没有，且dirty中有read中不存在的k-v
    // 那么尝试从dirty中读取
		if !ok && read.amended {
			e, ok = m.dirty[key]
			// Regardless of whether the entry was present, record a miss: this key
			// will take the slow path until the dirty map is promoted to the read
			// map.
    //  6. 由于在read中没有命中，对未命中行为进行计次。
			m.missLocked()
		}
		m.mu.Unlock()
	}
	if !ok {
		return nil, false
	}
    // 7. 返回数据
	return e.load()
}
```

<strong>假设这是一个刚创建的sync.Map，且查询的是一个map中不存在的k-v，来看看发生了什么？</strong>
read.amended是初始值false（初始状态下，read和dirty都是为空，不存在dirty中有read中不存在的k-v）。
那么会执行下面的代码
```Golang
		if !ok && read.amended {
			e, ok = m.dirty[key]
			// Regardless of whether the entry was present, record a miss: this key
			// will take the slow path until the dirty map is promoted to the read
			// map.
			m.missLocked()
		}
```

来看看``missLocked``做了什么
```Golang
func (m *Map) missLocked() {
    // 1. misses++，misses是计数器，在上面的场景中，是在read中未命中k的场景，misses用于统计这个场景的次数
	m.misses++
	if m.misses < len(m.dirty) {
		return
	}
    // 2. 当未命中的次数与dirty中k-v的数量相等时
    // 将dirty存储到read中
    // 将dirty置为nil
    // 重置计数器
	m.read.Store(readOnly{m: m.dirty})
	m.dirty = nil
	m.misses = 0
}
```
从上面的代码来看，当read中未命中的次数达到了dirty中k-v数量的时候，dirty中数据全部迁移到read中。为什么要这么做？

## 如何向sync.Map中存储数据

```Golang
func (m *Map) Store(key, value interface{}) {
    // 1. 判断read中是否有数据，如果有，那么直接更新read中的。
	read, _ := m.read.Load().(readOnly)
	if e, ok := read.m[key]; ok && e.tryStore(&value) {
		return
	}

    // 如果read中没有，需要加锁。
	m.mu.Lock()
    // 2. 再次判断在不在readonly中(疑问三：和疑问二是相同的)
	read, _ = m.read.Load().(readOnly)
	if e, ok := read.m[key]; ok {
    // 3. 如果在read中
		if e.unexpungeLocked() {
			// The entry was previously expunged, which implies that there is a
			// non-nil dirty map and this entry is not in it.
			m.dirty[key] = e
		}
		e.storeLocked(&value)
	} else if e, ok := m.dirty[key]; ok {
    // 4. 如果在dirty中
		e.storeLocked(&value)
	} else {
    // 5. 如果即不在read中，也不在dirty中
		if !read.amended {
			// We're adding the first new key to the dirty map.
			// Make sure it is allocated and mark the read-only map as incomplete.
			m.dirtyLocked()
			m.read.Store(readOnly{m: read.m, amended: true})
		}
		m.dirty[key] = newEntry(value)
	}
	m.mu.Unlock()
}
```

假设是新k-v，那么既不在read中，也不在dirty中。执行的代码如下
```Golang
	if !read.amended {
			// We're adding the first new key to the dirty map.
			// Make sure it is allocated and mark the read-only map as incomplete.
		m.dirtyLocked()
        // 对于新增的k-v，read和dirty中都没有，那么会存储到dirty中
        // 此时dirty中就有了read中不存在的数据了，那么read.amended标识位就需要发生变更
        // 下面的操作是为了变更标识位
		m.read.Store(readOnly{m: read.m, amended: true})
	}
	m.dirty[key] = newEntry(value)
```
看看``dirtyLocked``干了什么
```Golang
func (m *Map) dirtyLocked() {
    // 1. 如果dirty不为空，直接返回
	if m.dirty != nil {
		return
	}

    // 2.如果dirty为空，遍历read，将read中的数据复制到dirty中
	read, _ := m.read.Load().(readOnly)
	m.dirty = make(map[interface{}]*entry, len(read.m))
	for k, e := range read.m {
    // 3. tryExpungeLocked执行了什么逻辑？
		if !e.tryExpungeLocked() {
			m.dirty[k] = e
		}
	}
}
```

## 解答疑问

<strong>疑问一：什么情况下read.amended会发生变更？</strong>
read.amended用于标识是否存在某些k-v只存在于dirty中而不在read中。
当新增一个k-v的时候，会存储到dirty中，此时就出现了上述场景，read.amended会发生变化。

<strong>疑问二：再次判断read中是否存在（为什么要再次判断？）</strong>
在Load OPERATION 1和Load OPERATION 2中间，可能存在其他goroutine调用了Load方法且抢占到了锁，且将dirty迁移到了read中。


## 总结一

从上面两个方法来看，流程如下：
1. 如果read中有，从read中操作
2. 如果read中没有，从dirty操作（需要加锁）
3. read中未命中的次数变多（表明读多），就把dirty中数据赋值到read中。read中读写开销小一些。


## 如何从sync.Map中删除一个k-v

```Golang
func (m *Map) LoadAndDelete(key interface{}) (value interface{}, loaded bool) {
	// 1. 判断read中是否存在
	read, _ := m.read.Load().(readOnly)
	e, ok := read.m[key]
	// 2. read中不存在，且dirty中存在不在read中k-v
	if !ok && read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnly)
		e, ok = read.m[key]
		if !ok && read.amended {
			e, ok = m.dirty[key]
	// 3. 如果k-v在dirty中，在dirty中删除
			delete(m.dirty, key)
			// Regardless of whether the entry was present, record a miss: this key
			// will take the slow path until the dirty map is promoted to the read
			// map.
			m.missLocked()
		}
		m.mu.Unlock()
	}
	if ok {
		return e.delete()
	}
	return nil, false
}
```
看看```e.delete```函数做了什么
```Golang
func (e *entry) delete() (value interface{}, ok bool) {
	// 通过CAS将entry.p置为nil
	for {
		p := atomic.LoadPointer(&e.p)
		if p == nil || p == expunged {
			return nil, false
		}
		if atomic.CompareAndSwapPointer(&e.p, p, nil) {
			return *(*interface{})(p), true
		}
	}
}
```
如果dirty中存在，将该k-v从dirty中删除，同时将entry中的p置为nil。
如果read中存在，只将entry.p置为nil。

有一个场景很重要：
当新增一个k-v的时候，如果dirty为nil，会将read中的k-v复制到dirty中，由于在read中执行删除操作的时候只会将entry.p置为nil，对于这样的k-v，无需从read中复制到dirty中。在复制的时候调用了``tryExpungeLocked``方法。
```Golang
func (e *entry) tryExpungeLocked() (isExpunged bool) {
	p := atomic.LoadPointer(&e.p)
	// 如果p是nil（表示被删除了）
	// 将e.p设置成expunged？？？？？这是为什么？
	// 从expunged的注释表明：该值表示k-v从dirty中删除。用nil表示不行吗？疑问三
	for p == nil {
		if atomic.CompareAndSwapPointer(&e.p, nil, expunged) {
			return true
		}
		p = atomic.LoadPointer(&e.p)
	}
	return p == expunged
}
```

## 解答疑问

<strong>疑问三：</strong>

参看Store方法
```Golang
func (m *Map) Store(key, value interface{}) {
	read, _ := m.read.Load().(readOnly)
	if e, ok := read.m[key]; ok && e.tryStore(&value) {
		return
	}

	m.mu.Lock()
	read, _ = m.read.Load().(readOnly)
	if e, ok := read.m[key]; ok {
		if e.unexpungeLocked() {	//NOTICE: OPERATION C
			// The entry was previously expunged, which implies that there is a
			// non-nil dirty map and this entry is not in it.
			m.dirty[key] = e
		}
		e.storeLocked(&value)
	}
```
在OPERATION C中，判断e在不在dirty中，如果e.p是expunged，那么表示不在dirty中，需要将e重新放到dirty中。

<strong>疑问四：只用read行不行？</strong>


