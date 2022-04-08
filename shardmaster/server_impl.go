package shardmaster

import (
	"time"

	"umich.edu/eecs491/proj5/common"
)

//
// Define what goes into "value" that Paxos is used to agree upon.
// Field names must start with capital letters.
//
const (
	Join int = iota + 10
	Leave
	Move
	Query
)

type Op struct {
	OpType  int
	GID     int64
	Servers []string
	Shard   int
	Num     int
}

//
// Method used by PaxosRSM to determine if two Op values are identical
//

func equalhelper(server1 []string, server2 []string) bool {
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
	if op1.OpType == op2.OpType && op1.GID == op2.GID && op1.Num == op2.Num && op1.Shard == op2.Shard {
		if equalhelper(op1.Servers, op2.Servers) {
			return true
		} else {
			return false
		}
	}
	return false
}

//
// additions to ShardMaster state
//
type ShardMasterImpl struct {
	curconfignum int
	GID2Shard    map[int64]int // number of shards belongs to gid
}

//
// initialize sm.impl.*
//
func (sm *ShardMaster) InitImpl() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.impl.curconfignum = 0

	// initialize the first config
	sm.configs[0] = Config{0, [16]int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, make(map[int64][]string)}
	sm.impl.GID2Shard = make(map[int64]int)
	sm.impl.GID2Shard[0] = 16
}

//
// RPC handlers for Join, Leave, Move, and Query RPCs
//
func (sm *ShardMaster) Join(args *JoinArgs, reply *JoinReply) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	op := Op{Join, args.GID, args.Servers, -1, -1}
	sm.rsm.AddOp(op)
	return nil
}

func (sm *ShardMaster) Leave(args *LeaveArgs, reply *LeaveReply) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	op := Op{Leave, args.GID, make([]string, 0), -1, -1}
	sm.rsm.AddOp(op)
	return nil
}

func (sm *ShardMaster) Move(args *MoveArgs, reply *MoveReply) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	op := Op{Move, args.GID, make([]string, 0), args.Shard, -1}
	sm.rsm.AddOp(op)
	return nil
}

func (sm *ShardMaster) Query(args *QueryArgs, reply *QueryReply) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	op := Op{Query, -1, make([]string, 0), -1, args.Num}
	sm.rsm.AddOp(op)
	if args.Num == -1 || args.Num > sm.impl.curconfignum {
		reply.Config = sm.configs[sm.impl.curconfignum]
	} else {
		reply.Config = sm.configs[args.Num]
	}
	return nil
}

func (sm *ShardMaster) Rebalance(shardassign [16]int64) [16]int64 {
	// Rebalance the load of each server
	if len(sm.impl.GID2Shard) <= 1 {
		return shardassign
	}
	average := int(16 / (len(sm.impl.GID2Shard) - 1))
	remain := 16 % (len(sm.impl.GID2Shard) - 1) // num of floor(average) + 1
	more := make(chan int64, 20)
	less := make(chan int64, 20)
	counter := 0
	lesscounter := 0

	num := sm.impl.GID2Shard[0]
	for i := 0; i < num; i++ {
		more <- int64(0)
		sm.impl.GID2Shard[0]--
		counter++
	}

	// find where there are more shards than needed
	for key, value := range sm.impl.GID2Shard {
		if key == 0 {
			continue
		}
		if remain > 0 {
			if value >= average+1 {
				for i := 0; i < value-average-1; i++ {
					more <- key
					sm.impl.GID2Shard[key]--
					counter++
				}
				remain--
			}
		} else {
			if value >= average {
				for i := 0; i < value-average; i++ {
					more <- key
					sm.impl.GID2Shard[key]--
					counter++
				}
			}
		}
	}

	// find where there are less shards than needed
	for key, value := range sm.impl.GID2Shard {
		if key == 0 {
			continue
		}
		if remain > 0 {
			if value < average+1 {
				for i := 0; i < average+1-value; i++ {
					less <- key
					sm.impl.GID2Shard[key]++
					lesscounter++
				}
				remain--
			}
		} else {
			if value <= average {
				for i := 0; i < average-value; i++ {
					less <- key
					sm.impl.GID2Shard[key]++
					lesscounter++
				}
			}
		}
	}

	// reallcoate
	for i := 0; i < counter; i++ {
		movefrom := <-more
		moveto := <-less
		for j := 0; j < 16; j++ {
			if shardassign[j] == movefrom {
				shardassign[j] = moveto
				break
			}
		}
	}
	return shardassign
}

//
// Execute operation encoded in decided value v and update local state
//
func (sm *ShardMaster) ApplyOp(v interface{}) {
	Val := v.(Op)
	//add the unique number of this operation to local KVLog
	newshardassign := [16]int64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	newgroup := make(map[int64][]string)
	if Val.OpType == Join {
		curconfig := sm.configs[sm.impl.curconfignum]
		if _, ok := sm.impl.GID2Shard[Val.GID]; ok {
			// already join the config
			sm.impl.curconfignum += 1
			newshardassign = curconfig.Shards
			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			newgroup[Val.GID] = Val.Servers
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		} else {
			// update local state
			sm.impl.curconfignum += 1
			sm.impl.GID2Shard[Val.GID] = 0

			newshardassign = sm.Rebalance(curconfig.Shards)

			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			newgroup[Val.GID] = Val.Servers
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		}

		// part b inform related servers

	} else if Val.OpType == Leave {
		curconfig := sm.configs[sm.impl.curconfignum]
		if _, ok := sm.impl.GID2Shard[Val.GID]; !ok {
			// already leave the config
			sm.impl.curconfignum += 1
			newshardassign = curconfig.Shards
			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		} else {
			// update local state
			sm.impl.curconfignum += 1
			shardassign := curconfig.Shards
			for i := 0; i < 16; i++ {
				if shardassign[i] == Val.GID {
					shardassign[i] = 0
				}
			}
			sm.impl.GID2Shard[0] += sm.impl.GID2Shard[Val.GID]
			delete(sm.impl.GID2Shard, Val.GID)

			newshardassign = sm.Rebalance(shardassign)

			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			delete(newgroup, Val.GID)
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		}

	} else if Val.OpType == Move {
		curconfig := sm.configs[sm.impl.curconfignum]
		if curconfig.Shards[Val.Shard] == Val.GID {
			// already in charge
			sm.impl.curconfignum += 1
			newshardassign = curconfig.Shards
			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		} else {
			// update local state
			sm.impl.curconfignum += 1
			shardassign := curconfig.Shards
			sm.impl.GID2Shard[shardassign[Val.Shard]] -= 1
			shardassign[Val.Shard] = Val.GID
			sm.impl.GID2Shard[Val.GID] += 1

			for key, value := range curconfig.Groups {
				newgroup[key] = value
			}
			newshardassign = shardassign
			newconfig := Config{sm.impl.curconfignum, newshardassign, newgroup}
			sm.configs = append(sm.configs, newconfig)
		}
	}

	// part b inform related servers
	if Val.OpType != Query {
		// inform all current groups
		for key := range sm.configs[sm.impl.curconfignum].Groups {
			for {
				if len(sm.configs[sm.impl.curconfignum].Groups[key]) == 0 {
					break
				}
				index := int(common.Nrand()) % len(sm.configs[sm.impl.curconfignum].Groups[key])
				args := &common.UpdateArgs{Unum: common.Nrand(), NewShardAssign: newshardassign, Allgroups: newgroup, Curconfignum: sm.impl.curconfignum}
				reply := common.UpdateReply{Err: ""}
				ok := common.Call(sm.configs[sm.impl.curconfignum].Groups[key][index], "ShardKV.UpdateConfig", args, &reply)
				if ok {
					break
				} else {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		// inform the leave group
		if Val.OpType == Leave {
			for {
				if servers, ok := sm.configs[sm.impl.curconfignum-1].Groups[Val.GID]; !ok || len(servers) == 0 {
					break
				}
				index := int(common.Nrand()) % len(sm.configs[sm.impl.curconfignum-1].Groups[Val.GID])
				args := &common.UpdateArgs{Unum: common.Nrand(), NewShardAssign: newshardassign, Allgroups: newgroup, Curconfignum: sm.impl.curconfignum}
				reply := common.UpdateReply{Err: ""}
				ok := common.Call(sm.configs[sm.impl.curconfignum-1].Groups[Val.GID][index], "ShardKV.UpdateConfig", args, &reply)
				if ok {
					break
				} else {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}
}
