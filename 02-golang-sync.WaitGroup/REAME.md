# sync.WaitGroup

sync.WaitGroup实现了等待一组goroutine结束的功能。main goroutine调用Add方法用于设置需要等待goroutine的数量。于此同时调用Wait方法的goroutine会被阻塞，直到所有的goroutine都结束了。
下面给出例子:
```Golang
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

