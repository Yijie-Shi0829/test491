package common

//
// define here any data types that you need to access in two packages without
// creating circular dependencies
//

type Config struct {
	Num    int                // config number
	Shards [NShards]int64     // shard -> gid
	Groups map[int64][]string // gid -> servers[]
}

type UpdateArgs struct {
	Unum           int64
	NewShardAssign [16]int64
	Allgroups      map[int64][]string
	Curconfignum   int
}

type UpdateReply struct {
	Err string
}

type QueryArgs struct {
	Num int // desired config number
}

type QueryReply struct {
	Config Config
}
