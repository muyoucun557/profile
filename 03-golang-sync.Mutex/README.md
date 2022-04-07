# sync.Mutex

golang提供的并发原语，互斥锁。下面是结构体代码
```Golang
type Mutex struct {
	state int32
	sema  uint32  // 信号量
}
```

## Lock方法
```Golang
func (m *Mutex) Lock() {
	// 先用CAS来尝试加锁，不一上来就使用信号量，节省开销
	if atomic.CompareAndSwapInt32(&m.state, 0, mutexLocked) {
		if race.Enabled {
			race.Acquire(unsafe.Pointer(m))
		}
		return
	}
	// 如果未获取到锁，就执行下一步
	m.lockSlow()
}
```

没看明白，下次继续。
大致逻辑如下
1. 使用CAS加锁，减少开销。
2. 如果CAS未加上，则开启自旋锁，只自旋几次，不会无休止的自旋下去
3. 如果自旋未锁上，则使用信号量加锁




