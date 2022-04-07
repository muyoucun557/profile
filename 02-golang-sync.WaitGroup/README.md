# sync.WaitGroup

sync.WaitGroup实现了等待一组goroutine结束的功能。main goroutine调用Add方法用于设置需要等待goroutine的数量。于此同时调用Wait方法的goroutine会被阻塞，直到所有的goroutine都结束了。
下面给出例子:

```Golang
// 设置等待5个goroutine完成
// 另外两个goroutine等待这5个goroutine完成
var wg sync.WaitGroup
	wg.Add(5)

	// 两个goroutine等待
	for i := 0; i < 2; i++ {
		go func(index int) {
			wg.Wait()
			fmt.Printf("index: %d\n", index)
		}(i)
	}

	// 等待下面5个goroutine完成
	for i := 0; i < 5; i++ {
		go func(index int) {
			t := rand.Intn(5)
			time.Sleep(time.Duration(t) * time.Second)
			wg.Done()
			fmt.Printf("%d finished\n", index)
		}(i)
	}
	time.Sleep(10 * time.Second)
```

## 信号量与runtime_Semrelease、runtime_Semacquire函数

想要去获取临界区的权限，有两种策略。第一种是不断的去尝试获取临界区，看能不能获取到权限，这种方式叫自旋。第二种是让出CPU，将自己加入到临界区的等待队列中，如果前一个线程释放了临界区，由os系统来调度唤起当前线程，让当前线程获取到临界区的权限，这种方式是操作系统提供的同步原语。

对比一下这两种方案的优缺点：
自旋方案：不让出CPU，一直自旋，直到获取到权限
信号量方案：发起系统调用，涉及到线程休眠、唤起、切换，开销大
如果能立马获取到临界区权限，很显然，自旋方案更优。
如果不能立马获取到临界区权限，信号量方案更优。

一般的情况下，可以结合两种方案的优点，先自旋n次，如果还未获取到锁，那么再采用信号量方案。

golang中对于这两种方案均做出了实现：
for + CAS实现了自旋锁
信号量机制提供了同步原语，这里的同步原语是由golang的runtime实现的。

golang中有一个叫semtable的长度为251的数组，来管理所有的信号量，其每个节点维护了一个等待队列。
``runtime_Semacquire``函数就是将当前的goroutine加入到等待信号的队列的。
``runtime_Semrelease``唤醒等待队列中的goroutine，每调用一次，唤醒一个。
关于信号量的实现可以参看 https://github.com/cch123/golang-notes/blob/master/semaphore.md

## 计数器

在WaitGroup中需要有两个计数器：
一个用于计数还剩多少个goroutine完成，称这个计数器叫counter。
一个用于计数有多少个waiter，称这个计数器叫waiter。
```Golang
type WaitGroup struct {
	noCopy noCopy

	// 64-bit value: high 32 bits are counter, low 32 bits are waiter count.
	// 64-bit atomic operations require 64-bit alignment, but 32-bit
	// compilers do not ensure it. So we allocate 12 bytes and then use
	// the aligned 8 bytes in them as state, and the other 4 as storage
	// for the sema.
	state1 [3]uint32
}
```
上面是WaitGroup的结构体，state1字段是计数器。state1是一个包含3个uint32元素的数组，其中不仅存储了计数器，还存储了信号量对应的地址。每个计数器的长度是32bit。
为了节省内存，两个计数器放在一起的（内存对齐）。
在64bit机器中，这3个变量的顺序是 counter、waiter、sema
在32bit机器中，这3个变量的顺序是 sema、counter、waiter
为什么要这么做呢？
为了节省内存，counter、waiter是作为一个64bit对象整体进行操作的，为了防止并发，使用的是atomic原子操作。在32bit机器中，atomic对一个64bit对象进行操作的时候，必须要求这个对象的地址是64bit对齐的（https://pkg.go.dev/sync/atomic#pkg-note-BUG）https://xargin.com/。

## Add方法

```Golang
func (wg *WaitGroup) Add(delta int) {
  // 获取计数器的地址和信号量的地址
	statep, semap := wg.state()
	if race.Enabled {
		_ = *statep // trigger nil deref early
		if delta < 0 {
			// Synchronize decrements with Wait.
			race.ReleaseMerge(unsafe.Pointer(wg))
		}
		race.Disable()
		defer race.Enable()
	}
	state := atomic.AddUint64(statep, uint64(delta)<<32)
  // 分别得到counter的值和waiter的值
	v := int32(state >> 32)
	w := uint32(state)
	if race.Enabled && delta > 0 && v == int32(delta) {
		// The first increment must be synchronized with Wait.
		// Need to model this as a read, because there can be
		// several concurrent wg.counter transitions from 0.
		race.Read(unsafe.Pointer(semap))
	}
  // 不允许counter计数器小于0
	if v < 0 {
		panic("sync: negative WaitGroup counter")
	}
  // Wait和Add并发调用了，不允许这种情况发生
	if w != 0 && delta > 0 && v == int32(delta) {
		panic("sync: WaitGroup misuse: Add called concurrently with Wait")
	}

	if v > 0 || w == 0 {
		return
	}
	// This goroutine has set counter to 0 when waiters > 0.
	// Now there can't be concurrent mutations of state:
	// - Adds must not happen concurrently with Wait,
	// - Wait does not increment waiters if it sees counter == 0.
	// Still do a cheap sanity check to detect WaitGroup misuse.
  // 在最后一次调用Add(-1)的时候，也就是v==0的时候且存在waiting groutine的时候，会走到这里
	if *statep != state {
		panic("sync: WaitGroup misuse: Add called concurrently with Wait")
	}
	// Reset waiters count to 0.
	*statep = 0
  // 循环调用runtime_Semrelease，释放信号量，让waiting goroutine继续执行
	for ; w != 0; w-- {
		runtime_Semrelease(semap, false, 0)
	}
}
```

## Wait方法
```Golang
// Wait blocks until the WaitGroup counter is zero.
func (wg *WaitGroup) Wait() {
  // 获取计数器和信号量地址
	statep, semap := wg.state()
	if race.Enabled {
		_ = *statep // trigger nil deref early
		race.Disable()
	}
  // 自旋锁
	for {
		state := atomic.LoadUint64(statep)
		v := int32(state >> 32)
		w := uint32(state)
		if v == 0 {
			// Counter is 0, no need to wait.
			if race.Enabled {
				race.Enable()
				race.Acquire(unsafe.Pointer(wg))
			}
			return
		}
		// Increment waiters count.
		if atomic.CompareAndSwapUint64(statep, state, state+1) {
			if race.Enabled && w == 0 {
				// Wait must be synchronized with the first Add.
				// Need to model this is as a write to race with the read in Add.
				// As a consequence, can do the write only for the first waiter,
				// otherwise concurrent Waits will race with each other.
				race.Write(unsafe.Pointer(semap))
			}
      // 将当前goroutine加入到信号量的等待队列中，且一直阻塞
			runtime_Semacquire(semap) // Add方法在释放信号量之前，将statep置为了0
			if *statep != 0 {
				panic("sync: WaitGroup is reused before previous Wait has returned")
			}
			if race.Enabled {
				race.Enable()
				race.Acquire(unsafe.Pointer(wg))
			}
			return
		}
	}
}
```

