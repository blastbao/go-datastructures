/*
Package datastructures exists solely to aid consumers of the go-datastructures
library when using dependency managers.  Depman, for instance, will work
correctly with any datastructure by simply importing this package instead of
each subpackage individually.

For more information about the datastructures package, see the README at

	http://github.com/Workiva/go-datastructures

*/
package datastructures

import (
	_ "github.com/blastbao/go-datastructures/augmentedtree"
	_ "github.com/blastbao/go-datastructures/bitarray"
	_ "github.com/blastbao/go-datastructures/btree/palm"
	_ "github.com/blastbao/go-datastructures/btree/plus"
	_ "github.com/blastbao/go-datastructures/fibheap"
	_ "github.com/blastbao/go-datastructures/futures"
	_ "github.com/blastbao/go-datastructures/hashmap/fastinteger"
	_ "github.com/blastbao/go-datastructures/numerics/optimization"
	_ "github.com/blastbao/go-datastructures/queue"
	_ "github.com/blastbao/go-datastructures/rangetree"
	_ "github.com/blastbao/go-datastructures/rangetree/skiplist"
	_ "github.com/blastbao/go-datastructures/set"
	_ "github.com/blastbao/go-datastructures/slice"
	_ "github.com/blastbao/go-datastructures/slice/skip"
	_ "github.com/blastbao/go-datastructures/sort"
	_ "github.com/blastbao/go-datastructures/threadsafe/err"
	_ "github.com/blastbao/go-datastructures/tree/avl"
	_ "github.com/blastbao/go-datastructures/trie/xfast"
	_ "github.com/blastbao/go-datastructures/trie/yfast"
)
