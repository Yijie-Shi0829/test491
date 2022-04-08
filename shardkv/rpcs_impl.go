package shardkv

// Field names must start with capital letters,
// otherwise RPC will break.

//
// additional state to include in arguments to PutAppend RPC.
//
type PutAppendArgsImpl struct {
	Unum int64
}

//
// additional state to include in arguments to Get RPC.
//
type GetArgsImpl struct {
	Unum int64
}

//
// for new RPCs that you add, declare types for arguments and reply
//

type KVOpUpdateArgs struct {
	Unum      int64
	Kvmap     map[string]string
	Opmap     map[int]map[int64]bool
	Confignum int
	Gid       int64
}

type KVOpUpdateReply struct {
	Err Err
}
