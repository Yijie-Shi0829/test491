package shardkv

import (
	"time"

	"umich.edu/eecs491/proj5/common"
)

//
// additions to Clerk state
//
type ClerkImpl struct {
	ShardAssign [16]int64
	Allgroups   map[int64][]string
}

//
// initialize ck.impl.*
//
func (ck *Clerk) InitImpl() {
	config := ck.sm.Query(-1)
	ck.impl.ShardAssign = config.Shards
	ck.impl.Allgroups = config.Groups
}

//
// fetch the current value for a key.
// return "" if the key does not exist.
// keep retrying forever in the face of all other errors.
//
func (ck *Clerk) Get(key string) string {
	config := ck.sm.Query(-1)
	ck.impl.ShardAssign = config.Shards
	ck.impl.Allgroups = config.Groups

	unum := common.Nrand()
	serverlist := ck.impl.Allgroups[ck.impl.ShardAssign[common.Key2Shard(key)]]
	for {
		// select a server
		for {
			index := int(common.Nrand()) % len(serverlist)
			args := &GetArgs{}
			var reply GetReply
			args.Key = key
			args.Impl.Unum = unum
			ok := common.Call(serverlist[index], "ShardKV.Get", args, &reply)
			if ok && reply.Err == "OK" {
				return reply.Value
			} else if reply.Err == "ErrWrongGroup" {
				config := ck.sm.Query(-1)
				ck.impl.ShardAssign = config.Shards
				ck.impl.Allgroups = config.Groups
				serverlist = config.Groups[config.Shards[common.Key2Shard(key)]]
				time.Sleep(100 * time.Millisecond)
				break
			}
		}
	}
}

//
// send a Put or Append request.
// keep retrying forever until success.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	config := ck.sm.Query(-1)
	ck.impl.ShardAssign = config.Shards
	ck.impl.Allgroups = config.Groups

	unum := common.Nrand()
	serverlist := ck.impl.Allgroups[ck.impl.ShardAssign[common.Key2Shard(key)]]

	for {
		// select a server
		for {
			index := int(common.Nrand()) % len(serverlist)
			args := &PutAppendArgs{}
			var reply PutAppendReply
			args.Key = key
			args.Value = value
			args.Impl.Unum = unum
			args.Op = op
			ok := common.Call(serverlist[index], "ShardKV.PutAppend", args, &reply)
			if ok && reply.Err == "OK" {
				return
			} else if reply.Err == "ErrWrongGroup" {
				config := ck.sm.Query(-1)
				ck.impl.ShardAssign = config.Shards
				ck.impl.Allgroups = config.Groups
				serverlist = config.Groups[config.Shards[common.Key2Shard(key)]]
				time.Sleep(100 * time.Millisecond)
				break
			}
		}
	}
}
