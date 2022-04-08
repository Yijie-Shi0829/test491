package shardmaster

import (
	"fmt"
	"sort"
	"umich.edu/eecs491/proj5/common"
)

//
// Define what goes into "value" that Paxos is used to agree upon.
// Field names must start with capital letters.
//
type Op struct {
	Operation string
	GID       int64
	Servers   []string
	ShardNum  int
	ConfigNum int
	Querynum  int
}

type GIDToShard struct {
	GID    int64
	Shards []int
}

type OrderedGIDToShard []GIDToShard

func (o OrderedGIDToShard) Len() int {
	return len(o)
}
func (o OrderedGIDToShard) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}
func (o OrderedGIDToShard) Less(i, j int) bool {
	return o[i].GID < o[j].GID
}

//
// Method used by PaxosRSM to determine if two Op values are identical
//
func equals(v1 interface{}, v2 interface{}) bool {
	v11 := v1.(Op)
	v22 := v2.(Op)
	// TODO: After Op is well implemented.
	if v11.Querynum == v22.Querynum && v11.Operation == v22.Operation && v11.GID == v22.GID && v11.ShardNum == v22.ShardNum && len(v11.Servers) == len(v22.Servers) {

	} else {
		return false
	}
	for k, _ := range v11.Servers {
		if v11.Servers[k] != v22.Servers[k] {
			return false
		}
	}
	return true
}

//
// additions to ShardMaster state
//
type ShardMasterImpl struct {
	ConfigNum int
}

//
// initialize sm.impl.*
//
func (sm *ShardMaster) InitImpl() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	configZero := Config{Num: 0, Shards: [common.NShards]int64{}, Groups: make(map[int64][]string, 0)}
	sm.configs = append(sm.configs, configZero)

}

//
// RPC handlers for Join, Leave, Move, and Query RPCs
//
func (sm *ShardMaster) Join(args *JoinArgs, reply *JoinReply) error {
	// As for join, we should first decide the mean of shards per server and the remainder.
	// Then, we start our assignment with GID who has the largest number of shards. Put those extra shards into an idle pool.
	// Finally, select shard from the idle pool and assign them to GIDs that have less shards.
	newGID := args.GID
	newServers := args.Servers
	maxConfigNum := len(sm.configs) - 1
	// if it is a new GID group, then we should do something. At this point, it is to add this operation to log.
	op := Op{Operation: "Join", GID: newGID, Servers: newServers, ConfigNum: maxConfigNum}
	sm.rsm.AddOp(op)

	// 1. Add this GID into the list.

	// 2. Reassign the shards to all GIDs in the GID map.
	return nil
}

func (sm *ShardMaster) Leave(args *LeaveArgs, reply *LeaveReply) error {
	// As for Leave, it is similar.
	oldGID := args.GID
	maxConfigNum := len(sm.configs) - 1
	// if it is an old GID group, then we should do something. At this point, it is to add this operation to log.
	op := Op{Operation: "Leave", GID: oldGID, ConfigNum: maxConfigNum}
	sm.rsm.AddOp(op)

	return nil
}

func (sm *ShardMaster) Move(args *MoveArgs, reply *MoveReply) error {

	GID := args.GID
	assignedShard := args.Shard
	maxConfigNum := len(sm.configs) - 1
	// 0. Check if this GID already added into the Shard Master.
	_, ok := sm.configs[maxConfigNum].Groups[GID]
	if !ok { // if already exists, then we should ignore this request.
		fmt.Println("GID in Move cannot be found")
		return nil
	}
	op := Op{Operation: "Move", GID: GID, ShardNum: assignedShard, ConfigNum: maxConfigNum}
	sm.rsm.AddOp(op)

	// 1. Add this GID into the list.

	// 2. Reassign the shards to all GIDs in the GID map.
	return nil
}

func (sm *ShardMaster) Query(args *QueryArgs, reply *QueryReply) error {
	queryNum := args.Num

	op := Op{Operation: "Query", Querynum: queryNum}
	sm.rsm.AddOp(op)
	if queryNum == -1 {
		queryNum = len(sm.configs) - 1
	}
	reply.Config = sm.configs[queryNum]
	return nil
}

//
// Execute operation encoded in decided value v and update local state
//
func (sm *ShardMaster) ApplyOp(v interface{}) {
	// Here we are going to handle operations.
	sm.mu.Lock()
	defer sm.mu.Unlock()
	op := v.(Op)
	// First, lets consider Join
	if op.Operation == "Join" {
		// TODO: Do you need to judge whether this join is the latest configuration update? No, because when the join is being executed,
		// TODO: it should always be the latest one. All old operations should already be handled in previous slots.
		// 1. Add this GID into the list.
		newGID := op.GID
		newServers := op.Servers
		maxConfigNum := len(sm.configs) - 1
		// If this GID already exists, ignore the request.
		_, ok := sm.configs[maxConfigNum].Groups[newGID]
		if ok {
			return
		}

		newConfig := Config{Num: maxConfigNum + 1, Shards: [common.NShards]int64{}, Groups: make(map[int64][]string, 0)}
		for k, val := range sm.configs[maxConfigNum].Shards {
			newConfig.Shards[k] = val
		}
		for k, val := range sm.configs[maxConfigNum].Groups {
			newConfig.Groups[k] = val
		}
		// Add GID into the config.
		newConfig.Groups[newGID] = newServers

		// 2. Reassign the shards to all GIDs in the GID map.
		// As for join, we should first decide the mean of shards per server and the remainder.
		// Then, we start our assignment with GID who has the largest number of shards. Put those extra shards into an idle pool.
		// Finally, select shard from the idle pool and assign them to GIDs that have less shards.
		GIDNum := len(newConfig.Groups)
		mean := common.NShards / GIDNum
		remainder := common.NShards % GIDNum
		orderGIDToShard := make([]GIDToShard, 0)
		// Initialize
		GIDtoShard := make(map[int64][]int, 0)
		for k, _ := range newConfig.Groups {
			GIDtoShard[k] = make([]int, 0)
			orderGIDToShard = append(orderGIDToShard, GIDToShard{GID: k, Shards: make([]int, 0)})
		}
		// This is a pool for storing extra shards.
		shardsPool := make([]int, 0)
		remainderCnt := remainder
		for k, val := range newConfig.Shards {
			// if it is the initialization case, GID = 0, then add it to the pool
			if val == 0 {
				shardsPool = append(shardsPool, k)
				continue
			}
			GIDtoShard[val] = append(GIDtoShard[val], k)
			// assign shard to gid for order
			for kk, gts := range orderGIDToShard {
				if gts.GID == val {
					orderGIDToShard[kk].Shards = append(orderGIDToShard[kk].Shards, k)
					break
				}
			}
		}
		// Sort the slice
		sort.Sort(OrderedGIDToShard(orderGIDToShard))

		/*for k, val := range GIDtoShard {
			limit := mean
			if remainderCnt > 0 {
				limit += 1
			}
			// If this GID has too many shards to handle
			tooMuch := len(val) - limit
			if tooMuch > 0 {
				shardsPool = append(shardsPool, val[:tooMuch]...)
				GIDtoShard[k] = append(val[:0], val[tooMuch:]...)
				tooMuch = 0
			}
			if tooMuch == 0 && limit > mean {
				remainderCnt -= 1
			}
		}*/
		for k, val := range orderGIDToShard {
			limit := mean
			if remainderCnt > 0 {
				limit += 1
			}
			// If this GID has too many shards to handle
			tooMuch := len(val.Shards) - limit
			if tooMuch > 0 {
				shardsPool = append(shardsPool, val.Shards[:tooMuch]...)
				orderGIDToShard[k].Shards = append(val.Shards[:0], val.Shards[tooMuch:]...)
				tooMuch = 0
			}
			if tooMuch == 0 && limit > mean {
				remainderCnt -= 1
			}
		}

		// Get shards from pool to assign them to those with little shards
		// remainderCnt = remainder
		/*for k, val := range GIDtoShard {
			if len(val) == mean+1 {
				continue
			}
			limit := mean
			if remainderCnt > 0 {
				limit += 1
				remainderCnt -= 1
			}
			// If this GID has too many shards to handle
			tooLess := limit - len(val)
			if tooLess > 0 {
				GIDtoShard[k] = append(val[:0], shardsPool[:tooLess]...)
				shardsPool = append(shardsPool[:0], shardsPool[tooLess:]...)
			}
		}*/
		for k, val := range orderGIDToShard {
			if len(val.Shards) == mean+1 {
				continue
			}
			limit := mean
			if remainderCnt > 0 {
				limit += 1
				remainderCnt -= 1
			}
			// If this GID has too many shards to handle
			tooLess := limit - len(val.Shards)
			if tooLess > 0 {
				orderGIDToShard[k].Shards = append(val.Shards[:0], shardsPool[:tooLess]...)
				shardsPool = append(shardsPool[:0], shardsPool[tooLess:]...)
			}
		}

		fmt.Println(GIDtoShard, orderGIDToShard)
		// Now assign the shards.
		/*for k, val := range GIDtoShard {
			for _, shardN := range val {
				newConfig.Shards[shardN] = k
			}
		}*/
		for _, val := range orderGIDToShard {
			for _, shardN := range val.Shards {
				newConfig.Shards[shardN] = val.GID
			}
		}

		sm.configs = append(sm.configs, newConfig)
		fmt.Println("Join: new config finish", newConfig)
	} else if op.Operation == "Leave" {
		leaveGID := op.GID
		maxConfigNum := len(sm.configs) - 1
		// If this GID already left, ignore the request.
		_, ok := sm.configs[maxConfigNum].Groups[leaveGID]
		if !ok {
			return
		}

		newConfig := Config{Num: maxConfigNum + 1, Shards: [common.NShards]int64{}, Groups: make(map[int64][]string, 0)}
		for k, val := range sm.configs[maxConfigNum].Shards {
			newConfig.Shards[k] = val
		}
		for k, val := range sm.configs[maxConfigNum].Groups {
			newConfig.Groups[k] = val
		}
		// delete GID into the config.
		fmt.Println(len(newConfig.Groups))
		delete(newConfig.Groups, leaveGID)
		fmt.Println(len(newConfig.Groups))

		// 2. Reassign the shards to all GIDs in the GID map.
		// As for join, we should first decide the mean of shards per server and the remainder.
		// Then, we start our assignment with GID who has the largest number of shards. Put those extra shards into an idle pool.
		// Finally, select shard from the idle pool and assign them to GIDs that have less shards.
		GIDNum := len(newConfig.Groups)
		mean := common.NShards / GIDNum
		remainder := common.NShards % GIDNum
		orderGIDToShard := make([]GIDToShard, 0)
		// Initialize
		GIDtoShard := make(map[int64][]int, 0)
		for k, _ := range newConfig.Groups {
			GIDtoShard[k] = make([]int, 0)
			orderGIDToShard = append(orderGIDToShard, GIDToShard{GID: k, Shards: make([]int, 0)})
		}
		/*for k, _ := range newConfig.Groups {
			GIDtoShard[k] = make([]int, 0)
		}*/
		shardsPool := make([]int, 0)
		remainderCnt := remainder
		for k, val := range newConfig.Shards {
			// if it is the initialization case, GID = 0, then add it to the pool
			if val == leaveGID {
				shardsPool = append(shardsPool, k)
				continue
			}
			GIDtoShard[val] = append(GIDtoShard[val], k)
			// assign shard to gid for order
			for kk, gts := range orderGIDToShard {
				if gts.GID == val {
					orderGIDToShard[kk].Shards = append(orderGIDToShard[kk].Shards, k)
					break
				}
			}
		}
		// Sort the slice
		sort.Sort(OrderedGIDToShard(orderGIDToShard))

		/*for k, val := range newConfig.Shards {
			// If this shard belongs to left GID, then add it to the pool
			if val == leaveGID {
				shardsPool = append(shardsPool, k)
				continue
			}
			GIDtoShard[val] = append(GIDtoShard[val], k)
		}*/

		/*for k, val := range GIDtoShard {
			limit := mean
			if remainderCnt > 0 {
				limit += 1
			}
			// If this GID has too many shards to handle
			tooMuch := len(val) - limit
			if tooMuch > 0 {
				shardsPool = append(shardsPool, val[:tooMuch]...)
				GIDtoShard[k] = append(val[:0], val[tooMuch:]...)
				tooMuch = 0
			}
			if tooMuch == 0 && limit > mean {
				remainderCnt -= 1
			}
		}*/

		for k, val := range orderGIDToShard {
			limit := mean
			if remainderCnt > 0 {
				limit += 1
			}
			// If this GID has too many shards to handle
			tooMuch := len(val.Shards) - limit
			if tooMuch > 0 {
				shardsPool = append(shardsPool, val.Shards[:tooMuch]...)
				orderGIDToShard[k].Shards = append(val.Shards[:0], val.Shards[tooMuch:]...)
				tooMuch = 0
			}
			if tooMuch == 0 && limit > mean {
				remainderCnt -= 1
			}
		}
		// Get shards from pool to assign them to those with little shards
		// remainderCnt = remainder
		/*for k, val := range GIDtoShard {
			if len(val) == mean+1 {
				continue
			}
			limit := mean
			if remainderCnt > 0 {
				limit += 1
				remainderCnt -= 1
			}
			// If this GID has too many shards to handle
			tooLess := limit - len(val)
			if tooLess > 0 {
				GIDtoShard[k] = append(val[:0], shardsPool[:tooLess]...)
				shardsPool = append(shardsPool[:0], shardsPool[tooLess:]...)
			}
		}*/
		for k, val := range orderGIDToShard {
			if len(val.Shards) == mean+1 {
				continue
			}
			limit := mean
			if remainderCnt > 0 {
				limit += 1
				remainderCnt -= 1
			}
			// If this GID has too many shards to handle
			tooLess := limit - len(val.Shards)
			if tooLess > 0 {
				orderGIDToShard[k].Shards = append(val.Shards[:0], shardsPool[:tooLess]...)
				shardsPool = append(shardsPool[:0], shardsPool[tooLess:]...)
			}
		}

		// Now assign the shards.
		/*for k, val := range GIDtoShard {
			for _, shardN := range val {
				newConfig.Shards[shardN] = k
			}
		}*/
		for _, val := range orderGIDToShard {
			for _, shardN := range val.Shards {
				newConfig.Shards[shardN] = val.GID
			}
		}

		sm.configs = append(sm.configs, newConfig)
		fmt.Println("Leave: new config finish", newConfig)
	} else if op.Operation == "Move" {
		GID := op.GID
		assignedShard := op.ShardNum
		maxConfigNum := len(sm.configs) - 1
		_, ok := sm.configs[maxConfigNum].Groups[GID]
		// if the GID is not existing, return
		if !ok {
			fmt.Println("Move: the GID doesn't exist", GID, assignedShard)
		}
		// Else, assign.
		newConfig := Config{Num: maxConfigNum + 1, Shards: [common.NShards]int64{}, Groups: make(map[int64][]string, 0)}
		for k, val := range sm.configs[maxConfigNum].Shards {
			newConfig.Shards[k] = val
		}
		for k, val := range sm.configs[maxConfigNum].Groups {
			newConfig.Groups[k] = val
		}
		newConfig.Shards[assignedShard] = GID

		sm.configs = append(sm.configs, newConfig)
		fmt.Println("Move: new config finish", newConfig)
	} else {
		// It is Query.
		// Nothing happens

	}
}
