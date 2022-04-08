package shardkv

import (
	"umich.edu/eecs491/proj5/common"
)

//
// Define what goes into "value" that Paxos is used to agree upon.
// Field names must start with capital letters
//

const (
	Get int = iota + 10
	Put
	Append
	Reconfig
	KVOpUpdate
	GetUpdate
)

type Op struct {
	OpType       int
	Key          string
	Value        string
	Unum         int64 //unique serial number of this operation
	ShardAssign  [16]int64
	Allgroups    map[int64][]string
	KVmap        map[string]string
	Opmap        map[int]map[int64]bool
	Curconfignum int
	Gid          int64
}

//
// Method used by PaxosRSM to determine if two Op values are identical
//

func equalhelperlist(server1 []string, server2 []string) bool {
	// compare the server list
	if server1 == nil && server2 == nil {
		return true
	}
	if server1 == nil || server2 == nil {
		return false
	}
	if len(server1) != len(server2) {
		return false
	}
	map1 := make(map[string]int)
	map2 := make(map[string]int)
	for i := 0; i < len(server1); i++ {
		map1[server1[i]] = 1
		map2[server2[i]] = 1
	}
	for key := range map1 {
		if _, ok := map2[key]; !ok {
			return false
		}
	}
	return true
}

func equals(v1 interface{}, v2 interface{}) bool {
	op1 := v1.(Op)
	op2 := v2.(Op)

	if op1.Unum == op2.Unum {
		return true
	}
	return false
}

//
// additions to ShardKV state
//
type ShardKVImpl struct {
	KVMap           map[string]string
	OpLog           map[int]map[int64]bool
	UpdateConfigLog map[int64]bool
	oldShardassign  [16]int64
	Shardassign     [16]int64
	Allgroups       map[int64][]string
	PassMapHist     map[int]map[int64]bool // judge whether I've received the transfered map
}

//
// initialize kv.impl.*
//
func (kv *ShardKV) InitImpl() {
	kv.impl.KVMap = make(map[string]string)
	kv.impl.OpLog = make(map[int]map[int64]bool)
	kv.impl.UpdateConfigLog = make(map[int64]bool)
	kv.impl.oldShardassign = [16]int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	kv.impl.Shardassign = [16]int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	kv.impl.Allgroups = make(map[int64][]string)
	kv.impl.PassMapHist = make(map[int]map[int64]bool)
}

//
// RPC handler for client Get requests
//
func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	emptylist := [16]int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	Operation := Op{Get, args.Key, "", args.Impl.Unum, emptylist, make(map[int64][]string), make(map[string]string), make(map[int]map[int64]bool), -1, -1}
	kv.rsm.AddOp(Operation)

	if kv.impl.Shardassign[common.Key2Shard(args.Key)] != kv.gid {
		reply.Err = "ErrWrongGroup"
		return nil
	} else {
		//AddOp only fails when rsm.px is disconnected or dead
		//check whether AddOp fails
		if _, ok := kv.impl.OpLog[common.Key2Shard(args.Key)][args.Impl.Unum]; ok {
			//succeed
			reply.Value = kv.impl.KVMap[args.Key]
			reply.Err = "OK"
		} else {
			//fail
			reply.Value = ""
			reply.Err = "FAIL"
		}
	}
	return nil
}

//
// RPC handler for client Put and Append requests
//
func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	emptylist := [16]int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	Operation := Op{Append, args.Key, args.Value, args.Impl.Unum, emptylist, make(map[int64][]string), make(map[string]string), make(map[int]map[int64]bool), -1, -1}
	if args.Op == "Put" {
		Operation.OpType = Put
	}

	kv.rsm.AddOp(Operation)

	if kv.impl.Shardassign[common.Key2Shard(args.Key)] != kv.gid {
		reply.Err = "ErrWrongGroup"
		return nil
	} else {
		if _, ok := kv.impl.OpLog[common.Key2Shard(args.Key)][args.Impl.Unum]; ok {
			//succeed
			reply.Err = "OK"
		} else {
			//fail
			reply.Err = "FAIL"
		}
	}

	return nil
}

//
// Execute operation encoded in decided value v and update local state
//
func (kv *ShardKV) ApplyOp(v interface{}) {
	Val := v.(Op)
	//add the unique number of this operation to local KVLog
	if Val.OpType == Reconfig {
		if _, ok := kv.impl.UpdateConfigLog[Val.Unum]; ok {
			return
		}
		kv.impl.UpdateConfigLog[Val.Unum] = true
		kv.impl.oldShardassign = kv.impl.Shardassign
		kv.impl.Shardassign = Val.ShardAssign
		kv.impl.Allgroups = Val.Allgroups
		return
	} else if Val.OpType == KVOpUpdate {
		// update local map
		if _, ok := kv.impl.UpdateConfigLog[Val.Unum]; ok {
			return
		}
		if _, ok := kv.impl.PassMapHist[Val.Curconfignum][Val.Gid]; ok {
			kv.impl.UpdateConfigLog[Val.Unum] = true
			return
		}

		kv.impl.UpdateConfigLog[Val.Unum] = true
		for key, value := range Val.KVmap {
			kv.impl.KVMap[key] = value
		}
		for key, value := range Val.Opmap {
			kv.impl.OpLog[key] = value
		}
		if _, ok := kv.impl.PassMapHist[Val.Curconfignum]; !ok {
			kv.impl.PassMapHist[Val.Curconfignum] = make(map[int64]bool)
		}
		kv.impl.PassMapHist[Val.Curconfignum][Val.Gid] = true

		return
	}

	if Val.OpType == Put {
		if _, ok := kv.impl.OpLog[common.Key2Shard(Val.Key)][Val.Unum]; ok {
			// already done
			return
		}
		if kv.impl.Shardassign[common.Key2Shard(Val.Key)] != kv.gid {
			// not in charge
			return
		}
		kv.impl.KVMap[Val.Key] = Val.Value
	} else if Val.OpType == Append {
		if _, ok := kv.impl.OpLog[common.Key2Shard(Val.Key)][Val.Unum]; ok {
			// already done
			return
		}
		if kv.impl.Shardassign[common.Key2Shard(Val.Key)] != kv.gid {
			// not in charge
			return
		}
		if _, ok := kv.impl.KVMap[Val.Key]; ok {
			kv.impl.KVMap[Val.Key] += Val.Value
		} else {
			kv.impl.KVMap[Val.Key] = Val.Value
		}
	}

	if _, ok := kv.impl.OpLog[common.Key2Shard(Val.Key)]; !ok {
		kv.impl.OpLog[common.Key2Shard(Val.Key)] = make(map[int64]bool)
	}
	kv.impl.OpLog[common.Key2Shard(Val.Key)][Val.Unum] = true
}

//
// Add RPC handlers for any other RPCs you introduce
//

func (kv *ShardKV) UpdateConfig(args *common.UpdateArgs, reply *common.UpdateReply) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	Operation := Op{Reconfig, "", "", args.Unum, args.NewShardAssign, args.Allgroups, make(map[string]string), make(map[int]map[int64]bool), -1, -1}
	kv.rsm.AddOp(Operation)

	passmap := make(map[int64][]int)
	num2gid := make(map[int]int64)
	for index, gid := range kv.impl.oldShardassign {
		// call others to update the group state
		newgid := args.NewShardAssign[index]
		if gid == kv.gid && newgid != kv.gid {
			if _, ok := passmap[newgid]; ok {
				passmap[newgid] = append(passmap[newgid], index)
			} else {
				passmap[newgid] = []int{index}
			}
			num2gid[index] = newgid
		}
	}

	// prepare kvmap and opmap needed and transfer
	for to_gid, shardlist := range passmap {
		kvmap := make(map[string]string)
		for key, value := range kv.impl.KVMap {
			if thisgid, ok := num2gid[common.Key2Shard(key)]; ok && thisgid == to_gid {
				kvmap[key] = value
			}
		}
		opmap := make(map[int]map[int64]bool)
		for _, shard := range shardlist {
			opmap[shard] = kv.impl.OpLog[shard]
		}
		for {
			index := int(common.Nrand()) % len(args.Allgroups[to_gid])
			args1 := &KVOpUpdateArgs{common.Nrand(), kvmap, opmap, args.Curconfignum, kv.gid}
			reply1 := KVOpUpdateReply{""}
			ok := common.Call(args.Allgroups[to_gid][index], "ShardKV.KvOpUpdate", args1, &reply1)
			if ok && reply1.Err == "OK" {
				break
			}
		}
	}

	// update local state
	if _, ok := kv.impl.UpdateConfigLog[args.Unum]; ok {
		//succeed
		reply.Err = "OK"
		for key := range kv.impl.OpLog {
			if kv.impl.Shardassign[key] != kv.gid {
				delete(kv.impl.OpLog, key)
			}
		}

	} else {
		//fail
		reply.Err = "FAIL"
	}
	return nil
}

func (kv *ShardKV) KvOpUpdate(args *KVOpUpdateArgs, reply *KVOpUpdateReply) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	emptylist := [16]int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	Operation := Op{KVOpUpdate, "", "", args.Unum, emptylist, make(map[int64][]string), args.Kvmap, args.Opmap, args.Confignum, args.Gid}
	kv.rsm.AddOp(Operation)
	//AddOp only fails when rsm.px is disconnected or dead
	//check whether AddOp fails
	if _, ok := kv.impl.UpdateConfigLog[args.Unum]; ok {
		//succeed
		reply.Err = "OK"
	} else {
		//fail
		reply.Err = "FAIL"
	}
	return nil
}
