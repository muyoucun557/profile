# Go并发模式：Pipelines和Cancellation

Go的并发原语能够很容易的构造出一个``流数据的管道``用于提升I/O和多CPUs的效率。This article presents examples of such pipelines, highlights subtleties that arise when operations fail, and introduces techniques for dealing with failures cleanly.

问题一：管道怎么提升I/O效率和多核CPU效率。

## 什么是管道

一个管道是一系列由channel连接起来的stage。每一个stage是一组运行相同函数的gorotuine。在每个stage中，goroutine会
1. 从上游的输入管道接收值
2. 执行一些函数，通常会产生新的值
3. 发送值到下游

除了第一个stage和最后一个stage之外，其他的stage都会有任意数量的输入channel和输出channel。第一个stage有时被称为``source或者producer``，最后一个stage被称为``sink或者consumer``。

## Squaring Numbers
下面是一个给一组数字求平方的例子。
```Go
func gen(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        for _, n := range nums {
            out <- n
        }
        close(out);
    }()
    return out
}

func sq(in <-chan int) <-chan int {
   out := make(chan int)
   go func() {
       for n := range in {
           out <- n * n
       }
       close(out)
   }()
   return out
}

func main() {
    c := gen(1,2,3)
    for n := range sq(c) {
        fmt.Println(n)
    }
}
```

## Fan-out,Fan-in
扇出扇入模式
多个函数对一个channel进行读操作直到channel被关闭，这种被称为扇出。它提供了让CPU或者IO进行并行操作的方式。

一个函数从多个输入channel中读，且将读到的数据发送到一个channel中，直到这些输入channel被关闭。这中被称为扇入。
```Go
func main() {
    in := gen(2,3)
    c1 := sq(in)
    c2 := sq(in)

    for n := range merge(c1,c2) {
        fmt.Println(n)
    }
}

func merge(cs ...<-chan int) <-chan int {
    var wg sync.WaitGroup
    wg.Add(len(cs))

    result := make(chan int)
    for _, c := range cs {
        go func() {
            for n := range c {
                result <- n
            }
            wg.Done()
        }()
    }
    go func() {
        wg.Wait()
    }()
    return result
}
```
## Stopping short
止损模式

对于流处理函数有一个模式
1. 当发送工作全部完成之后，stage关闭它们的输出channel(outbound channel)
2. stage一直从输入channel中读取数据直到这些channel被关闭
   
## Explicit cancellation

当main函数决定离开，也就意味着不再接收来自上游的数据，那么main函数需要通知上游函数禁止传输数据。
```Go
func main() {
  done := make(chan struct{})
  defer close(done)

  in := gen(done,1,2,3)
  c1 := sq(done, in)
  c2 := sq(done, in)
  
  out := merge(done, c1, c2)
  for n := range out {
    fmt.Println(n)
  }
  
}
```
