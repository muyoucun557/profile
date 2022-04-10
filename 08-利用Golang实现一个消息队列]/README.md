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


