# Go并发模式：Pipelines和Cancellation

Go的并发原语能够很容易的构造出一个``流数据的管道``用于提升I/O和多CPUs的效率。This article presents examples of such pipelines, highlights subtleties that arise when operations fail, and introduces techniques for dealing with failures cleanly.

问题一：管道怎么提升I/O效率和多核CPU效率。

## 什么是管道

一个管道是一系列由channel连接起来的stage。每一个stage是一组运行相同函数的gorotuine。在每个stage中，goroutine会
1. 从上游的输入管道接收值
2. 执行一些函数，通常会产生新的值
3. 发送值到下游

除了第一个stage和最后一个stage之外，其他的stage都会有任意数量的输入channel和输出channel。
