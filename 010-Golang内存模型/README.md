# Golang内存模型

Golang的内存模型是解决并发相关问题的，并非内存分配原理。
Golang官方的介绍是<https://golang.google.cn/ref/mem>

## What

当两个goroutine同时对共享变量进行读写的时候，Go内存模型定义了在什么情况下读goroutine能确保读到写goroutine的写入值。

## Happen Before

在同一个goroutine中，对多个变量进行读写，如果重排代码不影响代码逻辑，编译器和处理器可能会进行重排（优化）。这样会导致在另外一个goroutine中看到的执行顺序是不一样的。

为了表示读写顺序，引入了``Happen Before``这样的概念，用于表示``一小段内存命令的执行顺序``。``Happen Before``定义了两个规则

规则一：假设存在一个共享变量v，如果满足下面两个条件，则读操作r能观察到写操作w写入的值。
1. r不在w之前发生
2. 不存在其他的w'在w之后发生，也不存在w'在r之前发生

规则二：为了保证读操作r能读到写操作w写入的值，需要满足下面两个条件。
1. r发生在w之后
2. 其他的写操作要么发生在w之前，要么发生在r之后

ps:这两个规则我的理解是一样的

## 如何能确保顺序

Golang里面提供了channel、lock、once、cas这些机制确保顺序。
