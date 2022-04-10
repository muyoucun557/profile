# 利用Golang实现一个消息队列

消息队列是先进先出，需要利用队列来实现。同时向队列中puush、pop数据的时候需要防止并发，可以采用锁机制来实现。加锁会增加开销，可以使用CAS来实现。

## 实现


```Go
type node struct {
	val  interface{}
	next unsafe.Pointer
}

type freeLockQueue struct {
	head unsafe.Pointer
	tail unsafe.Pointer
}
```

```Go
func NewFreeLockQueue() Queue {
	q := new(freeLockQueue)
  // 这里创建了一个head节点，为了实现方便，head节点不存储任何业务数据
	q.head = unsafe.Pointer(new(node))
	q.tail = q.head
	return q
}
```


```Go
// 向队列里增加一个元素
// 向尾部追加
func (self *freeLockQueue) Push(v interface{}) {
	newRecord := unsafe.Pointer(&node{v, nil})
	var tail unsafe.Pointer
	for {
		tail = self.tail

		// CAS操作：判断tail.next是否为nil，如果为nil，那么将newRecord赋值给tail.next
		if atomic.CompareAndSwapPointer(&(*node)(tail).next, nil, newRecord) {  // NOTICE:这里记为OPERATION A
			break
		}
	}
  self.tail = newRecord       // NOTICE: OPERATION B
}


func (self *freeLockQueue) Pop() interface{} {
	var head unsafe.Pointer

	for {
		head = self.head

		next := (*node)(head).next
		if next == nil {
			return nil
		}

    // 通过CAS来保证：没有其他goroutine改变self.head的情况下，将head右移
		if atomic.CompareAndSwapPointer(&self.head, head, next) {
			return (*node)(next).val
		}
	}
}
```

## 如果Goroutine在OPERATION A之后，会无故挂起，那么怎么办？
在Golang不知道会不会存在这种情况，如果是C++或者JAVA编写的程序可能会出现这种情况。操作系统线程调度，高优先级的会抢占低优先级的。

如果一旦挂起，那么OPERATION B会等到goroutine唤醒之后才会执行，在这段时间内其他goroutine在执行OPERATION A的时候，会陷入循环。此时队列会陷入阻塞状态。

为了防止这种情况发生，应当从tail向后遍历，直到tail->next为nil，然后再进行CAS操作。

```Go
// 向队列里增加一个元素
// 向尾部追加
func (self *freeLockQueue) Push(v interface{}) {
	newRecord := unsafe.Pointer(&node{v, nil})
	var tail unsafe.Pointer
	for {
		tail = self.tail
		if (*node)(tail).next != nil {
			tail = (*node)(tail).next
		}

		// CAS操作：判断tail.next是否为nil，如果为nil，那么将newRecord赋值给tail.next
		if atomic.CompareAndSwapPointer(&(*node)(tail).next, nil, newRecord) {
			break
		}
	}
	atomic.CompareAndSwapPointer(&self.tail, tail, newRecord) // OPERATION C
}
```

在这里需要注意一下OPERATION B，因为存在goroutine无故阻塞，那么这里需要用CAS。
