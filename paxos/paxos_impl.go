package paxos

import (
	"time"

	"umich.edu/eecs491/proj5/common"
)

//
// additions to Paxos state.
//

type Log_slot struct {
	np      int //highest proposal number promised to accept
	na      int //highest proposal number accrpted
	va      interface{}
	decided Fate //whether this slot has been decided
}

type PaxosImpl struct {
	n        int //number for proposing
	Log      map[int]*Log_slot
	peerDone map[int]int
}

//
// your px.impl.* initializations here.
//
func (px *Paxos) initImpl() {
	px.mu.Lock()
	defer px.mu.Unlock()

	px.impl.n = 1
	px.impl.Log = make(map[int]*Log_slot)
	px.impl.peerDone = make(map[int]int)
	px.impl.peerDone[px.me] = -1
}

//
// the application wants paxos to start agreement on
// instance seq, with proposed value v.
// Start() returns right away; the application will
// call Status() to find out if/when agreement
// is reached.
//

func StartHelper(px *Paxos, seq int, v interface{}, N int, sendDone int) string {
	count_prep := 0
	count_acc := 0
	//na == -1 means currently no accepted value
	accepted_na := -1
	var accepted_va interface{}
	//Prepare

	for i := 0; i < len(px.peers); i++ {
		ok := true
		prepare_args := &PrepareArgs{N, seq, px.me, sendDone}
		var prepare_reply PrepareReply
		prepare_reply.Err = "OK"

		if i != px.me {
			ok = common.Call(px.peers[i], "Paxos.PrepareWithLock", prepare_args, &prepare_reply)
			//update the local Done values of other paxos
			if ok {
				px.mu.Lock()
				px.impl.peerDone[i] = prepare_reply.ReplyDone
				px.mu.Unlock()
			}
		} else {
			//send message to local acceptor without using RPC
			px.PrepareWithLock(prepare_args, &prepare_reply)
		}

		if ok && prepare_reply.Err == "OK" {
			count_prep += 1
			//find the accepted va with highest na
			if prepare_reply.Curr_Na > accepted_na {
				accepted_na = prepare_reply.Curr_Na
				accepted_va = prepare_reply.Curr_Va
			}
		} else if ok && prepare_reply.Err == "SlotDecided" {
			px.mu.Lock()
			if _, ok := px.impl.Log[seq]; !ok {
				px.impl.Log[seq] = &Log_slot{}
			}
			px.impl.Log[seq].decided = Decided
			px.impl.Log[seq].na = prepare_reply.Curr_Na
			px.impl.Log[seq].va = prepare_reply.Curr_Va
			px.mu.Unlock()
			SpreadLearn(px, seq, prepare_reply.Curr_Va, prepare_reply.Curr_Na, sendDone)
			return "SlotDecided"
		} else if ok && prepare_reply.Err == "Reject" {
			px.mu.Lock()
			if px.impl.n < prepare_reply.Curr_Np+1 {
				px.impl.n = prepare_reply.Curr_Np + 1
			}
			px.mu.Unlock()
		}
	}

	if count_prep <= len(px.peers)/2 {
		return "PrepareCountReject"
	}

	//Val is the value to be sent
	var Val interface{}
	if accepted_na != -1 {
		Val = accepted_va
	} else {
		Val = v
	}

	//Accept
	for i := 0; i < len(px.peers); i++ {
		accept_args := &AcceptArgs{N, seq, sendDone, px.me, Val}
		var accept_reply AcceptReply
		accept_reply.Err = "OK"

		ok := true
		if i != px.me {
			ok = common.Call(px.peers[i], "Paxos.AcceptWithLock", accept_args, &accept_reply)
			//update the local Done values of other paxos
			if ok {
				px.mu.Lock()
				px.impl.peerDone[i] = accept_reply.ReplyDone
				px.mu.Unlock()
			}
		} else {
			//send message to local acceptor without using RPC
			px.AcceptWithLock(accept_args, &accept_reply)
		}
		if ok && accept_reply.Err == "OK" {
			count_acc += 1
		} else if ok && accept_reply.Err == "SlotDecided" {
			px.mu.Lock()
			if _, ok := px.impl.Log[seq]; !ok {
				px.impl.Log[seq] = &Log_slot{}
			}
			px.impl.Log[seq].decided = Decided
			px.impl.Log[seq].na = accept_reply.Curr_Na
			px.impl.Log[seq].va = accept_reply.Curr_Va
			px.mu.Unlock()
			SpreadLearn(px, seq, accept_reply.Curr_Va, accept_reply.Curr_Na, sendDone)
			return "SlotDecided"
		} else if ok && accept_reply.Err == "Reject" {
			px.mu.Lock()
			if px.impl.n < accept_reply.Curr_Np+1 {
				px.impl.n = accept_reply.Curr_Np + 1
			}
			px.mu.Unlock()
		}
	}

	if count_acc <= len(px.peers)/2 {
		return "AccepctCountReject"
	}

	//Learn
	SpreadLearn(px, seq, Val, N, sendDone)

	return "OK"
}

func SpreadLearn(px *Paxos, seq int, Val interface{}, N int, sendDone int) {
	for i := 0; i < len(px.peers); i++ {
		learn_args := &DecidedArgs{}
		var learn_reply DecidedReply
		learn_args.Na = N
		learn_args.Va = Val
		learn_args.Seq = seq
		learn_args.SenderIdx = px.me
		learn_args.SendDone = sendDone

		if i != px.me {
			ok := common.Call(px.peers[i], "Paxos.LearnWithLock", learn_args, &learn_reply)
			if ok {
				px.mu.Lock()
				px.impl.peerDone[i] = learn_reply.ReplyDone
				px.mu.Unlock()
			}
		} else {
			//send message to local learner without using RPC
			px.LearnWithLock(learn_args, &learn_reply)
		}
	}
}

func (px *Paxos) Start(seq int, v interface{}) {
	if px.isdead() {
		return
	}
	px.mu.Lock()
	defer px.mu.Unlock()
	//start goroutine
	go func(seq int, v interface{}) {
		state, _ := px.Status(seq)
		for state == Pending {
			if px.isdead() {
				return
			}
			px.mu.Lock()
			N := px.impl.n
			sendDone := px.impl.peerDone[px.me]
			px.mu.Unlock()

			StartHelper(px, seq, v, N, sendDone)
			state, _ = px.Status(seq)
			time.Sleep(100 * time.Millisecond)
		}
	}(seq, v)
}

//
// the application on this machine is done with
// all instances <= seq.
//
// see the comments for Min() for more explanation.
//
func (px *Paxos) Done(seq int) {
	px.mu.Lock()
	defer px.mu.Unlock()
	px.impl.peerDone[px.me] = seq
}

//ADDED FUNCTION
func (px *Paxos) GetDoneValue() int {
	px.mu.Lock()
	defer px.mu.Unlock()
	return px.impl.peerDone[px.me]
}

//
// the application wants to know the
// highest instance sequence known to
// this peer.
//
func (px *Paxos) Max() int {
	px.mu.Lock()
	defer px.mu.Unlock()

	max := px.impl.peerDone[px.me]
	for idx, elem := range px.impl.Log {
		if elem.decided == Decided && idx > max {
			max = idx
		}
	}
	return max
}

//
// Min() should return one more than the minimum among z_i,
// where z_i is the highest number ever passed
// to Done() on peer i. A peer's z_i is -1 if it has
// never called Done().
//
// Paxos is required to have forgotten all information
// about any instances it knows that are < Min().
// The point is to free up memory in long-running
// Paxos-based servers.
//
// Paxos peers need to exchange their highest Done()
// arguments in order to implement Min(). These
// exchanges can be piggybacked on ordinary Paxos
// agreement protocol messages, so it is OK if one
// peer's Min does not reflect another peer's Done()
// until after the next instance is agreed to.
//
// The fact that Min() is defined as a minimum over
// *all* Paxos peers means that Min() cannot increase until
// all peers have been heard from. So if a peer is dead
// or unreachable, other peers' Min()s will not increase
// even if all reachable peers call Done(). The reason for
// this is that when the unreachable peer comes back to
// life, it will need to catch up on instances that it
// missed -- the other peers therefore cannot forget these
// instances.
//
func (px *Paxos) Min() int {
	px.mu.Lock()
	defer px.mu.Unlock()

	if len(px.impl.peerDone) != len(px.peers) {
		return 0
	}
	//find min
	min := -1
	for _, v := range px.impl.peerDone {
		if min == -1 {
			min = v
		} else if v < min {
			min = v
		}
	}
	//update states in local log
	for seq, elem := range px.impl.Log {
		if seq <= min {
			elem.decided = Forgotten
			delete(px.impl.Log, seq)
		}
	}
	return min + 1
}

//
// the application wants to know whether this
// peer thinks an instance has been decided,
// and if so, what the agreed value is. Status()
// should just inspect the local peer state;
// it should not contact other Paxos peers.
//
func (px *Paxos) Status(seq int) (Fate, interface{}) {
	px.mu.Lock()
	defer px.mu.Unlock()

	if seq <= px.impl.peerDone[px.me] {
		return Forgotten, ""
	}

	if elem, ok := px.impl.Log[seq]; ok {
		return elem.decided, elem.va
	} else {
		px.impl.Log[seq] = &Log_slot{}
		px.impl.Log[seq].decided = Pending
		px.impl.Log[seq].np = 0
		px.impl.Log[seq].na = -1
		px.impl.Log[seq].va = nil
		return Pending, ""
	}
}
