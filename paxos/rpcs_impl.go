package paxos

// In all data types that represent RPC arguments/reply, field names
// must start with capital letters, otherwise RPC will break.

const (
	OK     = "OK"
	Reject = "Reject"
)

type Response string

type PrepareArgs struct {
	N         int
	Seq       int
	SenderIdx int
	SendDone  int
}

type PrepareReply struct {
	Curr_Np   int
	Curr_Na   int
	Curr_Va   interface{}
	ReplyDone int
	Err       string
}

type AcceptArgs struct {
	N         int
	Seq       int
	SendDone  int
	SenderIdx int
	Val       interface{}
}

type AcceptReply struct {
	Curr_Np   int
	Curr_Na   int
	Curr_Va   interface{}
	ReplyDone int
	Err       string
}

type DecidedArgs struct {
	Na        int
	Va        interface{}
	Seq       int
	SenderIdx int
	SendDone  int
}

type DecidedReply struct {
	ReplyDone int
	Err       string
}

func (px *Paxos) Prepare(args *PrepareArgs, reply *PrepareReply) error {
	if px.isdead() {
		return nil
	}
	if args.SenderIdx != px.me {
		px.impl.peerDone[args.SenderIdx] = args.SendDone
		reply.ReplyDone = px.impl.peerDone[px.me]
	}

	if elem, ok := px.impl.Log[args.Seq]; !ok {

		//this slot hasn't been accessed by any proposer
		//initialize the Seq-th slot of this log
		px.impl.Log[args.Seq] = &Log_slot{}
		px.impl.Log[args.Seq].np = args.N
		px.impl.Log[args.Seq].na = -1
		px.impl.Log[args.Seq].decided = Pending

		reply.Curr_Np = args.N
		reply.Curr_Na = -1
		reply.Err = "OK"
	} else if ok && elem.decided == Decided {
		reply.Curr_Np = elem.np
		reply.Curr_Na = elem.na
		reply.Curr_Va = elem.va
		reply.Err = "SlotDecided"
	} else if ok && args.N > elem.np {
		elem.np = args.N
		reply.Curr_Np = elem.np
		reply.Curr_Na = elem.na
		reply.Curr_Va = elem.va
		reply.Err = "OK"
	} else {
		reply.Curr_Np = elem.np
		reply.Err = "Reject"
	}
	return nil
}

func (px *Paxos) PrepareWithLock(args *PrepareArgs, reply *PrepareReply) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	px.Prepare(args, reply)
	return nil
}

func (px *Paxos) Accept(args *AcceptArgs, reply *AcceptReply) error {
	if px.isdead() {
		return nil
	}
	if args.SenderIdx != px.me {
		px.impl.peerDone[args.SenderIdx] = args.SendDone
		reply.ReplyDone = px.impl.peerDone[px.me]
	}

	if elem, ok := px.impl.Log[args.Seq]; !ok {
		//the Prepare phase has been skipped
		px.impl.Log[args.Seq] = &Log_slot{}
		px.impl.Log[args.Seq].np = args.N
		px.impl.Log[args.Seq].na = args.N
		px.impl.Log[args.Seq].va = args.Val
		px.impl.Log[args.Seq].decided = Pending
		reply.Err = "OK"
	} else if ok && px.impl.Log[args.Seq].decided == Decided {
		// Decided
		reply.Curr_Np = elem.np
		reply.Curr_Na = elem.na
		reply.Curr_Va = elem.va
		reply.Err = "SlotDecided"
	} else if ok && args.N >= elem.np {
		// update
		elem.na = args.N
		elem.np = args.N
		elem.va = args.Val
		reply.Err = "OK"
	} else {
		// reject
		reply.Curr_Np = elem.np
		reply.Err = "Reject"
	}
	//notify all other learners
	return nil
}

func (px *Paxos) AcceptWithLock(args *AcceptArgs, reply *AcceptReply) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	px.Accept(args, reply)
	return nil
}

func (px *Paxos) Learn(args *DecidedArgs, reply *DecidedReply) error {
	if px.isdead() {
		return nil
	}
	if args.SenderIdx != px.me {
		px.impl.peerDone[args.SenderIdx] = args.SendDone
		reply.ReplyDone = px.impl.peerDone[px.me]
	}

	if args.Seq <= px.impl.peerDone[px.me] {
		reply.Err = "UpdateUnnecessary"
		return nil
	}
	if elem, ok := px.impl.Log[args.Seq]; ok {
		if elem.decided == Decided {
			reply.Err = "SlotAlreadyDecided"
			return nil
		}
	} else {
		px.impl.Log[args.Seq] = &Log_slot{}
	}

	px.impl.Log[args.Seq].na = args.Na
	px.impl.Log[args.Seq].va = args.Va
	px.impl.Log[args.Seq].decided = Decided
	reply.Err = "OK"
	return nil
}

func (px *Paxos) LearnWithLock(args *DecidedArgs, reply *DecidedReply) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	px.Learn(args, reply)
	return nil
}

//
// add RPC handlers for any RPCs you introduce.
//
