/*
Copyright 2014 Workiva, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package queue

import (
	"runtime"
	"sync/atomic"
	"time"
)

// roundUp takes a uint64 greater than 0 and rounds it up to the next
// power of 2.
func roundUp(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}

type node struct {
	position uint64
	data     interface{}
}

type nodes []node

// RingBuffer is a MPMC buffer that achieves threadsafety with CAS operations
// only.  A put on full or get on empty call will block until an item
// is put or retrieved.  Calling Dispose on the RingBuffer will unblock
// any blocked threads with an error.  This buffer is similar to the buffer
// described here: http://www.1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue
// with some minor additions.
type RingBuffer struct {
	_padding0      [8]uint64
	tail           uint64 		// 队尾指针
	_padding1      [8]uint64
	head           uint64 		// 队头指针
	_padding2      [8]uint64
	mask, disposed uint64
	_padding3      [8]uint64
	nodes          nodes
}

func (rb *RingBuffer) init(size uint64) {
	size = roundUp(size)
	rb.nodes = make(nodes, size)
	for i := uint64(0); i < size; i++ {
		rb.nodes[i] = node{position: i}
	}
	rb.mask = size - 1 // so we don't have to do this with every put/get operation
}

// Put adds the provided item to the tail.  If the tail is full, this
// call will block until an item is added to the tail or Dispose is called
// on the tail.  An error will be returned if the tail is disposed.
func (rb *RingBuffer) Put(item interface{}) error {
	_, err := rb.put(item, false)
	return err
}

// Offer adds the provided item to the tail if there is space.  If the tail
// is full, this call will return false.  An error will be returned if the
// tail is disposed.
func (rb *RingBuffer) Offer(item interface{}) (bool, error) {
	return rb.put(item, true)
}

// 入队函数
// 	1. 获取插入的位置 pos = rb.tail
//	2. 获取 pos 处的 buffer node, 即 n = &rb.nodes[pos&rb.mask]
//	3. 判断 pos 是否等于 n.position
//		3.1 若相等，尝试占领 pos 这个位置（cas），让 rb.tail 加一，跳出循环
//		3.2 若 n.position < rb.tail, 出错，panic
//		3.3 若 n.position > rb.tail, 说明 n 处已经被写入数据，更新 pos , 重新进入第2步
//  4. 调用 atomic.StoreUint64(&n.position, tail+1) 将 n.position 置为 tail+1
//
//
//
// 疑问
//  为何要通过 atomic.StoreUint64(&n.position, tail+1) 将 node 的 position 设为 pos+1 ？
//
// 个人见解
// 	主要作用是标记 pos 处已经放置数据了。
// 	若其他线程获得相同的 pos ，当其再比较 pos 和 sequence 时将不会再相等，就不会再次在相同的 pos 处写入数据。
// 	另外，此处的 pos+1 和出队时的判断 dif := seq - (pos + 1) 相对应。
//
func (rb *RingBuffer) put(item interface{}, offer bool) (bool, error) {
	var n *node
	tail := atomic.LoadUint64(&rb.tail)
L:
	for {

		// 是否 disposed
		if atomic.LoadUint64(&rb.disposed) == 1 {
			return false, ErrDisposed
		}

		// 取队尾节点，用于放置新数据 item
		n = &rb.nodes[tail&rb.mask]
		pos := atomic.LoadUint64(&n.position)
		switch diff := pos - tail; {
		case diff == 0:
			// cas 成功，则跳出循环，否则继续循环
			if atomic.CompareAndSwapUint64(&rb.tail, tail, tail+1) {
				break L
			}
		case diff < 0:
			panic(`Ring buffer in a compromised state during a put operation.`)
		default:
			tail = atomic.LoadUint64(&rb.tail)
		}

		if offer {
			return false, nil
		}

		runtime.Gosched() // free up the cpu before the next iteration
	}

	n.data = item
	atomic.StoreUint64(&n.position, tail+1)
	return true, nil
}

// Get will return the next item in the tail.  This call will block
// if the tail is empty.  This call will unblock when an item is added
// to the tail or Dispose is called on the tail.  An error will be returned
// if the tail is disposed.
func (rb *RingBuffer) Get() (interface{}, error) {
	return rb.Poll(0)
}

// Poll will return the next item in the tail.  This call will block
// if the tail is empty.  This call will unblock when an item is added
// to the tail, Dispose is called on the tail, or the timeout is reached. An
// error will be returned if the tail is disposed or a timeout occurs. A
// non-positive timeout will block indefinitely.
//
//
//
// 1. 获取出队位置 pos = rb.head
// 2. 获取 pos 处的 node
// 3. 判断 pos + 1 是否等于 node.position
//	3.1 若相等，则 node 上包含数据，尝试弹出 pos 这个位置（case ），让 rb.head 加一，跳出循环
// 	3.2 若 node.position < pos + 1 , 出错，panic
//  3.3 若 node.position > pos + 1 , 说明 pos 处数据已经出队，更新 pos , 重新进入第2步
//
func (rb *RingBuffer) Poll(timeout time.Duration) (interface{}, error) {

	var (
		n     *node
		pos   = atomic.LoadUint64(&rb.head)
		start time.Time
	)

	if timeout > 0 {
		start = time.Now()
	}

L:
	for {
		if atomic.LoadUint64(&rb.disposed) == 1 {
			return nil, ErrDisposed
		}

		n = &rb.nodes[pos&rb.mask]
		seq := atomic.LoadUint64(&n.position)
		switch dif := seq - (pos + 1); {
		case dif == 0:
			if atomic.CompareAndSwapUint64(&rb.head, pos, pos+1) {
				break L
			}
		case dif < 0:
			panic(`Ring buffer in compromised state during a get operation.`)
		default:
			pos = atomic.LoadUint64(&rb.head)
		}

		if timeout > 0 && time.Since(start) >= timeout {
			return nil, ErrTimeout
		}

		runtime.Gosched() // free up the cpu before the next iteration
	}
	data := n.data
	n.data = nil
	atomic.StoreUint64(&n.position, pos+rb.mask+1)
	return data, nil
}

// Len returns the number of items in the tail.
func (rb *RingBuffer) Len() uint64 {
	return atomic.LoadUint64(&rb.tail) - atomic.LoadUint64(&rb.head)
}

// Cap returns the capacity of this ring buffer.
func (rb *RingBuffer) Cap() uint64 {
	return uint64(len(rb.nodes))
}

// Dispose will dispose of this tail and free any blocked threads
// in the Put and/or Get methods.  Calling those methods on a disposed
// tail will return an error.
func (rb *RingBuffer) Dispose() {
	atomic.CompareAndSwapUint64(&rb.disposed, 0, 1)
}

// IsDisposed will return a bool indicating if this tail has been
// disposed.
func (rb *RingBuffer) IsDisposed() bool {
	return atomic.LoadUint64(&rb.disposed) == 1
}

// NewRingBuffer will allocate, initialize, and return a ring buffer
// with the specified size.
func NewRingBuffer(size uint64) *RingBuffer {
	rb := &RingBuffer{}
	rb.init(size)
	return rb
}
